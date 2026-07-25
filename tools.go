package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"fudge/rules"
	"fudge/ui"

	"github.com/memmaker/gollum/llm"
)

func settingCreationTools() []llm.Tool {
	return []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "create_setting", Description: "Initialize a new setting with a chosen name. Call this FIRST before writing any setting files. The name becomes the folder name and setting identifier (converted to lowercase, spaces to hyphens). Example: 'Haunted Deadlands' becomes 'haunted-deadlands'.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string", "description": "The setting name (will be slugified for folder)"}}, "required": []string{"name"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "write_setting_file", Description: "Write one setting file in .rec format. Valid filenames: species.rec, attributes.rec, skills.rec, gifts.rec, faults.rec. Format: each entry separated by blank line. REQUIRED FIELDS per entry: slug (lowercase+digits+hyphens only, NO underscores), display_name (human-readable name), description (1 sentence). OPTIONAL: scale (0-3, species only). Slug format: use hyphens not underscores, e.g. \"dust-witch\" not \"dust_witch\". Example entry:\nslug: dust-witch\ndisplay_name: Dust Witch\ndescription: Occult wanderers channeling desert spirits.\nscale: 2", Parameters: map[string]any{"type": "object", "properties": map[string]any{"filename": map[string]any{"type": "string"}, "mode": map[string]any{"type": "string", "enum": []string{"replace", "append"}}, "content": map[string]any{"type": "string"}}, "required": []string{"filename", "mode", "content"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "user_choice", Description: "Present the user with 2-4 mutually exclusive options. ONLY use when: (1) the player's action has exactly 2-4 meaningful, dramatically distinct outcomes; (2) you cannot narrate forward without knowing which they choose; (3) the moment is critical enough to warrant explicit choice. REQUIREMENTS: Write a clear prompt/question before calling this tool. The menu appears after your text. Never include a 'Type your answer' option—it's added automatically.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"options": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "List of 2-4 mutually exclusive options. Do NOT include a 'Type your answer' or freeform option—it is added automatically."}}, "required": []string{"options"}}}},
	}
}

func characterCreationTools() []llm.Tool {
	return []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "create_character", Description: "Create a new character. The character ID is automatically generated from the name. Only use traits (attributes, skills, gifts, faults) that exist in the setting. Species is required.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}, "species": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}, "location": map[string]any{"type": "string"}, "attributes": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, "skills": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, "gifts": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "faults": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, "required": []string{"name", "species"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "user_choice", Description: "Present the user with 2-4 mutually exclusive options. ONLY use when: (1) the player's action has exactly 2-4 meaningful, dramatically distinct outcomes; (2) you cannot narrate forward without knowing which they choose; (3) the moment is critical enough to warrant explicit choice. REQUIREMENTS: Write a clear prompt/question before calling this tool. The menu appears after your text. Never include a 'Type your answer' option—it's added automatically.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"options": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "List of 2-4 mutually exclusive options. Do NOT include a 'Type your answer' or freeform option—it is added automatically."}}, "required": []string{"options"}}}},
	}
}

