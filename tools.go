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

// userChoiceTool is the menu, offered wherever the model needs the player to
// pick between a small number of distinct options.
func userChoiceTool() llm.Tool {
	description := prompts.MustFill("tool-user-choice.md", nil)
	optionDescription := prompts.MustFill("tool-user-choice-option.md", nil)
	return llm.Tool{Type: "function", Function: llm.ToolFunction{
		Name:        "user_choice",
		Description: description,
		Parameters: map[string]any{"type": "object", "properties": map[string]any{
			"options": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": optionDescription},
		}, "required": []string{"options"}},
	}}
}

func settingCreationTools() []llm.Tool {
	createDescription := prompts.MustFill("tool-create-setting.md", nil)
	createNameDescription := prompts.MustFill("tool-create-setting-name.md", nil)
	writeDescription := prompts.MustFill("tool-write-setting.md", nil)
	writeContentDescription := prompts.MustFill("tool-write-setting-content.md", nil)
	return []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{
			Name:        "create_setting",
			Description: createDescription,
			Parameters: map[string]any{"type": "object", "properties": map[string]any{
				"name": map[string]any{"type": "string", "description": createNameDescription},
			}, "required": []string{"name"}},
		}},
		{Type: "function", Function: llm.ToolFunction{
			Name:        "write_setting_file",
			Description: writeDescription,
			Parameters: map[string]any{"type": "object", "properties": map[string]any{
				"content": map[string]any{"type": "string", "description": writeContentDescription},
			}, "required": []string{"content"}},
		}},
		userChoiceTool(),
	}
}

func adventureCreationTools() []llm.Tool {
	description := prompts.MustFill("tool-write-adventure.md", nil)
	titleDescription := prompts.MustFill("tool-write-adventure-title.md", nil)
	contentDescription := prompts.MustFill("tool-write-adventure-content.md", nil)
	return []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{
			Name:        "write_adventure",
			Description: description,
			Parameters: map[string]any{"type": "object", "properties": map[string]any{
				"title":   map[string]any{"type": "string", "description": titleDescription},
				"content": map[string]any{"type": "string", "description": contentDescription},
			}, "required": []string{"title", "content"}},
		}},
	}
}

// gameTools is everything the DM can do during a turn: roll a check, read a
// secret, hand over the sheet and ask the player a question. The session
// directory is derived rather than created and a milestone costs no request, so
// nothing else has a consumer left. A tool the DM is never shown is one it
// cannot reach for by mistake. See docs/adr/007.
func gameTools() []llm.Tool {
	checkDescription := prompts.MustFill("tool-check.md", nil)
	levelDescription := prompts.MustFill("tool-check-level.md", nil)
	vsDescription := prompts.MustFill("tool-check-vs.md", nil)
	modifierDescription := prompts.MustFill("tool-check-modifier.md", nil)
	whatDescription := prompts.MustFill("tool-check-what.md", nil)
	secretDescription := prompts.MustFill("tool-secret.md", nil)
	secretNameDescription := prompts.MustFill("tool-secret-name.md", nil)
	return []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{
			Name:        "check",
			Description: checkDescription,
			Parameters: map[string]any{"type": "object", "properties": map[string]any{
				"level":    map[string]any{"type": "string", "description": levelDescription},
				"vs":       map[string]any{"type": "string", "description": vsDescription},
				"modifier": map[string]any{"type": "integer", "description": modifierDescription},
				"what":     map[string]any{"type": "string", "description": whatDescription},
			}, "required": []string{"level", "vs", "what"}},
		}},
		{Type: "function", Function: llm.ToolFunction{
			Name:        "secret",
			Description: secretDescription,
			Parameters: map[string]any{"type": "object", "properties": map[string]any{
				"name": map[string]any{"type": "string", "description": secretNameDescription},
			}, "required": []string{"name"}},
		}},
		{Type: "function", Function: llm.ToolFunction{
			Name:        worldUpdateTool,
			Description: prompts.MustFill("tool-world-update.md", nil),
			Parameters: map[string]any{"type": "object", "properties": map[string]any{
				"sheet": map[string]any{"type": "string", "description": prompts.MustFill("tool-world-update-sheet.md", nil)},
			}, "required": []string{"sheet"}},
		}},
		userChoiceTool(),
	}
}

