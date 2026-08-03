package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/memmaker/gollum/llm"
)

func TestSplitReplyCutsTheImageBlockOut(t *testing.T) {
	told := splitReply("The cellar is cold.\n\n<img>\nA low stone cellar.\n</img>\n")
	if told.prose != "The cellar is cold." {
		t.Fatalf("prose = %q", told.prose)
	}
	if told.img != "A low stone cellar." {
		t.Fatalf("img = %q", told.img)
	}
	if told.sheet != "" {
		t.Fatalf("a reply with no block produced a sheet: %q", told.sheet)
	}
}

func TestSplitReplyLeavesPlainProseAlone(t *testing.T) {
	told := splitReply("  The cellar is cold.  ")
	if told.prose != "The cellar is cold." || told.img != "" || told.sheet != "" || told.tolerated {
		t.Fatalf("%+v", told)
	}
}

// Recognition is generous: whatever the model reached for, the harness would
// rather write the sheet than lose it. What it must never do is leave any of it
// in the prose, since the prose is the whole of what the player reads.
func TestSplitReplyRecognisesTheBlockInEveryToleratedForm(t *testing.T) {
	for _, form := range []string{
		"<world-update>\n# Garret\n</world-update>",
		"<world_update>\n# Garret\n</world_update>",
		"<WORLD-UPDATE>\n# Garret\n</WORLD-UPDATE>",
		"<World Update>\n# Garret\n</World Update>",
		"```xml\n<world-update>\n# Garret\n</world-update>\n```",
		"<world-update>\n```markdown\n# Garret\n```\n</world-update>",
	} {
		told := splitReply("The cellar is cold.\n\n" + form)
		if told.sheet != "# Garret" {
			t.Errorf("%q: sheet = %q", form, told.sheet)
		}
		if told.prose != "The cellar is cold." {
			t.Errorf("%q: prose = %q", form, told.prose)
		}
	}
}

// The canonical form is the only one that passes silently; anything else the
// harness had to be generous about earns a note on the next player line.
func TestSplitReplyFlagsToleratedForms(t *testing.T) {
	if splitReply("Cold.\n<world-update>\n# Garret\n</world-update>").tolerated {
		t.Error("the canonical form was flagged")
	}
	for _, form := range []string{
		"<World_Update>\n# Garret\n</World_Update>",
		"```\n<world-update>\n# Garret\n</world-update>\n```",
	} {
		if !splitReply("Cold.\n" + form).tolerated {
			t.Errorf("%q was accepted without a note", form)
		}
	}
}

// A DM that restates or corrects itself emits two blocks. The last is the one it
// meant, and neither may survive into the prose: a block left behind would be
// printed to the player as a raw character sheet.
func TestSplitReplyTakesTheLastOfSeveralBlocks(t *testing.T) {
	told := splitReply("The cellar is cold.\n\n<world-update>\n- Location: the hall\n</world-update>\n\nSorry, corrected:\n\n<world-update>\n- Location: the cellar\n</world-update>")
	if told.sheet != "- Location: the cellar" {
		t.Fatalf("sheet = %q", told.sheet)
	}
	if strings.Contains(told.prose, "world-update") || strings.Contains(told.prose, "Location") {
		t.Fatalf("a block reached the player: %q", told.prose)
	}
}

// Acceptance is strict, and for one reason: a truncated block would write a
// truncated sheet, and the sheet is the only world there is. Nothing is
// written, and the fragment still has to be kept off the player's screen.
func TestSplitReplyRefusesAnIncompleteBlock(t *testing.T) {
	for _, reply := range []string{
		"The cellar is cold.\n\n<world-update>\n# Garret\n- Silver: 2",
		"The cellar is cold.\n\n<world-update></world-update>",
		"The cellar is cold.\n\n<world-update>\n\n</world-update>",
	} {
		told := splitReply(reply)
		if told.sheet != "" {
			t.Errorf("%q wrote a sheet: %q", reply, told.sheet)
		}
		if told.prose != "The cellar is cold." {
			t.Errorf("%q leaked into the prose: %q", reply, told.prose)
		}
	}
}

