package main

import (
	"strings"
	"testing"

	"github.com/memmaker/gollum/llm"
)

// A second /style must replace the first, not stack on it, and clearing must
// leave the prompt byte-identical to what it was before any /style.
func TestSetStyleReplacesAndClears(t *testing.T) {
	base := "DM prompt"
	messages := []llm.Message{{Role: llm.RoleSystem, Content: base}}

	setStyle(&messages, "Be brief.")
	if !strings.Contains(messages[0].Content, "Be brief.") {
		t.Fatalf("style not applied: %q", messages[0].Content)
	}

	setStyle(&messages, "Be terse.")
	if strings.Contains(messages[0].Content, "Be brief.") {
		t.Fatalf("previous style survived: %q", messages[0].Content)
	}

	setStyle(&messages, "")
	if messages[0].Content != base {
		t.Fatalf("clear left residue: %q", messages[0].Content)
	}
}
