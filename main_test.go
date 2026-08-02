package main

import (
	"os"
	"path/filepath"
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

// replies drives the turn loop without a model: one canned response per round.
func replies(responses ...llm.Message) chatRound {
	i := 0
	return func([]llm.Message, []llm.Tool) (llm.Response, error) {
		msg := llm.Message{Role: llm.RoleAssistant}
		if i < len(responses) {
			msg = responses[i]
		}
		i++
		msg.Role = llm.RoleAssistant
		return llm.Response{Message: msg}, nil
	}
}

func TestTurnEndsOnProse(t *testing.T) {
	t.Chdir(t.TempDir())
	s := newSession()
	messages := []llm.Message{{Role: llm.RoleSystem, Content: "ground rules"}}

	s.processGameTurn(replies(llm.Message{Content: "The door swings open."}), &messages, "open the door")

	if len(messages) != 3 {
		t.Fatalf("history holds %d messages, want system + player + reply", len(messages))
	}
	if s.lastNarration != "The door swings open." {
		t.Fatalf("lastNarration = %q", s.lastNarration)
	}
}

func TestUnnarratedTurnIsRolledBack(t *testing.T) {
	t.Chdir(t.TempDir())
	s := newSession()
	messages := []llm.Message{{Role: llm.RoleSystem, Content: "ground rules"}}

	// Every round comes back empty, so the turn spends its whole budget and
	// never reaches the player. What it added must not survive.
	s.processGameTurn(replies(), &messages, "open the door")

	if len(messages) != 1 {
		t.Fatalf("a turn the player never saw left %d messages behind", len(messages)-1)
	}
	if s.lastNarration != "" {
		t.Fatal("an unnarrated turn set lastNarration")
	}
}

func TestMilestoneKeepsTheAdventureAndTheLastScene(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	s := newSession()
	s.dir = dir
	s.adventure = "The bridge at Dryfall has burned."
	s.lastNarration = "Smoke still hangs over the water."

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: "ground rules"},
		{Role: llm.RoleUser, Content: "opening briefing with the adventure in it"},
		{Role: llm.RoleUser, Content: "I look around"},
	}

	if err := s.milestone(replies(llm.Message{Content: "Noted."}), &messages); err != nil {
		t.Fatal(err)
	}

	joined := strings.Join(contents(messages), "\n")
	if !strings.Contains(joined, "The bridge at Dryfall has burned.") {
		t.Fatalf("the adventure did not survive the milestone:\n%s", joined)
	}
	if !strings.Contains(joined, "Smoke still hangs over the water.") {
		t.Fatal("the chat did not reopen on the last narration")
	}
	if !strings.Contains(joined, "I look around") {
		return // the played chat is meant to be cleared
	}
	t.Fatal("the milestone left the old conversation in place")
}

func TestMilestoneArchivesTheRawChatAndOpensTheFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	s := newSession()
	s.dir = dir
	messages := []llm.Message{{Role: llm.RoleSystem, Content: "ground rules"}}

	if err := s.milestone(replies(llm.Message{Content: "Noted."}), &messages); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "milestone-01-raw.md")); err != nil {
		t.Fatalf("raw chat not archived: %v", err)
	}
	// The summary file the DM was told to write must be writable now, and only
	// because the harness named it.
	if _, err := s.writeFile("milestone-01.md", "# The bridge"); err != nil {
		t.Fatalf("milestone file was not authorised: %v", err)
	}
	if _, err := s.writeFile("milestone-02.md", "unasked for"); err == nil {
		t.Fatal("a milestone file nobody named was writable")
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
