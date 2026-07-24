package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fudge/rules"

	"github.com/chzyer/readline"
	"github.com/memmaker/gollum/llm"
	"golang.org/x/term"
)

var debug bool

// LLM-guided game setup with adventure loading
func main() {
	flag.BoolVar(&debug, "debug", false, "Enable debug mode to show tool usage")
	flag.Parse()

	clearScreen()
	w := &rules.World{
		Characters: make(map[string]*rules.Character),
		Seed:       time.Now().UnixNano(),
		Rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	// Setup readline instance with history and line editing
	rl, err := readline.New("> ")
	if err != nil {
		fmt.Println("Error initializing readline:", err)
		return
	}
	defer rl.Close()

	// Setup phase: LLM guides through setting, character, and adventure selection
	adventure := runSetupPhase(w, rl)
	if adventure == "" {
		fmt.Println("Setup cancelled.")
		return
	}

	// Game phase: LLM acts as DM
	runGamePhase(w, rl, adventure)
}

func loadPrompt(filename string) (string, error) {
	always, err := os.ReadFile("system-prompts/always.md")
	if err != nil {
		return "", fmt.Errorf("failed to load always.md: %w", err)
	}

	content, err := os.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("failed to load prompt file %q: %w", filename, err)
	}

	// Replace ESC placeholders with actual escape character
	alwaysStr := strings.ReplaceAll(string(always), "ESC", "\033")
	return alwaysStr + "\n\n" + string(content), nil
}

func loadCharacterPrompt(promptFile, adventureContent, settingName string, w *rules.World) (string, error) {
	always, err := os.ReadFile("system-prompts/always.md")
	if err != nil {
		return "", fmt.Errorf("failed to load always.md: %w", err)
	}

	content, err := os.ReadFile(promptFile)
	if err != nil {
		return "", fmt.Errorf("failed to load character prompt: %w", err)
	}

	// Replace ESC placeholders with actual escape character
	alwaysStr := strings.ReplaceAll(string(always), "ESC", "\033")
	prompt := alwaysStr + "\n\n" + string(content)

	// Append setting information
	settingInfo := fmt.Sprintf("\n## Setting Information:\n\nThe setting is: %s\n\nAvailable traits:\n%s\n\nThe adventure they've chosen is:\n%s",
		settingName, formatAvailableTraits(w), adventureContent)

	return prompt + settingInfo, nil
}

func loadDMPrompt(w *rules.World) (string, error) {
	always, err := os.ReadFile("system-prompts/always.md")
	if err != nil {
		return "", fmt.Errorf("failed to load always.md: %w", err)
	}

	content, err := os.ReadFile("system-prompts/dm.md")
	if err != nil {
		return "", fmt.Errorf("failed to load dm.md: %w", err)
	}

	// Replace ESC placeholders with actual escape character
	alwaysStr := strings.ReplaceAll(string(always), "ESC", "\033")
	prompt := alwaysStr + "\n\n" + string(content)

	// Append setting information
	settingInfo := fmt.Sprintf("\n## Setting Information:\n\nThe setting is: %s\n\nAvailable traits:\n%s",
		w.Ruleset.Slug, formatAvailableTraits(w))

	return prompt + settingInfo, nil
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
		fmt.Printf("  \033[92mResult:\033[0m %s\n", result)
	}
}

func runSetupPhase(w *rules.World, rl *readline.Instance) string {
	fmt.Println("Welcome to Fudge!")
	fmt.Println()

	// Step 1: Select or create setting
	settingName := setupSetting(w, rl)
	if settingName == "" {
		return ""
	}
	fmt.Println()

	// Step 2: Select or improvise adventure
	adventureContent := setupAdventure(w, rl, settingName)
	if adventureContent == "" {
		return ""
	}
	fmt.Println()

	// Step 3: Create character with LLM
	setupCharacter(w, rl, settingName, adventureContent)
	if len(w.Characters) == 0 {
		return ""
	}

	// Set player to the created character
	for id := range w.Characters {
		w.Player = id
		break
	}

	fmt.Println()
	fmt.Println(wrapText(adventureContent, terminalWidth()-4))
	return adventureContent
}

