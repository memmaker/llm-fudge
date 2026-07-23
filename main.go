package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fudge/rules"

	"github.com/memmaker/gollum/llm"
	"golang.org/x/term"
)

// LLM-guided game setup with adventure loading
func main() {
	w := &rules.World{Characters: make(map[string]*rules.Character)}

	reader := bufio.NewReader(os.Stdin)

	// Setup phase: LLM guides through setting, character, and adventure selection
	adventure := runSetupPhase(w, reader)
	if adventure == "" {
		fmt.Println("Setup cancelled.")
		return
	}

	// Game phase: LLM acts as DM
	runGamePhase(w, reader, adventure)
}

const setupPrompt = `You are guiding a player through setting up a new Fudge RPG adventure. Follow these steps:

1. Greet the player and welcome them to Fudge
2. Ask them to choose a setting - use list_settings to show options
3. Guide them to load their chosen setting using load_setting
4. Help them create a character:
   - First use query_setting to show available traits
   - Guide them through using create_character with valid traits
5. Use list_adventures to show available adventures in the loaded setting
6. Guide them to load an adventure using load_adventure
7. Present the adventure story opening and ask what they do
8. Return control to the DM for gameplay

Be enthusiastic and encouraging throughout!`

func runSetupPhase(w *rules.World, reader *bufio.Reader) string {
	client, model, _, err := llm.InitClientFromHome()
	if err != nil {
		fmt.Println("LLM error:", err)
		return ""
	}

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: setupPrompt},
		{Role: llm.RoleUser, Content: "Let's start a new adventure!"},
	}

	adventure := ""

	for {
		response, err := chatWithTimeout(client, model, messages, actionTools())
		if err != nil {
			fmt.Println("LLM error:", err)
			return ""
		}

		messages = append(messages, response.Message)

		// Keep processing tool calls until we get narration
		for len(response.Message.ToolCalls) > 0 {
			for _, toolCall := range response.Message.ToolCalls {
				result, err := executeAction(w, "", "", toolCall)
				if err != nil {
					result = "Error: " + err.Error()
				}
				messages = append(messages, llm.Message{
					Role:       llm.RoleTool,
					ToolCallID: toolCall.ID,
					Name:       toolCall.Function.Name,
					Content:    result,
				})

				// Capture adventure content
				if toolCall.Function.Name == "load_adventure" {
					adventure = result
				}
			}

			// Get follow-up response after tool calls
			response, err = chatWithTimeout(client, model, messages, actionTools())
			if err != nil {
				fmt.Println("LLM error:", err)
				return ""
			}
			messages = append(messages, response.Message)
		}

		// Print final narration
		if response.Message.Content != "" {
			fmt.Println("\nDM:", response.Message.Content)
		}

		// Check if setup is complete
		if w.Ruleset != nil && len(w.Characters) > 0 && adventure != "" {
			return adventure
		}

		// Get player input for next turn
		fmt.Print("\nYou: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return ""
		}
		input = strings.TrimSpace(input)
		if input == "quit" || input == "exit" {
			return ""
		}
		if input != "" {
			messages = append(messages, llm.Message{Role: llm.RoleUser, Content: input})
		}
	}
}

func runGamePhase(w *rules.World, reader *bufio.Reader, adventureOpening string) {
	client, model, _, err := llm.InitClientFromHome()
	if err != nil {
		fmt.Println("LLM error:", err)
		return
	}

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: dmPrompt},
		{Role: llm.RoleUser, Content: "The adventure begins:\n\n" + adventureOpening},
	}

	var narration []string

	for {
		// Print narration
		if len(narration) > 0 {
			printNarration(narration)
		}

		// Get player input
		fmt.Print("You: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading input; exiting.")
			return
		}
		line = strings.TrimSpace(line)
		if line == "quit" || line == "exit" {
			fmt.Println("Session ended.")
			return
		}

		if line == "" {
			continue
		}

		messages = append(messages, llm.Message{Role: llm.RoleUser, Content: line})
		response, err := chatWithTimeout(client, model, messages, actionTools())
		if err != nil {
			fmt.Println("LLM error:", err)
			continue
		}

		messages = append(messages, response.Message)

		// Process tool calls if any
		if len(response.Message.ToolCalls) > 0 {
			for _, toolCall := range response.Message.ToolCalls {
				result, err := executeAction(w, w.Player, "", toolCall)
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

			// Get final narration from LLM
			finalResponse, err := chatWithTimeout(client, model, messages, nil)
			if err != nil {
				fmt.Println("LLM error:", err)
				continue
			}
			if finalResponse.Message.Content != "" {
				narration = []string{finalResponse.Message.Content}
			}
			messages = append(messages, finalResponse.Message)
		} else if response.Message.Content != "" {
			narration = []string{response.Message.Content}
		}
	}
}