// Both blocks in one reply, which is what an -img-gen turn looks like.
func TestSplitReplySeparatesBothBlocks(t *testing.T) {
	told := splitReply("The cellar is cold.\n\n<img>\nA low cellar.\n</img>\n\n<world-update>\n# Garret\n</world-update>\n")
	if told.prose != "The cellar is cold." {
		t.Fatalf("prose = %q", told.prose)
	}
	if told.img != "A low cellar." || told.sheet != "# Garret" {
		t.Fatalf("img = %q, sheet = %q", told.img, told.sheet)
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

// block is a world update block carrying the given sheet, as it arrives at the
// end of a narration response.
func block(sheet string) string {
	return "\n\n<world-update>\n" + sheet + "\n</world-update>"
}

// played opens a session with a sheet already on disk, which is every turn but
// the first.
func played(t *testing.T) (*session, []llm.Message) {
	t.Helper()
	s, dir := playSession(t)
	s.dir = dir
	if err := s.writeCharsheet("- Name: Garret\n- Location: the hall"); err != nil {
		t.Fatal(err)
	}
	return s, []llm.Message{{Role: llm.RoleSystem, Content: "ground rules"}}
}

func TestTurnEndsOnProse(t *testing.T) {
	s, messages := played(t)

	s.processGameTurn(replies(
		llm.Message{Content: "The door swings open." + block("- Location: the cellar")},
	), &messages, "open the door")

	if s.lastNarration != "The door swings open." {
		t.Fatalf("lastNarration = %q", s.lastNarration)
	}
}

// The block is the sheet's only writer, and the scene is what lies outside it.
// Both halves of that are one test: what the player read, and what is on disk.
func TestTheBlockIsWrittenAndNeverShown(t *testing.T) {
	s, messages := played(t)

	s.processGameTurn(replies(
		llm.Message{Content: "You step through into the cellar." + block("- Name: Garret\n- Location: the cellar")},
	), &messages, "go through the door")

	if s.lastNarration != "You step through into the cellar." {
		t.Fatalf("the player was shown the block: %q", s.lastNarration)
	}
	sheet, err := os.ReadFile(s.charsheetPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sheet), "- Location: the cellar") {
		t.Fatalf("the block did not reach the sheet: %q", sheet)
	}
	if strings.Contains(strings.Join(contents(messages), "\n"), "world-update") {
		t.Fatal("the block reached the chat history, where a second copy of the sheet would sit")
	}
}

// Every accepted block is logged raw, so a line that quietly stopped being
// restated can be found by diffing one sheet against the next. It is the only
// automated detector for the risk this design takes on.
func TestEveryBlockIsLoggedRaw(t *testing.T) {
	s, messages := played(t)

	s.processGameTurn(replies(llm.Message{Content: "One." + block("- Location: the cellar")}), &messages, "go on")
	s.processGameTurn(replies(llm.Message{Content: "Two." + block("- Location: the stair")}), &messages, "go on")

	log, err := os.ReadFile(filepath.Join(s.dir, charsheetLog))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"- Location: the hall", "- Location: the cellar", "- Location: the stair"} {
		if !strings.Contains(string(log), want) {
			t.Fatalf("the log lost %q:\n%s", want, log)
		}
	}
}

// A turn the DM forgot the block on costs the player nothing: the sheet stands,
// and the correction rides in on the next player line rather than on a request
// of its own.
func TestAMissingBlockLeavesTheSheetAndCarriesANote(t *testing.T) {
	s, messages := played(t)

	s.processGameTurn(replies(llm.Message{Content: "The hall is as you left it."}), &messages, "look around")

	sheet, err := os.ReadFile(s.charsheetPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sheet), "- Location: the hall") {
		t.Fatalf("a missing block damaged the sheet: %q", sheet)
	}
	if s.carried == "" {
		t.Fatal("no note was carried")
	}

	s.processGameTurn(replies(llm.Message{Content: "You go north." + block("- Location: the stair")}), &messages, "go north")

	joined := strings.Join(contents(messages), "\n")
	if !strings.Contains(joined, "carried no world update block") {
		t.Fatalf("the note never reached the DM:\n%s", joined)
	}
	if s.carried != "" {
		t.Fatal("the note was carried twice")
	}
}

