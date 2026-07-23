package main

import (
	"context"
	"encoding/json"
	"fmt"
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

// LLM-guided game setup with adventure loading
func main() {
	clearScreen()
	w := &rules.World{Characters: make(map[string]*rules.Character)}

	// Setup readline instance with history and line editing
	rl, err := readline.New("You: ")
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
	content, err := os.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("failed to load prompt file %q: %w", filename, err)
	}
	return string(content), nil
}

func runSetupPhase(w *rules.World, rl *readline.Instance) string {
	setupPrompt, err := loadPrompt("new-game-prompt.md")
	if err != nil {
		fmt.Println("Error:", err)
		return ""
	}

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
		response, err := chatWithSpinner(client, model, messages, actionTools())
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

			// Update charsheet after all mutations
			writeCharsheet(w)

			// Get follow-up response after tool calls
			response, err = chatWithSpinner(client, model, messages, actionTools())
			if err != nil {
				fmt.Println("LLM error:", err)
				return ""
			}
			messages = append(messages, response.Message)
		}

		// Print final narration
		if response.Message.Content != "" {
			fmt.Println("\nDM:", wrapText(response.Message.Content, terminalWidth()-5))
		}

		// Check if setup is complete
		if w.Ruleset != nil && len(w.Characters) > 0 && adventure != "" {
			return adventure
		}

		// Get player input for next turn
		fmt.Print("\n")
		input, err := readlineInput(rl, "You: ")
		if err != nil {
			return ""
		}
		if input == "quit" || input == "exit" {
			return ""
		}
		if input != "" {
			messages = append(messages, llm.Message{Role: llm.RoleUser, Content: input})
		}
	}
}

func runGamePhase(w *rules.World, rl *readline.Instance, adventureOpening string) {
	dmPrompt, err := loadPrompt("dm-prompt.md")
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

	var narration []string

	for {
		// Print narration
		if len(narration) > 0 {
			printNarration(narration)
		}

		// Get player input
		line, err := readlineInput(rl, "You: ")
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
		response, err := chatWithSpinner(client, model, messages, actionTools())
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

			// Update charsheet after all mutations
			writeCharsheet(w)

			// Get final narration from LLM
			finalResponse, err := chatWithSpinner(client, model, messages, nil)
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

const llmRequestTimeout = time.Minute

func chatWithTimeout(client *llm.Client, model string, messages []llm.Message, tools []llm.Tool) (llm.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), llmRequestTimeout)
	defer cancel()
	return client.Chat(ctx, llm.Request{Model: model, Messages: messages, Tools: tools})
}

func chatWithSpinner(client *llm.Client, model string, messages []llm.Message, tools []llm.Tool) (llm.Response, error) {
	done := make(chan struct{})
	go showLoadingSpinner(llmRequestTimeout, done)
	defer func() { close(done) }()

	return chatWithTimeout(client, model, messages, tools)
}

func actionTools() []llm.Tool {
	return []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "list_settings", Description: "List all available settings.", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "load_setting", Description: "Load a specific setting by name.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}, "required": []string{"name"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "list_adventures", Description: "List all available adventures in the current setting.", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "load_adventure", Description: "Load an adventure by name from the current setting.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}, "required": []string{"name"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "query_setting", Description: "Query available attributes, skills, species, gifts, and faults in the current setting.", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "user_choice", Description: "Present the user with multiple choice options to select from using cursor keys.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"options": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "List of options to present to the user. A 'Type your answer' option is automatically added."}}, "required": []string{"options"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "unopposed_check", Description: "Resolve an unopposed check against a difficulty using a chosen trait.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"trait_type": map[string]any{"type": "string", "enum": []string{"attribute", "skill"}}, "trait_slug": map[string]any{"type": "string"}, "difficulty": map[string]any{"type": "integer"}, "situational_modifier": map[string]any{"type": "integer"}, "add_one": map[string]any{"type": "boolean"}, "reroll": map[string]any{"type": "boolean"}}, "required": []string{"trait_type", "trait_slug", "difficulty"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "opposed_check", Description: "Resolve an opposed check between two actors using independently chosen traits.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"actor_trait_type": map[string]any{"type": "string", "enum": []string{"attribute", "skill"}}, "actor_trait_slug": map[string]any{"type": "string"}, "actor_modifier": map[string]any{"type": "integer"}, "opponent_id": map[string]any{"type": "string"}, "opponent_trait_type": map[string]any{"type": "string", "enum": []string{"attribute", "skill"}}, "opponent_trait_slug": map[string]any{"type": "string"}, "opponent_modifier": map[string]any{"type": "integer"}, "add_one": map[string]any{"type": "boolean"}}, "required": []string{"actor_trait_type", "actor_trait_slug", "opponent_id", "opponent_trait_type", "opponent_trait_slug"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "write_setting_file", Description: "Write one recognized setting file in replace or append mode.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"filename": map[string]any{"type": "string"}, "mode": map[string]any{"type": "string", "enum": []string{"replace", "append"}}, "content": map[string]any{"type": "string"}}, "required": []string{"filename", "mode", "content"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "create_character", Description: "Create a new character in the selected setting.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"slug": map[string]any{"type": "string"}, "name": map[string]any{"type": "string"}, "species": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}, "location": map[string]any{"type": "string"}, "attributes": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, "skills": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, "gifts": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "faults": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, "required": []string{"name", "species"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "apply_damage", Description: "Apply damage to a character, accumulating scratches and escalating wound levels.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": map[string]any{"type": "string"}, "damage_points": map[string]any{"type": "integer"}}, "required": []string{"character_id", "damage_points"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "heal", Description: "Heal a character by reducing wound levels. Cannot heal the dead.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": map[string]any{"type": "string"}, "levels": map[string]any{"type": "integer"}}, "required": []string{"character_id", "levels"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "declare_dead", Description: "Permanently declare a character dead. Dead characters cannot act or be healed.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": map[string]any{"type": "string"}}, "required": []string{"character_id"}}}},
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

func writeCharsheet(w *rules.World) error {
	if w.Player == "" {
		return nil
	}
	content := rules.StateFileText(w)
	return os.WriteFile("charsheet.txt", []byte(content), 0o644)
}

func getCharacterFromJSON(w *rules.World, jsonArgs string) (*rules.Character, map[string]interface{}, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(jsonArgs), &raw); err != nil {
		return nil, nil, fmt.Errorf("parse arguments: %w", err)
	}
	charID, ok := raw["character_id"].(string)
	if !ok {
		return nil, nil, fmt.Errorf("character_id required")
	}
	ch, err := w.Character(charID)
	if err != nil {
		return nil, nil, err
	}
	return ch, raw, nil
}

