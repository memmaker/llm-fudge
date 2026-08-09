package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
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
	if !strings.Contains(joined, "handed over no sheet") {
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
	if !strings.Contains(strings.Join(contents(messages), "\n"), "handed over no sheet") {
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

// The directory is open from the start now, so it is no longer evidence that a
// sheet exists. What still has to hold is the other half of the old rule: the DM
// is not nagged for a sheet during character creation, and is from the first one
// on.
func TestCharacterCreationIsNotNaggedForASheet(t *testing.T) {
	s, _ := playSession(t)
	s.adventureName = "Dryfall Bridge"
	if _, err := s.createDir("saves", s.adventureName); err != nil {
		t.Fatal(err)
	}
	messages := []llm.Message{{Role: llm.RoleSystem, Content: "ground rules"}}

	s.processGameTurn(replies(llm.Message{Content: "What kind of person is she?"}), &messages, "")
	if s.carried != "" {
		t.Fatal("the DM was nagged for a sheet that does not exist yet")
	}

	s.processGameTurn(replies(
		llm.Message{Content: "The hall is cold." + block("## Identity\n\n- Name: Mira the Sly")},
	), &messages, "let's play")

	if base := filepath.Base(s.dir); base != "0001-dryfall-bridge" {
		t.Fatalf("session directory = %q", base)
	}
	sheet, err := os.ReadFile(s.charsheetPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sheet), "Mira the Sly") {
		t.Fatalf("charsheet = %q", sheet)
	}

	// From here a blockless turn is a lapse, and the note rides in on the next
	// player line.
	s.processGameTurn(replies(llm.Message{Content: "You go north."}), &messages, "go north")
	if s.carried == "" {
		t.Fatal("a blockless turn after the first sheet carried no note")
	}
}

// Prose the DM wrote beside a tool call is still prose it wrote for the player,
// so it reaches them and joins the scene. The block beside it does not reach the
// sheet: that one is a claim about the world, made before the dice came back.
func TestProseBesideAToolCallIsKeptAndItsBlockIsNot(t *testing.T) {
	s, messages := played(t)

	s.processGameTurn(replies(
		llm.Message{
			Content: "You crouch and wait for the guard to pass." + block("- Location: past the guard"),
			ToolCalls: []llm.ToolCall{{
				ID:       "1",
				Function: llm.ToolCallFunction{Name: "check", Arguments: `{"level":"Fair","vs":"Average","what":"sneaking"}`},
			}},
		},
		llm.Message{Content: "The guard turns. You freeze." + block("- Location: the hall, seen")},
	), &messages, "sneak past")

	want := "You crouch and wait for the guard to pass.\n\nThe guard turns. You freeze."
	if s.lastNarration != want {
		t.Fatalf("the turn lost prose:\n got %q\nwant %q", s.lastNarration, want)
	}
	sheet, err := os.ReadFile(s.charsheetPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sheet), "past the guard") {
		t.Fatalf("a block written before the roll reached the sheet: %q", sheet)
	}
	if !strings.Contains(string(sheet), "the hall, seen") {
		t.Fatalf("the scene's own block did not reach the sheet: %q", sheet)
	}
}

