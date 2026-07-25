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

func debugToolCall(call llm.ToolCall, result string, err error) {
	if !debug {
		return
	}
	fmt.Printf("\n\033[1;35m[TOOL]\033[0m \033[1m%s\033[0m\n", call.Function.Name)
	if call.Function.Arguments != "" {
		fmt.Printf("  \033[90mArgs:\033[0m %s\n", call.Function.Arguments)
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
		response, err := streamChat(client, model, clarifyMessages, nil)
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
		response, err := streamChat(client, model, messages, settingCreationTools())
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
		response, err := streamChatSilent(client, model, messages, adventureCreationTools())
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
		response, err := streamChat(client, model, messages, characterCreationTools())
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

			fmt.Println()
			response, err = streamChat(client, model, messages, characterCreationTools())
			if err != nil {
				fmt.Println("\nLLM error:", err)
				return
			}
			fmt.Println()
			messages = append(messages, response.Message)
		}

		// Check if character is created
		if len(w.Characters) > 0 {
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

type stateUpdate struct {
	CondensedState string `json:"condensed_state"`
}

// openingBriefing gives the DM the adventure it must open with, plus the player
// character it was created without knowing about.
func openingBriefing(w *rules.World) string {
	var b strings.Builder
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

	b.WriteString("Open the adventure: narrate the first scene as plain text, then call write_state. Put the player character in it and stop where they can act. Do not summarize the briefing—play it.")
	return b.String()
}

// processGameTurn drives the DM until it hands back narration for the player.
// Tool calls (checks, NPC creation, write_state, ...) are all executed and fed
// back in the same round; only the absence of any narration prose forces
// another request, since the DM must narrate before it knows what to save.
func processGameTurn(w *rules.World, client *llm.Client, model string, messages *[]llm.Message) {
	turnStart := len(*messages)
	var savedState *stateUpdate

	for attempt := 0; attempt < maxTurnIterations; attempt++ {
		response, err := streamChat(client, model, *messages, gameplayTools())
		if err != nil {
			fmt.Println("\nLLM error:", err)
			return
		}

		*messages = append(*messages, response.Message)

		for _, toolCall := range response.Message.ToolCalls {
			var result string
			if toolCall.Function.Name == "write_state" {
				state, err := parseStateUpdate(toolCall)
				if err != nil {
					result = fmt.Sprintf("Error: could not read your state update (%v).", err)
				} else {
					result = "state saved"
					savedState = &state
				}
			} else {
				result, err = executeAction(w, toolCall)
				debugToolCall(toolCall, result, err)
				if err != nil {
					result = "Error: " + err.Error()
				}
			}
			*messages = append(*messages, llm.Message{
				Role:       llm.RoleTool,
				ToolCallID: toolCall.ID,
				Name:       toolCall.Function.Name,
				Content:    result,
			})
		}

		// Update charsheet after all mutations
		if err := writeCharsheet(w); err != nil {
			fmt.Println("\nOutput error:", err)
		}

		if response.Message.Content == "" {
			// Nothing streamed to the player yet. If it was pure setup (checks,
			// NPC creation) the tool results above give it what it needs to
			// narrate next round; otherwise nudge it to actually narrate.
			if len(response.Message.ToolCalls) == 0 {
				*messages = append(*messages, llm.Message{
					Role:    llm.RoleUser,
					Content: "Narrate this turn as plain text.",
				})
			}
			continue
		}

		// Prose streamed to the player: the turn is over. If state was saved,
		// collapse this turn's exchange down to the condensed line to keep
		// history compact; otherwise leave the full exchange in place.
		if savedState != nil {
			if debug {
				fmt.Printf("\n\033[90m[%s]\033[0m\n", savedState.CondensedState)
			}
			*messages = append((*messages)[:turnStart], llm.Message{
				Role:    llm.RoleAssistant,
				Content: savedState.CondensedState,
			})
		}
		return
	}

	fmt.Println("\nError: DM did not narrate this turn. Try rephrasing your action.")
}

func parseStateUpdate(call llm.ToolCall) (stateUpdate, error) {
	var args stateUpdate
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return stateUpdate{}, err
	}
	if args.CondensedState == "" {
		return stateUpdate{}, errors.New("condensed_state is empty")
	}
	return args, nil
}

const llmRequestTimeout = time.Minute

func streamChat(client *llm.Client, model string, messages []llm.Message, tools []llm.Tool) (llm.Response, error) {
	return streamChatInternal(client, model, messages, tools, true)
}

func streamChatSilent(client *llm.Client, model string, messages []llm.Message, tools []llm.Tool) (llm.Response, error) {
	return streamChatInternal(client, model, messages, tools, false)
}

func streamChatInternal(client *llm.Client, model string, messages []llm.Message, tools []llm.Tool, printOutput bool) (llm.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), llmRequestTimeout)
	defer cancel()

	tracker := newUsageTracker(model)
	for _, msg := range messages {
		tracker.addMessage(msg)
	}

	if debug {
		fmt.Printf("\n\033[90m[LLM API call: model=%s messages=%d tools=%d]\033[0m\n", model, len(messages), len(tools))
	}

	done := make(chan struct{})
	go ui.ShowLoadingSpinner(llmRequestTimeout, done)

	messageChan, err := client.Stream(ctx, llm.Request{
		Model:    model,
		Messages: messages,
		Tools:    tools,
	})
	close(done)
	if err != nil {
		return llm.Response{}, err
	}

	var accumulatedMessage llm.Message
	accumulatedMessage.Role = llm.RoleAssistant

	buffer := ""
	width := ui.TerminalWidth()

	for msg := range messageChan {
		accumulatedMessage.Content += msg.Content
		if msg.Content != "" && printOutput {
			buffer += msg.Content

			// Process complete lines, leaving partial content in buffer
			lines := strings.Split(buffer, "\n")

			// Print all complete lines (all but the last one)
			for i := 0; i < len(lines)-1; i++ {
				wrapped := ui.WrapLine(lines[i], width)
				fmt.Println(strings.Join(wrapped, "\n"))
			}

			// Keep the last incomplete line in buffer
			buffer = lines[len(lines)-1]
		}
		if len(msg.ToolCalls) > 0 {
			accumulatedMessage.ToolCalls = append(accumulatedMessage.ToolCalls, msg.ToolCalls...)
		}
	}

	// Print any remaining buffered content
	if buffer != "" && printOutput {
		wrapped := ui.WrapLine(buffer, width)
		fmt.Println(strings.Join(wrapped, "\n"))
	}

	tracker.addMessage(accumulatedMessage)
	ui.DisplayContextUsage(tracker.getPercentage(), debug)

	return llm.Response{Message: accumulatedMessage}, nil
}
