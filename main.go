package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"fudge/prompts"
	"fudge/ui"

	"github.com/chzyer/readline"
	"github.com/memmaker/gollum/llm"
)

var debug bool
var imgGen bool

// language is what the player reads: the narration, the choice menus and the
// setup conversation. Filenames and setting folders stay English whatever this
// is set to.
var language string

const (
	// maxAdventureAttempts caps the retries when the LLM won't save the adventure.
	maxAdventureAttempts = 3
	// maxTurnRequests is every request a single game turn may make of the model,
	// failed ones included. The last of them is spent demanding the narration, so
	// a turn has maxTurnRequests-1 rounds to roll, read and edit, and one to end
	// on. A turn still unnarrated after that is an error.
	maxTurnRequests = 6
	// maxBookkeepingRequests bounds the round after the narration. One response
	// is the normal case, since a reply can carry every edit at once; the second
	// exists for a DM that reads the sheet before writing to it.
	maxBookkeepingRequests = 2
	// maxRetryDelay is the ceiling on the wait between retried requests.
	maxRetryDelay = 30 * time.Second
)

// requestRetryDelay is the pause before retrying a failed request, multiplied by
// the number of failures so far. A variable so tests can drive the retry path
// without waiting out a backoff written for a network.
var requestRetryDelay = 2 * time.Second

// Model context window sizes in tokens
var contextWindowSizes = map[string]int{
	"claude-opus-4-1":            200000,
	"claude-opus-4":              200000,
	"claude-3-5-sonnet-20241022": 200000,
	"claude-3-5-sonnet":          200000,
	"claude-3-opus":              200000,
	"claude-3-sonnet":            200000,
	"claude-3-haiku":             200000,
	"gpt-5.3-chat-latest":        128000,
	"gpt-4-turbo":                128000,
	"gpt-4":                      8192,
	"gpt-3.5-turbo":              4096,
	"gemini-pro":                 32000,
	"llama-2-70b":                4096,
}

type UsageTracker struct {
	model          string
	contextWindow  int
	estimatedUsage int
}

func newUsageTracker(model string) *UsageTracker {
	contextSize := contextWindowSizes[model]
	if contextSize == 0 {
		contextSize = 200000
	}
	return &UsageTracker{
		model:         model,
		contextWindow: contextSize,
	}
}

func (ut *UsageTracker) addMessage(msg llm.Message) {
	ut.estimatedUsage += len(msg.Content) / 4
	for _, call := range msg.ToolCalls {
		ut.estimatedUsage += len(call.Function.Arguments) / 4
	}
}

// record prefers the provider's own token count, which a non-streaming reply
// carries, and falls back to estimating from the text when one is missing.
func (ut *UsageTracker) record(messages []llm.Message, response llm.Response) {
	if total := response.Usage.TotalTokens; total > 0 {
		ut.estimatedUsage = total
		return
	}
	if reported := response.Usage.InputTokens + response.Usage.OutputTokens; reported > 0 {
		ut.estimatedUsage = reported
		return
	}
	for _, msg := range messages {
		ut.addMessage(msg)
	}
	ut.addMessage(response.Message)
}

func (ut *UsageTracker) getPercentage() int {
	if ut.contextWindow == 0 {
		return 0
	}
	percent := (ut.estimatedUsage * 100) / ut.contextWindow
	if percent > 100 {
		percent = 100
	}
	return percent
}

func main() {
	flag.BoolVar(&debug, "debug", false, "Enable debug mode to show tool usage")
	flag.BoolVar(&imgGen, "img-gen", false, "Enable image generation feature")
	flag.StringVar(&language, "lang", prompts.DefaultLanguage, "Language the player reads: narration, menus and setup. A name or an ISO code (\"German\", \"Deutsch\", \"de\").")
	scriptFile := flag.String("script", "", "Play non-interactively from a file of answers, one per line (\"-\" reads stdin). Menus are answered by naming the option; everything else is typed verbatim. The session ends when the file does.")
	flag.Parse()
	language = prompts.NormalizeLanguage(language)

	if *scriptFile != "" {
		in := io.Reader(os.Stdin)
		if *scriptFile != "-" {
			f, err := os.Open(*scriptFile)
			if err != nil {
				fmt.Println("Error opening script:", err)
				return
			}
			defer f.Close()
			in = f
		}
		if err := ui.LoadScript(in); err != nil {
			fmt.Println("Error reading script:", err)
			return
		}
	}

	ui.ClearScreen()
	ui.HideCursor()
	defer ui.ShowCursor()

	s := newSession()

	rl, err := readline.New("> ")
	if err != nil {
		fmt.Println("Error initializing readline:", err)
		return
	}
	defer rl.Close()

	// Setup picks the world and the scenario. The character is made in the game
	// phase, in conversation, because there is no engine left to build one in.
	if err := runSetupPhase(s, rl); err != nil {
		fmt.Println(err)
		return
	}

	runGamePhase(s, rl)
}

