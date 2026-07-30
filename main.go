package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fudge/archive"
	"fudge/prompts"
	"fudge/rules"
	"fudge/ui"

	"github.com/chzyer/readline"
	"github.com/memmaker/gollum/llm"
)

var debug bool
var imgGen bool

// language is what the player reads: the narration, the choice menus and the
// setup conversation. Everything the engine records stays English whatever this
// is set to—ids, slugs and world_update are matched literally against what was
// written the turn before, and a translated key matches nothing.
var language string

const (
	// maxAdventureAttempts caps the retries when the LLM won't save the adventure.
	maxAdventureAttempts = 3
	// maxTurnRequests is every request a single game turn may make of the model,
	// failed ones included. The last of them is spent demanding the narration, so
	// a turn has maxTurnRequests-1 rounds to resolve whatever it needs and one to
	// end on. A turn still unnarrated after that is an error.
	maxTurnRequests = 5
	// maxRecordRequests is what the bookkeeping phase after the narration may
	// spend: one round to write the prose down, and one to answer whatever the
	// engine refused. It is a budget of its own rather than the remainder of the
	// turn's, because the two phases fail differently — a turn out of resolution
	// rounds owes the player a scene, and a turn out of bookkeeping rounds has
	// already given them one.
	maxRecordRequests = 2
	// maxRetryDelay is the ceiling on the wait between retried requests.
	maxRetryDelay = 30 * time.Second
	// pinnedMessages is how many messages sit above the ENGINE STATE block and
	// never change again: the DM prompt and the opening briefing, built once in
	// runGamePhase. They are the whole of the history between turns, and being
	// byte-identical all session is what makes them worth caching. Anything
	// added to that pair has to be counted here too, or the block would be
	// pinned above it.
	pinnedMessages = 2
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

// LLM-guided game setup with adventure loading
func main() {
	flag.BoolVar(&debug, "debug", false, "Enable debug mode to show tool usage")
	flag.BoolVar(&imgGen, "img-gen", false, "Enable image generation feature")
	flag.StringVar(&language, "lang", prompts.DefaultLanguage, "Language the player reads: narration, menus and setup. A name or an ISO code (\"German\", \"Deutsch\", \"de\"). Engine data stays English.")
	flag.Parse()
	language = prompts.NormalizeLanguage(language)

	ui.ClearScreen()
	ui.HideCursor()
	defer ui.ShowCursor()

	seed := time.Now().UnixNano()

	w := &rules.World{
		Characters: make(map[string]*rules.Character),
		Seed:       seed,
		Rng:        rand.New(rand.NewSource(seed)),
	}

	// Setup readline instance with history and line editing
	rl, err := readline.New("> ")
	if err != nil {
		fmt.Println("Error initializing readline:", err)
		return
	}
	defer rl.Close()

	// Setup phase: LLM guides through setting, character, and adventure selection
	err = runSetupPhase(w, rl)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Game phase: LLM acts as DM
	runGamePhase(w, rl)
}

func loadCharacterPrompt(promptFile string, w *rules.World) (string, error) {
	return prompts.LoadPrompt(w.Setting, language, promptFile)
}

func loadDMPrompt(w *rules.World) (string, error) {
	files := []string{"system-prompts/dm.md"}
	if imgGen {
		files = append(files, "system-prompts/img-style.md")
	}
	return prompts.LoadPrompt(w.Setting, language, files...)
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

// debugResponse reports what came back: whether the reply carried prose—text
// written outside any tool call—which tools it called, and what it cost. Prose
// is worth calling out on its own line: during gameplay it reaches nobody, since
// the player only ever reads output_narration.
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
		// Collapsed to a single line: a paragraph break in the reply would
		// otherwise scatter this across the log.
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
	} else if result != "" {
		// A state-changing tool answers with its headline and then the whole
		// character underneath. That block is for the DM; a debug line is meant
		// to be glanceable, and the sheet it repeats is in charsheet.txt.
		headline := resultHeadline(result)
		if headline != result {
			headline += " \033[90m+state\033[0m"
		}
		// For dice-rolling tools, also display roll breakdown
		switch call.Function.Name {
		case "unopposed_check", "opposed_check", "opposed_attack", "unopposed_attack":
			fmt.Printf("  \033[92mDice:\033[0m %s\n", headline)
		default:
			fmt.Printf("  \033[92mResult:\033[0m %s\n", headline)
		}
	}
}

func runSetupPhase(w *rules.World, rl *readline.Instance) error {
	fmt.Println("Welcome to Fudge!")
	fmt.Println()

	// Step 1: Select or create setting
	err := setupSetting(w, rl)
	if err != nil {
		return err
	}
	fmt.Println()

	// Step 2: Select or improvise adventure
	err = setupAdventure(w, rl)
	if err != nil {
		return err
	}
	fmt.Println()

	// Step 3: Create character with LLM
	err = setupCharacter(w, rl)
	if err != nil {
		return err
	}

	return nil
}