func setupSetting(w *rules.World, rl *readline.Instance) string {
	settings, err := listAvailableSettings()
	if err != nil {
		fmt.Println("Error:", err)
		return ""
	}

	options := settings
	if len(settings) > 0 {
		fmt.Println("Select a setting:")
		choice, err := showChoiceMenu(options)
		if err != nil {
			return ""
		}

		if choice != typeYourAnswer {
			if _, err := loadSettingTool(w, choice); err != nil {
				fmt.Println("Error loading setting:", err)
				return ""
			}
			return choice
		}
	}

	// User wants custom setting - use LLM
	fmt.Println("Describe the setting you'd like:")
	description, err := readlineInput(rl, "Setting description: ")
	if err != nil || description == "" {
		return ""
	}

	prompt, _ := loadPrompt("system-prompts/setting-creation.md")
	client, model, _, err := llm.InitClientFromHome()
	if err != nil {
		fmt.Println("LLM error:", err)
		return ""
	}

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: prompt},
		{Role: llm.RoleUser, Content: fmt.Sprintf("Create a custom Fudge setting with this concept: %s", description)},
	}

	fmt.Println()
	response, err := streamChat(client, model, messages, actionTools())
	if err != nil {
		fmt.Println("\nLLM error:", err)
		return ""
	}

	fmt.Println()

	// Process tool calls to save the setting
	for _, toolCall := range response.Message.ToolCalls {
		if toolCall.Function.Name == "write_setting_file" {
			result, err := executeAction(w, "", "", toolCall)
			debugToolCall(toolCall, result, err)
			if err != nil {
				fmt.Println("Error creating setting:", err)
				return ""
			}
			fmt.Println(result)
		}
	}

	// Return a placeholder name (user should have created it via tool)
	if len(w.Ruleset.Species) > 0 {
		return "custom"
	}
	return ""
}

func setupAdventure(w *rules.World, rl *readline.Instance, settingName string) string {
	adventures, err := listAvailableAdventures(w)
	if err != nil {
		fmt.Println("Error:", err)
		return ""
	}

	if len(adventures) > 0 {
		fmt.Println("Select an adventure:")
		choice, err := showChoiceMenu(adventures)
		if err != nil {
			return ""
		}

		if choice != typeYourAnswer {
			adventureContent, err := loadAdventureTool(w, choice)
			if err != nil {
				fmt.Println("Error loading adventure:", err)
				return ""
			}
			return adventureContent
		}
	}

	// User wants improvised adventure - use LLM
	fmt.Println("Describe the adventure scenario you'd like:")
	scenario, err := readlineInput(rl, "Scenario: ")
	if err != nil || scenario == "" {
		return ""
	}

	prompt, _ := loadPrompt("system-prompts/adventure-creation.md")
	client, model, _, err := llm.InitClientFromHome()
	if err != nil {
		fmt.Println("LLM error:", err)
		return ""
	}

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: prompt},
		{Role: llm.RoleUser, Content: fmt.Sprintf("Improvise an adventure for the %s setting with this scenario: %s", settingName, scenario)},
	}

	fmt.Println()
	response, err := streamChat(client, model, messages, nil)
	if err != nil {
		fmt.Println("\nLLM error:", err)
		return ""
	}

	fmt.Println()
	return response.Message.Content
}

func setupCharacter(w *rules.World, rl *readline.Instance, settingName string, adventureContent string) {
	fmt.Println("How would you like to create your character?")
	choice, err := showChoiceMenu([]string{"Automatic - I describe my concept, you decide the traits", "Custom - I want to choose all my traits"})
	if err != nil {
		return
	}

	if choice == "Automatic - I describe my concept, you decide the traits" {
		setupCharacterAutomatic(w, rl, settingName, adventureContent)
	} else {
		setupCharacterCustom(w, rl, settingName, adventureContent)
	}
}

