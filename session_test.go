package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fudge/prompts"

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
	first, err := s.createDir(root, "Dryfall Bridge")
	if err != nil {
		t.Fatal(err)
	}
	// No character in the name: it only exists once the first block arrives, and
	// a directory that waits for it takes every log of a session that never
	// emits one down with it. See ADR 009.
	if base := filepath.Base(first); base != "0001-dryfall-bridge" {
		t.Fatalf("directory name = %q", base)
	}
	second, err := newSession().createDir(root, "Dryfall Bridge")
	if err != nil {
		t.Fatal(err)
	}
	if base := filepath.Base(second); base != "0002-dryfall-bridge" {
		t.Fatalf("second directory name = %q", base)
	}
}

// The directory is opened before anything is played, so what identifies the run
// has to be written there rather than derived from a name later.
func TestSessionDirectoryCarriesItsMetadata(t *testing.T) {
	s := newSession()
	s.settingName, s.model = "medieval-fantasy", "some-model"
	dir, err := s.createDir(t.TempDir(), "The Hall of Colors")
	if err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(filepath.Join(dir, metadataFile))
	if err != nil {
		t.Fatal(err)
	}
	var meta sessionMeta
	if err := json.Unmarshal(body, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Setting != "medieval-fantasy" || meta.Adventure != "The Hall of Colors" {
		t.Fatalf("metadata = %+v", meta)
	}
	if meta.Model != "some-model" || meta.Language != prompts.Language() || meta.Started == "" {
		t.Fatalf("metadata = %+v", meta)
	}
}

// The sheet a turn was played with is the last one written before it, which is
// not always the turn before: a turn that emitted no block leaves a gap.
func TestSheetAtTurnTakesTheLastBlockAtOrBeforeIt(t *testing.T) {
	log := "--- turn 2 ---\n\n- Location: the hall\n\n" +
		"--- turn 5 ---\n\n- Location: the cellar\n\n"

	for _, c := range []struct {
		turn int
		want string
	}{
		{turn: 1, want: ""},
		{turn: 2, want: "- Location: the hall"},
		{turn: 4, want: "- Location: the hall"},
		{turn: 5, want: "- Location: the cellar"},
		{turn: 9, want: "- Location: the cellar"},
	} {
		if got := sheetAtTurn(log, c.turn); got != c.want {
			t.Errorf("sheetAtTurn(turn %d) = %q, want %q", c.turn, got, c.want)
		}
	}
}

// A recording is injected, never re-played: what comes back is the conversation
// to hand the model, with the two rebuilt-from-prompts messages and the turn's
// own player line taken off the ends.
func TestRecordingDropsTheRebuiltHeadAndTheTrailingLine(t *testing.T) {
	dir := t.TempDir()
	logDir := filepath.Join(dir, requestLogDir)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(metadataFile, `{"setting":"medieval-fantasy","adventure":"The-Hall-of-Colors"}`)
	write(charsheetLog, "--- turn 5 ---\n\n- Location: the hall\n\n")

	sent := []llm.Message{
		{Role: llm.RoleSystem, Content: "ground rules as they were"},
		{Role: llm.RoleUser, Content: "the briefing as it was"},
		{Role: llm.RoleUser, Content: "I look around."},
		{Role: llm.RoleAssistant, Content: "Cold stone."},
		{Role: llm.RoleUser, Content: "I close one eye.\n\nCHARACTER SHEET…\n- Location: the hall"},
	}
	body, err := json.Marshal(sent)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "context-007.json"), body, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "context-007.md"), []byte("# request 007, turn 6\n"), 0644); err != nil {
		t.Fatal(err)
	}

	rec, err := loadRecording(filepath.Join(logDir, "context-007.json"))
	if err != nil {
		t.Fatal(err)
	}

	if rec.meta.Setting != "medieval-fantasy" || rec.meta.Adventure != "The-Hall-of-Colors" {
		t.Fatalf("metadata = %+v", rec.meta)
	}
	if rec.turn != 6 {
		t.Fatalf("turn = %d", rec.turn)
	}
	// The sheet comes from the charsheet log, not out of the folded tail: cutting
	// the fold would mean matching the header text as it read when recorded, and
	// that text is one of the things under test.
	if rec.sheet != "- Location: the hall" {
		t.Fatalf("sheet = %q", rec.sheet)
	}
	if len(rec.messages) != 2 {
		t.Fatalf("conversation = %v", contents(rec.messages))
	}
	joined := strings.Join(contents(rec.messages), "\n")
	if strings.Contains(joined, "ground rules") || strings.Contains(joined, "briefing") {
		t.Fatalf("the rebuilt head survived: %q", joined)
	}
	if strings.Contains(joined, "CHARACTER SHEET") || strings.Contains(joined, "I close one eye") {
		t.Fatalf("the trailing line survived: %q", joined)
	}
}