func setupSetting(w *rules.World, rl *readline.Instance) error {
	settings, err := listAvailableSettings()
	if err != nil {
		return err
	}

	var choice string
	if len(settings) > 0 {
		fmt.Println("Select a setting:")
		var err error
		choice, err = ui.ShowChoiceMenu(settings)
		if err != nil {
			return err
		}

		// Check if user selected an existing setting
		if choice != ui.TypeYourAnswer() {
			if err := loadSetting(w, choice); err != nil {
				return err
			}
			return nil
		}
	}

	// User wants custom setting - get description
	description, err := ui.GetInput(rl, "Setting description: ")
	if err != nil {
		return err
	}

	client, model, _, err := llm.InitClientFromHome()
	if err != nil {
		fmt.Println("LLM error:", err)
		return err
	}

	// Phase 1: Clarification (with loop for refinement)
	clarifyPrompt, _ := prompts.LoadPrompt(nil, language, "system-prompts/setting-clarification.md")
	clarifyMessages := []llm.Message{
		{Role: llm.RoleSystem, Content: clarifyPrompt},
		{Role: llm.RoleUser, Content: fmt.Sprintf("I'd like to create a setting with this concept: %s", description)},
	}

	// The agreed-upon proposal, carried into the creation phase
	var agreedConcept string

	for {
		fmt.Println()
		response, err := chat(client, model, clarifyMessages, nil)
		if err != nil {
			return err
		}

		fmt.Println()

		// Get user confirmation - their response may include feedback
		userResponse, err := ui.GetInput(rl, "Does this sound right? (yes/no): ")
		if err != nil {
			return err
		}

		if userResponse == "yes" || userResponse == "y" {
			agreedConcept = response.Message.Content
			break
		}

		// User said no (possibly with feedback) - keep the real proposal in
		// history and send their feedback back for refinement
		clarifyMessages = append(clarifyMessages, response.Message)
		clarifyMessages = append(clarifyMessages, llm.Message{
			Role:    llm.RoleUser,
			Content: userResponse,
		})
		fmt.Println()
	}

	// Phase 2: Creation - LLM will call create_setting and write_setting_file tools
	// The player is still being talked to here, so this phase speaks their
	// language—while the files it writes stay English, which the directive says
	// in as many words. Those files are read back as the setting's list of
	// slugs, and a German skill slug is a skill no tool call can ever name.
	createPrompt, _ := prompts.LoadPrompt(nil, language, "system-prompts/setting-creation.md")
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: createPrompt},
		{Role: llm.RoleUser, Content: fmt.Sprintf("Create a custom Fudge setting with this concept: %s\n\nThe agreed-upon design:\n%s", description, agreedConcept)},
	}

	fmt.Println()

	// Agentic loop: continue until LLM stops making tool calls
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

		// Process each tool call and collect results
		for _, toolCall := range response.Message.ToolCalls {
			result, err := executeAction(w, toolCall)
			debugToolCall(toolCall, result, err)
			if err != nil {
				// Feed the error back so the LLM can correct and retry
				result = "Error: " + err.Error()
			}
			fmt.Println(result)

			messages = append(messages, llm.Message{
				Role:       llm.RoleTool,
				ToolCallID: toolCall.ID,
				Name:       toolCall.Function.Name,
				Content:    result,
			})
		}
	}

	// Return a placeholder name (user should have created it via tool)
	return nil
}

func setupAdventure(w *rules.World, rl *readline.Instance) error {
	adventures, err := listAvailableAdventures(w)
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
			adventureContent, err := loadAdventureTool(w, choice)
			if err != nil {
				return err
			}
			w.AdventureContent = adventureContent
			return nil
		}
	}

	// User wants improvised adventure - use LLM
	fmt.Println("Describe the adventure scenario you'd like:")
	scenario, err := ui.GetInput(rl, "Scenario: ")
	if err != nil {
		return err
	} else if scenario == "" {
		return errors.New("no scenario specified")
	}

	// English regardless of what the player reads: this phase says nothing to
	// them at all—it runs silent and its whole output is a file, which becomes
	// the DM's briefing. Briefings are the engine's side of the wall.
	prompt, _ := prompts.LoadPrompt(nil, prompts.DefaultLanguage, "system-prompts/adventure-creation.md")
	client, model, _, err := llm.InitClientFromHome()
	if err != nil {
		return err
	}

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: prompt},
		{Role: llm.RoleUser, Content: fmt.Sprintf("Improvise an adventure for the %s setting with this scenario: %s", w.Setting.Description, scenario)},
	}

	fmt.Println()
	fmt.Println("Improvising an adventure...")

	// The adventure arrives through write_adventure, which persists it and sets
	// w.AdventureContent. Loop so a rejected write can be corrected.
	for attempt := 0; attempt < maxAdventureAttempts && w.AdventureContent == ""; attempt++ {
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
				Content: "You did not save the adventure. Call write_adventure with the title and full content now.",
			})
			continue
		}

		messages = append(messages, response.Message)

		for _, toolCall := range response.Message.ToolCalls {
			result, err := executeAction(w, toolCall)
			debugToolCall(toolCall, result, err)
			if err != nil {
				result = "Error: " + err.Error()
			}
			messages = append(messages, llm.Message{
				Role:       llm.RoleTool,
				ToolCallID: toolCall.ID,
				Name:       toolCall.Function.Name,
				Content:    result,
			})
		}
	}

	if w.AdventureContent == "" {
		return errors.New("no adventure was created")
	}
	return nil
}

func setupCharacter(w *rules.World, rl *readline.Instance) error {
	fmt.Println("How would you like to create your character?")
	automaticString := "Automatic - I describe my concept, you decide the traits"
	choice, err := ui.ShowChoiceMenu([]string{automaticString, "Custom - I want to choose all my traits"})
	if err != nil {
		return err
	}
	var concept string
	if choice == automaticString {
		fmt.Println("Describe your character concept:")
		var cerr error
		for concept == "" {
			concept, cerr = ui.GetInput(rl, "Concept: ")
			if cerr != nil {
				return cerr
			}
		}
	}

	setupCharacterFlow(w, rl, concept)
	return nil
}

