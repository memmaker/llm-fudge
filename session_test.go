package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/memmaker/gollum/llm"
)

func newTestSession(t *testing.T) *session {
	t.Helper()
	root := t.TempDir()
	s := newSession()
	loaded, err := loadGamePrompts()
	if err != nil {
		t.Fatal(err)
	}
	s.prompts = loaded
	s.dir = filepath.Join(root, "0001-garret-dryfall")
	if err := os.MkdirAll(s.dir, 0755); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestFileToolsRefuseEscapingTheSessionDirectory(t *testing.T) {
	s := newTestSession(t)
	for _, name := range []string{
		"../escape.md",
		"sub/../../escape.md",
		"/etc/passwd",
		"..",
		"",
	} {
		if _, err := s.resolve(name); err == nil {
			t.Fatalf("%q resolved instead of being refused", name)
		}
	}
}

func TestFileToolsRefuseSubdirectories(t *testing.T) {
	// Bare filenames only — a path is how a typo becomes a write somewhere else.
	s := newTestSession(t)
	if _, err := s.resolve("notes/plan.md"); err == nil {
		t.Fatal("a path with a directory was accepted")
	}
}

func TestWritesAreLimitedToTheWhitelist(t *testing.T) {
	s := newTestSession(t)
	if _, err := s.writeFile("charsheet.md", "# Garret"); err != nil {
		t.Fatalf("charsheet.md is always writable: %v", err)
	}
	if _, err := s.writeFile("notes.md", "anything"); err == nil {
		t.Fatal("a file outside the whitelist was written")
	}
	s.allowWrite("milestone-01.md")
	if _, err := s.writeFile("milestone-01.md", "# The bridge"); err != nil {
		t.Fatalf("authorised milestone file refused: %v", err)
	}
}

func TestReadsAreOpenWithinTheDirectory(t *testing.T) {
	s := newTestSession(t)
	if err := os.WriteFile(filepath.Join(s.dir, "milestone-01-raw.md"), []byte("raw chat"), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := s.readFile("milestone-01-raw.md")
	if err != nil {
		t.Fatalf("reading a non-writable file in the directory failed: %v", err)
	}
	if got != "raw chat" {
		t.Fatalf("readFile = %q", got)
	}
}

func TestTemplateIsReadableFromTheRepoRoot(t *testing.T) {
	s := newTestSession(t)
	got, err := s.readFile(charsheetTemplate)
	if err != nil {
		t.Fatalf("template not readable: %v", err)
	}
	if !strings.Contains(got, "## Identity") {
		t.Fatalf("template content unexpected: %.60q", got)
	}
	if _, err := s.writeFile(charsheetTemplate, "clobbered"); err == nil {
		t.Fatal("the template is writable")
	}
}

func TestEditReplacesExactlyOnce(t *testing.T) {
	s := newTestSession(t)
	if _, err := s.writeFile("charsheet.md", "- Silver: 2\n- Torches: 5\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.editFile("charsheet.md", "- Silver: 2", "- Silver: 7", false); err != nil {
		t.Fatal(err)
	}
	got, _ := s.readFile("charsheet.md")
	if !strings.Contains(got, "- Silver: 7") || strings.Contains(got, "- Silver: 2") {
		t.Fatalf("edit did not apply: %q", got)
	}
}

func TestEditRefusesMissingAndAmbiguousMatches(t *testing.T) {
	s := newTestSession(t)
	if _, err := s.writeFile("charsheet.md", "hurt\nhurt\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.editFile("charsheet.md", "absent", "x", false); err == nil {
		t.Fatal("missing oldString accepted")
	}
	_, err := s.editFile("charsheet.md", "hurt", "fine", false)
	if err == nil {
		t.Fatal("ambiguous oldString accepted")
	}
	if !strings.Contains(err.Error(), "replace_all") {
		t.Fatalf("ambiguity error does not say how to fix it: %v", err)
	}
	if _, err := s.editFile("charsheet.md", "hurt", "fine", true); err != nil {
		t.Fatalf("replace_all refused: %v", err)
	}
	got, _ := s.readFile("charsheet.md")
	if got != "fine\nfine\n" {
		t.Fatalf("replace_all result = %q", got)
	}
}

func TestEditRefusesNoOpReplacement(t *testing.T) {
	s := newTestSession(t)
	if _, err := s.writeFile("charsheet.md", "same"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.editFile("charsheet.md", "same", "same", false); err == nil {
		t.Fatal("newString identical to oldString accepted")
	}
}

func TestWriteRefusesBlindOverwrite(t *testing.T) {
	s := newTestSession(t)
	// A file that exists but was never read this session: overwriting it would
	// destroy content nobody in this session has seen.
	if err := os.WriteFile(filepath.Join(s.dir, "charsheet.md"), []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.writeFile("charsheet.md", "replacement"); err == nil {
		t.Fatal("blind overwrite accepted")
	}
	if _, err := s.readFile("charsheet.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.writeFile("charsheet.md", "replacement"); err != nil {
		t.Fatalf("overwrite after read refused: %v", err)
	}
}

func TestSessionDirectoryNameIsSlugAndCounter(t *testing.T) {
	root := t.TempDir()
	s := newSession()
	first, err := s.createDir(root, "Garret the Sly", "Dryfall Bridge")
	if err != nil {
		t.Fatal(err)
	}
	if base := filepath.Base(first); base != "0001-garret-the-sly-dryfall-bridge" {
		t.Fatalf("directory name = %q", base)
	}
	second, err := newSession().createDir(root, "Mira", "Dryfall Bridge")
	if err != nil {
		t.Fatal(err)
	}
	if base := filepath.Base(second); base != "0002-mira-dryfall-bridge" {
		t.Fatalf("second directory name = %q", base)
	}
}

func TestCharsheetIsPinnedOnceAtTheTail(t *testing.T) {
	s := newTestSession(t)
	if _, err := s.writeFile("charsheet.md", "# Garret"); err != nil {
		t.Fatal(err)
	}
	msgs := []llm.Message{
		{Role: llm.RoleSystem, Content: "ground rules"},
		{Role: llm.RoleUser, Content: "I open the door"},
	}

	req := s.withCharsheet(msgs)
	if len(req) != len(msgs)+1 {
		t.Fatalf("withCharsheet added %d messages", len(req)-len(msgs))
	}
	last := req[len(req)-1]
	if !strings.Contains(last.Content, "# Garret") {
		t.Fatalf("charsheet is not the last message: %q", last.Content)
	}

	// Called twice, the charsheet still appears exactly once — the block is
	// assembled for the request, never stored back into the history.
	req = s.withCharsheet(msgs)
	if n := strings.Count(strings.Join(contents(req), "\n"), "# Garret"); n != 1 {
		t.Fatalf("charsheet appears %d times", n)
	}
	if len(msgs) != 2 {
		t.Fatal("withCharsheet mutated the caller's messages")
	}
}

func TestNoCharsheetYetLeavesTheRequestAlone(t *testing.T) {
	s := newTestSession(t)
	msgs := []llm.Message{{Role: llm.RoleSystem, Content: "ground rules"}}
	if got := s.withCharsheet(msgs); len(got) != 1 {
		t.Fatalf("a missing charsheet added %d messages", len(got)-1)
	}
}

func contents(msgs []llm.Message) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.Content
	}
	return out
}
