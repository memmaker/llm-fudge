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
	if err := s.writeCharsheet("# Garret"); err != nil {
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
