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

	"fudge/prompts"
	"fudge/rules"
	"fudge/ui"

	"github.com/chzyer/readline"
	"github.com/memmaker/gollum/llm"
)

var debug bool

const (
	// maxAdventureAttempts caps the retries when the LLM won't save the adventure.
	maxAdventureAttempts = 3
	// maxTurnIterations caps the tool-call rounds spent on a single game turn.
	maxTurnIterations = 6
)

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
	flag.Parse()

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
	return prompts.LoadPrompt(promptFile, w.Setting)
}

func loadDMPrompt(w *rules.World) (string, error) {
	return prompts.LoadPrompt("system-prompts/dm.md", w.Setting)
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
		// For dice-rolling tools, also display roll breakdown
		if call.Function.Name == "unopposed_check" || call.Function.Name == "opposed_check" {
			fmt.Printf("  \033[92mDice:\033[0m %s\n", result)
		} else {
			fmt.Printf("  \033[92mResult:\033[0m %s\n", result)
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
	clarifyPrompt, _ := prompts.LoadPrompt("system-prompts/setting-clarification.md", nil)
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
	createPrompt, _ := prompts.LoadPrompt("system-prompts/setting-creation.md", nil)
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

	prompt, _ := prompts.LoadPrompt("system-prompts/adventure-creation.md", nil)
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

			// The character exists, so setup is over. Handing the LLM another
			// turn here only buys a paragraph of congratulation that the opening
			// scene is about to narrate over anyway.
			if len(w.Characters) > 0 {
				if pc, err := w.Character(w.Player); err == nil {
					fmt.Printf("\n%s is ready.\n", pc.Name)
				}
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

func handleMetaCommand(line string, messages *[]llm.Message) {
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
		err := dumpContextToFile("context.txt", messages)
		if err != nil {
			fmt.Printf("Error writing context: %v\n", err)
		} else {
			fmt.Println("Context written to context.txt")
		}
	default:
		fmt.Printf("Unknown command: %s\n", cmd)
		fmt.Println("Available commands:")
		fmt.Println("  /quit, /exit - Exit the game")
		fmt.Println("  /context - Write current message context to context.txt")
	}
}

func dumpContextToFile(filename string, messages *[]llm.Message) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := &strings.Builder{}

	writer.WriteString("=== MESSAGE CONTEXT ===\n\n")
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

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: dmPrompt},
		{Role: llm.RoleUser, Content: openingBriefing(w)},
	}

	// Setup is done and the character exists—open the scene right away instead
	// of making the player prod us for it.
	fmt.Println()
	processGameTurn(w, client, model, &messages)

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
			handleMetaCommand(line, &messages)
			continue
		}

		messages = append(messages, llm.Message{Role: llm.RoleUser, Content: line})
		fmt.Println()
		processGameTurn(w, client, model, &messages)
	}
}

// narrationOutput is the payload of an output_narration call: the prose goes to
// the player, the condensed state stays in our context in its place.
type narrationOutput struct {
	Prose          string `json:"prose"`
	CondensedState string `json:"condensed_state"`
}

// openingBriefing gives the DM the adventure it must open with, plus the player
// character it was created without knowing about. Setup is over and gameplay
// starts here, so the instruction leads and closes the message: everything
// between the two is reference material, not something to reply about.
func openingBriefing(w *rules.World) string {
	var b strings.Builder
	b.WriteString("Character creation is finished and play begins now. This is turn one: there is no player input to react to, you open the game.\n\n")
	b.WriteString("Your entire reply is the first scene, passed as the prose argument of output_narration. Put the player character in it and stop where they can act. Do not acknowledge, summarize, or comment on the briefing below—play it.\n\n")

	b.WriteString("ADVENTURE BRIEFING (the player has not read this):\n\n")
	b.WriteString(w.AdventureContent)
	b.WriteString("\n\n")

	if pc, err := w.Character(w.Player); err == nil {
		b.WriteString(fmt.Sprintf("PLAYER CHARACTER: %s|id:%s|species:%s", pc.Name, pc.ID, pc.Species))
		if pc.Description != "" {
			b.WriteString("|" + pc.Description)
		}
		b.WriteString("\n\n")
	}

	b.WriteString("Nothing needs resolving to open a scene, so this turn is a single response: one output_narration call carrying the opening prose and, as condensed_state, the full baseline of everything that prose establishes. Narrate the opening scene now.")
	return b.String()
}

