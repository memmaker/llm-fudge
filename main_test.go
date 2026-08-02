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

// playSession loads the prompt files from the repo root, then moves the test
// into a scratch directory. The order matters: prompts live beside the source,
// and everything a turn writes must not.
func playSession(t *testing.T) (*session, string) {
	t.Helper()
	loaded, err := loadGamePrompts()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Chdir(dir)
	s := newSession()
	s.prompts = loaded
	return s, dir
}

func TestTurnEndsOnProse(t *testing.T) {
	s, _ := playSession(t)
	messages := []llm.Message{{Role: llm.RoleSystem, Content: "ground rules"}}

	s.processGameTurn(replies(
		llm.Message{Content: "The door swings open."},
		llm.Message{Content: "NOTHING"},
	), &messages, "open the door")

	if s.lastNarration != "The door swings open." {
		t.Fatalf("lastNarration = %q", s.lastNarration)
	}
}

// Phase three: a narrated turn that wrote nothing gets a round of its own to put
// the scene on the sheet. Without it there is no moment in a turn at which the
// bookkeeping can happen, which is why it was not happening.
func TestNarratedTurnGetsABookkeepingRound(t *testing.T) {
	s, dir := playSession(t)
	s.dir = dir
	if _, err := s.writeFile("charsheet.md", "- Location: the hall\n"); err != nil {
		t.Fatal(err)
	}
	messages := []llm.Message{{Role: llm.RoleSystem, Content: "ground rules"}}

	s.processGameTurn(replies(
		llm.Message{Content: "You step through into the cellar."},
		llm.Message{ToolCalls: []llm.ToolCall{{
			ID: "1",
			Function: llm.ToolCallFunction{
				Name:      "edit_file",
				Arguments: `{"filename":"charsheet.md","old_string":"- Location: the hall","new_string":"- Location: the cellar"}`,
			},
		}}},
	), &messages, "go through the door")

	sheet, err := s.readFile("charsheet.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sheet, "the cellar") {
		t.Fatalf("the bookkeeping round did not reach the sheet: %q", sheet)
	}
	if !strings.Contains(strings.Join(contents(messages), "\n"), "the turn's narration is closed") {
		t.Fatal("the bookkeeping demand was never sent")
	}
}

// A DM that kept its own books mid-turn should not be asked again: the round
// costs a request, and it only exists because the writing usually does not
// happen on its own.
func TestTurnThatAlreadyWroteSkipsBookkeeping(t *testing.T) {
	s, dir := playSession(t)
	s.dir = dir
	if _, err := s.writeFile("charsheet.md", "- Location: the hall\n"); err != nil {
		t.Fatal(err)
	}
	messages := []llm.Message{{Role: llm.RoleSystem, Content: "ground rules"}}

	s.processGameTurn(replies(llm.Message{
		Content: "You step through into the cellar.",
		ToolCalls: []llm.ToolCall{{
			ID: "1",
			Function: llm.ToolCallFunction{
				Name:      "edit_file",
				Arguments: `{"filename":"charsheet.md","old_string":"- Location: the hall","new_string":"- Location: the cellar"}`,
			},
		}},
	}), &messages, "go through the door")

	if strings.Contains(strings.Join(contents(messages), "\n"), "the turn's narration is closed") {
		t.Fatal("a turn that already wrote was asked to write again")
	}
}

// Character creation happens before the session directory exists, so there is
// nothing for a bookkeeping round to write to and it must not cost a request.
func TestBookkeepingWaitsForTheSessionDirectory(t *testing.T) {
	s, _ := playSession(t)
	messages := []llm.Message{{Role: llm.RoleSystem, Content: "ground rules"}}

	s.processGameTurn(replies(llm.Message{Content: "What kind of person is Rob?"}), &messages, "")

	if strings.Contains(strings.Join(contents(messages), "\n"), "the turn's narration is closed") {
		t.Fatal("bookkeeping ran before there was a sheet to write")
	}
}

// Nothing in the bookkeeping round can roll dice or stop for the player: the
// scene is already told, and there is nobody left to ask.
func TestBookkeepingToolsAreFilesOnly(t *testing.T) {
	for _, tool := range bookkeepingTools() {
		switch tool.Function.Name {
		case "read_file", "write_file", "edit_file":
		default:
			t.Errorf("%s is available after the narration", tool.Function.Name)
		}
	}
	if len(bookkeepingTools()) != 3 {
		t.Fatalf("bookkeeping offers %d tools, want the three file tools", len(bookkeepingTools()))
	}
}

func TestProseWrittenAlongsideARollIsHeldBack(t *testing.T) {
	s, _ := playSession(t)
	messages := []llm.Message{{Role: llm.RoleSystem, Content: "ground rules"}}

	// The DM narrates the outcome and rolls for it in the same response. The
	// prose predates the dice, so it must not reach the player; the turn goes on
	// and the next round narrates from the result.
	s.processGameTurn(replies(
		llm.Message{
			Content: "You slip past the guard unseen.",
			ToolCalls: []llm.ToolCall{{
				ID:       "1",
				Function: llm.ToolCallFunction{Name: "check", Arguments: `{"level":"Fair","vs":"Average","what":"sneaking"}`},
			}},
		},
		llm.Message{Content: "The guard turns. You freeze."},
	), &messages, "sneak past")

	if s.lastNarration != "The guard turns. You freeze." {
		t.Fatalf("the player was shown prose written before the roll: %q", s.lastNarration)
	}
}

func TestUnnarratedTurnIsRolledBack(t *testing.T) {
	s, _ := playSession(t)
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
	s, dir := playSession(t)
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
	s, dir := playSession(t)
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