const dmPrompt = `You are the dungeon master for a Fudge RPG. Trivial character actions don't need any skill checks. If the player's action warrants a skill check (action needing some skill, complex action, uncertain outcomes, opposed actions), call exactly one available tool to resolve it. Then narrate the outcome briefly using the tool result. Do not use dice or randomness yourself.

When creating a character:
1. First call query_setting to see what attributes, skills, species, gifts, and faults are available in the current setting
2. Use only those available traits (lowercase slugs like "strength", "agility", not "Dexterity" or "Will")
3. Then call create_character with the character details using the valid trait slugs
4. Narrate the character creation result

Always check the setting first before assuming standard RPG traits exist.`

const llmRequestTimeout = time.Minute

func narrateAction(w *rules.World, heroID, goblinID, prompt string) (string, error) {
	client, model, _, err := llm.InitClientFromHome()
	if err != nil {
		return "", err
	}

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: dmPrompt},
		{Role: llm.RoleUser, Content: prompt},
	}
	response, err := chatWithTimeout(client, model, messages, actionTools())
	if err != nil {
		return "", err
	}
	if len(response.Message.ToolCalls) == 0 {
		if response.Message.Content == "" {
			return "", fmt.Errorf("LLM returned neither narration nor an action tool call")
		}
		return response.Message.Content, nil
	}
	if len(response.Message.ToolCalls) != 1 {
		return "", fmt.Errorf("expected one action tool call, got %d", len(response.Message.ToolCalls))
	}

	messages = append(messages, response.Message)
	result, err := executeAction(w, heroID, goblinID, response.Message.ToolCalls[0])
	if err != nil {
		return "", err
	}
	messages = append(messages, llm.Message{
		Role:       llm.RoleTool,
		ToolCallID: response.Message.ToolCalls[0].ID,
		Name:       response.Message.ToolCalls[0].Function.Name,
		Content:    result,
	})
	response, err = chatWithTimeout(client, model, messages, nil)
	if err != nil {
		return "", err
	}
	if response.Message.Content == "" {
		return "", fmt.Errorf("LLM returned no narration")
	}
	return response.Message.Content, nil
}

func chatWithTimeout(client *llm.Client, model string, messages []llm.Message, tools []llm.Tool) (llm.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), llmRequestTimeout)
	defer cancel()
	return client.Chat(ctx, llm.Request{Model: model, Messages: messages, Tools: tools})
}

func actionTools() []llm.Tool {
	return []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "list_settings", Description: "List all available settings.", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "load_setting", Description: "Load a specific setting by name.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}, "required": []string{"name"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "list_adventures", Description: "List all available adventures in the current setting.", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "load_adventure", Description: "Load an adventure by name from the current setting.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}, "required": []string{"name"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "query_setting", Description: "Query available attributes, skills, species, gifts, and faults in the current setting.", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "unopposed_check", Description: "Resolve an unopposed check against a difficulty using a chosen trait.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"trait_type": map[string]any{"type": "string", "enum": []string{"attribute", "skill"}}, "trait_slug": map[string]any{"type": "string"}, "difficulty": map[string]any{"type": "integer"}, "situational_modifier": map[string]any{"type": "integer"}, "add_one": map[string]any{"type": "boolean"}, "reroll": map[string]any{"type": "boolean"}}, "required": []string{"trait_type", "trait_slug", "difficulty"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "opposed_check", Description: "Resolve an opposed check between two actors using independently chosen traits.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"actor_trait_type": map[string]any{"type": "string", "enum": []string{"attribute", "skill"}}, "actor_trait_slug": map[string]any{"type": "string"}, "actor_modifier": map[string]any{"type": "integer"}, "opponent_id": map[string]any{"type": "string"}, "opponent_trait_type": map[string]any{"type": "string", "enum": []string{"attribute", "skill"}}, "opponent_trait_slug": map[string]any{"type": "string"}, "opponent_modifier": map[string]any{"type": "integer"}, "add_one": map[string]any{"type": "boolean"}}, "required": []string{"actor_trait_type", "actor_trait_slug", "opponent_id", "opponent_trait_type", "opponent_trait_slug"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "write_setting_file", Description: "Write one recognized setting file in replace or append mode.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"filename": map[string]any{"type": "string"}, "mode": map[string]any{"type": "string", "enum": []string{"replace", "append"}}, "content": map[string]any{"type": "string"}}, "required": []string{"filename", "mode", "content"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "create_character", Description: "Create a new character in the selected setting.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"slug": map[string]any{"type": "string"}, "name": map[string]any{"type": "string"}, "species": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}, "location": map[string]any{"type": "string"}, "attributes": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, "skills": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, "gifts": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "faults": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, "required": []string{"name", "species"}}}},
	}
}

