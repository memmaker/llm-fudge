package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fudge/rules"
	llm "github.com/memmaker/gollum/llm"
)

func TestExecuteActionUsesWorldChecks(t *testing.T) {
	w, err := rules.NewWorld(1, "rulesets/example")
	if err != nil {
		t.Fatalf("create world: %v", err)
	}
	hero := w.CreateCharacter(
		"hero",
		"Hero",
		map[string]int{"strength": 1, "agility": 1, "perception": 0},
		map[string]int{"athletics": 2, "stealth": 1, "fight": 1},
	)
	goblin := w.CreateCharacter(
		"goblin",
		"Goblin",
		map[string]int{"strength": 0, "agility": 1, "perception": 0},
		map[string]int{"athletics": 1, "stealth": 0, "fight": 1},
	)

	result, err := executeAction(w, hero.ID, goblin.ID, llm.ToolCall{Function: llm.ToolCallFunction{
		Name:      "unopposed_skill_check",
		Arguments: `{"skill":"athletics","difficulty":1}`,
	}})
	if err != nil {
		t.Fatalf("execute action: %v", err)
	}
	if !strings.Contains(result, "Hero rolled") {
		t.Fatalf("unexpected result: %q", result)
	}
}

func TestActionToolsIncludeCreateCharacter(t *testing.T) {
	tools := actionTools()
	if len(tools) != 4 {
		t.Fatalf("got %d tools, want 4", len(tools))
	}
	found := false
	for _, tool := range tools {
		if tool.Function.Name == "create_character" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("create_character tool not found in action tools")
	}
}

func TestExecuteActionWritesRulesetFile(t *testing.T) {
	path := t.TempDir()
	writeFile(t, filepath.Join(path, "attributes.rec"), "slug: strength\ndisplay_name: Strength\ndescription: Raw might\n")
	writeFile(t, filepath.Join(path, "skills.rec"), "slug: athletics\ndisplay_name: Athletics\ndescription: Move quickly\n")
	writeFile(t, filepath.Join(path, "species.rec"), "slug: human\ndisplay_name: Human\ndescription: Human test\nscale: 0\n")
	w, err := rules.NewWorld(3, path)
	if err != nil {
		t.Fatalf("create world: %v", err)
	}
	result, err := executeAction(w, "", "", llm.ToolCall{Function: llm.ToolCallFunction{
		Name:      "write_ruleset_file",
		Arguments: `{"filename":"skills.rec","mode":"replace","content":"slug: athletics\ndisplay_name: Athletics\ndescription: Move quickly\n"}`,
	}})
	if err != nil {
		t.Fatalf("execute action: %v", err)
	}
	if !strings.Contains(result, "wrote skills.rec") {
		t.Fatalf("unexpected result: %q", result)
	}
	loaded, err := rules.LoadRuleset(path)
	if err != nil {
		t.Fatalf("reload ruleset: %v", err)
	}
	if _, ok := loaded.Skills["athletics"]; !ok {
		t.Fatalf("expected athletics skill after write")
	}
}

func TestExecuteActionCreateCharacter(t *testing.T) {
	path := t.TempDir()
	writeFile(t, filepath.Join(path, "attributes.rec"), "slug: testattr\ndisplay_name: TestAttr\ndescription: A test attribute\n")
	writeFile(t, filepath.Join(path, "skills.rec"), "slug: testskill\ndisplay_name: TestSkill\ndescription: A test skill\n")
	writeFile(t, filepath.Join(path, "species.rec"), "slug: human\ndisplay_name: Human\ndescription: Test human\nscale: 0\n")
	w, err := rules.NewWorld(5, path)
	if err != nil {
		t.Fatalf("create world: %v", err)
	}
	args := `{"slug":"hero","name":"Hero","species":"human","description":"desc","location":"start","attributes":{"testattr":1},"skills":{"testskill":2}}`
	result, err := executeAction(w, "", "", llm.ToolCall{Function: llm.ToolCallFunction{Name: "create_character", Arguments: args}})
	if err != nil {
		t.Fatalf("executeAction create_character: %v", err)
	}
	if !strings.Contains(result, "created character hero") {
		t.Fatalf("unexpected result: %q", result)
	}
	ch, err := w.Character("hero")
	if err != nil {
		t.Fatalf("character not found: %v", err)
	}
	if ch.Name != "Hero" || ch.Description != "desc" || ch.Species != "human" || ch.Location != "start" {
		t.Fatalf("unexpected character fields: %+v", ch)
	}
	if ch.Attributes["testattr"] != 1 {
		t.Fatalf("attribute not set: %v", ch.Attributes)
	}
	if ch.Skills["testskill"] != 2 {
		t.Fatalf("skill not set: %v", ch.Skills)
	}
}

func TestCreateCharacterFreezesRulesetOnSuccess(t *testing.T) {
	path := t.TempDir()
	writeFile(t, filepath.Join(path, "attributes.rec"), "slug: strength\ndisplay_name: Strength\n")
	writeFile(t, filepath.Join(path, "skills.rec"), "slug: fight\ndisplay_name: Fight\n")
	writeFile(t, filepath.Join(path, "species.rec"), "slug: human\ndisplay_name: Human\nscale: 0\n")
	w, err := rules.NewWorld(7, path)
	if err != nil {
		t.Fatalf("create world: %v", err)
	}
	if w.Ruleset.Frozen {
		t.Fatal("ruleset should not be frozen initially")
	}

	args := `{"name":"Test","species":"human"}`
	_, err = executeAction(w, "", "", llm.ToolCall{Function: llm.ToolCallFunction{Name: "create_character", Arguments: args}})
	if err != nil {
		t.Fatalf("character creation failed: %v", err)
	}
	if !w.Ruleset.Frozen {
		t.Fatal("ruleset should be frozen after creating character")
	}
}