func loadDMPrompt(s *session) (string, error) {
	files := []string{"dm.md"}
	if imgGen {
		// image-guidance.md is the shared half: what makes a good image prompt,
		// independent of when one is asked for. img-style.md is only the part
		// specific to doing it every turn; /image's on-demand prompt reads the
		// same guidance file without it.
		files = append(files, "img-style.md", "image-guidance.md")
	}
	return prompts.LoadPrompt(s.settingDesc, language, files...)
}

// debugArgLimit caps the arguments shown for a tool call. Narration prose and
// setting files run to paragraphs, and a debug line is meant to be glanceable.
// Nothing is lost: /context dumps the arguments in full.
const debugArgLimit = 160

func truncateForDebug(s string) string {
	runes := []rune(s)
	if len(runes) <= debugArgLimit {
		return s
	}
	return string(runes[:debugArgLimit]) + "…"
}

// debugResponse reports what came back: whether the reply carried a body, which
// tools it called, and what it cost.
func debugResponse(response llm.Response) {
	if !debug {
		return
	}

	calls := "none"
	if len(response.Message.ToolCalls) > 0 {
		names := make([]string, 0, len(response.Message.ToolCalls))
		for _, call := range response.Message.ToolCalls {
			names = append(names, call.Function.Name)
		}
		calls = strings.Join(names, ", ")
	}

	prose := strings.TrimSpace(response.Message.Content)
	proseState := "none"
	if prose != "" {
		proseState = fmt.Sprintf("%d chars", len([]rune(prose)))
	}

	// Providers vary in what they report, so only name the parts they sent.
	tokens := ""
	switch u := response.Usage; {
	case u.InputTokens > 0 || u.OutputTokens > 0:
		total := u.TotalTokens
		if total == 0 {
			total = u.InputTokens + u.OutputTokens
		}
		tokens = fmt.Sprintf(" tokens=%d(in%d/out%d)", total, u.InputTokens, u.OutputTokens)
	case u.TotalTokens > 0:
		tokens = fmt.Sprintf(" tokens=%d", u.TotalTokens)
	}

	fmt.Printf("\n\033[90m[LLM response: prose=%s calls=%s%s]\033[0m\n", proseState, calls, tokens)
	if prose != "" {
		fmt.Printf("  \033[90mProse:\033[0m %s\n", truncateForDebug(strings.Join(strings.Fields(prose), " ")))
	}
}

func debugToolCall(call llm.ToolCall, result string, err error) {
	if !debug {
		return
	}
	fmt.Printf("\n\033[1;35m[TOOL]\033[0m \033[1m%s\033[0m\n", call.Function.Name)
	if call.Function.Arguments != "" {
		fmt.Printf("  \033[90mArgs:\033[0m %s\n", truncateForDebug(call.Function.Arguments))
	}
	if err != nil {
		fmt.Printf("  \033[91mError:\033[0m %s\n", err.Error())
		return
	}
	if result == "" {
		return
	}
	label := "Result"
	color := "\033[92m"
	if call.Function.Name == "check" {
		label = "Dice"
	}
	fmt.Printf("  %s%s:\033[0m %s\n", color, label, truncateForDebug(result))
}

func runSetupPhase(s *session, rl *readline.Instance) error {
	fmt.Println("Welcome to Fudge!")
	fmt.Println()

	if err := setupSetting(s, rl); err != nil {
		return err
	}
	fmt.Println()

	return setupAdventure(s, rl)
}