func listSettings() (string, error) {
	entries, err := os.ReadDir("settings")
	if err != nil {
		return "", fmt.Errorf("failed to read settings directory: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("Available settings:\n\n")

	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			sb.WriteString(fmt.Sprintf("  - %s\n", entry.Name()))
			count++
		}
	}

	if count == 0 {
		return "No settings found.", nil
	}
	return sb.String(), nil
}

func loadSettingTool(w *rules.World, name string) (string, error) {
	path := filepath.Join("settings", name)
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("setting not found: %s", name)
	}

	setting, err := rules.LoadSetting(path)
	if err != nil {
		return "", fmt.Errorf("failed to load setting: %w", err)
	}

	w.Ruleset = setting
	w.Characters = make(map[string]*rules.Character)
	w.Player = ""

	return fmt.Sprintf("Loaded setting: %s", name), nil
}

func listAdventures(w *rules.World) (string, error) {
	if w.Ruleset == nil {
		return "", fmt.Errorf("no setting loaded")
	}

	adventuresPath := filepath.Join(w.Ruleset.Path, "adventures")
	entries, err := os.ReadDir(adventuresPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "No adventures available.", nil
		}
		return "", fmt.Errorf("failed to read adventures directory: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("Available adventures:\n\n")

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			name := strings.TrimSuffix(entry.Name(), ".md")
			sb.WriteString(fmt.Sprintf("  - %s\n", name))
			count++
		}
	}

	if count == 0 {
		return "No adventures found.", nil
	}
	return sb.String(), nil
}

func loadAdventureTool(w *rules.World, name string) (string, error) {
	if w.Ruleset == nil {
		return "", fmt.Errorf("no setting loaded")
	}

	adventurePath := filepath.Join(w.Ruleset.Path, "adventures", name+".md")
	content, err := os.ReadFile(adventurePath)
	if err != nil {
		return "", fmt.Errorf("failed to load adventure: %w", err)
	}

	return string(content), nil
}

func querySetting(w *rules.World) (string, error) {
	if w.Ruleset == nil {
		return "", fmt.Errorf("world has no selected setting")
	}

	var sb strings.Builder
	sb.WriteString("Available traits in setting:\n\n")

	sb.WriteString("ATTRIBUTES:\n")
	for _, attr := range sortedAttributeKeys(w.Ruleset.Attributes) {
		desc := w.Ruleset.Attributes[attr].Description
		sb.WriteString(fmt.Sprintf("  - %s: %s\n", attr, desc))
	}

	sb.WriteString("\nSKILLS:\n")
	for _, skill := range sortedSkillKeys(w.Ruleset.Skills) {
		desc := w.Ruleset.Skills[skill].Description
		sb.WriteString(fmt.Sprintf("  - %s: %s\n", skill, desc))
	}

	sb.WriteString("\nSPECIES:\n")
	for _, spec := range sortedSpeciesKeys(w.Ruleset.Species) {
		desc := w.Ruleset.Species[spec].Description
		scale := w.Ruleset.Species[spec].Scale
		sb.WriteString(fmt.Sprintf("  - %s (scale %d): %s\n", spec, scale, desc))
	}

	sb.WriteString("\nGIFTS:\n")
	for _, gift := range sortedGiftKeys(w.Ruleset.Gifts) {
		desc := w.Ruleset.Gifts[gift].Description
		sb.WriteString(fmt.Sprintf("  - %s: %s\n", gift, desc))
	}

	sb.WriteString("\nFAULTS:\n")
	for _, fault := range sortedFaultKeys(w.Ruleset.Faults) {
		desc := w.Ruleset.Faults[fault].Description
		sb.WriteString(fmt.Sprintf("  - %s: %s\n", fault, desc))
	}

	return sb.String(), nil
}

