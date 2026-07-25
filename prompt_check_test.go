package main

import (
	"strings"
	"testing"

	"fudge/rules"
)

// The DM prompt is assembled from two files when image generation is on. Each
// used to be wrapped in always.md and the setting separately, duplicating ~3k
// tokens of rules reference in every request.
func TestDMPromptWrapsOnce(t *testing.T) {
	w := &rules.World{Setting: &rules.SettingInfos{Slug: "deadlands", Path: "settings/deadlands"}}

	defer func(prev bool) { imgGen = prev }(imgGen)
	imgGen = true

	prompt, err := loadDMPrompt(w)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"UNIVERSAL OUTPUT CONSTRAINTS", "# Fudge Rules", "## Setting Information:"} {
		if n := strings.Count(prompt, marker); n != 1 {
			t.Errorf("%q appears %d times, want 1", marker, n)
		}
	}
	if !strings.Contains(prompt, "## Image Generation Rules") {
		t.Error("img-style.md missing from prompt")
	}
}