func setupCharacterFlow(w *rules.World, rl *readline.Instance, concept string) {
	var promptFile string
	var initialMessage string

	if concept != "" {
		promptFile = "system-prompts/character-automatic.md"
		initialMessage = fmt.Sprintf("I want to create a character with this concept: %s", concept)
	} else {
		promptFile = "system-prompts/character-custom.md"
		initialMessage = "I'm ready to create my character. Let's start!"
	}

	characterPrompt, err := loadCharacterPrompt(promptFile, w)
	if err != nil {
		fmt.Println("Error loading character creation prompt:", err)
		return
	}

	client, model, _, err := llm.InitClientFromHome()
	if err != nil {
		fmt.Println("LLM error:", err)
		return
	}

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: characterPrompt},
		{Role: llm.RoleUser, Content: initialMessage},
	}

	// Creation ends once the character exists and carries something. Their gear
	// usually lands one response after they do — the id add_item needs only
	// comes back in the create_character result — so the response that creates
	// them buys exactly one follow-up turn to outfit them, and creation ends
	// after that whether or not they took it. Handing the model any turn beyond
	// that only buys a paragraph of congratulation that the opening scene is
	// about to narrate over anyway.
	characterCreated := false
	creationDone := func() bool {
		pc, err := w.Character(w.Player)
		if err != nil {
			return false
		}
		if len(pc.Inventory) > 0 || pc.Coins > 0 || characterCreated {
			fmt.Printf("\n%s is ready.\n", pc.Name)
			return true
		}
		characterCreated = true // spend the one follow-up turn
		return false
	}

	for {
		fmt.Println()
		response, err := chat(client, model, messages, characterCreationTools())
		if err != nil {
			fmt.Println("\nLLM error:", err)
			return
		}

		messages = append(messages, response.Message)

		// Process tool calls
		for len(response.Message.ToolCalls) > 0 {
			for _, toolCall := range response.Message.ToolCalls {
				result, err := executeAction(w, toolCall)
				debugToolCall(toolCall, result, err)
				if err != nil {
					result = "Error: " + err.Error()
				}
				messages = append(messages, llm.Message{
					Role:       llm.RoleTool,
					ToolCallID: toolCall.ID,
					Name:       toolCall.Function.Name,
					Content:    result,
				})
			}

			err := writeCharsheet(w)
			if err != nil {
				fmt.Println("Error writing char sheet:", err)
			}

			if creationDone() {
				return
			}

			fmt.Println()
			response, err = chat(client, model, messages, characterCreationTools())
			if err != nil {
				fmt.Println("\nLLM error:", err)
				return
			}
			fmt.Println()
			messages = append(messages, response.Message)
		}

		// The model ended its turn on plain text. If that was the follow-up turn
		// after create_character, the character stands equipped or not and there
		// is nothing left to ask the player.
		if creationDone() {
			return
		}

		// Get player input
		fmt.Println()
		input, err := ui.GetInput(rl, "> ")

		if input != "" {
			messages = append(messages, llm.Message{Role: llm.RoleUser, Content: input})
		}
	}
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

func listAvailableAdventures(w *rules.World) ([]string, error) {
	if w.Setting == nil {
		return nil, fmt.Errorf("no setting loaded")
	}
	adventuresPath := filepath.Join(w.Setting.Path, "adventures")
	entries, err := os.ReadDir(adventuresPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var adventures []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			name := strings.TrimSuffix(entry.Name(), ".md")
			adventures = append(adventures, name)
		}
	}
	sort.Strings(adventures)
	return adventures, nil
}

func slugify(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, " ", "-"))
}

func (s *session) handleMetaCommand(line string, messages *[]llm.Message) {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return
	}

	cmd := parts[0]
	switch cmd {
	case "/quit", "/exit":
		fmt.Println("Session ended.")
		os.Exit(0)
	case "/context":
		err := s.dumpContextToFile("context.txt", messages)
		if err != nil {
			fmt.Printf("Error writing context: %v\n", err)
		} else {
			fmt.Println("Context written to context.txt")
		}
	case "/style":
		setStyle(messages, strings.TrimSpace(strings.TrimPrefix(line, cmd)))
	default:
		fmt.Printf("Unknown command: %s\n", cmd)
		fmt.Println("Available commands:")
		fmt.Println("  /quit, /exit - Exit the game")
		fmt.Println("  /context - Write current message context to context.txt")
		fmt.Println("  /style <text> - Append a style directive to the DM prompt (no text clears it)")
	}
}

// styleMarker separates the DM prompt from whatever /style last appended. The
// override is rewritten from the marker down, so the base prompt is never stored
// twice and never has to be rebuilt to drop it.
const styleMarker = "\n\n## Style Override\n\nThis outranks any style rule above it:\n\n"

// setStyle rewrites the DM's style directive in place. It moves the system
// message, which is the head of the cached prefix, so the turn after a /style
// pays for the whole prompt again — cheap for a debug session, which is what
// this is for, and it is why the directive goes last: nothing below it moves.
func setStyle(messages *[]llm.Message, style string) {
	base, _, _ := strings.Cut((*messages)[0].Content, styleMarker)
	if style == "" {
		(*messages)[0].Content = base
		fmt.Println("Style override cleared.")
		return
	}
	(*messages)[0].Content = base + styleMarker + style
	fmt.Printf("Style override set: %s\n", style)
}