func setupSetting(s *session, rl *readline.Instance) error {
	settings, err := listAvailableSettings()
	if err != nil {
		return err
	}

	var choice string
	if len(settings) > 0 {
		fmt.Println("Select a setting:")
		choice, err = ui.ShowChoiceMenu(settings)
		if err != nil {
			return err
		}
		if choice != ui.TypeYourAnswer() {
			return loadSetting(s, choice)
		}
	}

	description, err := ui.GetInput(rl, "Setting description: ")
	if err != nil {
		return err
	}

	client, model, _, err := llm.InitClientFromHome()
	if err != nil {
		fmt.Println("LLM error:", err)
		return err
	}

	// Phase 1: clarification, refined until the player says yes.
	clarifyPrompt, err := prompts.LoadPrompt("", language, "setting-clarification.md")
	if err != nil {
		return err
	}
	conceptRequest, err := prompts.Fill("setting-concept-request.md", map[string]string{"CONCEPT": description})
	if err != nil {
		return err
	}
	clarifyMessages := []llm.Message{
		{Role: llm.RoleSystem, Content: clarifyPrompt},
		{Role: llm.RoleUser, Content: conceptRequest},
	}

	var agreedConcept string
	for {
		fmt.Println()
		response, err := chat(client, model, clarifyMessages, nil)
		if err != nil {
			return err
		}
		fmt.Println()

		userResponse, err := ui.GetInput(rl, "Does this sound right? (yes/no): ")
		if err != nil {
			return err
		}
		if userResponse == "yes" || userResponse == "y" {
			agreedConcept = response.Message.Content
			break
		}

		clarifyMessages = append(clarifyMessages, response.Message, llm.Message{
			Role:    llm.RoleUser,
			Content: userResponse,
		})
		fmt.Println()
	}

	// Phase 2: creation. The player is still being spoken to, so this phase
	// speaks their language; the folder name it writes stays English.
	createPrompt, err := prompts.LoadPrompt("", language, "setting-creation.md")
	if err != nil {
		return err
	}
	creationRequest, err := prompts.Fill("setting-creation-request.md", map[string]string{
		"CONCEPT": description,
		"DESIGN":  agreedConcept,
	})
	if err != nil {
		return err
	}
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: createPrompt},
		{Role: llm.RoleUser, Content: creationRequest},
	}

	fmt.Println()
	for {
		response, err := chat(client, model, messages, settingCreationTools())
		if err != nil {
			return err
		}
		fmt.Println()

		if len(response.Message.ToolCalls) == 0 {
			break
		}
		messages = append(messages, response.Message)
		messages = append(messages, s.runToolCalls(response.Message.ToolCalls, true)...)
	}
	return nil
}

func setupAdventure(s *session, rl *readline.Instance) error {
	adventures, err := listAvailableAdventures(s)
	if err != nil {
		return err
	}

	if len(adventures) > 0 {
		fmt.Println("Select an adventure:")
		choice, err := ui.ShowChoiceMenu(adventures)
		if err != nil {
			return err
		}
		if choice != ui.TypeYourAnswer() {
			content, err := loadAdventure(s, choice)
			if err != nil {
				return err
			}
			s.adventure = content
			return nil
		}
	}

	fmt.Println("Describe the adventure scenario you'd like:")
	scenario, err := ui.GetInput(rl, "Scenario: ")
	if err != nil {
		return err
	} else if scenario == "" {
		return errors.New("no scenario specified")
	}

	// English regardless of what the player reads: this phase says nothing to
	// them at all — it runs silent and its whole output is a file, which becomes
	// the DM's briefing.
	prompt, err := prompts.LoadPrompt("", prompts.DefaultLanguage, "adventure-creation.md")
	if err != nil {
		return err
	}
	request, err := prompts.Fill("adventure-request.md", map[string]string{
		"SETTING":  s.settingDesc,
		"SCENARIO": scenario,
	})
	if err != nil {
		return err
	}
	retry, err := prompts.Fill("adventure-retry.md", nil)
	if err != nil {
		return err
	}
	client, model, _, err := llm.InitClientFromHome()
	if err != nil {
		return err
	}

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: prompt},
		{Role: llm.RoleUser, Content: request},
	}

	fmt.Println()
	fmt.Println("Improvising an adventure...")

	for attempt := 0; attempt < maxAdventureAttempts && s.adventure == ""; attempt++ {
		response, err := chatSilent(client, model, messages, adventureCreationTools())
		if err != nil {
			return err
		}

		if len(response.Message.ToolCalls) == 0 {
			if response.Message.Content != "" {
				messages = append(messages, response.Message)
			}
			messages = append(messages, llm.Message{
				Role:    llm.RoleUser,
				Content: retry,
			})
			continue
		}

		messages = append(messages, response.Message)
		messages = append(messages, s.runToolCalls(response.Message.ToolCalls, false)...)
	}

	if s.adventure == "" {
		return errors.New("no adventure was created")
	}
	return nil
}