func TestCreateCharacterRejectsInvalidSpecies(t *testing.T) {
	path := t.TempDir()
	writeFile(t, filepath.Join(path, "attributes.rec"), "slug: strength\n")
	writeFile(t, filepath.Join(path, "skills.rec"), "slug: fight\n")
	writeFile(t, filepath.Join(path, "species.rec"), "slug: human\ndisplay_name: Human\nscale: 0\n")
	w, err := rules.NewWorld(8, path)
	if err != nil {
		t.Fatalf("create world: %v", err)
	}

	args := `{"name":"Test","species":"invalid_species"}`
	result, err := executeAction(w, "", "", llm.ToolCall{Function: llm.ToolCallFunction{Name: "create_character", Arguments: args}})
	if err == nil {
		t.Fatalf("expected error for invalid species, got result: %q", result)
	}
	if !strings.Contains(err.Error(), "species") && !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateCharacterDefaultsAttributesToFairSkillsToPoor(t *testing.T) {
	path := t.TempDir()
	writeFile(t, filepath.Join(path, "attributes.rec"), "slug: strength\ndisplay_name: Strength\n\nslug: agility\ndisplay_name: Agility\n")
	writeFile(t, filepath.Join(path, "skills.rec"), "slug: fight\ndisplay_name: Fight\n\nslug: dodge\ndisplay_name: Dodge\n")
	writeFile(t, filepath.Join(path, "species.rec"), "slug: human\ndisplay_name: Human\nscale: 0\n")
	w, err := rules.NewWorld(9, path)
	if err != nil {
		t.Fatalf("create world: %v", err)
	}

	args := `{"name":"Test","species":"human"}`
	_, err = executeAction(w, "", "", llm.ToolCall{Function: llm.ToolCallFunction{Name: "create_character", Arguments: args}})
	if err != nil {
		t.Fatalf("character creation failed: %v", err)
	}

	ch, err := w.Character("test")
	if err != nil {
		t.Fatalf("character not found: %v", err)
	}

	for attr, val := range ch.Attributes {
		if val != 0 {
			t.Fatalf("attribute %s should default to 0 (Fair), got %d", attr, val)
		}
	}

	for skill, val := range ch.Skills {
		if val != -2 {
			t.Fatalf("skill %s should default to -2 (Poor), got %d", skill, val)
		}
	}
}

func TestCreateCharacterDesignatesPlayer(t *testing.T) {
	path := t.TempDir()
	writeFile(t, filepath.Join(path, "attributes.rec"), "slug: strength\n")
	writeFile(t, filepath.Join(path, "skills.rec"), "slug: fight\n")
	writeFile(t, filepath.Join(path, "species.rec"), "slug: human\ndisplay_name: Human\nscale: 0\n")
	w, err := rules.NewWorld(10, path)
	if err != nil {
		t.Fatalf("create world: %v", err)
	}

	args := `{"slug":"pc1","name":"PlayerChar","species":"human"}`
	_, err = executeAction(w, "", "", llm.ToolCall{Function: llm.ToolCallFunction{Name: "create_character", Arguments: args}})
	if err != nil {
		t.Fatalf("character creation failed: %v", err)
	}

	if w.Player != "pc1" {
		t.Fatalf("player should be designated as pc1, got %q", w.Player)
	}
}

func TestCreateCharacterRejectsDuplicateSlug(t *testing.T) {
	path := t.TempDir()
	writeFile(t, filepath.Join(path, "attributes.rec"), "slug: strength\n")
	writeFile(t, filepath.Join(path, "skills.rec"), "slug: fight\n")
	writeFile(t, filepath.Join(path, "species.rec"), "slug: human\ndisplay_name: Human\nscale: 0\n")
	w, err := rules.NewWorld(11, path)
	if err != nil {
		t.Fatalf("create world: %v", err)
	}

	args1 := `{"slug":"hero","name":"Hero","species":"human"}`
	_, err = executeAction(w, "", "", llm.ToolCall{Function: llm.ToolCallFunction{Name: "create_character", Arguments: args1}})
	if err != nil {
		t.Fatalf("first character creation failed: %v", err)
	}

	// After first character, ruleset is frozen and no more characters can be created
	args2 := `{"slug":"hero2","name":"AnotherHero","species":"human"}`
	result, err := executeAction(w, "", "", llm.ToolCall{Function: llm.ToolCallFunction{Name: "create_character", Arguments: args2}})
	if err == nil {
		t.Fatalf("expected error when ruleset is frozen, got result: %q", result)
	}
	if !strings.Contains(err.Error(), "frozen") {
		t.Fatalf("unexpected error (expected 'frozen'): %v", err)
	}
}

// writeFile is a test helper to write files
func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestWrapTextRespectsWidthAndParagraphs(t *testing.T) {
	wrapped := wrapText("one two three four\n\nfive", 7)
	lines := strings.Split(wrapped, "\n")
	if !strings.Contains(wrapped, "\n\n") {
		t.Fatalf("paragraph break was removed: %q", wrapped)
	}
	for _, line := range lines {
		if len([]rune(line)) > 7 {
			t.Fatalf("line exceeds width: %q", line)
		}
	}
}