// worldUpdateTool is the other way a turn hands over its sheet, and for this
// model the only one it uses: offered the tool, nvidia/nemotron-3-ultra stopped
// writing the block in prose entirely. The block stays supported because a
// different model may do the opposite. Whichever arrives is the sheet.
const worldUpdateTool = "world_update"

// worldUpdateSheet returns the sheet a response handed over by tool call, if any.
func worldUpdateSheet(calls []llm.ToolCall) string {
	for _, call := range calls {
		if call.Function.Name != worldUpdateTool {
			continue
		}
		var args struct {
			Sheet string `json:"sheet"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			continue
		}
		return strings.TrimSpace(args.Sheet)
	}
	return ""
}

// secretsDir is where an adventure keeps what the DM is not told up front: one
// file per secret, beside the adventure and named after it. Each file opens
// The briefing lists the triggers and the names and nothing else: the text of a
// secret stays out of the context window until the DM asks for it by name, so a
// DM that never earns it can never leak it. See docs/adr/008.
func secretsDir(settingName, adventureFile string) string {
	return filepath.Join(settingsRoot(), settingName, "adventures", adventureFile+".secrets")
}

func loadSetting(s *session, name string) error {
	path := filepath.Join(settingsRoot(), name)
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

// loadWorld puts a named setting and adventure on the session, which is what
// the two setup menus do once they have an answer.
func loadWorld(s *session, setting, adventure string) error {
	if err := loadSetting(s, setting); err != nil {
		return err
	}
	content, err := loadAdventure(s, adventure)
	if err != nil {
		return err
	}
	s.adventure, s.adventureName = content, adventure
	return nil
}

func loadAdventure(s *session, name string) (string, error) {
	if s.settingName == "" {
		return "", fmt.Errorf("no setting loaded")
	}
	content, err := os.ReadFile(filepath.Join(settingsRoot(), s.settingName, "adventures", name+".md"))
	if err != nil {
		return "", fmt.Errorf("failed to load adventure: %w", err)
	}
	return string(content), nil
}

func settingsRoot() string {
	return filepath.Join("settings", prompts.Language())
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
	case worldUpdateTool:
		// The turn loop has already taken the sheet out of the call and written
		// it. All that is owed here is a result: a tool call needs one, and a
		// silent one reads as a failure.
		return "Sheet saved.", nil
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

	case "secret":
		// The name is a file name, so it is taken apart rather than trusted: a
		// model that answers with a path walks out of the adventure's directory.
		name := slugify(filepath.Base(str("name")))
		if name == "" {
			return "", fmt.Errorf("which secret?")
		}
		body, err := os.ReadFile(filepath.Join(secretsDir(s.settingName, s.adventureName), name+".md"))
		if err != nil {
			// Reported as text, not as an error: a wrong guess should send the
			// model back to the briefing's list, not stop the turn.
			return fmt.Sprintf("There is no secret called %q. Only the names in the briefing exist.", name), nil
		}
		return string(body), nil

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
		if err := os.MkdirAll(filepath.Join(settingsRoot(), name, "adventures"), 0755); err != nil {
			return "", err
		}
		s.settingName = name
		return fmt.Sprintf("Created %s/%s. Write description.md next.", settingsRoot(), name), nil

	case "write_setting_file":
		if s.settingName == "" {
			return "", fmt.Errorf("call create_setting first")
		}
		content := str("content")
		if err := os.WriteFile(filepath.Join(settingsRoot(), s.settingName, "description.md"), []byte(content), 0644); err != nil {
			return "", err
		}
		s.settingDesc = content
		return fmt.Sprintf("Wrote %s/%s/description.md.", settingsRoot(), s.settingName), nil

	case "write_adventure":
		if s.settingName == "" {
			return "", fmt.Errorf("no setting loaded")
		}
		title := strings.TrimSpace(str("title"))
		if title == "" {
			return "", fmt.Errorf("the adventure needs a title")
		}
		path := filepath.Join(settingsRoot(), s.settingName, "adventures", slugify(title)+".md")
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