// runToolCalls executes each call and returns the tool messages to append. An
// error becomes the result the model reads, so it can correct itself instead of
// the loop stalling on a call that will fail the same way twice.
func (s *session) runToolCalls(calls []llm.ToolCall, echo bool) []llm.Message {
	out := make([]llm.Message, 0, len(calls))
	for _, call := range calls {
		result, err := s.executeAction(call)
		debugToolCall(call, result, err)
		if err != nil {
			result = "Error: " + err.Error()
		} else if echo && result != "" {
			fmt.Println(result)
		}
		out = append(out, llm.Message{
			Role:       llm.RoleTool,
			ToolCallID: call.ID,
			Name:       call.Function.Name,
			Content:    result,
		})
	}
	return out
}

func listAvailableSettings() ([]string, error) {
	entries, err := os.ReadDir("settings")
	if err != nil {
		return nil, err
	}
	var settings []string
	for _, entry := range entries {
		if entry.IsDir() {
			settings = append(settings, entry.Name())
		}
	}
	sort.Strings(settings)
	return settings, nil
}

func listAvailableAdventures(s *session) ([]string, error) {
	if s.settingName == "" {
		return nil, fmt.Errorf("no setting loaded")
	}
	entries, err := os.ReadDir(filepath.Join("settings", s.settingName, "adventures"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var adventures []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			adventures = append(adventures, strings.TrimSuffix(entry.Name(), ".md"))
		}
	}
	sort.Strings(adventures)
	return adventures, nil
}

func (s *session) handleMetaCommand(chat chatRound, line string, messages *[]llm.Message) {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return
	}

	switch cmd := parts[0]; cmd {
	case "/quit", "/exit":
		fmt.Println("Session ended.")
		// os.Exit runs no deferred calls, so main's ShowCursor never fires and
		// the player is left with an invisible cursor in their shell.
		ui.ShowCursor()
		os.Exit(0)
	case "/context":
		if err := s.dumpContextToFile("context.txt", messages); err != nil {
			fmt.Printf("Error writing context: %v\n", err)
		} else {
			fmt.Println("Context written to context.txt")
		}
	case "/style":
		s.setStyle(messages, strings.TrimSpace(strings.TrimPrefix(line, cmd)))
	case "/image":
		if err := s.requestImagePrompt(chat); err != nil {
			fmt.Printf("Error generating image prompt: %v\n", err)
		} else {
			fmt.Println("Image prompt written to img-gen.md")
		}
	case "/inv":
		// The nag, and the only enforcement the sheet has. It goes in as an
		// ordinary player line so the DM answers it in the ordinary way.
		s.processGameTurn(chat, messages, s.prompts.sheetNag)
	case "/milestone":
		if err := s.milestone(chat, messages); err != nil {
			fmt.Printf("Error writing milestone: %v\n", err)
		}
	case "/help":
		printMetaCommands()
	default:
		fmt.Printf("Unknown command: %s\n", cmd)
		printMetaCommands()
	}
}

func printMetaCommands() {
	fmt.Println("Available commands:")
	fmt.Println("  /help - List these commands")
	fmt.Println("  /quit, /exit - Exit the game")
	fmt.Println("  /context - Write current message context to context.txt")
	fmt.Println("  /style <text> - Append a style directive to the DM prompt (no text clears it)")
	fmt.Println("  /image - Write an image prompt for the current scene to img-gen.md")
	fmt.Println("  /inv - Tell the DM to bring charsheet.md up to date")
	fmt.Println("  /milestone - Archive the session so far and start a fresh chat from a summary")
}