// A turn that never reached the player is rolled back, and the player's line
// goes with it. The note that rode in on that line has to survive the rollback,
// or the correction is made once and lost.
func TestACarriedNoteSurvivesAFailedTurn(t *testing.T) {
	s, messages := played(t)

	s.processGameTurn(replies(llm.Message{Content: "The hall is as you left it."}), &messages, "look around")
	if s.carried == "" {
		t.Fatal("no note was carried")
	}

	// Every round comes back empty, so this turn reaches nobody.
	s.processGameTurn(replies(), &messages, "go north")
	if s.carried == "" {
		t.Fatal("the note was lost with the rolled-back turn")
	}

	s.processGameTurn(replies(llm.Message{Content: "You go north." + block("- Location: the stair")}), &messages, "go north")
	if !strings.Contains(strings.Join(contents(messages), "\n"), "carried no world update block") {
		t.Fatal("the note never reached the DM")
	}
}

// A tag the harness had to be generous about is still written — losing a turn of
// the world over punctuation is the more expensive mistake — and still earns a
// note.
func TestAToleratedBlockIsWrittenAndAlsoCarriesANote(t *testing.T) {
	s, messages := played(t)

	s.processGameTurn(replies(
		llm.Message{Content: "You step out.\n\n<World_Update>\n- Location: the yard\n</World_Update>"},
	), &messages, "step out")

	sheet, err := os.ReadFile(s.charsheetPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sheet), "- Location: the yard") {
		t.Fatalf("a tolerated block was refused: %q", sheet)
	}
	if !strings.Contains(s.carried, "expected form") {
		t.Fatalf("carried = %q", s.carried)
	}
}

// A block cut off mid-sheet writes nothing at all. Half a sheet on disk is worse
// than yesterday's whole one, and the next block restates everything anyway.
func TestATruncatedBlockWritesNothing(t *testing.T) {
	s, messages := played(t)

	s.processGameTurn(replies(
		llm.Message{Content: "You step out.\n\n<world-update>\n- Location: the yard"},
	), &messages, "step out")

	sheet, err := os.ReadFile(s.charsheetPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sheet), "- Location: the hall") {
		t.Fatalf("a truncated block reached the sheet: %q", sheet)
	}
	if s.lastNarration != "You step out." {
		t.Fatalf("the fragment reached the player: %q", s.lastNarration)
	}
}

// The first block is what opens the save directory, which is why the opening
// turn looks like every other turn instead of having an order to get wrong.
func TestTheFirstBlockCreatesTheSessionDirectory(t *testing.T) {
	s, _ := playSession(t)
	s.adventureName = "Dryfall Bridge"
	messages := []llm.Message{{Role: llm.RoleSystem, Content: "ground rules"}}

	s.processGameTurn(replies(llm.Message{Content: "What kind of person is she?"}), &messages, "")
	if s.dir != "" {
		t.Fatalf("character creation opened a directory: %q", s.dir)
	}
	if s.carried != "" {
		t.Fatal("the DM was nagged for a sheet that does not exist yet")
	}

	s.processGameTurn(replies(
		llm.Message{Content: "The hall is cold." + block("## Identity\n\n- Name: Mira the Sly")},
	), &messages, "let's play")

	if base := filepath.Base(s.dir); base != "0001-mira-the-sly-dryfall-bridge" {
		t.Fatalf("session directory = %q", base)
	}
	sheet, err := os.ReadFile(s.charsheetPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sheet), "Mira the Sly") {
		t.Fatalf("charsheet = %q", sheet)
	}
}

