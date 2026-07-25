package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fudge/rules"
	"fudge/ui"

	"github.com/memmaker/gollum/llm"
)

func settingCreationTools() []llm.Tool {
	return []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "create_setting", Description: "Initialize a new setting with a chosen name. Call this FIRST before writing any setting files. The name becomes the folder name and setting identifier (converted to lowercase, spaces to hyphens). Example: 'Haunted Deadlands' becomes 'haunted-deadlands'.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string", "description": "The setting name (will be slugified for folder)"}}, "required": []string{"name"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "write_setting_file", Description: "Write one setting file. CRITICAL SEQUENCE: After create_setting, call this tool once for EACH of these files in this order: (1) description.md, (2) species.rec, (3) attributes.rec, (4) skills.rec, (5) gifts.rec, (6) faults.rec, (7) powers.rec.\n\nFor .rec files: Format each entry separated by blank line. REQUIRED FIELDS per entry: slug (lowercase+digits+hyphens only, NO underscores), display_name (human-readable name), description (1 sentence). OPTIONAL: scale (species only; integer -10 to +10 where 0 is human-sized, so anything roughly person-shaped is 0). Slug format: use hyphens not underscores, e.g. \"dust-witch\" not \"dust_witch\". Example entry:\nslug: dust-witch\ndisplay_name: Dust Witch\ndescription: Occult wanderers channeling desert spirits.\nscale: 0\n\nFor description.md: Plain markdown text describing the setting's atmosphere, themes, and core concept (2-3 sentences).", Parameters: map[string]any{"type": "object", "properties": map[string]any{"filename": map[string]any{"type": "string"}, "mode": map[string]any{"type": "string", "enum": []string{"replace", "append"}}, "content": map[string]any{"type": "string"}}, "required": []string{"filename", "mode", "content"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "user_choice", Description: "Present the user with 2-4 mutually exclusive options. ONLY use when: (1) the player's action has exactly 2-4 meaningful, dramatically distinct outcomes; (2) you cannot narrate forward without knowing which they choose; (3) the moment is critical enough to warrant explicit choice. REQUIREMENTS: Write a clear prompt/question before calling this tool. The menu appears after your text. Never include a 'Type your answer' option—it's added automatically.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"options": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "List of 2-4 mutually exclusive options. Do NOT include a 'Type your answer' or freeform option—it is added automatically."}}, "required": []string{"options"}}}},
	}
}

func adventureCreationTools() []llm.Tool {
	return []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "write_adventure", Description: "Persist the improvised adventure to disk so it can be replayed later, and hand it to the DM as the briefing for this session. REQUIRED: call this exactly once, as your only output—do not write the adventure as plain text. It is saved under the current setting's adventures folder, named after the title.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"title": map[string]any{"type": "string", "description": "Short evocative adventure title (3-6 words), used as the filename"}, "content": map[string]any{"type": "string", "description": "The full adventure text: hook, situation, immediate stakes"}}, "required": []string{"title", "content"}}}},
	}
}

func characterCreationTools() []llm.Tool {
	return []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "create_character", Description: "Create a new character. The character ID is automatically generated from the name. Only use traits (attributes, skills, gifts, faults) that exist in the setting. Species is required.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}, "species": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}, "location": map[string]any{"type": "string"}, "attributes": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, "skills": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, "gifts": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "faults": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, "required": []string{"name", "species"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "user_choice", Description: "Present the user with 2-4 mutually exclusive options. ONLY use when: (1) the player's action has exactly 2-4 meaningful, dramatically distinct outcomes; (2) you cannot narrate forward without knowing which they choose; (3) the moment is critical enough to warrant explicit choice. REQUIREMENTS: Write a clear prompt/question before calling this tool. The menu appears after your text. Never include a 'Type your answer' option—it's added automatically.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"options": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "List of 2-4 mutually exclusive options. Do NOT include a 'Type your answer' or freeform option—it is added automatically."}}, "required": []string{"options"}}}},
	}
}