// milestone is the compaction, and the player decides when it happens. The DM
// writes the summary, because "events, states and changes" is judgment rather
// than concatenation; the harness archives the raw chat beside it, because a
// summary is lossy and the loss would be permanent.
func (s *session) milestone(chat chatRound, messages *[]llm.Message) error {
	if s.dir == "" {
		return fmt.Errorf("no session directory yet — play a turn first")
	}

	s.milestones++
	name := fmt.Sprintf("milestone-%02d.md", s.milestones)
	s.allowWrite(name)

	raw := filepath.Join(s.dir, fmt.Sprintf("milestone-%02d-raw.md", s.milestones))
	if err := os.WriteFile(raw, []byte(transcript(*messages)), 0644); err != nil {
		return err
	}

	demand, err := prompts.Fill("milestone-demand.md", map[string]string{"MILESTONE_FILE": name})
	if err != nil {
		return err
	}

	*messages = append(*messages, llm.Message{Role: llm.RoleUser, Content: demand})

	for request := 1; request <= maxTurnRequests; request++ {
		response, err := chat(s.withCharsheet(*messages), gameTools())
		if err != nil {
			return err
		}
		*messages = append(*messages, response.Message)
		if len(response.Message.ToolCalls) == 0 {
			break
		}
		*messages = append(*messages, s.runToolCalls(response.Message.ToolCalls, false)...)
	}

	// The chat is cleared down to the ground rules and the adventure, and reopens
	// on the last narration so the DM's voice carries across the seam. The
	// charsheet needs no reinstating — it rides at the tail of every request —
	// but the adventure does: it was only ever in the opening briefing, and that
	// briefing is part of what is being cleared.
	adventure, err := s.adventureBriefing()
	if err != nil {
		return err
	}
	*messages = append((*messages)[:1], llm.Message{Role: llm.RoleUser, Content: adventure})
	if s.lastNarration != "" {
		*messages = append(*messages, llm.Message{
			Role:    llm.RoleAssistant,
			Content: s.lastNarration,
		})
	}

	fmt.Printf("\nMilestone %d archived to %s. The story continues.\n\n", s.milestones, name)
	return nil
}

// transcript renders the message list as the archived raw chat.
func transcript(messages []llm.Message) string {
	var b strings.Builder
	for _, msg := range messages {
		fmt.Fprintf(&b, "--- %s ---\n", msg.Role)
		if msg.Content != "" {
			b.WriteString(msg.Content)
			b.WriteString("\n")
		}
		for _, call := range msg.ToolCalls {
			fmt.Fprintf(&b, "[call %s] %s\n", call.Function.Name, call.Function.Arguments)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// requestImagePrompt is -img-gen for one scene instead of every one: the same
// guidance, asked for once, on demand, rather than paid on every turn whether
// anyone wanted a picture or not. It is a request of its own rather than a reply
// to the ongoing conversation, asked from the last scene and the sheet alone.
func (s *session) requestImagePrompt(chat chatRound) error {
	if s.lastNarration == "" {
		return fmt.Errorf("no scene yet — play a turn first")
	}
	guidance, err := os.ReadFile(prompts.Path("image-guidance.md"))
	if err != nil {
		return err
	}

	request, err := prompts.Fill("image-request.md", nil)
	if err != nil {
		return err
	}

	var sys strings.Builder
	sys.WriteString(request)
	sys.WriteString("\n\n")
	sys.Write(guidance)
	if s.settingDesc != "" {
		fmt.Fprintf(&sys, "\nSetting: %s\n", s.settingDesc)
	}

	response, err := chat(s.withCharsheet([]llm.Message{
		{Role: llm.RoleSystem, Content: sys.String()},
		{Role: llm.RoleUser, Content: s.lastNarration},
	}), nil)
	if err != nil {
		return err
	}

	prompt := strings.TrimSpace(response.Message.Content)
	if prompt == "" {
		return fmt.Errorf("the model returned no prompt")
	}
	return writeImagePrompt(prompt)
}

// setStyle rewrites the DM's style directive in place. It moves the system
// message, which is the head of the cached prefix, so the turn after a /style
// pays for the whole prompt again — cheap for a debug session, which is what
// this is for, and it is why the directive goes last: nothing below it moves.
func (s *session) setStyle(messages *[]llm.Message, style string) {
	// The marker separates the DM prompt from whatever /style last appended. The
	// override is rewritten from the marker down, so the base prompt is never
	// stored twice and never has to be rebuilt to drop it.
	marker := "\n\n" + s.prompts.styleMarker + "\n\n"
	base, _, _ := strings.Cut((*messages)[0].Content, marker)
	if style == "" {
		(*messages)[0].Content = base
		fmt.Println("Style override cleared.")
		return
	}
	(*messages)[0].Content = base + marker + style
	fmt.Printf("Style override set: %s\n", style)
}

// dumpContextToFile writes what a request is actually made of: the messages, the
// charsheet block pinned under them, and the tool schemas that ride alongside —
// the part of the bill nothing else prints.
func (s *session) dumpContextToFile(filename string, messages *[]llm.Message) error {
	var b strings.Builder

	b.WriteString("=== TOOLS (sent with every request) ===\n\n")
	total := 0
	for _, tool := range gameTools() {
		encoded, err := json.Marshal(tool)
		if err != nil {
			continue
		}
		total += len(encoded)
		fmt.Fprintf(&b, "%-28s %6d bytes\n", tool.Function.Name, len(encoded))
	}
	fmt.Fprintf(&b, "%-28s %6d bytes\n", "TOTAL", total)

	request := s.withCharsheet(*messages)
	fmt.Fprintf(&b, "\n\n=== MESSAGES (%d, charsheet block included) ===\n\n", len(request))
	for i, msg := range request {
		fmt.Fprintf(&b, "--- Message %d ---\nRole: %s\n", i+1, msg.Role)
		if msg.Content != "" {
			b.WriteString("Content:\n")
			b.WriteString(msg.Content)
			b.WriteString("\n")
		}
		for _, tc := range msg.ToolCalls {
			fmt.Fprintf(&b, "  - %s\n", tc.Function.Name)
			if tc.Function.Arguments != "" {
				fmt.Fprintf(&b, "    Args: %s\n", tc.Function.Arguments)
			}
		}
		if msg.ToolCallID != "" {
			fmt.Fprintf(&b, "ToolCallID: %s\n", msg.ToolCallID)
		}
		b.WriteString("\n")
	}

	return os.WriteFile(filename, []byte(b.String()), 0644)
}

func runGamePhase(s *session, rl *readline.Instance) {
	// Every prompt the turn loop will need, loaded before the first request. A
	// missing file is a startup failure rather than an empty string sent to the
	// model three turns in.
	loaded, err := loadGamePrompts()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	s.prompts = loaded

	dmPrompt, err := loadDMPrompt(s)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	briefing, err := s.openingBriefing()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	client, model, _, err := llm.InitClientFromHome()
	if err != nil {
		fmt.Println("LLM error:", err)
		return
	}

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: dmPrompt},
		{Role: llm.RoleUser, Content: briefing},
	}

	chat := func(msgs []llm.Message, tools []llm.Tool) (llm.Response, error) {
		return chatSilent(client, model, msgs, tools)
	}

	// The opening turn makes the character and then plays the first scene. It
	// reacts to no player line, so it opens on the briefing alone.
	fmt.Println()
	s.processGameTurn(chat, &messages, "")

	for {
		line, err := ui.GetInput(rl, "> ")
		if err != nil {
			if !errors.Is(err, io.EOF) {
				fmt.Println("Error reading input; exiting.")
			}
			return
		}
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "/") {
			s.handleMetaCommand(chat, line, &messages)
			continue
		}

		ui.ScrollLastLineToTop()
		s.processGameTurn(chat, &messages, line)
	}
}