// A turn that says something and then fails to finish still said it. What the
// player has read cannot be taken back, so it is kept rather than rolled back
// into an error message.
func TestProseFromAFailedTurnIsStillTheScene(t *testing.T) {
	s, messages := played(t)

	// The DM speaks beside a roll and then goes silent for the rest of the turn.
	s.processGameTurn(replies(llm.Message{
		Content: "You put your shoulder to the door.",
		ToolCalls: []llm.ToolCall{{
			ID:       "1",
			Function: llm.ToolCallFunction{Name: "check", Arguments: `{"level":"Fair","vs":"Hard","what":"shoving"}`},
		}},
	}), &messages, "shove the door")

	if s.lastNarration != "You put your shoulder to the door." {
		t.Fatalf("prose the player had already read was thrown away: %q", s.lastNarration)
	}
	if len(messages) == 1 {
		t.Fatal("a turn the player read was rolled back")
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

	if s.lastNarration != "Two doors. Which?\n\nYou take the left door." {
		t.Fatalf("lastNarration = %q", s.lastNarration)
	}
}

// Under -debug every exchange is written down whole, including the ones made
// before there was a directory to write them to — a game that fails to start
// fails in exactly those requests.
func TestDebugLogsEveryRequestWholeOnceThereIsADirectory(t *testing.T) {
	debug = true
	t.Cleanup(func() { debug = false })

	s, _ := playSession(t)
	s.adventureName = "Dryfall Bridge"
	s.model = "test-model"
	messages := []llm.Message{{Role: llm.RoleSystem, Content: "ground rules"}}

	// The harness only sees a request through chatInternal, which needs a
	// client. The log is fed the same way the real path feeds it.
	s.logRequest(
		[]llm.Message{{Role: llm.RoleUser, Content: "what do you want to play?"}},
		gameTools(),
		llm.Response{Message: llm.Message{
			Role:    llm.RoleAssistant,
			Content: "A cautious mercenary, then.",
			ToolCalls: []llm.ToolCall{{
				Function: llm.ToolCallFunction{Name: "check", Arguments: `{"level":"Fair"}`},
			}},
		}},
		nil,
	)
	if _, err := os.Stat(filepath.Join("saves", requestLogDir)); err == nil {
		t.Fatal("a log directory was made before the session directory existed")
	}

	if _, err := s.createDir("saves", s.adventureName); err != nil {
		t.Fatal(err)
	}
	s.processGameTurn(replies(
		llm.Message{Content: "The hall is cold." + block("- Name: Mira")},
	), &messages, "let's play")

	body, err := os.ReadFile(filepath.Join(s.dir, requestLogDir, "context-000.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"turn 0",                        // which turn it belonged to
		"test-model",                    // which model answered
		"what do you want to play?",     // everything sent
		`"name":"check"`,                // every tool schema sent with it
		"A cautious mercenary, then.",   // the prose that came back
		`[call check] {"level":"Fair"}`, // every call, with its arguments
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("the log is missing %q:\n%s", want, body)
		}
	}

	// Beside it, the same request as bytes: what -resume reads, so it must be a
	// message list and not prose about one.
	sidecar, err := os.ReadFile(filepath.Join(s.dir, requestLogDir, "context-000.json"))
	if err != nil {
		t.Fatal(err)
	}
	var sent []llm.Message
	if err := json.Unmarshal(sidecar, &sent); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 || sent[0].Content != "what do you want to play?" {
		t.Fatalf("sidecar = %+v", sent)
	}
}

// A tool the DM is never shown is one it cannot reach for by mistake. Every
// other game tool lost its consumer when the block took over the sheet — what
// is left is the dice, the secrets the briefing withheld, and the menu.
func TestATurnOffersOnlyTheDiceTheSecretsTheSheetAndTheMenu(t *testing.T) {
	var names []string
	for _, tool := range gameTools() {
		names = append(names, tool.Function.Name)
	}
	if strings.Join(names, ",") != "check,secret,world_update,user_choice" {
		t.Fatalf("a turn offers %v", names)
	}
}

// A secret is a file the DM can only reach by name. The name comes from a
// model, so it is a name and never a path: the escape below must land on the
// missing-secret sentence rather than on a file outside the adventure.
func TestSecretReadsByNameOnly(t *testing.T) {
	s, dir := playSession(t)
	s.settingName, s.adventureName = "medieval-fantasy", "The-Hall-of-Colors"
	if err := os.MkdirAll(secretsDir(s.settingName, s.adventureName), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secretsDir(s.settingName, s.adventureName), "door.md"), []byte("it opens"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "outside.md"), []byte("not a secret"), 0644); err != nil {
		t.Fatal(err)
	}

	call := func(name string) string {
		result, err := s.executeAction(llm.ToolCall{Function: llm.ToolCallFunction{
			Name: "secret", Arguments: `{"name":` + strconv.Quote(name) + `}`,
		}})
		if err != nil {
			t.Fatalf("secret %q: %v", name, err)
		}
		return result
	}

	if got := call("door"); got != "it opens" {
		t.Errorf("secret door returned %q", got)
	}
	if got := call("../../../../outside"); !strings.Contains(got, "no secret") {
		t.Errorf("a path escaped the adventure: %q", got)
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

// loadWorld is what -setting/-adventure, the menus and a resume all go through,
// so a wrong name has to fail there rather than reaching the DM as an empty
// briefing.
func TestLoadWorldNamesBothOrFails(t *testing.T) {
	s := &session{}
	if err := loadWorld(s, "medieval-fantasy", "The-Hall-of-Colors"); err != nil {
		t.Fatalf("loading a world that exists: %v", err)
	}
	if s.settingDesc == "" || s.adventure == "" || s.adventureName != "The-Hall-of-Colors" {
		t.Fatalf("world half-loaded: desc=%d adventure=%d name=%q",
			len(s.settingDesc), len(s.adventure), s.adventureName)
	}

	before := *s
	if err := loadWorld(s, "medieval-fantasy", "No-Such-Adventure"); err == nil {
		t.Fatal("a missing adventure loaded without complaint")
	}
	if s.adventure != before.adventure || s.adventureName != before.adventureName {
		t.Fatal("a failed load overwrote the adventure")
	}
}

// The DM that writes its scene beside the roll and then writes it again once
// the dice land has said one thing, not two. The history keeps both replies;
// the player is told once.
func TestProseRepeatedAfterTheRollIsShownOnce(t *testing.T) {
	s, messages := played(t)

	scene := "You put your shoulder to the door."
	s.processGameTurn(replies(
		llm.Message{
			Content: scene,
			ToolCalls: []llm.ToolCall{{
				ID:       "1",
				Function: llm.ToolCallFunction{Name: "check", Arguments: `{"level":"Fair","vs":"Hard","what":"shoving"}`},
			}},
		},
		llm.Message{Content: scene + block("- Location: the doorway")},
	), &messages, "shove the door")

	if s.lastNarration != scene {
		t.Fatalf("the scene was told twice:\n got %q\nwant %q", s.lastNarration, scene)
	}
	played, err := os.ReadFile(filepath.Join(s.dir, transcriptFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(played), scene) != 1 {
		t.Fatalf("the transcript carries the scene twice:\n%s", played)
	}
}

// updateCall is a world_update tool call carrying the given sheet.
func updateCall(sheet string) llm.ToolCall {
	args, err := json.Marshal(map[string]string{"sheet": sheet})
	if err != nil {
		panic(err)
	}
	return llm.ToolCall{ID: "u", Function: llm.ToolCallFunction{Name: "world_update", Arguments: string(args)}}
}

// The tool is the other way in. A scene and a world_update in one response is a
// finished turn: the sheet is written and the turn does not ask again.
func TestASheetHandedOverByToolIsWritten(t *testing.T) {
	s, messages := played(t)

	s.processGameTurn(replies(
		llm.Message{Content: "You step into the cellar.", ToolCalls: []llm.ToolCall{updateCall("- Name: Garret\n- Location: the cellar")}},
		llm.Message{Content: "A second scene nobody asked for."},
	), &messages, "go through the door")

	if s.lastNarration != "You step into the cellar." {
		t.Fatalf("the turn did not end on its own scene: %q", s.lastNarration)
	}
	sheet, err := os.ReadFile(s.charsheetPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sheet), "- Location: the cellar") {
		t.Fatalf("the tool's sheet did not reach disk: %q", sheet)
	}
}

// A sheet handed over before the scene exists still belongs to the turn, even
// when the narration that closes it carries no block of its own.
func TestASheetHandedOverBeforeTheSceneStillCounts(t *testing.T) {
	s, messages := played(t)

	s.processGameTurn(replies(
		llm.Message{ToolCalls: []llm.ToolCall{updateCall("- Name: Garret\n- Location: the stair")}},
		llm.Message{Content: "You climb the stair."},
	), &messages, "climb")

	sheet, err := os.ReadFile(s.charsheetPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sheet), "- Location: the stair") {
		t.Fatalf("a sheet sent before the scene was lost: %q", sheet)
	}
	if s.carried != "" {
		t.Fatalf("a turn that did hand over a sheet was nagged: %q", s.carried)
	}
}

// Handovers are capped, not banned: the newest few keep their sheets and the
// oldest are taken out as they are superseded, results and all, so the history
// stops growing by a charsheet a turn.
func TestOnlyTheNewestSheetsStayInTheHistory(t *testing.T) {
	s, messages := played(t)

	// One more turn than the cap, so the cap always binds whatever it is set to.
	var places []string
	for i := 0; i <= sheetsKeptInHistory; i++ {
		places = append(places, "room "+strconv.Itoa(i))
	}
	for _, place := range places {
		s.processGameTurn(replies(
			llm.Message{Content: "You walk on.", ToolCalls: []llm.ToolCall{updateCall("- Name: Garret\n- Location: " + place)}},
		), &messages, "walk on")
	}

	var kept []string
	live := map[string]bool{}
	for _, msg := range messages {
		for _, call := range msg.ToolCalls {
			live[call.ID] = true
			if call.Function.Name == worldUpdateTool {
				kept = append(kept, call.Function.Arguments)
			}
		}
		if msg.Role == llm.RoleTool && !live[msg.ToolCallID] {
			t.Fatalf("a result was left for a call that is gone: %q", msg.Content)
		}
	}

	if len(kept) != sheetsKeptInHistory {
		t.Fatalf("the history holds %d sheets, want %d", len(kept), sheetsKeptInHistory)
	}
	for i, place := range places[len(places)-sheetsKeptInHistory:] {
		if !strings.Contains(kept[i], place) {
			t.Fatalf("sheet %d is not %q: %s", i, place, kept[i])
		}
	}
	if strings.Contains(strings.Join(kept, "\n"), places[0]) {
		t.Fatal("the oldest sheet outlived its cap")
	}
}

// A handover with no prose leaves no message in the history by design, so the
// resolve phase cannot ask again — the request would be byte-identical and the
// answer with it, all the way to the ceiling. It goes straight to narration.
func TestASheetAloneDoesNotStallTheResolvePhase(t *testing.T) {
	s, messages := played(t)

	var requests int
	counted := func(msgs []llm.Message, tools []llm.Tool) (llm.Response, error) {
		requests++
		if requests == 1 {
			return llm.Response{Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{updateCall("- Name: Garret\n- Location: the stair")}}}, nil
		}
		return llm.Response{Message: llm.Message{Role: llm.RoleAssistant, Content: "You climb the stair."}}, nil
	}

	s.processGameTurn(counted, &messages, "climb")

	if requests > 2 {
		t.Fatalf("the turn spent %d requests on a handover and a scene", requests)
	}
	if s.lastNarration != "You climb the stair." {
		t.Fatalf("no scene reached the player: %q", s.lastNarration)
	}
	sheet, err := os.ReadFile(s.charsheetPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sheet), "- Location: the stair") {
		t.Fatalf("the handover was lost: %q", sheet)
	}
}