func gameplayTools() []llm.Tool {
	return []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "output_narration", Description: "Output narration and world state. REQUIRED: Call this as your final action every turn. Do not output any text directly—use only this tool. Arguments: prose (human-readable narration for the player), condensed_state (ultra-dense LLM-readable state: no articles/fluff, compact notation, world facts, character positions, meaningful events only).", Parameters: map[string]any{"type": "object", "properties": map[string]any{"prose": map[string]any{"type": "string", "description": "Human-readable narration for the player"}, "condensed_state": map[string]any{"type": "string", "description": "Ultra-condensed state for LLM context: no articles/fluff, dense notation, world facts only"}}, "required": []string{"prose", "condensed_state"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "unopposed_check", Description: "Resolve an action that doesn't involve direct opposition. Use when: (1) a character attempts something that requires skill with an uncertain outcome but no opponent actively resisting (climbing a wall, solving a puzzle, crafting); (2) difficulty is set (difficulty 1-5 typical). Do NOT use this when another character is actively opposing the action—use opposed_check instead. Narrate the result using only the tool output; do not add your own dice rolls or modify the outcome.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": map[string]any{"type": "string"}, "trait_type": map[string]any{"type": "string", "enum": []string{"attribute", "skill"}}, "trait_slug": map[string]any{"type": "string"}, "difficulty": map[string]any{"type": "integer"}, "situational_modifier": map[string]any{"type": "integer"}, "add_one": map[string]any{"type": "boolean"}, "reroll": map[string]any{"type": "boolean"}}, "required": []string{"character_id", "trait_type", "trait_slug", "difficulty"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "opposed_check", Description: "Resolve a direct conflict between two characters (attack, grapple, dodge, deception, resist, etc.). Use when two characters have conflicting intentions. CRITICAL SEQUENCE: (1) If an NPC doesn't exist yet, call create_character FIRST in the same response; (2) THEN immediately call opposed_check with both actor_id and opponent_id (use the character's actual ID). actor_id is whoever initiated the action; opponent_id is whoever reacts. Do NOT narrate the defense separately—the check outcome tells the story. Narrate using only the tool result.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"actor_id": map[string]any{"type": "string"}, "actor_trait_type": map[string]any{"type": "string", "enum": []string{"attribute", "skill"}}, "actor_trait_slug": map[string]any{"type": "string"}, "actor_modifier": map[string]any{"type": "integer"}, "opponent_id": map[string]any{"type": "string"}, "opponent_trait_type": map[string]any{"type": "string", "enum": []string{"attribute", "skill"}}, "opponent_trait_slug": map[string]any{"type": "string"}, "opponent_modifier": map[string]any{"type": "integer"}, "add_one": map[string]any{"type": "boolean"}}, "required": []string{"actor_id", "actor_trait_type", "actor_trait_slug", "opponent_id", "opponent_trait_type", "opponent_trait_slug"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "create_character", Description: "Create a new NPC. CRITICAL: call this BEFORE any check involving that NPC—the character must exist before you can make a check against them. If a player targets an NPC for the first time, create them immediately, THEN call the appropriate check (opposed_check or unopposed_check) in the same response. The character ID is automatically generated from the name. Only use traits (attributes, skills, gifts, faults) that exist in the setting—call query_setting if unsure. Species is required.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}, "species": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}, "location": map[string]any{"type": "string"}, "attributes": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, "skills": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, "gifts": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "faults": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, "required": []string{"name", "species"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "apply_damage", Description: "Apply damage to a character. Damage accumulates as scratches and escalates wound levels. Use after combat, falling, explosions, etc.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": map[string]any{"type": "string"}, "damage_points": map[string]any{"type": "integer"}}, "required": []string{"character_id", "damage_points"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "heal", Description: "Heal a character by reducing wound levels. Cannot heal the dead.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": map[string]any{"type": "string"}, "levels": map[string]any{"type": "integer"}}, "required": []string{"character_id", "levels"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "declare_dead", Description: "Permanently declare a character dead. Dead characters cannot act or be healed.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": map[string]any{"type": "string"}}, "required": []string{"character_id"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "user_choice", Description: "Present the user with 2-4 mutually exclusive options. ONLY use when: (1) the player's action has exactly 2-4 meaningful, dramatically distinct outcomes; (2) you cannot narrate forward without knowing which they choose; (3) the moment is critical enough to warrant explicit choice. For normal gameplay where the player types an action, just narrate the outcome. Do NOT use this as a generic \"What do you do?\" prompt. REQUIREMENTS: Write a clear prompt/question before calling this tool. The menu appears after your text. Never include a 'Type your answer' option—it's added automatically.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"options": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "List of 2-4 mutually exclusive options. Do NOT include a 'Type your answer' or freeform option—it is added automatically."}}, "required": []string{"options"}}}},
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

func loadSetting(w *rules.World, name string) error {
	path := filepath.Join("settings", name)
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("setting not found: %s", name)
	}

	setting, err := rules.LoadSetting(path)
	if err != nil {
		return fmt.Errorf("failed to load setting: %w", err)
	}

	w.Setting = setting

	return nil
}

func listAdventures(w *rules.World) (string, error) {
	if w.Setting == nil {
		return "", fmt.Errorf("no setting loaded")
	}

	adventuresPath := filepath.Join(w.Setting.Path, "adventures")
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
	if w.Setting == nil {
		return "", fmt.Errorf("no setting loaded")
	}

	adventurePath := filepath.Join(w.Setting.Path, "adventures", name+".md")
	content, err := os.ReadFile(adventurePath)
	if err != nil {
		return "", fmt.Errorf("failed to load adventure: %w", err)
	}

	return string(content), nil
}

func querySetting(w *rules.World) (string, error) {
	if w.Setting == nil {
		return "", fmt.Errorf("world has no selected setting")
	}

	var sb strings.Builder
	sb.WriteString("Available traits in setting:\n\n")

	sb.WriteString("ATTRIBUTES:\n")
	for _, attr := range sortedAttributeKeys(w.Setting.Attributes) {
		desc := w.Setting.Attributes[attr].Description
		sb.WriteString(fmt.Sprintf("  - %s: %s\n", attr, desc))
	}

	sb.WriteString("\nSKILLS:\n")
	for _, skill := range sortedSkillKeys(w.Setting.Skills) {
		desc := w.Setting.Skills[skill].Description
		sb.WriteString(fmt.Sprintf("  - %s: %s\n", skill, desc))
	}

	sb.WriteString("\nSPECIES:\n")
	for _, spec := range sortedSpeciesKeys(w.Setting.Species) {
		desc := w.Setting.Species[spec].Description
		scale := w.Setting.Species[spec].Scale
		sb.WriteString(fmt.Sprintf("  - %s (scale %d): %s\n", spec, scale, desc))
	}

	sb.WriteString("\nGIFTS:\n")
	for _, gift := range sortedGiftKeys(w.Setting.Gifts) {
		desc := w.Setting.Gifts[gift].Description
		sb.WriteString(fmt.Sprintf("  - %s: %s\n", gift, desc))
	}

	sb.WriteString("\nFAULTS:\n")
	for _, fault := range sortedFaultKeys(w.Setting.Faults) {
		desc := w.Setting.Faults[fault].Description
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
	case "create_setting":
		var args struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse create_setting: %w", err)
		}
		if args.Name == "" {
			return "", fmt.Errorf("setting name required")
		}
		settingSlug := slugify(args.Name)
		settingPath := filepath.Join("settings", settingSlug)
		if err := os.MkdirAll(settingPath, 0o755); err != nil {
			return "", fmt.Errorf("failed to create setting directory: %w", err)
		}
		w.Setting = &rules.SettingInfos{
			Slug:       settingSlug,
			Path:       settingPath,
			Attributes: make(map[string]*rules.Attribute),
			Skills:     make(map[string]*rules.Skill),
			Species:    make(map[string]*rules.Species),
			Gifts:      make(map[string]*rules.Gift),
			Faults:     make(map[string]*rules.Fault),
			Powers:     make(map[string]*rules.Power),
		}
		return fmt.Sprintf("Setting '%s' created (folder: %s)", args.Name, settingSlug), nil
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
		choice, err := ui.ShowChoiceMenu(args.Options)
		if err != nil {
			return "", err
		}
		return choice, nil
	case "unopposed_check":
		var args struct {
			ActorID             string `json:"actor_id"`
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
		if args.ActorID == "" || args.TraitType == "" || args.TraitSlug == "" {
			return "", fmt.Errorf("unopposed check requires character_id, trait_type, and trait_slug")
		}
		result, err := w.UnopposedCheck(args.ActorID, args.TraitType, args.TraitSlug, args.Difficulty, args.SituationalModifier, args.AddOne, args.Reroll)
		if err != nil {
			return "", err
		}
		actor, _ := w.Character(args.ActorID)
		var critStr string
		if result.CritSuccess {
			critStr = "|CRIT"
		} else if result.CritFailure {
			critStr = "|FAIL"
		}
		successStr := "no"
		if result.Success {
			successStr = "ok"
		}
		return fmt.Sprintf("%s|roll%+d|tot%d|diff%d|%s%s", actor.Name, result.Roll, result.Total, args.Difficulty, successStr, critStr), nil
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
			critStr += "|" + actor.Name + "-CRIT"
		}
		if result.OpponentCrit {
			critStr += "|" + opponent.Name + "-CRIT"
		}
		outcomeStr := "tie"
		if result.ActorWins {
			outcomeStr = actor.Name
		} else {
			outcomeStr = opponent.Name
		}
		return fmt.Sprintf("%s|%d|vs|%s|%d|deg%+d|win:%s%s", actor.Name, result.ActorTotal, opponent.Name, result.OpponentTotal, result.RelativeDegree, outcomeStr, critStr), nil
	case "write_setting_file":
		var args struct {
			Filename string `json:"filename"`
			Mode     string `json:"mode"`
			Content  string `json:"content"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse write setting file: %w", err)
		}
		if w.Setting == nil {
			return "", fmt.Errorf("world has no setting selected")
		}
		if args.Filename == "" || args.Mode == "" {
			return "", fmt.Errorf("write setting file requires filename, mode, and content")
		}
		if err := w.Setting.WriteFile(args.Filename, args.Mode, []byte(args.Content)); err != nil {
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
		return fmt.Sprintf("%s|dmg%d|%s|scratch:%d|pen%+d", ch.Name, damagePoints, result, ch.Wounds.Scratches, ch.Wounds.Penalty()), nil
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
		return fmt.Sprintf("%s|heal%d|%s|level:%s|pen%+d", ch.Name, lvl, result, ch.Wounds.Level, ch.Wounds.Penalty()), nil
	case "declare_dead":
		ch, _, err := getCharacterFromJSON(w, call.Function.Arguments)
		if err != nil {
			return "", err
		}
		ch.Wounds.DeclareDead()
		return fmt.Sprintf("%s|DEAD", ch.Name), nil
	default:
		return "", fmt.Errorf("unknown action tool %q", call.Function.Name)
	}
}