// A sidecar whose readable half is missing has no turn number in it. It still
// resumes: it counts from zero and takes the newest sheet there is, which beats
// starting a mid-game turn with no sheet at all.
func TestRecordingWithoutAHeaderTakesTheNewestSheet(t *testing.T) {
	dir := t.TempDir()
	logDir := filepath.Join(dir, requestLogDir)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, metadataFile), []byte(`{"setting":"s","adventure":"a"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, charsheetLog),
		[]byte("--- turn 2 ---\n\n- Location: the hall\n\n--- turn 5 ---\n\n- Location: the cellar\n\n"), 0644); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal([]llm.Message{
		{Role: llm.RoleSystem, Content: "rules"},
		{Role: llm.RoleUser, Content: "briefing"},
		{Role: llm.RoleUser, Content: "I look around."},
	})
	if err := os.WriteFile(filepath.Join(logDir, "context-009.json"), body, 0644); err != nil {
		t.Fatal(err)
	}

	rec, err := loadRecording(filepath.Join(logDir, "context-009.json"))
	if err != nil {
		t.Fatal(err)
	}
	if rec.turn != 0 {
		t.Errorf("turn = %d, want 0 when no header says otherwise", rec.turn)
	}
	if rec.sheet != "- Location: the cellar" {
		t.Errorf("sheet = %q, want the newest one", rec.sheet)
	}
}

// Only a turn's opening request is a resume point. Anything else — a tool round,
// the narration demand — would start the run mid-turn.
func TestRecordingRefusesAContextThatIsNotATurnStart(t *testing.T) {
	dir := t.TempDir()
	logDir := filepath.Join(dir, requestLogDir)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, metadataFile), []byte(`{"setting":"s","adventure":"a"}`), 0644); err != nil {
		t.Fatal(err)
	}
	sent := []llm.Message{
		{Role: llm.RoleSystem, Content: "ground rules"},
		{Role: llm.RoleUser, Content: "briefing"},
		{Role: llm.RoleUser, Content: "I shove the statue."},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "c1"}}},
		{Role: llm.RoleTool, ToolCallID: "c1", Content: "the shove: +2"},
	}
	body, _ := json.Marshal(sent)
	if err := os.WriteFile(filepath.Join(logDir, "context-004.json"), body, 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := loadRecording(filepath.Join(logDir, "context-004.json")); err == nil {
		t.Fatal("a mid-turn context was accepted as a resume point")
	}
}

// The transcript is the player's own record: what they read and what they typed,
// unwrapped, and nothing the harness said to the DM behind their back.
func TestTranscriptHoldsBothSidesAndNoCarriedNote(t *testing.T) {
	s := newTestSession(t)
	s.appendTranscript("> I examine the sign and read it out loud.")
	s.appendTranscript("You crouch by the low stone sign, brush dust off its face, and read the single word aloud.")

	body, err := os.ReadFile(filepath.Join(s.dir, transcriptFile))
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	if !strings.Contains(got, "> I examine the sign") || !strings.Contains(got, "You crouch by the low stone sign") {
		t.Fatalf("transcript = %q", got)
	}
	// One paragraph, one line: a hard wrap is how a phrase stops matching.
	if !strings.Contains(got, "read the single word aloud.") {
		t.Fatalf("the prose was rewrapped: %q", got)
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
	last := req[len(req)-1]
	if !strings.Contains(last.Content, "# Garret") {
		t.Fatalf("charsheet is not the last message: %q", last.Content)
	}
	// It rides in the player's own message rather than a second one: a user
	// message after another user message is fatal to the strict-alternation
	// templates, the way a second system message is fatal to llama.cpp's.
	if len(req) != len(msgs) {
		t.Fatalf("withCharsheet added %d messages", len(req)-len(msgs))
	}
	if !strings.Contains(last.Content, "I open the door") {
		t.Fatalf("folding lost the player's own words: %q", last.Content)
	}

	// Called twice, the charsheet still appears exactly once — the block is
	// assembled for the request, never stored back into the history.
	req = s.withCharsheet(msgs)
	if n := strings.Count(strings.Join(contents(req), "\n"), "# Garret"); n != 1 {
		t.Fatalf("charsheet appears %d times", n)
	}
	if len(msgs) != 2 || msgs[1].Content != "I open the door" {
		t.Fatal("withCharsheet mutated the caller's messages")
	}
}

// Every tail the two request phases can end on has to come back as a message
// list no chat template will reject: exactly one system message, at the head,
// and no two messages from the player's side in a row.
func TestCharsheetTailsStayTemplateLegal(t *testing.T) {
	s := newTestSession(t)
	if err := s.writeCharsheet("# Garret"); err != nil {
		t.Fatal(err)
	}
	head := []llm.Message{
		{Role: llm.RoleSystem, Content: "ground rules"},
		{Role: llm.RoleUser, Content: "I open the door"},
		{Role: llm.RoleAssistant, Content: "The door gives."},
	}
	tails := map[string][]llm.Message{
		"user": {{Role: llm.RoleUser, Content: "the scene, now"}},
		"tool": {
			{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "c1"}}},
			{Role: llm.RoleTool, ToolCallID: "c1", Content: "4"},
		},
		"assistant": {{Role: llm.RoleAssistant, Content: "Dust falls from the hinges."}},
	}

	for name, tail := range tails {
		t.Run(name, func(t *testing.T) {
			req := s.withCharsheet(append(append([]llm.Message{}, head...), tail...))

			joined := strings.Join(contents(req), "\n")
			if n := strings.Count(joined, "# Garret"); n != 1 {
				t.Fatalf("charsheet appears %d times", n)
			}
			if !strings.Contains(req[len(req)-1].Content, "# Garret") {
				t.Fatal("charsheet is not at the tail")
			}
			if last := tail[len(tail)-1].Content; !strings.Contains(joined, last) {
				t.Fatalf("folding lost the %s tail's content", name)
			}
			for i, msg := range req {
				if msg.Role == llm.RoleSystem && i != 0 {
					t.Fatalf("system message at index %d", i)
				}
				// Tool results count as the player's side: the templates that
				// alternate treat anything that is not the model's own turn as
				// one half of the exchange.
				if i > 0 && msg.Role != llm.RoleAssistant && req[i-1].Role == msg.Role {
					t.Fatalf("two %s messages in a row at index %d", msg.Role, i)
				}
			}
		})
	}
}

// The turn that has to produce the first sheet is the one turn with no sheet to
// pin, and it used to be the only turn the demand never reached. Now the shape
// goes in the sheet's place, so every request carries the demand exactly once.
func TestNoCharsheetYetSendsTheShapeInstead(t *testing.T) {
	s := newTestSession(t)
	msgs := []llm.Message{{Role: llm.RoleSystem, Content: "ground rules"}}

	got := s.withCharsheet(msgs)
	if len(got) != 2 {
		t.Fatalf("the shape was not sent: %d messages", len(got))
	}
	tail := got[len(got)-1].Content
	for _, want := range []string{"world_update", "## Identity", "## Wounds"} {
		if !strings.Contains(tail, want) {
			t.Fatalf("the empty-handed tail is missing %q:\n%s", want, tail)
		}
	}
	if strings.Count(tail, "world_update") != 1 {
		t.Fatalf("the demand was stated more than once:\n%s", tail)
	}
}

func contents(msgs []llm.Message) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.Content
	}
	return out
}
