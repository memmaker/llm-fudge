package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"fudge/rules"

	"github.com/memmaker/gollum/llm"
)

func actionTools() []llm.Tool {
	return []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "list_settings", Description: "List all available settings.", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "load_setting", Description: "Load a specific setting by name.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}, "required": []string{"name"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "list_adventures", Description: "List all available adventures in the current setting.", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "load_adventure", Description: "Load an adventure by name from the current setting.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}, "required": []string{"name"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "query_setting", Description: "Query available attributes, skills, species, gifts, and faults in the current setting.", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "user_choice", Description: "Present the user with multiple choice options using an interactive menu. REQUIREMENTS: (1) You MUST write a clear prompt/question before calling this tool. The menu will appear directly after your text. (2) A 'Type your answer' fallback is ALWAYS included automatically—do NOT add it yourself.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"options": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "List of 2-4 mutually exclusive options. Do NOT include a 'Type your answer' or freeform option—it is added automatically."}}, "required": []string{"options"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "unopposed_check", Description: "Resolve an unopposed check against a difficulty using a chosen trait.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"trait_type": map[string]any{"type": "string", "enum": []string{"attribute", "skill"}}, "trait_slug": map[string]any{"type": "string"}, "difficulty": map[string]any{"type": "integer"}, "situational_modifier": map[string]any{"type": "integer"}, "add_one": map[string]any{"type": "boolean"}, "reroll": map[string]any{"type": "boolean"}}, "required": []string{"trait_type", "trait_slug", "difficulty"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "opposed_check", Description: "Resolve an opposed check between two actors using independently chosen traits.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"actor_id": map[string]any{"type": "string"}, "actor_trait_type": map[string]any{"type": "string", "enum": []string{"attribute", "skill"}}, "actor_trait_slug": map[string]any{"type": "string"}, "actor_modifier": map[string]any{"type": "integer"}, "opponent_id": map[string]any{"type": "string"}, "opponent_trait_type": map[string]any{"type": "string", "enum": []string{"attribute", "skill"}}, "opponent_trait_slug": map[string]any{"type": "string"}, "opponent_modifier": map[string]any{"type": "integer"}, "add_one": map[string]any{"type": "boolean"}}, "required": []string{"actor_id", "actor_trait_type", "actor_trait_slug", "opponent_id", "opponent_trait_type", "opponent_trait_slug"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "write_setting_file", Description: "Write one recognized setting file in replace or append mode.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"filename": map[string]any{"type": "string"}, "mode": map[string]any{"type": "string", "enum": []string{"replace", "append"}}, "content": map[string]any{"type": "string"}}, "required": []string{"filename", "mode", "content"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "create_character", Description: "Create a new character in the selected setting. The character ID is automatically generated from the name.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}, "species": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}, "location": map[string]any{"type": "string"}, "attributes": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, "skills": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, "gifts": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "faults": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, "required": []string{"name", "species"}}}},
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

func executeAction(w *rules.World, call llm.ToolCall) (string, error) {
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
		result, err := w.UnopposedCheck(w.Player, args.TraitType, args.TraitSlug, args.Difficulty, args.SituationalModifier, args.AddOne, args.Reroll)
		if err != nil {
			return "", err
		}
		hero, _ := w.Character(w.Player)
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
			ActorID           string `json:"actor_id"`
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
		if args.ActorID == "player" {
			args.ActorID = w.Player
		}
		if args.OpponentID == "player" {
			args.OpponentID = w.Player
		}
		if args.ActorID == "" || args.ActorTraitType == "" || args.ActorTraitSlug == "" || args.OpponentID == "" || args.OpponentTraitType == "" || args.OpponentTraitSlug == "" {
			return "", fmt.Errorf("opposed check requires actor_id, all trait types/slugs, and opponent_id")
		}
		result, err := w.OpposedCheck(args.ActorID, args.ActorTraitType, args.ActorTraitSlug, args.ActorModifier, args.OpponentID, args.OpponentTraitType, args.OpponentTraitSlug, args.OpponentModifier, args.AddOne)
		if err != nil {
			return "", err
		}
		actor, _ := w.Character(args.ActorID)
		opponent, _ := w.Character(args.OpponentID)
		var critStr string
		if result.ActorCrit {
			critStr += fmt.Sprintf(" (%s critical success!)", actor.Name)
		}
		if result.OpponentCrit {
			critStr += fmt.Sprintf(" (%s critical success!)", opponent.Name)
		}
		outcomeStr := "tie or loss"
		if result.ActorWins {
			outcomeStr = fmt.Sprintf("%s wins", actor.Name)
		}
		return fmt.Sprintf("%s scored %d against %s's %d (relative degree %+d): %s%s", actor.Name, result.ActorTotal, opponent.Name, result.OpponentTotal, result.RelativeDegree, outcomeStr, critStr), nil
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
		charReq := rules.CharacterRequest{
			Name:        req.Name,
			Species:     req.Species,
			Description: req.Description,
			Location:    req.Location,
			Attributes:  req.Attributes,
			Skills:      req.Skills,
			Gifts:       req.Gifts,
			Faults:      req.Faults,
		}
		ch, err := w.CreateCharacterValidated(charReq)
		if err != nil {
			if strings.Contains(err.Error(), "character slug already exists") {
				return "", fmt.Errorf("a character named %q already exists; please choose a different name", req.Name)
			}
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