func sortedAttributeKeys(m map[string]*rules.Attribute) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedSkillKeys(m map[string]*rules.Skill) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedSpeciesKeys(m map[string]*rules.Species) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedGiftKeys(m map[string]*rules.Gift) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedFaultKeys(m map[string]*rules.Fault) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func executeAction(w *rules.World, heroID, goblinID string, call llm.ToolCall) (string, error) {
	switch call.Function.Name {
	case "list_settings":
		return listSettings()
	case "load_setting":
		var args struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse load_setting: %w", err)
		}
		return loadSettingTool(w, args.Name)
	case "list_adventures":
		return listAdventures(w)
	case "load_adventure":
		var args struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse load_adventure: %w", err)
		}
		return loadAdventureTool(w, args.Name)
	case "query_setting":
		return querySetting(w)
	case "unopposed_check":
		var args struct {
			TraitType             string `json:"trait_type"`
			TraitSlug             string `json:"trait_slug"`
			Difficulty            int    `json:"difficulty"`
			SituationalModifier   int    `json:"situational_modifier"`
			AddOne                bool   `json:"add_one"`
			Reroll                bool   `json:"reroll"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse unopposed check: %w", err)
		}
		if args.TraitType == "" || args.TraitSlug == "" {
			return "", fmt.Errorf("unopposed check requires trait_type and trait_slug")
		}
		result, err := w.UnopposedCheck(heroID, args.TraitType, args.TraitSlug, args.Difficulty, args.SituationalModifier, args.AddOne, args.Reroll)
		if err != nil {
			return "", err
		}
		hero, _ := w.Character(heroID)
		var critStr string
		if result.CritSuccess {
			critStr = " (CRITICAL SUCCESS!)"
		} else if result.CritFailure {
			critStr = " (CRITICAL FAILURE!)"
		}
		successStr := "failure"
		if result.Success {
			successStr = "success"
		}
		return fmt.Sprintf("%s rolled %+d (total %d against %d): %s%s", hero.Name, result.Roll, result.Total, args.Difficulty, successStr, critStr), nil
	case "opposed_check":
		var args struct {
			ActorTraitType      string `json:"actor_trait_type"`
			ActorTraitSlug      string `json:"actor_trait_slug"`
			ActorModifier       int    `json:"actor_modifier"`
			OpponentID          string `json:"opponent_id"`
			OpponentTraitType   string `json:"opponent_trait_type"`
			OpponentTraitSlug   string `json:"opponent_trait_slug"`
			OpponentModifier    int    `json:"opponent_modifier"`
			AddOne              bool   `json:"add_one"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse opposed check: %w", err)
		}
		if args.ActorTraitType == "" || args.ActorTraitSlug == "" || args.OpponentID == "" || args.OpponentTraitType == "" || args.OpponentTraitSlug == "" {
			return "", fmt.Errorf("opposed check requires all trait and opponent parameters")
		}
		if args.OpponentID == "" {
			args.OpponentID = goblinID
		}
		result, err := w.OpposedCheck(heroID, args.ActorTraitType, args.ActorTraitSlug, args.ActorModifier, args.OpponentID, args.OpponentTraitType, args.OpponentTraitSlug, args.OpponentModifier, args.AddOne)
		if err != nil {
			return "", err
		}
		hero, _ := w.Character(heroID)
		opponent, _ := w.Character(args.OpponentID)
		var critStr string
		if result.ActorCrit {
			critStr += " (actor critical!)"
		}
		if result.OpponentCrit {
			critStr += " (opponent critical!)"
		}
		outcomeStr := "tie or loss"
		if result.ActorWins {
			outcomeStr = "actor wins"
		}
		return fmt.Sprintf("%s scored %d against %s's %d (relative degree %+d): %s%s", hero.Name, result.ActorTotal, opponent.Name, result.OpponentTotal, result.RelativeDegree, outcomeStr, critStr), nil
	case "write_setting_file":
		var args struct {
			Filename string `json:"filename"`
			Mode     string `json:"mode"`
			Content  string `json:"content"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse write setting file: %w", err)
		}
		if w.Ruleset == nil {
			return "", fmt.Errorf("world has no setting selected")
		}
		if args.Filename == "" || args.Mode == "" {
			return "", fmt.Errorf("write setting file requires filename, mode, and content")
		}
		if err := w.Ruleset.WriteFile(args.Filename, args.Mode, []byte(args.Content)); err != nil {
			return "", err
		}
		return fmt.Sprintf("wrote %s (%s)", args.Filename, args.Mode), nil
	case "create_character":
		var req struct {
			Slug        string         `json:"slug"`
			Name        string         `json:"name"`
			Species     string         `json:"species"`
			Description string         `json:"description"`
			Location    string         `json:"location"`
			Attributes  map[string]int `json:"attributes"`
			Skills      map[string]int `json:"skills"`
			Gifts       []string       `json:"gifts"`
			Faults      []string       `json:"faults"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &req); err != nil {
			return "", fmt.Errorf("parse create_character: %w", err)
		}
		ch, err := w.CreateCharacterValidated(rules.CharacterRequest(req))
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("created character %s", ch.ID), nil
	default:
		return "", fmt.Errorf("unknown action tool %q", call.Function.Name)
	}
}

func printNarration(log []string) {
	const maxLines = 12
	start := 0
	if len(log) > maxLines {
		start = len(log) - maxLines
	}
	fmt.Println("--- DM Narration ---")
	for i := start; i < len(log); i++ {
		fmt.Println(wrapText(log[i], terminalWidth()))
	}
	fmt.Println("--------------------")
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
