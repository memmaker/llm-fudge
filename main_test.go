package main

import (
	"strings"
	"testing"

	"github.com/memmaker/gollum/llm"
)

func TestSplitImagePromptCutsTheBlockOut(t *testing.T) {
	prose, img := splitImagePrompt("The cellar is cold.\n\n<img>\nA low stone cellar.\n</img>\n")
	if prose != "The cellar is cold." {
		t.Fatalf("prose = %q", prose)
	}
	if img != "A low stone cellar." {
		t.Fatalf("img = %q", img)
	}
}

func TestSplitImagePromptLeavesPlainProseAlone(t *testing.T) {
	prose, img := splitImagePrompt("  The cellar is cold.  ")
	if prose != "The cellar is cold." || img != "" {
		t.Fatalf("prose = %q, img = %q", prose, img)
	}
}

func TestTranscriptCarriesToolCalls(t *testing.T) {
	got := transcript([]llm.Message{
		{Role: llm.RoleUser, Content: "I pick the lock"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{Function: llm.ToolCallFunction{Name: "check", Arguments: `{"level":"Good"}`}},
		}},
	})
	for _, want := range []string{"I pick the lock", "check", `{"level":"Good"}`} {
		if !strings.Contains(got, want) {
			t.Fatalf("transcript missing %q:\n%s", want, got)
		}
	}
}