// A model whose chat template leaks its own call syntax hands the sheet over as
// prose. The player must not be shown the markup, and the turn must not lose the
// sheet — the same two failures the block handling exists to prevent.
func TestSplitReplyReadsACallWrittenOutAsText(t *testing.T) {
	reply := "You cut the rope in two.\n\n" +
		"<｜｜DSML｜｜tool_calls>\n<｜｜DSML｜｜invoke name=\"world_update\">\n" +
		"<｜｜DSML｜｜parameter name=\"sheet\" string=\"true\">## Identity\n\n- Name: Testa</｜｜DSML｜｜parameter>\n" +
		"</｜｜DSML｜｜invoke>\n</｜｜DSML｜｜tool_calls>"

	told := splitReply(reply)
	if told.sheet != "## Identity\n\n- Name: Testa" {
		t.Fatalf("the sheet was not recovered: %q", told.sheet)
	}
	if told.prose != "You cut the rope in two." {
		t.Fatalf("markup reached the player: %q", told.prose)
	}
	if !told.tolerated {
		t.Fatal("a repaired form earned no note")
	}
}

// The missing-block note rides the next player line, and the last turn of a
// session has none: without one more request at the end, the change the player
// just made never reaches the sheet. A session that ended cleanly must not
// spend that request.
func TestTheLastTurnsMissedSheetIsAskedForOnce(t *testing.T) {
	s, messages := played(t)

	s.processGameTurn(replies(llm.Message{Content: "You pocket the key."}), &messages, "take the key")
	if s.carried == "" {
		t.Fatal("a turn with no block carried no note")
	}

	s.flushSheet(replies(llm.Message{Content: "Noted." + block("- Name: Garret\n- Location: the hall\n- Inventory: brass key")}), &messages)

	sheet, err := os.ReadFile(s.charsheetPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sheet), "brass key") {
		t.Fatalf("the last turn's change was lost: %q", sheet)
	}
	joined := strings.Join(contents(messages), "\n")
	if !strings.Contains(joined, "Do not describe the edit") {
		t.Fatalf("the session-end note never reached the DM:\n%s", joined)
	}
}