func setupCharacterAutomatic(w *rules.World, rl *readline.Instance, settingName string, adventureContent string) {
	fmt.Println("Describe your character concept:")
	concept, err := readlineInput(rl, "Concept: ")
	if err != nil || concept == "" {
		return
	}

	characterPrompt, err := loadCharacterPrompt("system-prompts/character-automatic.md", adventureContent, settingName, w)
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
		{Role: llm.RoleUser, Content: fmt.Sprintf("I want to create a character with this concept: %s", concept)},
	}

	for {
		fmt.Println()
		response, err := streamChat(client, model, messages, actionTools())
		if err != nil {
			fmt.Println("\nLLM error:", err)
			return
		}

		messages = append(messages, response.Message)

		// Process tool calls
		for len(response.Message.ToolCalls) > 0 {
			fmt.Println()
			for _, toolCall := range response.Message.ToolCalls {
				result, err := executeAction(w, "", "", toolCall)
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

			writeCharsheet(w)

			fmt.Println()
			response, err = streamChat(client, model, messages, actionTools())
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
		fmt.Print("\n")
		input, err := getPlayerInput(rl)
		if err != nil || input == "quit" || input == "exit" {
			return
		}
		if input != "" {
			messages = append(messages, llm.Message{Role: llm.RoleUser, Content: input})
		}
	}
}

func setupCharacterCustom(w *rules.World, rl *readline.Instance, settingName string, adventureContent string) {
	characterPrompt, err := loadCharacterPrompt("system-prompts/character-custom.md", adventureContent, settingName, w)
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
		{Role: llm.RoleUser, Content: "I'm ready to create my character. Let's start!"},
	}

	for {
		fmt.Println()
		response, err := streamChat(client, model, messages, actionTools())
		if err != nil {
			fmt.Println("\nLLM error:", err)
			return
		}

		messages = append(messages, response.Message)

		// Process tool calls
		for len(response.Message.ToolCalls) > 0 {
			fmt.Println()
			for _, toolCall := range response.Message.ToolCalls {
				result, err := executeAction(w, "", "", toolCall)
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

			writeCharsheet(w)

			fmt.Println()
			response, err = streamChat(client, model, messages, actionTools())
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
		fmt.Print("\n")
		input, err := getPlayerInput(rl)
		if err != nil || input == "quit" || input == "exit" {
			return
		}
		if input != "" {
			messages = append(messages, llm.Message{Role: llm.RoleUser, Content: input})
		}
	}
}

func formatAvailableTraits(w *rules.World) string {
	var sb strings.Builder

	sb.WriteString("SPECIES:\n")
	for _, species := range sortedSpeciesKeys(w.Ruleset.Species) {
		sb.WriteString(fmt.Sprintf("  - %s\n", species))
	}

	sb.WriteString("\nATTRIBUTES:\n")
	for _, attr := range sortedAttributeKeys(w.Ruleset.Attributes) {
		sb.WriteString(fmt.Sprintf("  - %s\n", attr))
	}

	sb.WriteString("\nSKILLS:\n")
	for _, skill := range sortedSkillKeys(w.Ruleset.Skills) {
		sb.WriteString(fmt.Sprintf("  - %s\n", skill))
	}

	sb.WriteString("\nGIFTS:\n")
	for _, gift := range sortedGiftKeys(w.Ruleset.Gifts) {
		sb.WriteString(fmt.Sprintf("  - %s\n", gift))
	}

	sb.WriteString("\nFAULTS:\n")
	for _, fault := range sortedFaultKeys(w.Ruleset.Faults) {
		sb.WriteString(fmt.Sprintf("  - %s\n", fault))
	}

	return sb.String()
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
	if w.Ruleset == nil {
		return nil, fmt.Errorf("no setting loaded")
	}
	adventuresPath := filepath.Join(w.Ruleset.Path, "adventures")
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

func runGamePhase(w *rules.World, rl *readline.Instance, adventureOpening string) {
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
		{Role: llm.RoleUser, Content: "The adventure begins:\n\n" + adventureOpening},
	}

	for {
		// Get player input
		line, err := getPlayerInput(rl)
		if err != nil {
			fmt.Println("Error reading input; exiting.")
			return
		}
		if line == "quit" || line == "exit" {
			fmt.Println("Session ended.")
			return
		}

		if line == "" {
			continue
		}

		messages = append(messages, llm.Message{Role: llm.RoleUser, Content: line})
		fmt.Println()
		response, err := streamChat(client, model, messages, actionTools())
		if err != nil {
			fmt.Println("\nLLM error:", err)
			continue
		}

		messages = append(messages, response.Message)

		// Process tool calls if any
		if len(response.Message.ToolCalls) > 0 {
			for _, toolCall := range response.Message.ToolCalls {
				result, err := executeAction(w, w.Player, "", toolCall)
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

			// Update charsheet after all mutations
			writeCharsheet(w)

			// Get final narration from LLM
			fmt.Println()
			finalResponse, err := streamChat(client, model, messages, nil)
			if err != nil {
				fmt.Println("\nLLM error:", err)
				continue
			}
			messages = append(messages, finalResponse.Message)
		}
	}
}

const llmRequestTimeout = time.Minute

func streamChat(client *llm.Client, model string, messages []llm.Message, tools []llm.Tool) (llm.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), llmRequestTimeout)
	defer cancel()

	done := make(chan struct{})
	go showLoadingSpinner(llmRequestTimeout, done)

	messageChan, err := client.Stream(ctx, llm.Request{Model: model, Messages: messages, Tools: tools})
	close(done)
	if err != nil {
		return llm.Response{}, err
	}

	var accumulatedMessage llm.Message
	accumulatedMessage.Role = llm.RoleAssistant

	buffer := ""
	width := terminalWidth()

	for msg := range messageChan {
		accumulatedMessage.Content += msg.Content
		if msg.Content != "" {
			buffer += msg.Content

			// Process complete lines, leaving partial content in buffer
			lines := strings.Split(buffer, "\n")

			// Print all complete lines (all but the last one)
			for i := 0; i < len(lines)-1; i++ {
				wrapped := wrapLine(lines[i], width)
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
	if buffer != "" {
		wrapped := wrapLine(buffer, width)
		fmt.Println(strings.Join(wrapped, "\n"))
	}

	return llm.Response{Message: accumulatedMessage}, nil
}


func showLoadingSpinner(duration time.Duration, done <-chan struct{}) {
	hideCursor()
	defer showCursor()

	brailleChars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	frame := 0
	for {
		select {
		case <-done:
			fmt.Print("\r\033[0K")
			return
		case <-ticker.C:
			fmt.Printf("\r%s  ", brailleChars[frame%len(brailleChars)])
			frame++
		}
	}
}

func readlineInput(rl *readline.Instance, prompt string) (string, error) {
	rl.SetPrompt(prompt)
	return rl.Readline()
}

func getPlayerInput(rl *readline.Instance) (string, error) {
	rl.SetPrompt("> ")
	return rl.Readline()
}

func terminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		return 80
	}
	return width
}

func wrapText(text string, width int) string {
	var wrapped []string
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			wrapped = append(wrapped, "")
			continue
		}
		wrapped = append(wrapped, wrapLine(line, width)...)
	}
	return strings.Join(wrapped, "\n")
}

