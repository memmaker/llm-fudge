package main

import (
	"strings"
	"testing"

	"fudge/rules"
	llm "github.com/memmaker/gollum/llm"
)

func TestExecuteActionUsesWorldChecks(t *testing.T) {
	w := rules.NewWorld(1)
	hero := w.CreateCharacter("hero", "Hero", nil, map[string]int{"athletics": 2})
	goblin := w.CreateCharacter("goblin", "Goblin", nil, map[string]int{"athletics": 1})

	result, err := executeAction(w, hero.ID, goblin.ID, llm.ToolCall{Function: llm.ToolCallFunction{
		Name:      "unopposed_skill_check",
		Arguments: `{"skill":"Athletics","difficulty":1}`,
	}})
	if err != nil {
		t.Fatalf("execute action: %v", err)
	}
	if !strings.Contains(result, "Hero rolled") {
		t.Fatalf("unexpected result: %q", result)
	}
}

func TestActionToolsOnlyExposeDeterministicChecks(t *testing.T) {
	tools := actionTools()
	if len(tools) != 2 {
		t.Fatalf("got %d tools, want 2", len(tools))
	}
	if tools[0].Function.Name != "unopposed_skill_check" || tools[1].Function.Name != "opposed_skill_check" {
		t.Fatalf("unexpected tools: %#v", tools)
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