// dumpContextToFile writes what the DM is working from. The messages are only
// part of that now and, between turns, much the smaller part: the block is the
// DM's entire memory of the session, and the archive is the record of how it
// came to hold what it holds. A dump of the messages alone would show a session
// that appears to have just started.
func (s *session) dumpContextToFile(filename string, messages *[]llm.Message) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := &strings.Builder{}

	writer.WriteString("=== ENGINE STATE, as the DM last saw it ===\n\n")
	if s.lastBlock != "" {
		writer.WriteString(s.lastBlock)
	} else {
		writer.WriteString("(no turn has been played yet)")
	}
	writer.WriteString("\n\n=== ARCHIVE ===\n\n")
	writer.WriteString(s.archive.Dump())
	writer.WriteString("\n\n=== MESSAGE CONTEXT ===\n\n")
	writer.WriteString(fmt.Sprintf("Total messages: %d\n\n", len(*messages)))

	for i, msg := range *messages {
		writer.WriteString(fmt.Sprintf("--- Message %d ---\n", i+1))
		writer.WriteString(fmt.Sprintf("Role: %s\n", msg.Role))

		if msg.Content != "" {
			writer.WriteString("Content:\n")
			writer.WriteString(msg.Content)
			writer.WriteString("\n")
		}

		if len(msg.ToolCalls) > 0 {
			writer.WriteString(fmt.Sprintf("Tool Calls (%d):\n", len(msg.ToolCalls)))
			for _, tc := range msg.ToolCalls {
				writer.WriteString(fmt.Sprintf("  - %s\n", tc.Function.Name))
				if tc.Function.Arguments != "" {
					writer.WriteString(fmt.Sprintf("    Args: %s\n", tc.Function.Arguments))
				}
			}
		}

		if msg.ToolCallID != "" {
			writer.WriteString(fmt.Sprintf("ToolCallID: %s\n", msg.ToolCallID))
		}

		if msg.Name != "" {
			writer.WriteString(fmt.Sprintf("Name: %s\n", msg.Name))
		}

		writer.WriteString("\n")
	}

	_, err = file.WriteString(writer.String())
	return err
}

func runGamePhase(w *rules.World, rl *readline.Instance) {
	dmPrompt, err := loadDMPrompt(w)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	client, model, _, err := llm.InitClientFromHome()
	if err != nil {
		fmt.Println("LLM error:", err)
		return
	}

	// These two are the whole of the history between turns, and they never
	// change again: everything a turn produces is folded into the session and
	// dropped. That is what keeps them byte-identical, and cached, all session.
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: dmPrompt},
		{Role: llm.RoleUser, Content: openingBriefing(w)},
	}

	s := newSession(w)

	chat := func(msgs []llm.Message, tools []llm.Tool) (llm.Response, error) {
		return chatSilent(client, model, msgs, tools)
	}

	// Setup is done and the character exists—open the scene right away instead
	// of making the player prod us for it. There is no player line to react to,
	// so the turn opens on the briefing alone.
	fmt.Println()
	s.processGameTurn(chat, &messages, "")

	for {
		// Get player input
		line, err := ui.GetInput(rl, "> ")
		if err != nil {
			fmt.Println("Error reading input; exiting.")
			return
		}

		if line == "" {
			continue
		}

		// Handle meta commands
		if strings.HasPrefix(line, "/") {
			s.handleMetaCommand(line, &messages)
			continue
		}

		ui.ScrollLastLineToTop()
		s.processGameTurn(chat, &messages, line)
	}
}

// narrationOutput is the payload of an output_narration call: the prose goes to
// the player, then to narration.txt and the archive, the world update goes to
// the ledger, and the image prompt—present only under --img-gen—goes to
// img-gen.md.
type narrationOutput struct {
	Prose       string `json:"prose"`
	Location    string `json:"location"`
	WorldUpdate string `json:"world_update"`
	ImgGen      string `json:"img_gen"`
}

// openingBriefing gives the DM the adventure it must open with. Setup is over
// and gameplay starts here, so the instruction leads and closes the message:
// everything between the two is reference material, not something to reply
// about.
//
// The player character is not in it. This message is written once and then
// never again, which is exactly wrong for a character who is about to start
// spending coins and taking wounds — a snapshot here would be contradicting the
// roster by the second turn. The roster states them instead, rebuilt every turn,
// and it is in front of the DM before this briefing is ever read.
func openingBriefing(w *rules.World) string {
	var b strings.Builder
	b.WriteString("Character creation is finished and play begins now. This is turn one: there is no player input to react to, you open the game.\n\n")
	b.WriteString("Your entire reply is the first scene, passed as the prose argument of output_narration. Put the player character in it and stop where they can act. Do not acknowledge, summarize, or comment on the briefing below—play it.\n\n")

	b.WriteString("ADVENTURE BRIEFING (the player has not read this):\n\n")
	b.WriteString(w.AdventureContent)
	b.WriteString("\n\n")

	b.WriteString("Nothing needs resolving to open a scene, so this turn is a single response, and it holds two calls. First create_location for the place the scene opens in: no location exists yet, and a narration can only name one that does. Then one output_narration carrying the opening prose, that location's slug as its location, and as world_update the full baseline of everything the prose establishes. The player character is in the ROSTER above, with everything they carry—put them in the scene as the engine holds them. Narrate the opening scene now.")
	return b.String()
}

// chatRound is one request to the model. The turn loop takes it as an argument
// rather than the client, because "a turn always ends narrated" is a property of
// the loop and not of any provider, and a loop holding its own client cannot be
// asked to prove it.
type chatRound func(messages []llm.Message, tools []llm.Tool) (llm.Response, error)

