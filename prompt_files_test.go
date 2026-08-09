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
		{"charsheet-header.md", nil},
		{"charsheet-none.md", nil},
		{"note-missing-block.md", nil},
		{"note-block-form.md", nil},
		{"style-override.md", nil},
		{"image-request.md", map[string]string{"GUIDANCE": "guidance", "SETTING": "setting"}},
		{"setting-creation.md", nil},
		{"setting-concept.md", map[string]string{"DESCRIPTION": "x"}},
		{"adventure-creation.md", nil},
		{"adventure-scenario.md", map[string]string{"SETTING": "x", "SCENARIO": "y"}},
		{"adventure-save-reminder.md", nil},
		{"scene-demand.md", nil},
		{"tool-user-choice.md", nil},
		{"tool-user-choice-option.md", nil},
		{"tool-create-setting.md", nil},
		{"tool-create-setting-name.md", nil},
		{"tool-write-setting.md", nil},
		{"tool-write-setting-content.md", nil},
		{"tool-write-adventure.md", nil},
		{"tool-write-adventure-title.md", nil},
		{"tool-write-adventure-content.md", nil},
		{"tool-check.md", nil},
		{"tool-check-level.md", nil},
		{"tool-check-vs.md", nil},
		{"tool-check-modifier.md", nil},
		{"tool-check-what.md", nil},
		{"tool-secret.md", nil},
		{"tool-secret-name.md", nil},
		{"tool-world-update.md", nil},
		{"tool-world-update-sheet.md", nil},
		{"adventure-briefing.md", map[string]string{"ADVENTURE": "x"}},
		{"opening-briefing.md", map[string]string{"ADVENTURE_BRIEFING": "x"}},
		{"dm.md", nil},
		{"setting-information.md", map[string]string{"SETTING": "x"}},
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

func TestPromptLanguages(t *testing.T) {
	for _, language := range []string{"en", "de"} {
		if err := prompts.SetLanguage(language); err != nil {
			t.Fatal(err)
		}
		text, err := prompts.Fill("scene-demand.md", nil)
		if err != nil {
			t.Fatalf("%s: %v", language, err)
		}
		if strings.TrimSpace(text) == "" {
			t.Fatalf("%s scene demand is empty", language)
		}
	}
	if err := prompts.SetLanguage("en"); err != nil {
		t.Fatal(err)
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
		if !strings.Contains(code.String(), name) {
			t.Errorf("%s is named by no Go source — dead prompt, or wiring that never landed", name)
		}
	}
}
