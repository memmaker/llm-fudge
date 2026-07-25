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

	// Phase 1: Clarification
	clarifyPrompt, _ := prompts.LoadPrompt("system-prompts/setting-clarification.md", nil)
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: clarifyPrompt},
		{Role: llm.RoleUser, Content: fmt.Sprintf("I'd like to create a setting with this concept: %s", description)},
	}

	fmt.Println()
	_, err = streamChat(client, model, messages, nil)
	if err != nil {
		return err
	}

	fmt.Println()

	// Get user confirmation
	confirmed, err := ui.GetInput(rl, "Does this sound right? (yes/no): ")
	if err != nil || (confirmed != "yes" && confirmed != "y") {
		return err
	}

	// Phase 2: Creation - LLM will call create_setting and write_setting_file tools
	createPrompt, _ := prompts.LoadPrompt("system-prompts/setting-creation.md", nil)
	messages = []llm.Message{
		{Role: llm.RoleSystem, Content: createPrompt},
		{Role: llm.RoleUser, Content: fmt.Sprintf("Create a custom Fudge setting with this concept: %s", description)},
	}

	fmt.Println()
	response, err := streamChat(client, model, messages, settingCreationTools())
	if err != nil {
		return err
	}

	fmt.Println()

	// Process tool calls to save the setting
	for _, toolCall := range response.Message.ToolCalls {
		result, err := executeAction(w, toolCall)
		debugToolCall(toolCall, result, err)
		if err != nil {
			return err
		}
		fmt.Println(result)
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

	response, err := streamChat(client, model, messages, nil)
	if err != nil {
		return err
	}
	if response.Message.Content == "" {
		return errors.New("no adventure found")
	}
	w.AdventureContent = response.Message.Content
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
		{Role: llm.RoleAssistant, Content: w.AdventureContent},
	}

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

func processGameTurn(w *rules.World, client *llm.Client, model string, messages *[]llm.Message) {
	response, err := streamChat(client, model, *messages, gameplayTools())
	if err != nil {
		fmt.Println("\nLLM error:", err)
		return
	}

	*messages = append(*messages, response.Message)

	// Process tool calls (queries, checks, character creation)
	if len(response.Message.ToolCalls) > 0 {
		for _, toolCall := range response.Message.ToolCalls {
			// Skip output_narration—it's handled in the dedicated narration section below
			if toolCall.Function.Name == "output_narration" {
				continue
			}
			result, err := executeAction(w, toolCall)
			debugToolCall(toolCall, result, err)
			if err != nil {
				result = "Error: " + err.Error()
			}
			*messages = append(*messages, llm.Message{
				Role:       llm.RoleTool,
				ToolCallID: toolCall.ID,
				Name:       toolCall.Function.Name,
				Content:    result,
			})
		}

		// Update charsheet after all mutations
		err := writeCharsheet(w)
		if err != nil {
			fmt.Println("\nOutput error:", err)
		}
	}

	// Get narration with output_narration tool required (suppress printing—we handle output manually)
	fmt.Println()
	finalResponse, err := streamChatSilent(client, model, *messages, []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{
			Name:        "output_narration",
			Description: "Output narration and world state. REQUIRED: Call this as your final action every turn. Do not output any text directly—use only this tool. Arguments: prose (human-readable narration for the player), condensed_state (ultra-dense LLM-readable state: no articles/fluff, compact notation, world facts, character positions, meaningful events only).",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"prose":           map[string]any{"type": "string", "description": "Human-readable narration for the player"},
					"condensed_state": map[string]any{"type": "string", "description": "Ultra-condensed state for LLM context: no articles/fluff, dense notation, world facts only"},
				},
				"required": []string{"prose", "condensed_state"},
			},
		}},
	})
	if err != nil {
		fmt.Println("\nLLM error:", err)
		return
	}

	// Extract output_narration tool call (should be the only tool in this response)
	narrationFound := false
	for _, toolCall := range finalResponse.Message.ToolCalls {
		if toolCall.Function.Name == "output_narration" {
			narrationFound = true
			var args struct {
				Prose          string `json:"prose"`
				CondensedState string `json:"condensed_state"`
			}
			if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
				fmt.Printf("\nError parsing output_narration: %v\n", err)
				return
			}

			// Display prose to player
			wrapped := ui.WrapText(args.Prose, ui.TerminalWidth()-4)
			fmt.Println(wrapped)

			// Debug: show condensed state in grey if debug mode enabled
			if debug {
				fmt.Printf("\n\033[90m[%s]\033[0m\n", args.CondensedState)
			}

			// Append only condensed_state to message history (not the tool call itself)
			*messages = append(*messages, llm.Message{
				Role:    llm.RoleAssistant,
				Content: args.CondensedState,
			})
			break
		}
	}

	if !narrationFound {
		fmt.Println("\nError: DM did not call output_narration. Retrying...")
		return
	}
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

	done := make(chan struct{})
	go ui.ShowLoadingSpinner(llmRequestTimeout, done)

	messageChan, err := client.Stream(ctx, llm.Request{Model: model, Messages: messages, Tools: tools})
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