func wrapLine(line string, width int) []string {
	if width <= 0 {
		return []string{line}
	}

	var wrapped []string
	var current string
	for _, word := range strings.Fields(line) {
		for len([]rune(word)) > width {
			if current != "" {
				wrapped = append(wrapped, current)
				current = ""
			}
			runes := []rune(word)
			wrapped = append(wrapped, string(runes[:width]))
			word = string(runes[width:])
		}
		if current == "" {
			current = word
			continue
		}
		if len([]rune(current))+1+len([]rune(word)) <= width {
			current += " " + word
			continue
		}
		wrapped = append(wrapped, current)
		current = word
	}
	if current != "" {
		wrapped = append(wrapped, current)
	}
	return wrapped
}

const typeYourAnswer = "[Type your answer]"

func showChoiceMenu(options []string) (string, error) {
	allOptions := append(options, typeYourAnswer)
	selected := 0

	rl, err := readline.New("")
	if err != nil {
		return "", err
	}
	defer rl.Close()

	hideCursor()
	defer showCursor()

	fmt.Println()
	firstDraw := true

	for {
		if !firstDraw {
			// Calculate total lines to clear (each option may wrap)
			totalLines := 2 // 1 for blank line before, 1 for help text at end
			for _, opt := range allOptions {
				wrapped := wrapText(opt, terminalWidth()-4)
				lineCount := strings.Count(wrapped, "\n") + 1
				totalLines += lineCount
			}
			fmt.Printf("\033[%dA\033[J", totalLines)
		}

		for i, opt := range allOptions {
			wrapped := wrapText(opt, terminalWidth()-4)
			lines := strings.Split(wrapped, "\n")

			for j, line := range lines {
				if i == selected {
					if j == 0 {
						fmt.Printf("  \033[94m> %s\033[0m\n", line)
					} else {
						fmt.Printf("    %s\n", line)
					}
				} else {
					fmt.Printf("    %s\n", line)
				}
			}
		}
		fmt.Printf("\n  \033[90m↑↓ to navigate, Enter to select\033[0m\n")

		firstDraw = false

		key, err := readKey()
		if err != nil {
			return "", err
		}

		switch key {
		case "up":
			if selected > 0 {
				selected--
			} else {
				selected = len(allOptions) - 1
			}
		case "down":
			if selected < len(allOptions)-1 {
				selected++
			} else {
				selected = 0
			}
		case "enter":
			fmt.Println()
			if selected == len(allOptions)-1 {
				rl.SetPrompt("Your answer: ")
				answer, err := rl.Readline()
				if err != nil {
					return "", err
				}
				return answer, nil
			}
			return allOptions[selected], nil
		case "escape":
			fmt.Println()
			return allOptions[len(allOptions)-1], nil
		}
	}
}

func readKey() (string, error) {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return "", fmt.Errorf("failed to set raw mode: %w", err)
	}
	defer term.Restore(fd, oldState)

	buf := make([]byte, 3)
	n, err := os.Stdin.Read(buf)
	if err != nil {
		return "", err
	}

	if n >= 1 {
		switch buf[0] {
		case 10, 13:
			return "enter", nil
		case 27:
			if n >= 3 && buf[1] == '[' {
				switch buf[2] {
				case 'A':
					return "up", nil
				case 'B':
					return "down", nil
				case 'C':
					return "right", nil
				case 'D':
					return "left", nil
				}
			}
			return "escape", nil
		}
	}
	return "", nil
}

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}

func hideCursor() {
	fmt.Print("\033[?25l")
}

func showCursor() {
	fmt.Print("\033[?25h")
}