// openingBriefing hands the DM the adventure and the two jobs that come before
// the first scene: make the character with the player, then put the game on
// disk. The briefing stays in the history — there is no block that would
// otherwise re-state it, and the adventure is the one thing the DM cannot look
// up again.
func (s *session) openingBriefing() (string, error) {
	adventure, err := s.adventureBriefing()
	if err != nil {
		return "", err
	}
	return prompts.Fill("opening-briefing.md", map[string]string{"ADVENTURE_BRIEFING": adventure})
}

// adventureBriefing is the adventure itself, without the instructions that only
// apply to the first turn. It is re-sent after a milestone: the adventure text
// exists nowhere the DM can look it up, so clearing the chat that carried it
// would take the scenario with it.
func (s *session) adventureBriefing() (string, error) {
	return prompts.Fill("adventure-briefing.md", map[string]string{"ADVENTURE": s.adventure})
}

// imgBlock is the one thing still cut out of a reply before the player sees it.
// Everything else the DM used to append — the state block, the event tags — is
// gone; an image prompt stays inline because -img-gen wants one per scene and a
// second request per turn to ask for it separately is the cost that design was
// avoiding.
var imgBlock = regexp.MustCompile(`(?s)<img>(.*?)</img>`)

// splitImagePrompt separates the prose the player reads from the image prompt
// riding under it.
func splitImagePrompt(reply string) (prose, img string) {
	match := imgBlock.FindStringSubmatch(reply)
	if match == nil {
		return strings.TrimSpace(reply), ""
	}
	return strings.TrimSpace(imgBlock.ReplaceAllString(reply, "")), strings.TrimSpace(match[1])
}

// chatRound is one request to the model. The turn loop takes it as an argument
// rather than the client, because "a turn always ends narrated" is a property of
// the loop and not of any provider, and a loop holding its own client cannot be
// asked to prove it.
type chatRound func(messages []llm.Message, tools []llm.Tool) (llm.Response, error)

