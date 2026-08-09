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
	"slices"
	"sort"
	"strings"
	"time"

	"fudge/prompts"
	"fudge/ui"

	"github.com/chzyer/readline"
	"github.com/memmaker/gollum/llm"
)

// inputLogFile is where a debug run records the player's side of the session.
const inputLogFile = "input-log.txt"

var debug bool
var imgGen bool

const (
	// maxAdventureAttempts caps the retries when the LLM won't save the adventure.
	maxAdventureAttempts = 3
	// maxCheckRequests bounds the resolve phase: the rounds in which the DM may
	// roll before the scene exists. It ends when a response comes back without
	// tool calls; this is the ceiling for one that never does.
	maxCheckRequests = 5
	// maxProseRequests bounds the narration phase, the toolless request that
	// turns resolved results into the scene. One is the normal case; the second
	// is for a reply that comes back empty.
	maxProseRequests = 2
	// maxRetryDelay is the ceiling on the wait between retried requests.
	maxRetryDelay = 30 * time.Second
)

// requestRetryDelay is the pause before retrying a failed request, multiplied by
// the number of failures so far. A variable so tests can drive the retry path
// without waiting out a backoff written for a network.
var requestRetryDelay = 2 * time.Second

// requestTokens is what a request cost, for the debug line. The provider's own
// count is preferred; a reply that omits one is estimated from the text at the
// usual four characters per token, which is close enough for a number nobody
// bills against.
func requestTokens(messages []llm.Message, response llm.Response) int {
	if total := response.Usage.TotalTokens; total > 0 {
		return total
	}
	if reported := response.Usage.InputTokens + response.Usage.OutputTokens; reported > 0 {
		return reported
	}
	estimate := 0
	for _, msg := range append(messages, response.Message) {
		estimate += len(msg.Content) / 4
		for _, call := range msg.ToolCalls {
			estimate += len(call.Function.Arguments) / 4
		}
	}
	return estimate
}