func gameplayTools() []llm.Tool {
	// Under --img-gen the scene's image prompt rides along as a third argument of
	// output_narration rather than a tool of its own: a separate tool is a
	// separate round-trip, and the DM ended its turn with the narration instead of
	// ever getting there. Required, so every narrated scene produces one.
	narrationDescription := "Show the turn's narration to the player. This is the ONLY channel to them: they read the prose argument and nothing else—no dice, no tool names, no tool results, and no text you write outside this call. Text written as a plain reply reaches nobody. Every turn ends with exactly one output_narration, and the turn is not over until you call it. Call it only once you know what happened: if a check, a new NPC, damage or healing decides the outcome, make those calls first with no narration, read the results when they come back, and narrate in your next response. It must be your LAST tool call, after every state-changing call, so it can reflect their outcome. Argument prose: the narration the player reads. Argument condensed_state (OPTIONAL): dense LLM-readable memory of what that prose just established—include it when something worth remembering changed (a fact, a location, an item, a wound, a job), omit it on turns where nothing did. The FIRST narration of a game ALWAYS carries a condensed_state, and it is a FULL BASELINE: everything the opening prose established (player, NPCs, location, features, gear, clothing, hooks), since nothing is recorded yet and anything omitted is lost. Every condensed_state after it is ONLY the DELTA since the last one—new facts and changed facts, nothing else. Every previous condensed_state is still in your context, so restating unchanged facts wastes tokens and creates stale duplicates that later contradict each other."
	narrationProperties := map[string]any{
		"prose":           map[string]any{"type": "string", "description": "The turn's narration exactly as the player will read it: normal, descriptive, engaging prose in full sentences. It must carry everything the tool calls established, since the player sees none of them. No titles or speaker prefixes (no \"DM:\"), no meta-commentary, no mention of dice, difficulty or mechanics, and no \"What do you do?\"—narrate and stop."},
		"condensed_state": map[string]any{"type": "string", "description": "Optional. First call of a game: full baseline—everything the opening prose established, err toward too much. Every later call: delta only, what changed this turn. Ultra-condensed notation throughout (no articles/fluff, dense notation, world facts only). After the baseline, NEVER repeat state that has not changed—it is already in context. Facts that stopped being true must be stated explicitly (e.g. `door|unlocked`); silence means unchanged, not gone."},
	}
	narrationRequired := []string{"prose"}

	if imgGen {
		narrationDescription += " Argument img_gen (REQUIRED): the prompt for an image generation AI depicting the scene this prose just narrated. Write it for every narration—it is not optional and not something to save for dramatic moments."
		narrationProperties["img_gen"] = map[string]any{"type": "string", "description": "Prompt for an image generation AI depicting the scene the prose narrates. Self-contained: the image AI sees only this text, none of your context. Describe the location and its lighting and atmosphere in detail; then EVERY character present, each with their clothing, appearance, where they stand in the scene, their pose, and what they are doing; then the state and items that show—wounds, drawn weapons, carried or worn gear, doors open or shut, fire, debris. Plain visual description, present tense, no story, no names the picture cannot show."}
		narrationRequired = append(narrationRequired, "img_gen")
	}

	tools := []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "output_narration", Description: narrationDescription, Parameters: map[string]any{"type": "object", "properties": narrationProperties, "required": narrationRequired}}},
		{Type: "function", Function: llm.ToolFunction{Name: "unopposed_check", Description: "Roll an action that nothing is actively resisting: climbing a wall, picking a lock, recalling lore, crafting. Use it whenever the outcome is uncertain and no opponent opposes it—if one does, use opposed_check instead. Skip it entirely for trivial actions with no real chance of failure and no stakes; just narrate those. You name the difficulty and the engine rolls; narrate only what comes back, and never invent a roll or adjust the result. situational_modifier is roughly -4 to +4 for circumstances (terrain, lighting, preparation, exhaustion, poor tools); 0 should be the common case and ±3/±4 is for genuinely extreme conditions. add_one and reroll each spend one of the character's Fudge Points.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"actor_id": map[string]any{"type": "string"}, "trait_type": map[string]any{"type": "string", "enum": []string{"attribute", "skill"}}, "trait_slug": map[string]any{"type": "string"}, "difficulty": map[string]any{"type": "string", "enum": rules.DifficultyNames(), "description": "How hard the task is. The engine converts this to a target number."}, "situational_modifier": map[string]any{"type": "integer"}, "add_one": map[string]any{"type": "boolean"}, "reroll": map[string]any{"type": "boolean"}}, "required": []string{"actor_id", "trait_type", "trait_slug", "difficulty"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "opposed_check", Description: "Resolve a direct conflict between two characters (attack, grapple, dodge, deception, resist, etc.). Use when two characters have conflicting intentions. CRITICAL SEQUENCE: (1) If an NPC doesn't exist yet, call create_character FIRST in the same response; (2) THEN immediately call opposed_check with both actor_id and opponent_id (use the character's actual ID). actor_id is whoever initiated the action; opponent_id is whoever reacts. Do NOT narrate the defense separately—the check outcome tells the story. Narrate using only the tool result.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"actor_id": map[string]any{"type": "string"}, "actor_trait_type": map[string]any{"type": "string", "enum": []string{"attribute", "skill"}}, "actor_trait_slug": map[string]any{"type": "string"}, "actor_modifier": map[string]any{"type": "integer"}, "opponent_id": map[string]any{"type": "string"}, "opponent_trait_type": map[string]any{"type": "string", "enum": []string{"attribute", "skill"}}, "opponent_trait_slug": map[string]any{"type": "string"}, "opponent_modifier": map[string]any{"type": "integer"}, "add_one": map[string]any{"type": "boolean"}}, "required": []string{"actor_id", "actor_trait_type", "actor_trait_slug", "opponent_id", "opponent_trait_type", "opponent_trait_slug"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "create_character", Description: "Create a new NPC. CRITICAL: call this BEFORE any check involving that NPC—the character must exist before you can make a check against them. If a player targets an NPC for the first time, create them immediately, THEN call the appropriate check (opposed_check or unopposed_check) in the same response. The character ID is automatically generated from the name. Only use traits (attributes, skills, gifts, faults) listed in the Setting Information section of your system prompt. Species is required.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}, "species": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}, "location": map[string]any{"type": "string"}, "attributes": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, "skills": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, "gifts": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "faults": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, "required": []string{"name", "species"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "apply_damage", Description: "Wound a character after a hit, a fall, an explosion. You name the wound the fiction calls for and the engine does the arithmetic: it accumulates scratches, escalates wound levels and applies the resulting penalty. A bigger margin on the winning check, a heavier weapon or greater attacker Scale argue for a worse severity; armor, toughness or greater defender Scale argue for a lighter one, and a glancing hit can be narrated with no call at all. Death is never a side effect: characters stop at near death and are killed only by an explicit declare_dead.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": map[string]any{"type": "string"}, "severity": map[string]any{"type": "string", "enum": rules.SeverityNames(), "description": "The wound this hit inflicts on an undamaged target. Existing wounds escalate from there."}}, "required": []string{"character_id", "severity"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "roll_on_table", Description: "Roll on a random table when the fiction needs an outcome that no character is choosing. Use it instead of deciding these yourself—that is the only way they are genuinely random. npc-reaction: how an NPC you have not characterized yet feels about the party on first meeting. npc-morale: whether an NPC presses on, wavers or breaks, checked when a fight turns against them. weather: conditions on the road or at the start of a day outdoors. situational-roll: how luck breaks when there is no actor to check at all. The modifier is roughly -4 to +4 for whatever the established fiction says should tilt it (a reputation, a bribe, the season, a rout already in progress); 0 is the common case. Narrate the result the table gives you.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"table": map[string]any{"type": "string", "enum": rules.TableNames()}, "modifier": map[string]any{"type": "integer"}}, "required": []string{"table"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "heal", Description: "Heal a character by reducing wound levels. Cannot heal the dead.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": map[string]any{"type": "string"}, "levels": map[string]any{"type": "integer"}}, "required": []string{"character_id", "levels"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "declare_dead", Description: "Permanently declare a character dead. Dead characters cannot act or be healed.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": map[string]any{"type": "string"}}, "required": []string{"character_id"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "user_choice", Description: "Present the player with 2-4 mutually exclusive options. ONLY use when: (1) the player's action has exactly 2-4 meaningful, dramatically distinct outcomes; (2) you cannot narrate forward without knowing which they choose; (3) the moment is critical enough to warrant explicit choice. For normal gameplay where the player types an action, just narrate the outcome. Do NOT use this as a generic \"What do you do?\" prompt. REQUIREMENTS: call output_narration in the same response, with prose that poses the question—the menu is shown underneath it. The player's answer comes back to you as this tool's result, and the turn continues: narrate what their choice leads to. Never include a 'Type your answer' option—it's added automatically.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"options": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "List of 2-4 mutually exclusive options. Do NOT include a 'Type your answer' or freeform option—it is added automatically."}}, "required": []string{"options"}}}},
	}

	return tools
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