func TestASessionThatHandedOverIsNotAskedAgain(t *testing.T) {
	s, messages := played(t)

	s.processGameTurn(replies(llm.Message{Content: "You go north." + block("- Location: the stair")}), &messages, "go north")
	before := len(messages)

	asked := false
	s.flushSheet(func([]llm.Message, []llm.Tool) (llm.Response, error) {
		asked = true
		return llm.Response{Message: llm.Message{Role: llm.RoleAssistant}}, nil
	}, &messages)

	if asked {
		t.Error("a session whose last turn handed over a sheet still spent a request")
	}
	if len(messages) != before {
		t.Errorf("the history grew by %d messages at the end of a clean session", len(messages)-before)
	}
}

// resolvePhase ends on any reply carrying no tool call, and at the end of a
// session a refusal is exactly that: prose. One refusal must not be the end of
// the matter, and a sheet on the second ask has to be taken.
func TestTheEndOfSessionAskSurvivesOneRefusal(t *testing.T) {
	s, messages := played(t)

	s.processGameTurn(replies(llm.Message{Content: "You pocket the key."}), &messages, "take the key")

	s.flushSheet(replies(
		llm.Message{Content: "I need to update the sheet to reflect the key."},
		llm.Message{Content: "Noted." + block("- Name: Garret\n- Inventory: brass key")},
	), &messages)

	sheet, err := os.ReadFile(s.charsheetPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sheet), "brass key") {
		t.Fatalf("the second ask was never made, or its sheet was dropped: %q", sheet)
	}
}