func main() {
	flag.BoolVar(&debug, "debug", false, "Enable debug mode: show tool usage, and record every answer to "+inputLogFile+" so the session can be replayed with -script")
	flag.BoolVar(&imgGen, "img-gen", false, "Enable image generation feature")
	language := flag.String("lang", "en", "Language for prompts (for example: en or de)")
	scriptFile := flag.String("script", "", "Play non-interactively from a file of answers, one per line (\"-\" reads stdin). Menus are answered by naming the option; everything else is typed verbatim. The session ends when the file does.")
	resumeFile := flag.String("resume", "", "Start at a recorded turn instead of playing up to it: the path of a log/context-NNN.json written under -debug. Nothing before that turn is played — the recorded conversation is handed to the model as it is, under ground rules rebuilt from today's prompt files. Setting, adventure and character come from the recording; the first line of the script is the turn's player line.")
	settingName := flag.String("setting", "", "Skip the setting menu and play in this one (a directory under settings/<lang>/). Requires -adventure.")
	adventureName := flag.String("adventure", "", "Skip the adventure menu and play this one (a file under the setting's adventures/, without .md). Requires -setting.")
	flag.Parse()
	if err := prompts.SetLanguage(*language); err != nil {
		fmt.Println("Error selecting language:", err)
		return
	}

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

	// A debug run writes down everything the player typed, in -script format, so
	// a bug found by playing is reproduced by replaying: fudge -script input-log.txt
	if debug {
		f, err := os.Create(inputLogFile)
		if err != nil {
			fmt.Println("Error opening input log:", err)
			return
		}
		defer f.Close()
		ui.LogInputs(f)
	}

	ui.ClearScreen()
	ui.HideCursor()
	defer ui.ShowCursor()

	s := newSession()
	if err := s.connect(); err != nil {
		fmt.Println("LLM error:", err)
		return
	}

	rl, err := readline.New("> ")
	if err != nil {
		fmt.Println("Error initializing readline:", err)
		return
	}
	defer rl.Close()

	// Setup picks the world and the scenario. The character is made in the game
	// phase, in conversation, because there is no engine left to build one in.
	// A resumed run skips all of it: the recording says which setting and
	// adventure it was played in, and the character is already in its charsheet.
	// -setting/-adventure skip only the two menus, naming what the menus would
	// have been answered with, so a scripted run still plays the game phase whole.
	var resumed *recording
	switch {
	case *resumeFile != "":
		rec, err := loadRecording(*resumeFile)
		if err != nil {
			fmt.Println("Resume error:", err)
			return
		}
		if err := loadWorld(s, rec.meta.Setting, rec.meta.Adventure); err != nil {
			fmt.Println("Resume error:", err)
			return
		}
		s.resumedFrom = *resumeFile
		resumed = &rec
	case *settingName != "" || *adventureName != "":
		if *settingName == "" || *adventureName == "" {
			fmt.Println("-setting and -adventure name one world between them: give both, or neither and pick from the menus.")
			return
		}
		if err := loadWorld(s, *settingName, *adventureName); err != nil {
			fmt.Println(err)
			return
		}
	default:
		if err := runSetupPhase(s, rl); err != nil {
			fmt.Println(err)
			return
		}
	}

	runGamePhase(s, rl, resumed)
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
	// The shape of the sheet is not here. It rides on the tail of every request
	// instead, beside the sheet itself and beside the one demand for it: a rule
	// stated in two places is a rule with two wordings to drift apart.
	return prompts.LoadPrompt(s.settingDesc, nil, files...)
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

	// One request. There was a clarification round in front of this — propose a
	// design, ask "does this sound right?", then build the agreed one — which
	// cost a prompt file, a request stub and a confirmation loop to produce
	// text that was handed straight back to the model. The setting is written to
	// disk either way, and a player who dislikes it can say so by running this
	// again.
	createPrompt, err := prompts.Fill("setting-creation.md", nil)
	if err != nil {
		return err
	}
	userPrompt, err := prompts.Fill("setting-concept.md", map[string]string{"DESCRIPTION": description})
	if err != nil {
		return err
	}
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: createPrompt},
		{Role: llm.RoleUser, Content: userPrompt},
	}

	fmt.Println()
	for {
		response, err := s.chat(messages, settingCreationTools())
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
			// The name is kept for the session directory, which the harness
			// builds out of it and the character's name from the first block.
			return loadWorld(s, s.settingName, choice)
		}
	}

	fmt.Println("Describe the adventure scenario you'd like:")
	scenario, err := ui.GetInput(rl, "Scenario: ")
	if err != nil {
		return err
	} else if scenario == "" {
		return errors.New("no scenario specified")
	}

	// This phase says nothing to the player at all: it runs silent and its whole
	// output is a file, which becomes the DM's briefing.
	prompt, err := prompts.Fill("adventure-creation.md", nil)
	if err != nil {
		return err
	}

	userPrompt, err := prompts.Fill("adventure-scenario.md", map[string]string{
		"SETTING":  s.settingDesc,
		"SCENARIO": scenario,
	})
	if err != nil {
		return err
	}
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: prompt},
		{Role: llm.RoleUser, Content: userPrompt},
	}

	fmt.Println()
	fmt.Println("Improvising an adventure...")

	for attempt := 0; attempt < maxAdventureAttempts && s.adventure == ""; attempt++ {
		response, err := s.chatSilent(messages, adventureCreationTools())
		if err != nil {
			return err
		}

		if len(response.Message.ToolCalls) == 0 {
			if response.Message.Content != "" {
				messages = append(messages, response.Message)
			}
			reminder, err := prompts.Fill("adventure-save-reminder.md", nil)
			if err != nil {
				return err
			}
			messages = append(messages, llm.Message{Role: llm.RoleUser, Content: reminder})
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
	entries, err := os.ReadDir(settingsRoot())
	if os.IsNotExist(err) {
		return nil, nil
	}
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
	entries, err := os.ReadDir(filepath.Join(settingsRoot(), s.settingName, "adventures"))
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
		s.flushSheet(chat, messages)
		fmt.Println("Session ended.")
		// os.Exit runs no deferred calls, so main's ShowCursor never fires and
		// the player is left with an invisible cursor in their shell — and the
		// recording's last context, which runGamePhase defers, is never written.
		s.writeFinalContext(*messages)
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
	case "/milestone":
		if err := s.milestone(messages); err != nil {
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
	fmt.Println("  /milestone - Archive the chat so far and continue from the character sheet")
}

// milestone is the compaction, and the player decides when it happens. It costs
// no request: the state is already whole in the charsheet, which every turn
// rewrites, so there is nothing left for a summary to carry. What the raw
// archive keeps is texture, and a model asked to summarise its own session
// would have lost that anyway.
func (s *session) milestone(messages *[]llm.Message) error {
	if s.dir == "" {
		return fmt.Errorf("no session directory yet — play a turn first")
	}

	s.milestones++
	raw := filepath.Join(s.dir, fmt.Sprintf("milestone-%02d-raw.md", s.milestones))
	if err := os.WriteFile(raw, []byte(transcript(*messages)), 0644); err != nil {
		return err
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

	fmt.Printf("\nMilestone %d archived to %s. The story continues.\n\n", s.milestones, filepath.Base(raw))
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
	guidance, err := prompts.Fill("image-guidance.md", nil)
	if err != nil {
		return err
	}

	request, err := prompts.Fill("image-request.md", map[string]string{
		"GUIDANCE": guidance,
		"SETTING":  s.settingDesc,
	})
	if err != nil {
		return err
	}

	response, err := chat(s.withCharsheet([]llm.Message{
		{Role: llm.RoleSystem, Content: request},
		{Role: llm.RoleUser, Content: s.lastNarration},
	}), nil)
	if err != nil {
		return err
	}

	prompt := strings.TrimSpace(response.Message.Content)
	if prompt == "" {
		return fmt.Errorf("the model returned no prompt")
	}
	return os.WriteFile(imagePromptFile, []byte(prompt), 0o644)
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

func runGamePhase(s *session, rl *readline.Instance, resumed *recording) {
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

	// A resumed run gets the adventure without the instructions that only apply
	// to the first turn, for the same reason a milestone does: the character it
	// would ask for has already been made, and asking again would restart the
	// game on top of the turn under test.
	briefing, err := s.openingBriefing()
	if resumed != nil {
		briefing, err = s.adventureBriefing()
	}
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	if _, err := s.createDir(savesRoot, s.adventureName); err != nil {
		fmt.Println("Session directory error:", err)
		return
	}
	fmt.Printf("\033[90m[saving to %s]\033[0m\n\n", s.dir)

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: dmPrompt},
		{Role: llm.RoleUser, Content: briefing},
	}

	chat := s.chatSilent
	// The last reply of a session is in no request's sidecar — a sidecar holds
	// what was sent, and nothing was sent after it. This is the file that closes
	// the recording's last gap.
	defer func() { s.writeFinalContext(messages) }()

	switch {
	case resumed != nil:
		// Nothing before this turn is played. The recorded conversation goes in
		// as it is, under ground rules and a briefing rebuilt from today's
		// prompt files, and the first call this run makes is the turn itself.
		messages = append(messages, resumed.messages...)
		s.turn = resumed.turn - 1
		if resumed.sheet != "" {
			if err := s.writeCharsheet(resumed.sheet); err != nil {
				fmt.Println("Charsheet error:", err)
				return
			}
		}
		fmt.Printf("\033[90m[resumed at turn %d; %d messages, %d ahead of the sheet]\033[0m\n\n",
			resumed.turn, len(messages), len(resumed.messages))
	default:
		// The opening turn makes the character and then plays the first scene.
		// It reacts to no player line, so it opens on the briefing alone.
		fmt.Println()
		s.processGameTurn(chat, &messages, "")
	}

	for {
		line, err := ui.GetInput(rl, "> ")
		if err != nil {
			if !errors.Is(err, io.EOF) {
				fmt.Println("Error reading input; exiting.")
			}
			// A script that runs out ends the session as surely as /quit does,
			// and its last turn is the one most likely to have missed.
			s.flushSheet(chat, &messages)
			return
		}

		if strings.HasPrefix(line, "/") {
			s.handleMetaCommand(chat, line, &messages)
			continue
		}

		// A stray Enter costs nothing. An empty line sent as a turn is a request
		// for the DM to invent one, which is the one thing it must not do.
		if strings.TrimSpace(line) == "" {
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

// imgBlock is the image prompt riding under the scene. It stays inline because
// -img-gen wants one per scene and a second request per turn to ask for it
// separately is the cost that design was avoiding.
var imgBlock = regexp.MustCompile(`(?s)<img>(.*?)</img>`)

// openTag is how generously the block's opening tag is recognised: any case,
// and a hyphen, underscore or space in the middle. A sheet the harness refused
// to recognise is a turn of the world lost over punctuation. The one spelling
// that passes without a carried note is the canonical one below.
const openTag = `<[ \t]*world[-_ ]?update[ \t]*>`

// worldUpdate is the whole charsheet, carried in the same response as the
// narration and cut out before anyone reads it. It is the harness's one
// positive test for what the player may see: the scene is what lies outside the
// tag. See docs/adr/007.
//
// Recognition is generous, including a code fence around the block. Acceptance
// is strict: both tags and a body, or nothing is written, because a truncated
// block would write a truncated sheet and the sheet is the only world there is.
var worldUpdate = regexp.MustCompile("(?is)(?:```[a-z]*[ \t]*\r?\n)?" + openTag + `(.*?)<[ \t]*/[ \t]*world[-_ ]?update[ \t]*>` + "(?:[ \t]*\r?\n?[ \t]*```)?")

// worldUpdateOpen is the loosest thing that could have been meant as the block.
// It catches the opening tag of a reply that was cut off mid-sheet, so the
// fragment can be dropped rather than printed as the scene.
var worldUpdateOpen = regexp.MustCompile(`(?i)` + openTag)

// textToolCall is a tool call the model wrote as prose instead of making it.
// Some chat templates leak their own call syntax into the content when the
// provider fails to parse it — deepseek's DSML markers are the case seen here —
// and the whole charsheet arrives inside it. Untreated the player is shown the
// markup and the turn loses its sheet, which are the two failures the block
// handling exists to prevent, so it is treated as one more tolerated form.
//
// ponytail: matched loosely on the marker name rather than on any one
// provider's punctuation, which is the cheap half of the fix. If a second
// template shows up with a different shape, parse the call properly instead of
// widening this.
var textToolCall = regexp.MustCompile(`(?is)<[^>]*tool_calls[^>]*>.*?name="sheet"[^>]*>(.*?)</[^>]*parameter[^>]*>.*?</[^>]*tool_calls[^>]*>`)

// fenceLine is a code fence on a line of its own. The sheet is markdown and
// never contains one, so removing them all is how a block written inside a
// fence comes out clean.
var fenceLine = regexp.MustCompile("(?m)^[ \t]*```[a-zA-Z]*[ \t]*$")

// splitReply separates the three things a narration response can carry: the
// prose the player reads, the image prompt under it, and the world update
// block.
func splitReply(reply string) scene {
	var told scene
	// Every block, not just the first. A DM that restates or corrects itself
	// emits two, and the last is the one it meant — while any block left behind
	// would be printed to the player as a raw charsheet, which is the one
	// failure this whole design exists to make impossible.
	// The written-out call is read first: its sheet is a real handover, and what
	// is left of the reply is the scene the player was owed.
	if match := textToolCall.FindStringSubmatch(reply); match != nil {
		told.sheet = strings.TrimSpace(match[1])
		told.tolerated = true
		reply = textToolCall.ReplaceAllString(reply, "")
	}

	if matches := worldUpdate.FindAllStringSubmatchIndex(reply, -1); matches != nil {
		last := matches[len(matches)-1]
		told.sheet = strings.TrimSpace(fenceLine.ReplaceAllString(reply[last[2]:last[3]], ""))
		whole := reply[last[0]:last[1]]
		told.tolerated = !strings.HasPrefix(whole, "<world-update>") || !strings.HasSuffix(whole, "</world-update>")
		reply = worldUpdate.ReplaceAllString(reply, "")
	}
	// Either there was no block at all, or the one that was there was cut off
	// before its closing tag. A partial sheet is not written and not shown.
	if told.sheet == "" {
		if open := worldUpdateOpen.FindStringIndex(reply); open != nil {
			reply = reply[:open[0]]
		}
	}

	if match := imgBlock.FindStringSubmatch(reply); match != nil {
		told.img = strings.TrimSpace(match[1])
		reply = imgBlock.ReplaceAllString(reply, "")
	}
	told.prose = strings.TrimSpace(reply)
	return told
}

// chatRound is one request to the model. The turn loop takes it as an argument
// rather than the client, because "a turn always ends narrated" is a property of
// the loop and not of any provider, and a loop holding its own client cannot be
// asked to prove it.
type chatRound func(messages []llm.Message, tools []llm.Tool) (llm.Response, error)

// scene is a narration response with the harness's cut already made: the prose
// the player reads, the image prompt under it, and the world update block that
// never reaches either the player or the chat.
type scene struct {
	prose string
	img   string
	sheet string
	// tolerated says the block arrived in a form it should not have been written
	// in. It was still accepted — refusing a whole turn of the world over
	// punctuation is the more expensive mistake — and the note rides in on the
	// next player line.
	tolerated bool
}

// processGameTurn drives the DM through the two phases of a turn: resolve,
// where checks are called, and narrate, which carries both the scene and the
// world update block. Only the second is compulsory; a turn that needs no dice
// skips the first.
//
// Unlike the engine-held design this replaces, nothing is folded away at the
// end: the whole exchange stays in the history until the player calls a
// milestone.
func (s *session) processGameTurn(chat chatRound, messages *[]llm.Message, playerInput string) {
	s.turn++

	// Recorded before the note is folded in, and before anything can fail: the
	// transcript is what the player typed, not what the DM was sent.
	if playerInput != "" {
		s.appendTranscript("> " + playerInput)
	}

	// The carried note is the harness correcting the DM without spending a
	// request on it. It goes in under the player's line and the player never
	// sees it. It stays on the session until the turn carrying it survives:
	// the rollback below takes the player's line with it, and a correction
	// dropped there would never be made again.
	if s.carried != "" {
		playerInput = strings.TrimSpace(playerInput + "\n\n" + s.carried)
	}

	turnStart := len(*messages)
	if playerInput != "" {
		*messages = append(*messages, llm.Message{Role: llm.RoleUser, Content: playerInput})
	}

	s.spoken, s.pendingSheet = nil, ""
	told, narrated := s.resolvePhase(chat, messages)
	if !narrated {
		told, _ = s.narrate(chat, messages)
	}
	s.showScene(told.prose, told.img)

	// A turn that reached the player is a turn, whichever phase the words came
	// out of. Only one that said nothing at all is half a turn, and carrying it
	// would leave the DM answering later requests beside a scene the player was
	// never shown.
	if len(s.spoken) == 0 {
		*messages = (*messages)[:turnStart]
		fmt.Println("\nError: the DM did not narrate this turn. Try rephrasing your action.")
		return
	}
	s.carried = ""

	s.recordSheet(told)

	if debug {
		fmt.Printf("\033[90m[turn %d done; history holds %d messages]\033[0m\n\n", s.turn, len(*messages))
	}
}

// recordSheet puts the turn's block on disk. A block that never came is not
// worth a request to ask for: the next one restates everything, including
// whatever happened while it was missing, so the whole correction is a note on
// the next player line.
func (s *session) recordSheet(told scene) {
	// A tool call earlier in the turn counts, even when the scene that closed the
	// turn carried no block of its own.
	if told.sheet == "" {
		told.sheet = s.pendingSheet
	}
	if told.sheet == "" {
		// The note waits for a sheet to exist, not for a directory — the
		// directory is opened at startup now and is evidence of nothing.
		// Character creation is a conversation, and a note asking for a sheet
		// that does not exist yet would only produce a half-made one. The cost
		// is recorded in AGENTS.md: a session whose opening turn emits no block
		// is never corrected at all.
		if s.sheetWritten {
			s.carried = s.prompts.noteMissing
		}
		if debug {
			fmt.Println("\033[90m[no world update block; the sheet stands and a note is carried]\033[0m")
		}
		return
	}
	if told.tolerated {
		s.carried = s.prompts.noteForm
	}
	if err := s.writeCharsheet(told.sheet); err != nil {
		fmt.Println("\nCharsheet error:", err)
	}
}

// flushSheet spends one request, at the end of a session, on a last turn that
// handed over nothing. Every other miss is repaired by the note riding the next
// player line — the final turn has no next line, so without this the change the
// player just made is lost for good.
//
// Only the sheet is wanted here, and the note that asks for it is its own:
// note-missing-block.md asks for the turn's narration *with* the sheet, and at
// the end of a session that is an invitation to do the half the model is better
// at. Measured — it narrated the scene again and called nothing in 4 of 4 runs.
// This one forbids prose and asks for the call alone.
//
// Asked twice, because resolvePhase ends on any reply that carries no tool
// call, and a refusal here is prose: measured over 20 sessions the first ask was
// answered with a call 11 times and with prose 9, and the prose was not a
// misunderstanding — it announced the very edit it was declining to make ("I
// need to update the sheet to reflect the last player action"). A model that
// close to the answer is worth one more request; a third would be arguing.
const flushAttempts = 2

func (s *session) flushSheet(chat chatRound, messages *[]llm.Message) {
	if s.carried != s.prompts.noteMissing {
		return
	}
	s.carried = ""
	var told scene
	for attempt := 1; attempt <= flushAttempts && told.sheet == ""; attempt++ {
		*messages = append(*messages, llm.Message{Role: llm.RoleUser, Content: s.prompts.noteSessionEnd})
		s.pendingSheet = ""
		told, _ = s.resolvePhase(chat, messages)
		if told.sheet == "" {
			told.sheet = s.pendingSheet
		}
	}
	if told.sheet == "" {
		if debug {
			fmt.Printf("\033[90m[the last turn's sheet was asked for %d times and did not come]\033[0m\n", flushAttempts)
		}
		return
	}
	if err := s.writeCharsheet(told.sheet); err != nil {
		fmt.Println("\nCharsheet error:", err)
	}
}

// resolvePhase is phase one: the rounds in which the DM rolls before there is a
// scene. It runs with the tools on the table and ends the moment a response
// comes back without tool calls.
//
// If that response carries prose, the turn needed no separate narration request
// and it is returned as the scene (done). Otherwise the caller moves to phase
// two, where the tools are gone and prose is the only thing left to produce.
func (s *session) resolvePhase(chat chatRound, messages *[]llm.Message) (scene, bool) {
	var failures int
	for request := 1; request <= maxCheckRequests; request++ {
		response, err := chat(s.withCharsheet(*messages), gameTools())
		if err != nil {
			// The call is spent either way. What is left of the budget decides
			// whether there is any point waiting.
			failures++
			fmt.Printf("\nLLM error: %v\n", err)
			if request == maxCheckRequests {
				return scene{}, false
			}
			fmt.Printf("Retrying in %s (%d of %d calls left).\n", retryDelay(failures), maxCheckRequests-request, maxCheckRequests)
			time.Sleep(retryDelay(failures))
			continue
		}
		failures = 0

		told := s.appendReply(messages, response.Message)

		if len(response.Message.ToolCalls) == 0 {
			// Nothing left to resolve. Prose here was written with every result
			// already in hand, so it is the scene and phase two is not needed.
			return told, told.prose != ""
		}

		// A sheet handed over by tool call belongs to whatever scene this turn
		// ends up with, which may be this response or the narration phase's. Read
		// from the response rather than from told, which also carries a sheet
		// written as a block — and a block beside a roll is written before the
		// dice land, so it must not end the turn the way a call does.
		handover := worldUpdateSheet(response.Message.ToolCalls) != ""
		if handover {
			s.pendingSheet = told.sheet
		}

		// Prose beside a tool call reaches the player like any other. It is
		// written before the call comes back, so it can claim an outcome the
		// dice then contradict — dm.md asks for the roll on its own for exactly
		// that reason — but a paragraph the model wrote for the player is not
		// the harness's to throw away, and a menu's question is only ever
		// written here, beside the call it belongs above.
		s.showScene(told.prose, told.img)
		*messages = append(*messages, s.runToolCalls(response.Message.ToolCalls, false)...)

		if handover {
			// Scene and sheet in one response is a finished turn; asking again
			// would only invite a second scene on top of the one already told.
			if told.prose != "" {
				return told, true
			}
			// A handover on its own resolved nothing, so there is nothing left to
			// roll for. The DM has updated the world and now owes a scene.
			if len(withoutSheetCalls(response.Message.ToolCalls)) == 0 {
				return scene{}, false
			}
		}
	}

	if debug {
		fmt.Printf("\033[90m[resolve phase hit its %d-request ceiling; demanding the scene]\033[0m\n", maxCheckRequests)
	}
	return scene{}, false
}

// narrate is phase two: one request with no tools at all, whose only possible
// output is the scene. The tools come off the table rather than being left
// unused, so that resolving further is not merely discouraged but unavailable.
func (s *session) narrate(chat chatRound, messages *[]llm.Message) (scene, bool) {
	*messages = append(*messages, llm.Message{Role: llm.RoleUser, Content: s.prompts.sceneDemand})

	for request := 1; request <= maxProseRequests; request++ {
		response, err := chat(s.withCharsheet(*messages), nil)
		if err != nil {
			fmt.Printf("\nLLM error: %v\n", err)
			if request == maxProseRequests {
				return scene{}, false
			}
			fmt.Printf("Retrying in %s (%d of %d calls left).\n", retryDelay(request), maxProseRequests-request, maxProseRequests)
			time.Sleep(retryDelay(request))
			continue
		}

		told := s.appendReply(messages, response.Message)
		if told.prose != "" {
			return told, true
		}
		// A reply with no tools available and no prose says nothing at all. It is
		// asked again, in the same words: there is only one thing left to do.
		*messages = append(*messages, llm.Message{Role: llm.RoleUser, Content: s.prompts.sceneDemand})
	}
	return scene{}, false
}

// appendReply makes the cut and puts what is left in the history. The block
// goes into the chat nowhere, ever: it is the whole charsheet, and the sheet is
// pinned to the tail of every request already. One copy, always current — the
// invariant from ADR 006 that this design had to survive.
func (s *session) appendReply(messages *[]llm.Message, reply llm.Message) scene {
	told := splitReply(reply.Content)
	reply.Content = told.prose
	// Handovers are left whole and capped in number: writing a real sheet is what
	// keeps the DM writing the next one, and a stub in its place measured worse
	// than no call at all. What cannot stand is one more copy of the charsheet
	// per turn forever, so the oldest go as the newest arrives. The tail pins the
	// current sheet to every request regardless; that is ADR 006's one copy.
	if sheet := worldUpdateSheet(reply.ToolCalls); sheet != "" {
		if told.sheet == "" {
			told.sheet = sheet
		}
		*messages = withoutOldestHandovers(*messages, sheetsKeptInHistory-1)
	}
	*messages = append(*messages, reply)
	return told
}

// withoutSheetCalls copies the calls with the sheet handovers dropped. A copy,
// because the caller still reads the originals.
func withoutSheetCalls(calls []llm.ToolCall) []llm.ToolCall {
	out := make([]llm.ToolCall, 0, len(calls))
	for _, call := range calls {
		if call.Function.Name != worldUpdateTool {
			out = append(out, call)
		}
	}
	return out
}

// sheetsKeptInHistory is how many handovers keep their sheet. The copies are a
// demonstration rather than storage — the current sheet reaches the model on the
// tail of every request regardless — and a DM that has just watched itself write
// three real sheets writes a fourth. Above one they stop being free: each is the
// whole charsheet, so this is the ceiling on what the history carries.
const sheetsKeptInHistory = 3

// withoutOldestHandovers drops sheet handovers until only the newest few remain,
// counting the one about to be appended. It takes the calls and the tool results
// addressed to them; an assistant message left with neither words nor calls goes
// too, since a provider has nothing to make of it.
func withoutOldestHandovers(msgs []llm.Message, keep int) []llm.Message {
	var handovers int
	for _, msg := range msgs {
		for _, call := range msg.ToolCalls {
			if call.Function.Name == worldUpdateTool {
				handovers++
			}
		}
	}

	// Only the excess goes, oldest first.
	toDrop := handovers - keep
	if toDrop <= 0 {
		return msgs
	}

	dropped := map[string]bool{}
	out := make([]llm.Message, 0, len(msgs))
	for _, msg := range msgs {
		if msg.Role == llm.RoleTool {
			if dropped[msg.ToolCallID] {
				continue
			}
			out = append(out, msg)
			continue
		}
		if len(msg.ToolCalls) > 0 {
			kept := make([]llm.ToolCall, 0, len(msg.ToolCalls))
			for _, call := range msg.ToolCalls {
				if call.Function.Name == worldUpdateTool && toDrop > 0 {
					dropped[call.ID] = true
					toDrop--
					continue
				}
				kept = append(kept, call)
			}
			msg.ToolCalls = kept
			if msg.Content == "" && len(msg.ToolCalls) == 0 {
				continue
			}
		}
		out = append(out, msg)
	}
	return out
}

// showScene puts the prose in front of the player and leaves it where the image
// generator can find it.
//
// Everything the DM says in one turn accumulates here, in the order it was
// said: a turn can speak more than once — beside a roll, above a menu, and
// again once the results are in — and the scene is all of it, not the last
// paragraph of it. Nothing the model wrote for the player is dropped for
// arriving in a response the harness did not expect it in.
func (s *session) showScene(prose, img string) {
	if prose == "" {
		return
	}
	// A DM that wrote its scene beside the roll tends to write it again once the
	// result is in, word for word. Both copies are real replies and both stay in
	// the history, but the player is only told once: a paragraph repeated in the
	// same turn reads as the scene happening twice.
	if slices.Contains(s.spoken, prose) {
		return
	}
	fmt.Println(ui.WrapText(prose, ui.TerminalWidth()))
	fmt.Println()
	// Wrapped for the terminal, unwrapped for the transcript: the wrap width
	// belongs to whatever terminal happened to run this, and a line break in the
	// middle of a sentence is how a phrase stops matching.
	s.appendTranscript(prose)
	s.spoken = append(s.spoken, prose)
	s.lastNarration = strings.Join(s.spoken, "\n\n")
	if err := os.WriteFile(narrationFile, []byte(s.lastNarration), 0o644); err != nil {
		fmt.Println("\nNarration file error:", err)
	}
	// A missing prompt is not worth interrupting play over: the player has read
	// the scene, and the next turn writes a new one.
	if img != "" {
		if err := os.WriteFile(imagePromptFile, []byte(img), 0o644); err != nil {
			fmt.Println("\nImage prompt error:", err)
		}
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
func (s *session) chat(messages []llm.Message, tools []llm.Tool) (llm.Response, error) {
	return s.chatInternal(messages, tools, true)
}

// chatSilent sends one request and prints nothing: the caller decides what, if
// anything, reaches the player.
func (s *session) chatSilent(messages []llm.Message, tools []llm.Tool) (llm.Response, error) {
	return s.chatInternal(messages, tools, false)
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

func (s *session) chatInternal(messages []llm.Message, tools []llm.Tool, printOutput bool) (llm.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), llmRequestTimeout)
	defer cancel()

	if debug {
		fmt.Printf("\n\033[90m[LLM request: model=%s messages=%d tools=%d]\033[0m\n", s.model, len(messages), len(tools))
	}

	stopSpinner := startSpinner()

	response, err := s.client.Chat(ctx, llm.Request{
		Model:           s.model,
		Messages:        messages,
		Tools:           tools,
		ReasoningEffort: llm.ReasoningEffortNone,
	})
	stopSpinner()
	// Every request goes through here, so this is the one place that sees both
	// halves of an exchange and can write them down together.
	s.logRequest(messages, tools, response, err)
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

	ui.DisplayContextUsage(requestTokens(messages, response), debug)

	return response, nil
}