// processGameTurn drives the DM until it hands back narration for the player, or
// until the turn has spent the maxTurnRequests calls it is allowed — a turn ends
// narrated or it ends in an error, and either way it ends inside this function.
// Everything the player reads arrives through output_narration, so nothing is
// streamed straight to them: tool calls (checks, NPC creation, ...) are executed
// and fed back in the same round, and a response without narration forces
// another request, since the DM must see the results before it can narrate them.
//
// A turn has two halves, and they are told apart by what the calls are for. The
// rounds below hold only consultTools: the dice, the lookups, the constructors —
// everything whose answer the DM needs before it can know what happened. The
// bookkeeping that follows from what happened is not on the table at all here; it
// waits for recordPhase, which runs once the scene is on screen and asks the same
// model to read its own prose back.
func (s *session) processGameTurn(chat chatRound, messages *[]llm.Message, playerInput string) {
	s.turn++

	// Built once, here, and held for the rest of the turn. A turn takes two
	// requests more often than one, and a block rebuilt per request would move
	// the cached prefix every time — while telling the DM nothing it is not
	// about to be told anyway, since a call that moves a character answers with
	// that character's state in the same round.
	block := s.engineState()
	s.lastBlock = block

	turnStart := len(*messages)
	if playerInput != "" {
		*messages = append(*messages, llm.Message{Role: llm.RoleUser, Content: playerInput})
	}

	// What the turn leaves behind. None of it stays in the history: the engine's
	// headlines and the prose go to the archive, the reported facts go to the
	// ledger, and the messages themselves are dropped where they stand.
	var log turnLog

	// A turn gets maxTurnRequests calls to the model and no more. Every one counts,
	// a call that came back an error included: the budget is there to bound what a
	// single line of player input can cost, and a request that failed cost the
	// same round trip as one that worked.
	//
	// The last call is not a resolution round. The engine comes off the table so
	// that the only call left is the narration, because the turn has to end and
	// this is the last chance it has to end properly.
	var failures int

	for request := 1; request <= maxTurnRequests; request++ {
		tools := consultTools()
		if request == maxTurnRequests {
			tools = narrationOnlyTools()
			// Said in words as well as by withdrawing the tools: a request whose
			// tools quietly vanish reads as a malfunction from the inside, and a
			// malfunction is answered by trying the call again.
			*messages = append(*messages, llm.Message{Role: llm.RoleUser, Content: narrationDemand})
		}

		response, err := chat(withEngineState(block, *messages), tools)
		if err != nil {
			// The call is spent either way. What is left of the budget decides
			// whether there is any point waiting: a hiccup with rounds to spare is
			// worth retrying, and the last call is not.
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

		// The calls in a response arrive in no guaranteed order, so the turn is
		// replayed in the order the player experiences it: first everything
		// that decides what happened, then the prose about it, and only then
		// the menu that asks them what happens next.
		var narration *narrationOutput
		results := make(map[string]string, len(response.Message.ToolCalls))

		// Everything that changes the world runs before the narration is read,
		// whatever order the model emitted them in. A turn that walks somewhere
		// new creates the location and narrates it in one response, and the
		// narration names a slug the create call has to have minted already.
		for _, toolCall := range response.Message.ToolCalls {
			var result string
			switch toolCall.Function.Name {
			case "user_choice", "output_narration":
				continue
			case "recall":
				result = s.recall(toolCall)
				debugToolCall(toolCall, result, nil)
			default:
				result, err = executeAction(s.world, toolCall)
				debugToolCall(toolCall, result, err)
				if err != nil {
					result = "Error: " + err.Error()
				}
				// The headline is the engine's own account of what happened, and
				// the archive keeps it so that events survive whether or not the
				// DM thought them worth reporting. The character state under it
				// does not: the roster states that, currently, every turn.
				log.events = append(log.events, toolCall.Function.Name+"|"+resultHeadline(result))
			}
			results[toolCall.ID] = result
		}

		// Now the narration, which the calls above have made sense of.
		for _, toolCall := range response.Message.ToolCalls {
			if toolCall.Function.Name != "output_narration" {
				continue
			}
			var result string
			parsed, parseErr := parseNarration(toolCall)
			// The scene has to happen somewhere the engine holds, or the
			// position it is about to record is a slug that means nothing.
			// Refusing costs one silent round; letting it through costs the
			// engine its grip on where everybody is.
			_, locErr := s.world.Location(parsed.Location)
			switch {
			case parseErr != nil:
				result = fmt.Sprintf("Error: could not read your narration (%v). The player was shown nothing—call output_narration again.", parseErr)
			case locErr != nil:
				result = fmt.Sprintf("Error: %v. The player was shown nothing—create the location if it is new, then call output_narration again with a slug that exists.", locErr)
			default:
				// Through the engine's own move, so that how a character
				// changes place has one home. A world with no player yet is not
				// an error here: the setup phases narrate before anybody exists.
				s.world.MoveCharacter(s.world.Player, parsed.Location)
				result = "narration shown to player"
				narration = &parsed
			}
			debugToolCall(toolCall, result, nil)
			results[toolCall.ID] = result
		}

		// Update charsheet after all mutations
		if err := writeCharsheet(s.world); err != nil {
			fmt.Println("\nOutput error:", err)
		}

		if narration != nil {
			// Reported facts reach the ledger the moment they land, rather than
			// at the end of the turn: a turn can narrate more than once — a
			// choice menu makes it — and the second narration is written by a DM
			// that should already be able to see what the first one established.
			for _, note := range s.ledger.Apply(narration.WorldUpdate, s.turn, s.isCharacter) {
				if debug {
					fmt.Printf("\033[90m[ledger: %s]\033[0m\n", note)
				}
			}
			log.shown = append(log.shown, narration.Prose)
			log.updates = append(log.updates, narration.WorldUpdate)

			fmt.Println(ui.WrapText(narration.Prose, ui.TerminalWidth()))
			fmt.Println()
			// The player has read the scene already, so a failed write is worth
			// reporting but not worth stopping the turn for.
			if err := writeNarration(narration.Prose); err != nil {
				fmt.Println("\nNarration file error:", err)
			}
			// A missing prompt is not worth interrupting play over: the player
			// has already read the scene, and the next turn writes a new one.
			if narration.ImgGen != "" {
				if err := writeImagePrompt(narration.ImgGen); err != nil {
					fmt.Println("\nImage prompt error:", err)
				}
			}
		}

		// A choice menu blocks on the player, so it runs under the prose that
		// poses the question rather than ahead of it.
		var answeredChoice bool
		for _, toolCall := range response.Message.ToolCalls {
			if toolCall.Function.Name != "user_choice" {
				continue
			}
			result, choiceErr := executeAction(s.world, toolCall)
			debugToolCall(toolCall, result, choiceErr)
			if choiceErr != nil {
				result = "Error: " + choiceErr.Error()
			} else {
				answeredChoice = true
				// What they picked is player input like any other, so it joins
				// the line that opened the turn rather than becoming a turn of
				// its own — the turn is still the same one.
				log.choices = append(log.choices, result)
			}
			results[toolCall.ID] = result
		}

		for _, toolCall := range response.Message.ToolCalls {
			*messages = append(*messages, llm.Message{
				Role:       llm.RoleTool,
				ToolCallID: toolCall.ID,
				Name:       toolCall.Function.Name,
				Content:    results[toolCall.ID],
			})
		}

		if narration == nil {
			// Setup calls (checks, NPC creation): their results above give the
			// DM what it needs to narrate next round. This is the round the turn
			// is allowed a finite number of.
			if len(response.Message.ToolCalls) > 0 {
				continue
			}
			// Prose written as a plain reply reaches nobody. Say what went wrong
			// rather than resending an identical context and hoping for a different
			// answer — and it costs a call of the turn's budget either way.
			if response.Message.Content != "" {
				*messages = append(*messages, llm.Message{
					Role:    llm.RoleUser,
					Content: "That reply went nowhere: the player only ever sees the prose argument of output_narration, and they have still been shown nothing. Send this turn's narration through output_narration now. Nothing you wrote outside the call is kept, so repeat all of it in the call—the narration as `prose`, and anything it established as `world_update`, even if you already wrote that state as plain text.",
				})
				continue
			}
			// A reply with neither calls nor text says nothing at all.
			*messages = append(*messages, llm.Message{
				Role:    llm.RoleUser,
				Content: "That reply was empty: no narration, no tool call, nothing the player can read. The turn is not over until you call output_narration. Send it now, with its world_update.",
			})
			continue
		}

		// The player just picked an option, so the turn is not over: handing
		// back to the prompt now would leave their answer unanswered, and the
		// collapse below would drop it from history unread.
		if answeredChoice {
			continue
		}

		// Narrated, so the story of the turn is settled and the books can be
		// squared against it. This is the one moment in the turn where the prose
		// exists and the history holding it has not been dropped yet.
		s.recordPhase(chat, messages, &log)

		// And now the turn is over — and it leaves nothing behind. Every part of
		// it now has exactly one home: what the engine computes is in the roster,
		// what only the prose knew is in the ledger, what happened is in the
		// archive. The messages are the fourth copy, and the one that used to
		// grow without bound, so they go.
		s.recordTurn(playerInput, log)
		*messages = (*messages)[:turnStart]
		if debug {
			fmt.Printf("\033[90m[turn %d folded into the block; history holds %d messages]\033[0m\n\n", s.turn, len(*messages))
		}
		return
	}

	// The budget is gone and the last call, the one that had nothing to offer but
	// the narration, did not produce it. The turn is over and it failed: say so
	// plainly rather than leaving the player waiting on a scene that is not coming.
	//
	// Everything the turn added is dropped. It is half a turn — calls whose
	// outcome nobody ever read — and carrying it would put the DM in every later
	// request beside a scene the player was never shown.
	//
	// The archive still takes it, marked for what it was. A gap in the record is
	// worth being able to explain, and the timeline leaves unnarrated turns out
	// anyway, so nothing the player never saw reaches the DM through it.
	//
	// A turn can run out of budget having already shown the player something — a
	// choice menu narrates and then goes back to resolving — and prose they have
	// read has to be squared with the engine however the turn ended. When nothing
	// reached them this returns without spending anything.
	s.recordPhase(chat, messages, &log)
	s.recordTurn(playerInput, log)
	*messages = (*messages)[:turnStart]
	fmt.Printf("\nError: the DM did not narrate this turn within %d calls. Try rephrasing your action.\n", maxTurnRequests)
}

// recordPhase is the second half of a turn: the player has read the scene, and
// now the DM squares the engine with what it just told them. Only recordTools
// are on the table, the prose is quoted back, and the block is rebuilt so the
// roster it reads names off is the one the turn has just left behind.
//
// The order is the whole point. Asked before the prose existed, "what changed?"
// is a question about text the model has not written yet — a prediction of its
// own output, made from nothing. Asked here it is a question about text sitting
// in front of it, which is a far easier thing to answer correctly. It costs a
// round trip, but the player is reading while it runs, so it is a round trip
// nobody waits on.
func (s *session) recordPhase(chat chatRound, messages *[]llm.Message, log *turnLog) {
	// Nothing reached the player, so there is no prose to read back and nothing
	// it committed the engine to.
	if len(log.shown) == 0 {
		return
	}

	// Rebuilt rather than reused. The turn's own calls have moved the roster
	// since the block above it was rendered, and this phase names items and
	// characters off it: reading a stale one is how a turn records an item onto
	// a corpse it just made or moves a knife that is already on the floor.
	//
	// It does not become lastBlock. That one is kept for the context dump, whose
	// question is always what the DM was looking at when it wrote the prose —
	// and this block is the one rendered after.
	block := s.engineState()
	*messages = append(*messages, llm.Message{Role: llm.RoleUser, Content: recordDemand(*log)})

	for request := 1; request <= maxRecordRequests; request++ {
		response, err := chat(withEngineState(block, *messages), recordTools())
		if err != nil {
			// The scene is already on screen and the turn is not at risk. Say
			// what was lost — the engine is now a little behind the fiction —
			// and let the next turn's block be the correction.
			fmt.Printf("\nLLM error while recording the turn: %v\n", err)
			return
		}
		*messages = append(*messages, response.Message)

		// The DM was asked to make no calls when the prose moved nothing, so a
		// reply without any is an answer and not a failure.
		if len(response.Message.ToolCalls) == 0 {
			return
		}

		var refused bool
		for _, toolCall := range response.Message.ToolCalls {
			result, err := executeAction(s.world, toolCall)
			debugToolCall(toolCall, result, err)
			if err != nil {
				result = "Error: " + err.Error()
				refused = true
			}
			log.events = append(log.events, toolCall.Function.Name+"|"+resultHeadline(result))
			*messages = append(*messages, llm.Message{
				Role:       llm.RoleTool,
				ToolCallID: toolCall.ID,
				Name:       toolCall.Function.Name,
				Content:    result,
			})
		}

		if err := writeCharsheet(s.world); err != nil {
			fmt.Println("\nOutput error:", err)
		}

		// A refusal is the one thing worth another round: the error names what
		// does exist, so the correction is one call away and the alternative is
		// an engine that quietly disagrees with a scene the player has read.
		// Anything else is finished.
		if !refused {
			return
		}
	}
}

// recordDemand opens the bookkeeping phase. It quotes the prose back instead of
// trusting the DM to find it among its own tool calls, and lists what the engine
// resolved during the turn, because the readiest way to get this phase wrong is
// to record something the attack tools already did.
func recordDemand(log turnLog) string {
	var b strings.Builder
	b.WriteString("The player has read this turn and the scene is closed. Nothing you write now reaches them: there is no narration left to send and no channel open to send it through.\n\nWhat is left is the record. Here is what you told them, word for word:\n\n")
	for _, prose := range log.shown {
		b.WriteString("> ")
		b.WriteString(strings.ReplaceAll(strings.TrimSpace(prose), "\n", "\n> "))
		b.WriteString("\n\n")
	}

	if len(log.events) > 0 {
		b.WriteString("The engine already resolved these during the turn. They are done, and recording any of them again would wound, spend or move a second time:\n\n")
		for _, event := range log.events {
			b.WriteString("- ")
			b.WriteString(event)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	} else {
		b.WriteString("The engine resolved nothing during this turn, so anything that moved is still unrecorded.\n\n")
	}

	b.WriteString("Now make the calls that prose implies and the engine has not made. Go looking for these, in this order:\n\n")
	b.WriteString("- **Anything that left a hand.** Dropped, thrown, thrown away, put down, set aside, stashed, hidden, buried, left behind, handed over, sold, given away, paid, stolen, lost. All of it is move_item, and the ground is a destination like any other: a thing on the floor is at a location, not gone. Something nobody is holding is still somewhere.\n")
	b.WriteString("- **Anything that came into a hand.** Picked up, taken, caught, retrieved, bought, accepted, pulled out of a pocket.\n")
	b.WriteString("\nWhich of the two calls a thing needs is settled by one test, and it is not a judgement: **look for the thing by name in ROSTER and LOCATIONS above.** If it is there, move_item it to whoever or wherever has it now. If it is not there, it does not exist — however many turns your prose has been describing it, however plainly the story treats it as real, whatever WORLD says about it — and add_item is what brings it into the world, into the hands or the spot that hold it now. WORLD is not a record of things. It is what your prose knows, and a prop can sit in it for ten turns without the engine having ever held it.\n\nThis is the ordinary case, not a strange one: it is what happens every time a scene invents a prop and the story then picks it up. add_item is not only for things that appear from nowhere — it is for the first time the engine hears of anything. A thing that turns out not to exist is never a reason to record nothing; it is the reason to add it.\n\n")
	b.WriteString("- **Anything genuinely destroyed, and nothing else.** Drunk, eaten, burned, smashed, fired off as a last charge. Thrown away is not destroyed. Broken is not destroyed either, unless your prose says it is in pieces beyond use.\n")
	b.WriteString("- **Coins that moved**, in either direction.\n")
	b.WriteString("- **Anybody who arrived, walked out, was dragged off, fled or died** — and anybody your prose introduced who does not exist yet.\n")
	b.WriteString("- **A wound from something no attack roll settled**: a fall, a fire, poison, an execution.\n\n")
	b.WriteString("Read your prose a sentence at a time rather than by its gist, and go by what it actually says rather than what you meant by it: if it is not in those words it did not happen, and if it is in them the engine has to hold it too. The commonest miss by far is the first item on that list — the prose says a thing left somebody's hands, the engine goes on saying it is carried, and neither of you notices until the sheet is read out loud.\n\n")
	b.WriteString("ROSTER and LOCATIONS above are current as of this moment, so a call that would only restate what is already there is one to skip. If, having gone through your prose that way, it moved nothing the record does not already hold, make no calls and say so in one line.\n\nDo not narrate, do not retell the scene, and do not record anything twice.")
	return b.String()
}

// recordTurn files the turn that just ended. A turn counts as narrated if the
// player was shown anything at all: a turn that posed a choice, took an answer
// and then ran out of budget did reach them, and calling that unnarrated would
// drop prose they have already read.
func (s *session) recordTurn(playerInput string, log turnLog) {
	input := playerInput
	if input == "" {
		input = "(the scene opens)"
	}
	for _, choice := range log.choices {
		input += " → " + choice
	}
	s.archive.Record(archive.Turn{
		Number:      s.turn,
		PlayerInput: input,
		Events:      log.events,
		Update:      strings.Join(log.updates, "; "),
		Prose:       strings.Join(log.shown, "\n\n"),
		Unnarrated:  len(log.shown) == 0,
	})
}

// turnLog is what a turn gathers as it runs, and the only thing that outlives
// it. The four move together from the moment they are filled to the moment the
// archive takes them, which is why they travel as one thing: a turn can narrate
// more than once, and each part accumulates across rounds rather than being
// settled by any single one of them.
type turnLog struct {
	events  []string
	choices []string
	shown   []string
	updates []string
}

// withEngineState returns the messages a request is actually made of. The two
// that never change lead, so they stay byte-identical and cached for the whole
// session; the block follows them, rebuilt each turn; and everything this turn
// has said comes after. Nothing here is stored — the block is assembled for the
// request and thrown away, which is what keeps it from ever going stale.
func withEngineState(block string, msgs []llm.Message) []llm.Message {
	pinned := pinnedMessages
	if len(msgs) < pinned {
		pinned = len(msgs)
	}
	request := make([]llm.Message, 0, len(msgs)+1)
	request = append(request, msgs[:pinned]...)
	request = append(request, llm.Message{Role: llm.RoleSystem, Content: block})
	return append(request, msgs[pinned:]...)
}

// resultHeadline is the first line of a tool result: the engine's own account of
// what the call did. What follows it is the character's state, which the roster
// now states in full every turn and which would only be a second copy here.
func resultHeadline(result string) string {
	if i := strings.IndexByte(result, '\n'); i >= 0 {
		return result[:i]
	}
	return result
}

// recall answers the DM's lookup into the archive. It is handled here rather
// than among the actions because it changes nothing in the world: it reads the
// record, and the answer lives exactly as long as the turn that asked for it.
func (s *session) recall(call llm.ToolCall) string {
	var args struct {
		Query        string `json:"query"`
		Turn         int    `json:"turn"`
		To           int    `json:"to"`
		IncludeProse bool   `json:"include_prose"`
		Limit        int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return fmt.Sprintf("Error: could not read your recall (%v). Ask again with a query or a turn.", err)
	}
	if args.Query == "" && args.Turn == 0 {
		return "Error: recall needs either a query or a turn number."
	}
	return s.archive.Lookup(archive.Query{
		Keyword:      args.Query,
		From:         args.Turn,
		To:           args.To,
		IncludeProse: args.IncludeProse,
		Limit:        args.Limit,
	})
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
const narrationDemand = "Stop resolving and narrate. This is the last call of the turn's first half: the checks, the dice and the rest of the engine are no longer available to you, output_narration is what you have left, and the player has been shown nothing since their last line. Narrate the turn now from what you already know—the results above are what happened, and anything still unsettled is yours to settle in the fiction. Pass world_update with it as always, and location: name a slug that exists in LOCATIONS, or call create_location in this same response first if the scene has moved somewhere new—create_location is the one other call you still have. If this response does not carry the narration, the player loses the turn."

func parseNarration(call llm.ToolCall) (narrationOutput, error) {
	var args narrationOutput
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return narrationOutput{}, err
	}
	if strings.TrimSpace(args.Prose) == "" {
		return narrationOutput{}, errors.New("prose is empty")
	}
	// Position rides on the narration because the narration is the one call a
	// turn cannot skip. A scene that does not say where it happened would leave
	// the engine asserting the player is wherever they were last seen, which is
	// exactly the stale fact the roster used to refuse to state.
	if strings.TrimSpace(args.Location) == "" {
		return narrationOutput{}, errors.New("location is empty: name the slug of the place this scene happens in")
	}
	// The prose is dropped the moment the turn collapses, so a narration without
	// state would leave nothing behind at all. Rejecting it here — before the
	// player is shown anything — costs one silent round; letting it through costs
	// the turn's memory.
	if strings.TrimSpace(args.WorldUpdate) == "" {
		return narrationOutput{}, errors.New("world_update is empty")
	}
	return args, nil
}

const llmRequestTimeout = time.Minute

// chat sends one request and prints the reply's text. Gameplay narration does
// not come back as text—it rides in an output_narration call—so this is for the
// setup phases, where the LLM really is talking to the user directly.
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
		Model:    model,
		Messages: messages,
		Tools:    tools,
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