// processGameTurn drives the DM until it hands back narration for the player.
// Everything the player reads arrives through output_narration, so nothing is
// streamed straight to them: tool calls (checks, NPC creation, ...) are executed
// and fed back in the same round, and a response without narration forces
// another request, since the DM must see the results before it can narrate them.
func processGameTurn(w *rules.World, client *llm.Client, model string, messages *[]llm.Message) {
	turnStart := len(*messages)

	// A turn can narrate more than once—prose, a choice menu, then the prose
	// reacting to it—so every condensed_state it produced is kept and the whole
	// exchange collapses onto all of them at once. Collapsing onto the last one
	// alone would throw away everything the earlier narration recorded.
	var condensedStates []string
	var narrated bool

	collapseTurn := func() {
		if len(condensedStates) == 0 {
			// Nothing was condensed, so the exchange itself is the only record
			// of what happened; leave it in place.
			return
		}
		state := strings.Join(condensedStates, "\n")
		if debug {
			fmt.Printf("\033[90m[%s]\033[0m\n\n", state)
		}
		*messages = append((*messages)[:turnStart], llm.Message{
			Role:    llm.RoleAssistant,
			Content: state,
		})
	}

	for attempt := 0; attempt < maxTurnIterations; attempt++ {
		response, err := chatSilent(client, model, *messages, gameplayTools())
		if err != nil {
			fmt.Println("\nLLM error:", err)
			return
		}

		*messages = append(*messages, response.Message)

		// The calls in a response arrive in no guaranteed order, so the turn is
		// replayed in the order the player experiences it: first everything
		// that decides what happened, then the prose about it, and only then
		// the menu that asks them what happens next.
		var narration *narrationOutput
		results := make(map[string]string, len(response.Message.ToolCalls))

		for _, toolCall := range response.Message.ToolCalls {
			var result string
			switch toolCall.Function.Name {
			case "user_choice":
				continue
			case "output_narration":
				parsed, parseErr := parseNarration(toolCall)
				if parseErr != nil {
					result = fmt.Sprintf("Error: could not read your narration (%v). The player was shown nothing—call output_narration again.", parseErr)
				} else {
					result = "narration shown to player"
					narration = &parsed
				}
				debugToolCall(toolCall, result, nil)
			default:
				result, err = executeAction(w, toolCall)
				debugToolCall(toolCall, result, err)
				if err != nil {
					result = "Error: " + err.Error()
				}
			}
			results[toolCall.ID] = result
		}

		// Update charsheet after all mutations
		if err := writeCharsheet(w); err != nil {
			fmt.Println("\nOutput error:", err)
		}

		if narration != nil {
			fmt.Println(ui.WrapText(narration.Prose, ui.TerminalWidth()))
			fmt.Println()
			narrated = true
			if narration.CondensedState != "" {
				condensedStates = append(condensedStates, narration.CondensedState)
			}
		}

		// A choice menu blocks on the player, so it runs under the prose that
		// poses the question rather than ahead of it.
		var answeredChoice bool
		for _, toolCall := range response.Message.ToolCalls {
			if toolCall.Function.Name != "user_choice" {
				continue
			}
			result, choiceErr := executeAction(w, toolCall)
			debugToolCall(toolCall, result, choiceErr)
			if choiceErr != nil {
				result = "Error: " + choiceErr.Error()
			} else {
				answeredChoice = true
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
			// DM what it needs to narrate next round.
			if len(response.Message.ToolCalls) > 0 {
				continue
			}
			// Prose written as a plain reply reaches nobody. Say so rather than
			// resending an identical context and hoping for a different answer.
			if response.Message.Content != "" {
				*messages = append(*messages, llm.Message{
					Role:    llm.RoleUser,
					Content: "That reply went nowhere: the player only ever sees the prose argument of output_narration. Send this turn's narration again through output_narration.",
				})
				continue
			}
			break
		}

		// The player just picked an option, so the turn is not over: handing
		// back to the prompt now would leave their answer unanswered, and the
		// collapse below would drop it from history unread.
		if answeredChoice {
			continue
		}

		collapseTurn()
		return
	}

	// Out of rounds. The player has already read narration, so end the turn on
	// that rather than scolding them for a turn that did happen.
	if narrated {
		collapseTurn()
		return
	}

	fmt.Println("\nError: DM did not narrate this turn. Try rephrasing your action.")
}

func parseNarration(call llm.ToolCall) (narrationOutput, error) {
	var args narrationOutput
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return narrationOutput{}, err
	}
	if strings.TrimSpace(args.Prose) == "" {
		return narrationOutput{}, errors.New("prose is empty")
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