// processGameTurn drives the DM until it hands back a scene for the player, or
// until the turn has spent the maxTurnRequests calls it is allowed.
//
// A turn is rounds of rolling and note-keeping followed by prose. Unlike the
// engine-held design this replaces, nothing is folded away at the end: the whole
// exchange stays in the history until the player calls a milestone.
func (s *session) processGameTurn(chat chatRound, messages *[]llm.Message, playerInput string) {
	s.turn++

	turnStart := len(*messages)
	if playerInput != "" {
		*messages = append(*messages, llm.Message{Role: llm.RoleUser, Content: playerInput})
	}

	// wrote records whether the DM has already put this turn's changes on the
	// sheet. If it has, the bookkeeping round below is not worth a request.
	var wrote bool

	var failures int
	for request := 1; request <= maxTurnRequests; request++ {
		tools := gameTools()
		if request == maxTurnRequests {
			// The tools come off the table so there is nothing left to resolve
			// with, and the demand says so — tools that quietly vanish read as a
			// malfunction from the inside, and a malfunction gets retried.
			tools = nil
			*messages = append(*messages, llm.Message{Role: llm.RoleUser, Content: s.prompts.narrationDemand})
		}

		response, err := chat(s.withCharsheet(*messages), tools)
		if err != nil {
			// The call is spent either way. What is left of the budget decides
			// whether there is any point waiting.
			failures++
			fmt.Printf("\nLLM error: %v\n", err)
			if request == maxTurnRequests {
				break
			}
			fmt.Printf("Retrying in %s (%d of %d calls left).\n", retryDelay(failures), maxTurnRequests-request, maxTurnRequests)
			time.Sleep(retryDelay(failures))
			continue
		}
		failures = 0

		*messages = append(*messages, response.Message)

		prose, img := splitImagePrompt(response.Message.Content)

		var answeredChoice, rolled bool
		for _, call := range response.Message.ToolCalls {
			switch call.Function.Name {
			case "user_choice":
				answeredChoice = true
			case "check":
				rolled = true
			case "edit_file", "write_file":
				wrote = true
			}
		}

		// Prose written in the same response as a roll was written before the
		// dice were known, whatever it claims happened. dm.md asks for the roll
		// on its own, and this is where that is made true rather than trusted:
		// the prose is held back, the result goes in, and the next round writes
		// the scene from what actually came up. Observed in play — the DM
		// narrated a successful sneak and then rolled for it.
		if rolled {
			prose = ""
		}

		// A menu blocks on the player, so the question above it has to be on
		// screen before the options are.
		if prose != "" {
			fmt.Println(ui.WrapText(prose, ui.TerminalWidth()))
			fmt.Println()
			s.lastNarration = prose
			if err := writeNarration(prose); err != nil {
				fmt.Println("\nNarration file error:", err)
			}
			// A missing prompt is not worth interrupting play over: the player
			// has read the scene, and the next turn writes a new one.
			if img != "" {
				if err := writeImagePrompt(img); err != nil {
					fmt.Println("\nImage prompt error:", err)
				}
			}
		}

		if len(response.Message.ToolCalls) > 0 {
			*messages = append(*messages, s.runToolCalls(response.Message.ToolCalls, false)...)
			// Rolls and edits are not the turn's output; the prose that follows
			// them is. A menu is the exception — the player has answered, and the
			// DM has to be given the answer.
			if prose == "" || answeredChoice {
				continue
			}
		}

		if prose == "" {
			// A reply with neither calls nor prose says nothing at all.
			*messages = append(*messages, llm.Message{Role: llm.RoleUser, Content: s.prompts.emptyReply})
			continue
		}

		// Phase three. The scene is on the player's screen and cannot be taken
		// back, so nothing here risks the turn — only how much of what just
		// happened ends up on the sheet. It is a separate request because asking
		// "what changed?" before the prose exists is asking for a prediction,
		// while asking after it is asking for a reading.
		// Not before the session directory exists: during character creation
		// there is no sheet to reconcile against, and the round would spend a
		// request asking for edits to a file that cannot be written yet.
		if !wrote && s.dir != "" {
			s.recordChanges(chat, messages)
		}

		if debug {
			fmt.Printf("\033[90m[turn %d done; history holds %d messages]\033[0m\n\n", s.turn, len(*messages))
		}
		return
	}

	// The budget is gone and the last call, the one that had nothing to offer but
	// the narration, did not produce it. Everything the turn added is dropped: it
	// is half a turn, and carrying it would leave the DM answering later requests
	// beside a scene the player was never shown.
	*messages = (*messages)[:turnStart]
	fmt.Printf("\nError: the DM did not narrate this turn within %d calls. Try rephrasing your action.\n", maxTurnRequests)
}