func executeAction(w *rules.World, heroID, goblinID string, call llm.ToolCall) (string, error) {
	switch call.Function.Name {
	case "user_choice":
		var args struct {
			Options []string `json:"options"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse user_choice: %w", err)
		}
		if len(args.Options) == 0 {
			return "", fmt.Errorf("user_choice requires at least one option")
		}
		choice, err := showChoiceMenu(args.Options)
		if err != nil {
			return "", err
		}
		return choice, nil
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
			TraitType           string `json:"trait_type"`
			TraitSlug           string `json:"trait_slug"`
			Difficulty          int    `json:"difficulty"`
			SituationalModifier int    `json:"situational_modifier"`
			AddOne              bool   `json:"add_one"`
			Reroll              bool   `json:"reroll"`
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
			ActorTraitType    string `json:"actor_trait_type"`
			ActorTraitSlug    string `json:"actor_trait_slug"`
			ActorModifier     int    `json:"actor_modifier"`
			OpponentID        string `json:"opponent_id"`
			OpponentTraitType string `json:"opponent_trait_type"`
			OpponentTraitSlug string `json:"opponent_trait_slug"`
			OpponentModifier  int    `json:"opponent_modifier"`
			AddOne            bool   `json:"add_one"`
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
	case "apply_damage":
		ch, raw, err := getCharacterFromJSON(w, call.Function.Arguments)
		if err != nil {
			return "", err
		}
		dp, ok := raw["damage_points"].(float64)
		if !ok {
			return "", fmt.Errorf("damage_points required")
		}
		damagePoints := int(dp)
		if damagePoints < 0 {
			return "", fmt.Errorf("damage points must be non-negative")
		}
		result := ch.Wounds.ApplyDamage(damagePoints)
		return fmt.Sprintf("%s takes damage: %s (scratches: %d, penalty: %+d)", ch.Name, result, ch.Wounds.Scratches, ch.Wounds.Penalty()), nil
	case "heal":
		ch, raw, err := getCharacterFromJSON(w, call.Function.Arguments)
		if err != nil {
			return "", err
		}
		levels, ok := raw["levels"].(float64)
		if !ok {
			return "", fmt.Errorf("levels required")
		}
		lvl := int(levels)
		if lvl <= 0 {
			return "", fmt.Errorf("levels to heal must be positive")
		}
		result := ch.Wounds.Heal(lvl)
		return fmt.Sprintf("%s: %s (now at %s, penalty: %+d)", ch.Name, result, ch.Wounds.Level, ch.Wounds.Penalty()), nil
	case "declare_dead":
		ch, _, err := getCharacterFromJSON(w, call.Function.Arguments)
		if err != nil {
			return "", err
		}
		ch.Wounds.DeclareDead()
		return fmt.Sprintf("%s has been declared dead (death is permanent)", ch.Name), nil
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
		wrapped := wrapText(log[i], terminalWidth()-4)
		fmt.Println(wrapped)
	}
	fmt.Println("--------------------")
}

func showLoadingSpinner(duration time.Duration, done <-chan struct{}) {
	brailleChars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	frame := 0
	for {
		select {
		case <-done:
			fmt.Print("\r    \r")
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

func showChoiceMenu(options []string) (string, error) {
	allOptions := append(options, "↳ Type your answer")
	selected := 0

	rl, err := readline.New("")
	if err != nil {
		return "", err
	}
	defer rl.Close()

	for {
		clearScreen()
		fmt.Println()
		fmt.Println("  Choose an option (↑↓ to navigate, Enter to select):")
		fmt.Println()

		for i, opt := range allOptions {
			if i == selected {
				fmt.Printf("  \033[1;44m▶ %s \033[0m\n", opt)
			} else {
				fmt.Printf("    %s\n", opt)
			}
		}
		fmt.Println()

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
			return "", nil
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
