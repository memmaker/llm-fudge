package main

import (
	"os"
	"strings"
	"testing"

	"fudge/rules"

	"github.com/memmaker/gollum/llm"
)

// requestImagePrompt is /image: the same guidance -img-gen bakes into every
// turn, asked for once, on demand, so a picture costs a request only when
// somebody actually wants one.

func TestImagePromptRefusesBeforeAnyTurnHasBeenPlayed(t *testing.T) {
	w, err := rules.NewWorld(1, "settings/deadlands")
	if err != nil {
		t.Fatal(err)
	}
	s := newSession(w)

	called := false
	chat := func(messages []llm.Message, tools []llm.Tool) (llm.Response, error) {
		called = true
		return llm.Response{}, nil
	}

	if err := s.requestImagePrompt(chat); err == nil {
		t.Fatal("expected an error before any scene has been rendered")
	}
	if called {
		t.Error("the model was asked before there was a scene to describe")
	}
}

// The call is its own request, not a reply appended to the ongoing
// conversation: the DM's own history carries tool calls and results an image
// prompt has no business seeing, so it gets a system message and the current
// block and nothing else.
func TestImagePromptSendsOnlyTheCurrentBlock(t *testing.T) {
	w, err := rules.NewWorld(1, "settings/deadlands")
	if err != nil {
		t.Fatal(err)
	}
	s := newSession(w)
	s.lastBlock = "entities:\n  rob:\n    type: character\n"

	var seen []llm.Message
	chat := func(messages []llm.Message, tools []llm.Tool) (llm.Response, error) {
		seen = messages
		return llm.Response{Message: llm.Message{Content: "A gaunt man in a dust-choked street."}}, nil
	}

	if err := s.requestImagePrompt(chat); err != nil {
		t.Fatal(err)
	}

	if len(seen) != 2 {
		t.Fatalf("sent %d messages, want exactly a system prompt and the block", len(seen))
	}
	if seen[0].Role != llm.RoleSystem {
		t.Errorf("first message is %s, want system", seen[0].Role)
	}
	if !strings.Contains(seen[1].Content, "rob:") {
		t.Errorf("the current block was not what got sent:\n%s", seen[1].Content)
	}
}

// The reply is the prompt, verbatim — no wrapper for the caller to strip, the
// way the <img> tag wraps the every-turn version.
func TestImagePromptWritesTheReplyVerbatim(t *testing.T) {
	w, err := rules.NewWorld(1, "settings/deadlands")
	if err != nil {
		t.Fatal(err)
	}
	s := newSession(w)
	s.lastBlock = "entities: {}"

	chat := func(messages []llm.Message, tools []llm.Tool) (llm.Response, error) {
		return llm.Response{Message: llm.Message{Content: "  A lone figure under a red moon.  \n"}}, nil
	}

	if err := s.requestImagePrompt(chat); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writeImagePrompt("") }()

	got, err := os.ReadFile("img-gen.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "A lone figure under a red moon." {
		t.Errorf("img-gen.md = %q, want the trimmed reply", got)
	}
}

// A model that answers with nothing has not written a usable prompt, and the
// file is better left alone than overwritten with an empty one.
func TestImagePromptRejectsAnEmptyReply(t *testing.T) {
	w, err := rules.NewWorld(1, "settings/deadlands")
	if err != nil {
		t.Fatal(err)
	}
	s := newSession(w)
	s.lastBlock = "entities: {}"

	chat := func(messages []llm.Message, tools []llm.Tool) (llm.Response, error) {
		return llm.Response{Message: llm.Message{Content: "   "}}, nil
	}

	if err := s.requestImagePrompt(chat); err == nil {
		t.Fatal("expected an error for an empty reply")
	}
}