// Anything the DM writes beside a tool call predates the result, and so does any
// block beside it: both were written before the dice came back.
func TestProseAndBlockBesideAToolCallAreDiscarded(t *testing.T) {
	s, messages := played(t)

	s.processGameTurn(replies(
		llm.Message{
			Content: "You slip past the guard unseen." + block("- Location: past the guard"),
			ToolCalls: []llm.ToolCall{{
				ID:       "1",
				Function: llm.ToolCallFunction{Name: "check", Arguments: `{"level":"Fair","vs":"Average","what":"sneaking"}`},
			}},
		},
		llm.Message{Content: "The guard turns. You freeze." + block("- Location: the hall, seen")},
	), &messages, "sneak past")

	if s.lastNarration != "The guard turns. You freeze." {
		t.Fatalf("the player was shown prose written before the roll: %q", s.lastNarration)
	}
	sheet, err := os.ReadFile(s.charsheetPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sheet), "past the guard") {
		t.Fatalf("a block written before the roll reached the sheet: %q", sheet)
	}
}

// A DM that never stops reaching for tools hits the ceiling, and the narration
// phase takes them away rather than asking again with them still on the table.
func TestCheckCeilingForcesAToollessNarration(t *testing.T) {
	s, _ := playSession(t)
	messages := []llm.Message{{Role: llm.RoleSystem, Content: "ground rules"}}

	rolling := llm.Message{ToolCalls: []llm.ToolCall{{
		ID:       "1",
		Function: llm.ToolCallFunction{Name: "check", Arguments: `{"level":"Fair","vs":"Average","what":"listening"}`},
	}}}

	var requests, toolsOnLast int
	chat := replies(rolling, rolling, rolling, rolling, rolling, llm.Message{Content: "You hear nothing."})
	counted := func(msgs []llm.Message, tools []llm.Tool) (llm.Response, error) {
		requests++
		toolsOnLast = len(tools)
		return chat(msgs, tools)
	}

	s.processGameTurn(counted, &messages, "listen at the door")

	if requests != maxCheckRequests+1 {
		t.Fatalf("the turn made %d requests, want %d checks and one narration", requests, maxCheckRequests+1)
	}
	if toolsOnLast != 0 {
		t.Fatalf("the narration request still carried %d tools", toolsOnLast)
	}
	if s.lastNarration != "You hear nothing." {
		t.Fatalf("lastNarration = %q", s.lastNarration)
	}
	if !strings.Contains(strings.Join(contents(messages), "\n"), "Stop resolving and narrate") {
		t.Fatal("the narration demand was never sent")
	}
}

// The menu is the exception: user_choice blocks on the player, and the question
// that belongs above the options is the prose beside the call.
func TestMenuQuestionReachesThePlayerBeforeTheOptions(t *testing.T) {
	s, _ := playSession(t)
	messages := []llm.Message{{Role: llm.RoleSystem, Content: "ground rules"}}

	s.processGameTurn(replies(
		llm.Message{
			Content: "Two doors. Which?",
			ToolCalls: []llm.ToolCall{{
				ID:       "1",
				Function: llm.ToolCallFunction{Name: "user_choice", Arguments: `{"options":["the left","the right"]}`},
			}},
		},
		llm.Message{Content: "You take the left door."},
	), &messages, "look at the doors")

	if s.lastNarration != "You take the left door." {
		t.Fatalf("lastNarration = %q", s.lastNarration)
	}
}

// A tool the DM is never shown is one it cannot reach for by mistake. Every
// other game tool lost its consumer when the block took over the sheet.
func TestATurnOffersOnlyTheDiceAndTheMenu(t *testing.T) {
	var names []string
	for _, tool := range gameTools() {
		names = append(names, tool.Function.Name)
	}
	if strings.Join(names, ",") != "check,user_choice" {
		t.Fatalf("a turn offers %v", names)
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

	if err := s.milestone(&messages); err != nil {
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

// The archive is the whole of what a milestone does with the model's words, and
// it costs no request to make: the state is already whole in the sheet, so there
// is nothing left for a summary to carry.
func TestMilestoneArchivesTheRawChatWithoutAModelCall(t *testing.T) {
	s, dir := playSession(t)
	s.dir = dir
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: "ground rules"},
		{Role: llm.RoleUser, Content: "I pick the lock"},
	}

	if err := s.milestone(&messages); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "milestone-01-raw.md"))
	if err != nil {
		t.Fatalf("raw chat not archived: %v", err)
	}
	if !strings.Contains(string(raw), "I pick the lock") {
		t.Fatalf("the archive lost the chat:\n%s", raw)
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
