package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fudge/prompts"
	"fudge/rules"
	"fudge/ui"

	"github.com/memmaker/gollum/llm"
)

// playerLanguageNote repeats the system prompt's language rule at the one place
// the model is actually filling a field the player will read. The prompt says it
// once, in a section; a tool description says it while the argument is being
// written, which is where English creeps back in. Empty when the player reads
// English, so the default game's tools stay exactly as they were.
func playerLanguageNote(what string) string {
	if language == "" || language == prompts.DefaultLanguage {
		return ""
	}
	return fmt.Sprintf(" LANGUAGE: %s is read by the player, so write it in %s.", what, language)
}

// userChoiceTool is the menu, offered wherever the model needs the player to
// pick between a small number of distinct options.
func userChoiceTool() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.ToolFunction{
		Name:        "user_choice",
		Description: "Present the player with 2-4 mutually exclusive options. ONLY use when you cannot continue without knowing which they pick. Write the question as ordinary text first; the menu appears under it. Never include a 'Type your answer' option—it is added automatically." + playerLanguageNote("The options are"),
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

// gameTools is everything the DM can do during play: roll a check, and read and
// write its own notes. There is nothing else — no engine to move an item in, no
// state to declare. See docs/adr/006.
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
		{Type: "function", Function: llm.ToolFunction{
			Name:        "create_session_directory",
			Description: "Create this game's directory. Call it once, as the first thing you do after the character is finished and before you write the character sheet.",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{
				"character": map[string]any{"type": "string", "description": "The player character's name."},
				"adventure": map[string]any{"type": "string", "description": "The adventure's title."},
			}, "required": []string{"character", "adventure"}},
		}},
		{Type: "function", Function: llm.ToolFunction{
			Name:        "read_file",
			Description: "Read a file from this game's directory. Pass a bare filename—there are no paths. 'charsheet_template.md' is also readable, and is what you shape the first character sheet from.",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{
				"filename": map[string]any{"type": "string", "description": "A bare filename, e.g. 'charsheet.md'."},
			}, "required": []string{"filename"}},
		}},
		{Type: "function", Function: llm.ToolFunction{
			Name:        "write_file",
			Description: "Create or overwrite a file in this game's directory with full contents. Use it to create charsheet.md, and to write a milestone summary. To change something in a file that already exists, use edit_file instead—overwriting the whole sheet to alter one line is how facts get silently dropped.",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{
				"filename": map[string]any{"type": "string", "description": "A bare filename."},
				"content":  map[string]any{"type": "string", "description": "The complete file contents."},
			}, "required": []string{"filename", "content"}},
		}},
		{Type: "function", Function: llm.ToolFunction{
			Name:        "edit_file",
			Description: "Replace an exact piece of text in a file. This is how the character sheet is kept current: one edit per thing that changed. Fails if old_string is absent, or if it matches more than once and replace_all was not set—copy enough surrounding text to be unambiguous, whitespace included.",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{
				"filename":    map[string]any{"type": "string", "description": "A bare filename, usually 'charsheet.md'."},
				"old_string":  map[string]any{"type": "string", "description": "The exact text to find, copied verbatim from the file."},
				"new_string":  map[string]any{"type": "string", "description": "What it becomes. Must differ from old_string."},
				"replace_all": map[string]any{"type": "boolean", "description": "Replace every occurrence instead of requiring a unique match."},
			}, "required": []string{"filename", "old_string", "new_string"}},
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

// writeImagePrompt persists the image prompt the DM attached to its narration.
// Each turn replaces the file: it describes the scene on screen right now, and
// whatever tool picks it up wants the current one.
func writeImagePrompt(prompt string) error {
	return os.WriteFile("img-gen.md", []byte(prompt), 0o644)
}

// writeNarration persists the prose the player was just shown, replacing what
// was there. This is the text on screen right now; the record of the run is the
// session directory's job.
func writeNarration(prose string) error {
	return os.WriteFile("narration.txt", []byte(prose), 0o644)
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

	case "create_session_directory":
		if s.dir != "" {
			return fmt.Sprintf("The session directory already exists: %s", filepath.Base(s.dir)), nil
		}
		dir, err := s.createDir(savesRoot, str("character"), str("adventure"))
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Created %s. Files you write go here; %s is what you may write.", dir, charsheetFile), nil

	case "read_file":
		return s.readFile(str("filename"))

	case "write_file":
		return s.writeFile(str("filename"), str("content"))

	case "edit_file":
		replaceAll, _ := args["replace_all"].(bool)
		return s.editFile(str("filename"), str("old_string"), str("new_string"), replaceAll)

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
		return fmt.Sprintf("Wrote %s.", path), nil
	}

	return "", fmt.Errorf("unknown tool: %s", call.Function.Name)
}