// recordChanges is the turn's last phase: the DM reads back the scene it just
// wrote and puts what it established on the sheet. It runs only when the turn
// did not already write, so an ordinary turn costs one request more and a turn
// that kept its own books costs nothing extra.
//
// Failure here is reported and dropped. The player has read the scene; a sheet
// that missed one turn is a smaller problem than a turn that appears to have
// gone wrong after it plainly did not.
func (s *session) recordChanges(chat chatRound, messages *[]llm.Message) {
	if debug {
		fmt.Println("\033[90m[bookkeeping round; nothing here reaches the player]\033[0m")
	}

	*messages = append(*messages, llm.Message{Role: llm.RoleUser, Content: s.prompts.bookkeeping})

	for request := 1; request <= maxBookkeepingRequests; request++ {
		response, err := chat(s.withCharsheet(*messages), bookkeepingTools())
		if err != nil {
			fmt.Printf("\nLLM error while recording the turn: %v\n", err)
			return
		}
		*messages = append(*messages, response.Message)
		if len(response.Message.ToolCalls) == 0 {
			if debug && strings.Contains(strings.ToUpper(response.Message.Content), "NOTHING") {
				fmt.Println("\033[90m[bookkeeping: the DM reports the scene changed nothing]\033[0m")
			}
			return
		}
		*messages = append(*messages, s.runToolCalls(response.Message.ToolCalls, false)...)
	}
}

// retryDelay is how long to wait before trying a failed request again: longer
// each time, up to a ceiling. The wait only ever happens with budget still to
// spend, so it is a hiccup being given a moment, not an outage being sat out.
func retryDelay(failures int) time.Duration {
	delay := time.Duration(failures) * requestRetryDelay
	if delay > maxRetryDelay {
		return maxRetryDelay
	}
	return delay
}

// narrationDemand rides on the last call a turn is allowed. The tools are gone by
// the time the DM reads it, so the message says why rather than leaving it to
// discover that its next call does not exist.
const llmRequestTimeout = time.Minute

// chat sends one request and prints the reply's text. It is for the setup
// phases, where the LLM is talking to the user as itself. Gameplay prints its
// own prose.
func chat(client *llm.Client, model string, messages []llm.Message, tools []llm.Tool) (llm.Response, error) {
	return chatInternal(client, model, messages, tools, true)
}

// chatSilent sends one request and prints nothing: the caller decides what, if
// anything, reaches the player.
func chatSilent(client *llm.Client, model string, messages []llm.Message, tools []llm.Tool) (llm.Response, error) {
	return chatInternal(client, model, messages, tools, false)
}

// startSpinner runs the loading spinner until the returned stop function is
// called. Stopping waits for the spinner to clear its line: the reply is printed
// the instant the request returns, so a frame drawn a moment later would land on
// top of it.
func startSpinner() func() {
	done := make(chan struct{})
	cleared := make(chan struct{})
	go func() {
		defer close(cleared)
		ui.ShowLoadingSpinner(llmRequestTimeout, done)
	}()
	return func() {
		close(done)
		<-cleared
	}
}

func chatInternal(client *llm.Client, model string, messages []llm.Message, tools []llm.Tool, printOutput bool) (llm.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), llmRequestTimeout)
	defer cancel()

	if debug {
		fmt.Printf("\n\033[90m[LLM request: model=%s messages=%d tools=%d]\033[0m\n", model, len(messages), len(tools))
	}

	stopSpinner := startSpinner()

	response, err := client.Chat(ctx, llm.Request{
		Model:           model,
		Messages:        messages,
		Tools:           tools,
		ReasoningEffort: llm.ReasoningEffortNone,
	})
	stopSpinner()
	if err != nil {
		return llm.Response{}, err
	}

	// Providers may leave the role off a reply; everything downstream appends
	// this message straight back into the history, where it has to be assistant.
	if response.Message.Role == "" {
		response.Message.Role = llm.RoleAssistant
	}

	debugResponse(response)

	if printOutput && response.Message.Content != "" {
		fmt.Println(ui.WrapText(response.Message.Content, ui.TerminalWidth()))
	}

	tracker := newUsageTracker(model)
	tracker.record(messages, response)
	ui.DisplayContextUsage(tracker.getPercentage(), debug)

	return response, nil
}
