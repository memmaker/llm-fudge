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

// userChoiceTool is the menu, offered wherever the model needs the player to
// pick between a small number of distinct options.
func userChoiceTool() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.ToolFunction{
		Name:        "user_choice",
		Description: "Present the player with 2-4 mutually exclusive options. ONLY use when you cannot continue without knowing which they pick. Write the question as ordinary text first; the menu appears under it. Never include a 'Type your answer' option—it is added automatically.",
		Parameters: map[string]any{"type": "object", "properties": map[string]any{
			"options": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "2-4 mutually exclusive options."},
		}, "required": []string{"options"}},
	}}
}

func settingCreationTools() []llm.Tool {
	return []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{
			Name:        "create_setting",
			Description: "Initialize a new setting with a chosen name. Call this FIRST. The name becomes the folder name (lowercased, spaces to hyphens): 'Haunted Deadlands' becomes 'haunted-deadlands'.",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{
				"name": map[string]any{"type": "string", "description": "The setting name."},
			}, "required": []string{"name"}},
		}},
		{Type: "function", Function: llm.ToolFunction{
			Name:        "write_setting_file",
			Description: "Write the setting's description.md: plain markdown describing the world's atmosphere, themes and core concept, and — because there is no separate data file any more — the species, professions, skills, gifts and faults that exist in it, as prose the DM reads. Two to six paragraphs. Call this once, after create_setting.",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{
				"content": map[string]any{"type": "string", "description": "The full description.md text."},
			}, "required": []string{"content"}},
		}},
		userChoiceTool(),
	}
}

func adventureCreationTools() []llm.Tool {
	return []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{
			Name:        "write_adventure",
			Description: "Persist the improvised adventure to disk and hand it to the DM as this session's briefing. REQUIRED: call this exactly once, as your only output—do not write the adventure as plain text.",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{
				"title":   map[string]any{"type": "string", "description": "Short evocative title (3-6 words), used as the filename."},
				"content": map[string]any{"type": "string", "description": "The full adventure text: hook, situation, immediate stakes."},
			}, "required": []string{"title", "content"}},
		}},
	}
}

// gameTools is everything the DM can do during a turn: roll a check and ask the
// player a question. The sheet is written by the world update block instead of
// by a tool, the session directory is derived rather than created, and a
// milestone costs no request — so nothing else has a consumer left. A tool the
// DM is never shown is one it cannot reach for by mistake. See docs/adr/007.
func gameTools() []llm.Tool {
	return []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{
			Name:        "check",
			Description: "Roll 4dF for one action and get the result. Use it whenever an outcome is genuinely uncertain; do not decide success or failure yourself, and never invent a roll. Trivial actions with no stakes need no check at all.",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{
				"level":    map[string]any{"type": "string", "description": "The acting trait's level from the character sheet: Terrible, Poor, Mediocre, Fair, Good, Great or Superb."},
				"vs":       map[string]any{"type": "string", "description": "What is being overcome. For an unopposed action, a difficulty: Trivially easy, Very easy, Easy, Average, Hard, Very hard, Extremely hard. For an opposed action, the opponent's trait level (Fair, Good, ...), and the opponent rolls too."},
				"modifier": map[string]any{"type": "integer", "description": "Situational modifier, roughly -4 to +4. Zero should be the common case. Include the wound penalty from the sheet here."},
				"what":     map[string]any{"type": "string", "description": "One short phrase naming the attempt, e.g. 'Garret picks the cellar lock'."},
			}, "required": []string{"level", "vs", "what"}},
		}},
		userChoiceTool(),
	}
}

func loadSetting(s *session, name string) error {
	path := filepath.Join("settings", name)
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("setting not found: %s", name)
	}
	description, err := os.ReadFile(filepath.Join(path, "description.md"))
	if err != nil {
		return fmt.Errorf("setting %s has no description.md: %w", name, err)
	}
	s.settingName = name
	s.settingDesc = string(description)
	return nil
}

func loadAdventure(s *session, name string) (string, error) {
	if s.settingName == "" {
		return "", fmt.Errorf("no setting loaded")
	}
	content, err := os.ReadFile(filepath.Join("settings", s.settingName, "adventures", name+".md"))
	if err != nil {
		return "", fmt.Errorf("failed to load adventure: %w", err)
	}
	return string(content), nil
}

// executeAction runs one tool call. Every handler returns text the model reads,
// so an error is a sentence telling it what to do differently, not a status.
func (s *session) executeAction(call llm.ToolCall) (string, error) {
	args := map[string]any{}
	if call.Function.Arguments != "" {
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("could not read the arguments of %s: %w", call.Function.Name, err)
		}
	}
	str := func(key string) string {
		v, _ := args[key].(string)
		return v
	}

	switch call.Function.Name {
	case "check":
		mod := 0
		if f, ok := args["modifier"].(float64); ok {
			mod = int(f)
		}
		result, err := rules.Check(str("level"), str("vs"), mod, s.rng)
		if err != nil {
			return "", err
		}
		what := str("what")
		if what == "" {
			what = "the attempt"
		}
		return what + ": " + result.Describe(), nil

	case "user_choice":
		raw, _ := args["options"].([]any)
		options := make([]string, 0, len(raw))
		for _, o := range raw {
			if option, ok := o.(string); ok {
				options = append(options, option)
			}
		}
		if len(options) == 0 {
			return "", fmt.Errorf("user_choice needs options")
		}
		choice, err := ui.ShowChoiceMenu(options)
		if err != nil {
			return "", err
		}
		// The menu's last entry is an opt-out, not an option: it means the player
		// wants to say something the options do not cover, so ask them here and
		// send what they typed. Handing the marker itself back once had a DM read
		// "[Type your answer]" as agreement and move a statue nobody touched.
		if choice == ui.TypeYourAnswer() {
			line, err := ui.GetInput(nil, "> ")
			if err != nil {
				return "", err
			}
			return "The player answered: " + line, nil
		}
		return "The player chose: " + choice, nil

	case "create_setting":
		name := slugify(str("name"))
		if name == "" {
			return "", fmt.Errorf("the setting needs a name")
		}
		if err := os.MkdirAll(filepath.Join("settings", name, "adventures"), 0755); err != nil {
			return "", err
		}
		s.settingName = name
		return fmt.Sprintf("Created settings/%s. Write description.md next.", name), nil

	case "write_setting_file":
		if s.settingName == "" {
			return "", fmt.Errorf("call create_setting first")
		}
		content := str("content")
		if err := os.WriteFile(filepath.Join("settings", s.settingName, "description.md"), []byte(content), 0644); err != nil {
			return "", err
		}
		s.settingDesc = content
		return fmt.Sprintf("Wrote settings/%s/description.md.", s.settingName), nil

	case "write_adventure":
		if s.settingName == "" {
			return "", fmt.Errorf("no setting loaded")
		}
		title := strings.TrimSpace(str("title"))
		if title == "" {
			return "", fmt.Errorf("the adventure needs a title")
		}
		path := filepath.Join("settings", s.settingName, "adventures", slugify(title)+".md")
		if err := os.WriteFile(path, []byte(str("content")), 0644); err != nil {
			return "", err
		}
		s.adventure = str("content")
		// Kept for the session directory's name, which the harness builds out of
		// this and the character's name from the first world update block.
		s.adventureName = title
		return fmt.Sprintf("Wrote %s.", path), nil
	}

	return "", fmt.Errorf("unknown tool: %s", call.Function.Name)
}