// Two asks and no more: a model that has refused twice is not going to be
// argued into it, and the player has already quit.
func TestTheEndOfSessionAskGivesUpAfterTwo(t *testing.T) {
	s, messages := played(t)

	s.processGameTurn(replies(llm.Message{Content: "You pocket the key."}), &messages, "take the key")

	asks := 0
	s.flushSheet(func([]llm.Message, []llm.Tool) (llm.Response, error) {
		asks++
		return llm.Response{Message: llm.Message{Role: llm.RoleAssistant, Content: "I should update the sheet."}}, nil
	}, &messages)

	if asks != flushAttempts {
		t.Errorf("the sheet was asked for %d times, want %d", asks, flushAttempts)
	}
}

// The one bookkeeping error the harness can see for itself: a part left on its
// own line after it went into the whole. Five wordings failed to fix this; the
// check does not restate the rule, it names the two lines.
func TestADoubledPartIsSeenAndNamed(t *testing.T) {
	item, whole := doubledItem("### Inventory\n- Rope (7.5 m)\n- Iron spike\n- Grappling line (iron spike + 7.5 m rope)")
	if item != "Iron spike" {
		t.Errorf("item = %q, want Iron spike", item)
	}
	if whole != "Grappling line (iron spike + 7.5 m rope)" {
		t.Errorf("whole = %q", whole)
	}
}

// A false positive would tell the DM to delete a line the player still has, so
// the check stays narrow: sizes, counts and unrelated brackets are not parts.
func TestASheetThatIsFineIsLeftAlone(t *testing.T) {
	for _, sheet := range []string{
		"- Rope (7.5 m) ×2\n- Iron spike\n- Torches ×3",
		"- Grappling line (iron spike + 7.5 m rope)\n- Rope (7.5 m)\n- Torches ×3",
		"- Coin purse (6 silver) ×2\n- Bolts ×12",
		"- Crossbow stock\n- Crossbow prod\n- Crossbow string",
	} {
		if item, whole := doubledItem(sheet); item != "" {
			t.Errorf("%q: falsely flagged %q inside %q", sheet, item, whole)
		}
	}
}

// The note names the pair and rides the next player line, like every other
// correction the harness makes without spending a request on it.
func TestADoubledPartCarriesANoteNamingIt(t *testing.T) {
	s, messages := played(t)

	s.processGameTurn(replies(llm.Message{Content: "You tie it on." +
		block("### Inventory\n- Rope (7.5 m)\n- Iron spike\n- Grappling line (iron spike + 7.5 m rope)")}),
		&messages, "tie the rope to the spike")

	if !strings.Contains(s.carried, "Iron spike") || !strings.Contains(s.carried, "Grappling line") {
		t.Fatalf("the note does not name the two lines: %q", s.carried)
	}
}

// Not a test of behaviour — a sweep of every sheet the runs have produced, so
// the check's false-positive rate is a number and not a hope. Run with
// `go test -run TestSweepCorpus -v`.
func TestSweepCorpus(t *testing.T) {
	paths, _ := filepath.Glob("consistency-out/*repro-item-bookkeeping/run*/charsheet.md")
	if len(paths) == 0 {
		t.Skip("no runs on disk")
	}
	flagged := 0
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if item, whole := doubledItem(string(b)); item != "" {
			flagged++
			t.Logf("FLAG %s\n     %s  |  %s", strings.TrimPrefix(p, "consistency-out/"), item, whole)
		}
	}
	t.Logf("%d of %d sheets flagged", flagged, len(paths))
}
