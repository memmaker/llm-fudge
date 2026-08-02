package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fudge/prompts"
)

// Every prompt the code names must exist and must come out fully filled. A
// missing file or a placeholder nobody supplies is the failure this extraction
// introduced, and it would otherwise surface mid-turn against a live model.
func TestEveryNamedPromptLoadsFilled(t *testing.T) {
	cases := []struct {
		file string
		vars map[string]string
	}{
		{"turn-narration-demand.md", nil},
		{"turn-empty-reply.md", nil},
		{"charsheet-header.md", nil},
		{"sheet-nag.md", nil},
		{"style-override.md", nil},
		{"image-request.md", nil},
		{"adventure-retry.md", nil},
		{"adventure-briefing.md", map[string]string{"ADVENTURE": "x"}},
		{"opening-briefing.md", map[string]string{"ADVENTURE_BRIEFING": "x"}},
		{"milestone-demand.md", map[string]string{"MILESTONE_FILE": "milestone-01.md"}},
		{"setting-concept-request.md", map[string]string{"CONCEPT": "x"}},
		{"setting-creation-request.md", map[string]string{"CONCEPT": "x", "DESIGN": "y"}},
		{"adventure-request.md", map[string]string{"SETTING": "x", "SCENARIO": "y"}},
	}

	for _, c := range cases {
		text, err := prompts.Fill(c.file, c.vars)
		if err != nil {
			t.Errorf("%s: %v", c.file, err)
			continue
		}
		if strings.TrimSpace(text) == "" {
			t.Errorf("%s is empty", c.file)
		}
	}
}

func TestUnfilledPlaceholderIsRefused(t *testing.T) {
	if _, err := prompts.Fill("adventure-briefing.md", nil); err == nil {
		t.Fatal("a prompt with an unfilled placeholder was returned")
	}
}

// The prompt directory is resolved at init, so the game works from any working
// directory rather than only from the repo root.
func TestPromptsSurviveAChdir(t *testing.T) {
	before, err := prompts.Fill("charsheet-header.md", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	after, err := prompts.Fill("charsheet-header.md", nil)
	if err != nil {
		t.Fatalf("prompt unreadable after chdir: %v", err)
	}
	if before != after {
		t.Fatal("chdir changed which prompt file was read")
	}
}

// Nothing in system-prompts/ should be orphaned: a prompt file no code names is
// either dead or a wiring mistake.
func TestNoOrphanedPromptFiles(t *testing.T) {
	entries, err := os.ReadDir(prompts.Path(""))
	if err != nil {
		t.Fatal(err)
	}
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var code strings.Builder
	for _, src := range sources {
		body, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		code.Write(body)
	}

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") {
			continue
		}
		// always.md and language.md are wrapped in by LoadPrompt itself.
		if name == "always.md" || name == "language.md" {
			continue
		}
		if !strings.Contains(code.String(), name) {
			t.Errorf("%s is named by no Go source — dead prompt, or wiring that never landed", name)
		}
	}
}