func writeCharsheet(w *rules.World) error {
	if w.Player == "" {
		return nil
	}
	content := rules.StateFileText(w)
	return os.WriteFile("charsheet.txt", []byte(content), 0o644)
}

// writeImagePrompt persists the image prompt the DM attached to its narration.
// Each turn replaces the file: it describes the scene on screen right now, and
// whatever tool picks it up wants the current one.
func writeImagePrompt(prompt string) error {
	return os.WriteFile("img-gen.md", []byte(prompt), 0o644)
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
			Difficulty          string `json:"difficulty"`
			SituationalModifier int    `json:"situational_modifier"`
			AddOne              bool   `json:"add_one"`
			Reroll              bool   `json:"reroll"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse unopposed check: %w", err)
		}
		if args.ActorID == "" || args.TraitType == "" || args.TraitSlug == "" {
			return "", fmt.Errorf("unopposed check requires actor_id, trait_type, and trait_slug")
		}
		difficulty, err := rules.DifficultyLevel(args.Difficulty)
		if err != nil {
			return "", err
		}
		result, err := w.UnopposedCheck(args.ActorID, args.TraitType, args.TraitSlug, difficulty, args.SituationalModifier, args.AddOne, args.Reroll)
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
		return fmt.Sprintf("%s|roll%+d|tot%d|%s(%+d)|%s%s", actor.Name, result.Roll, result.Total, args.Difficulty, difficulty, successStr, critStr), nil
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
	case "write_adventure":
		var args struct {
			Title   string `json:"title"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse write_adventure: %w", err)
		}
		if w.Setting == nil {
			return "", fmt.Errorf("world has no setting selected")
		}
		if args.Title == "" || args.Content == "" {
			return "", fmt.Errorf("write_adventure requires title and content")
		}
		path, err := w.Setting.WriteAdventure(args.Title, []byte(args.Content))
		if err != nil {
			return "", err
		}
		w.AdventureContent = args.Content
		return fmt.Sprintf("saved adventure %q to %s", args.Title, path), nil
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
		severity, ok := raw["severity"].(string)
		if !ok {
			return "", fmt.Errorf("severity required (%s)", strings.Join(rules.SeverityNames(), ", "))
		}
		damagePoints, err := rules.DamagePointsForSeverity(severity)
		if err != nil {
			return "", err
		}
		result := ch.Wounds.ApplyDamage(damagePoints)
		return fmt.Sprintf("%s|hit:%s|now:%s|scratch:%d|pen%+d", ch.Name, severity, result, ch.Wounds.Scratches, ch.Wounds.Penalty()), nil
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
	case "roll_on_table":
		var args struct {
			Table    string `json:"table"`
			Modifier int    `json:"modifier"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse roll_on_table: %w", err)
		}
		roll, value, err := w.RollTable(args.Table, args.Modifier)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s|roll%+d|mod%+d|%s", args.Table, roll, args.Modifier, value), nil
	default:
		return "", fmt.Errorf("unknown action tool %q", call.Function.Name)
	}
}
