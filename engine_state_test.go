package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fudge/archive"
	"fudge/rules"

	"github.com/memmaker/gollum/llm"
)

func rosterWorld(t *testing.T) *rules.World {
	t.Helper()
	w, err := rules.NewWorld(1, "settings/deadlands")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Kyle Katarn", "Dust Bandit", "Preacher Vane"} {
		if _, err := w.CreateCharacterValidated(rules.CharacterRequest{
			Name: name, Species: "human",
			Attributes: map[string]int{"grit": 1},
			Skills:     map[string]int{"shootin": 1},
		}); err != nil {
			t.Fatal(err)
		}
	}
	return w
}

// The roster is what replaced a history full of half-stale copies of the same
// sheet, so it has to hold everybody — one copy each, as they stand now.
func TestRosterHoldsEveryCharacterOnce(t *testing.T) {
	w := rosterWorld(t)
	got := roster(w)

	for _, id := range []string{"kyle-katarn", "dust-bandit", "preacher-vane"} {
		if n := strings.Count(got, "id:"+id); n != 1 {
			t.Errorf("%s appears %d times in the roster, want exactly 1:\n%s", id, n, got)
		}
	}
}

// The roster is rendered fresh, so a wound taken twenty turns ago and a wound
// taken this turn read the same way: as the only thing the engine says is true.
func TestRosterIsCurrent(t *testing.T) {
	w := rosterWorld(t)
	bandit, err := w.Character("dust-bandit")
	if err != nil {
		t.Fatal(err)
	}
	bandit.Wounds.ApplyDamage(4)

	got := roster(w)
	if !strings.Contains(got, "wounds:hurt") {
		t.Errorf("roster does not show the wound that was applied:\n%s", got)
	}
	if strings.Count(got, "wounds:undamaged") != 2 {
		t.Errorf("the two unwounded characters are not both reported undamaged:\n%s", got)
	}
}

// The player character leads. Every turn is about them, and the DM should not
// have to hunt for them among the NPCs it invented along the way.
func TestRosterLeadsWithThePlayer(t *testing.T) {
	w := rosterWorld(t)
	got := roster(w)

	player := strings.Index(got, "id:kyle-katarn")
	for _, other := range []string{"id:dust-bandit", "id:preacher-vane"} {
		if i := strings.Index(got, other); i < player {
			t.Errorf("%s is listed above the player character:\n%s", other, got)
		}
	}
}

// A map is iterated in whatever order it feels like, and a block that shuffles
// between turns would rewrite the cached prefix for nothing.
func TestRosterIsDeterministic(t *testing.T) {
	w := rosterWorld(t)
	first := roster(w)
	for i := 0; i < 5; i++ {
		if got := roster(w); got != first {
			t.Fatalf("roster differs between renders:\n%s\n---\n%s", first, got)
		}
	}
}

// The dead stay on the roster. They are still the reason a body is on the floor,
// and pruning them was considered and deliberately left out of the first cut.
func TestRosterKeepsTheDead(t *testing.T) {
	w := rosterWorld(t)
	bandit, err := w.Character("dust-bandit")
	if err != nil {
		t.Fatal(err)
	}
	bandit.Wounds.DeclareDead()

	got := roster(w)
	if !strings.Contains(got, "id:dust-bandit") {
		t.Errorf("a dead character fell off the roster:\n%s", got)
	}
	if !strings.Contains(got, "wounds:dead") {
		t.Errorf("the roster does not report them as dead:\n%s", got)
	}
}

func TestRosterOfAnEmptyWorldIsNotBlank(t *testing.T) {
	got := roster(&rules.World{Characters: map[string]*rules.Character{}})
	if strings.TrimSpace(got) == "" {
		t.Error("an empty roster renders as nothing at all, which reads as a missing section")
	}
}

// played fills an archive with n turns, each one distinguishable from the others
// in input, changes and prose.
func played(n int) *archive.Archive {
	a := archive.New()
	for i := 1; i <= n; i++ {
		a.Record(archive.Turn{
			Number:      i,
			PlayerInput: fmt.Sprintf("player line <%d>", i),
			Update:      fmt.Sprintf("thing|state:%d", i),
			Prose:       fmt.Sprintf("narration <%d>", i),
		})
	}
	return a
}

// A chronology is followed forward. The DM reads it to see how the state came
// about, which only works if it arrives in the order it happened.
func TestTimelineRunsOldestFirst(t *testing.T) {
	got := timeline(played(3))
	first := strings.Index(got, "player line <1>")
	last := strings.Index(got, "player line <3>")
	if first < 0 || last < 0 {
		t.Fatalf("timeline is missing turns:\n%s", got)
	}
	if first > last {
		t.Errorf("timeline runs backwards:\n%s", got)
	}
}

// Prose is the expensive part of the block, so only the newest turns carry it —
// enough to hold a scene, not enough to re-send the session.
func TestTimelineKeepsProseOnTheNewestTurnsOnly(t *testing.T) {
	got := timeline(played(timelineWindow))

	for i := timelineWindow - proseWindow + 1; i <= timelineWindow; i++ {
		if !strings.Contains(got, fmt.Sprintf("narration <%d>", i)) {
			t.Errorf("turn %d should have kept its prose:\n%s", i, got)
		}
	}
	for i := 1; i <= timelineWindow-proseWindow; i++ {
		if strings.Contains(got, fmt.Sprintf("narration <%d>", i)) {
			t.Errorf("turn %d kept prose it should have shed:\n%s", i, got)
		}
		// The turn itself stays; it is only its prose that goes.
		if !strings.Contains(got, fmt.Sprintf("player line <%d>", i)) {
			t.Errorf("turn %d fell out of the window early:\n%s", i, got)
		}
	}
}

// The window slides. This is the whole reason chronology lives in a block that
// is rebuilt rather than in messages that would have to be deleted to age out.
func TestTimelineAgesTurnsOut(t *testing.T) {
	got := timeline(played(timelineWindow + 5))

	if strings.Contains(got, "player line <5>") {
		t.Errorf("a turn past the window is still in the timeline:\n%s", got)
	}
	if !strings.Contains(got, fmt.Sprintf("player line <%d>", timelineWindow+5)) {
		t.Errorf("the newest turn is missing:\n%s", got)
	}
	if n := strings.Count(got, "player line <"); n != timelineWindow {
		t.Errorf("timeline holds %d turns, want %d:\n%s", n, timelineWindow, got)
	}
}

func TestTimelineBeforePlayStartsIsNotBlank(t *testing.T) {
	if strings.TrimSpace(timeline(archive.New())) == "" {
		t.Error("an empty timeline renders as nothing, which reads as a missing section")
	}
}

// The block is the DM's entire memory, so every section has to be in it and
// every section has to say what it is authoritative for.
func TestEngineStateCarriesAllThreeSections(t *testing.T) {
	s := newSession(rosterWorld(t))
	s.ledger.Apply("bar|patrons:scattered", 1, s.isCharacter)
	s.archive.Record(archive.Turn{Number: 1, PlayerInput: "I shoot the fixer", Update: "bar|patrons:scattered", Prose: "The room empties."})

	got := s.engineState()
	for _, want := range []string{
		"ENGINE STATE", "ROSTER", "WORLD", "TIMELINE",
		"id:kyle-katarn",        // roster
		"bar|patrons:scattered", // ledger
		"I shoot the fixer",     // timeline
		"The room empties.",     // prose of the newest turn
	} {
		if !strings.Contains(got, want) {
			t.Errorf("block is missing %q:\n%s", want, got)
		}
	}
}

// The engine owns wounds and renders them itself. A DM that reports one anyway
// must not get it into the ledger, or the block starts disagreeing with itself
// again — which is the defect this whole design was built to end.
func TestEngineOwnedFactsNeverReachTheWorldSection(t *testing.T) {
	s := newSession(rosterWorld(t))
	s.ledger.Apply("kyle-katarn|wounds:hurt|disposition:rattled", 1, s.isCharacter)

	got := s.engineState()
	world := got[strings.Index(got, "WORLD"):]
	if strings.Contains(world, "wounds:hurt") {
		t.Errorf("an engine-owned fact reached the ledger:\n%s", world)
	}
	if !strings.Contains(world, "disposition:rattled") {
		t.Errorf("the fiction fact on the same line was lost with it:\n%s", world)
	}
}

// Between turns the message array holds two messages and nothing else, so a dump
// of it alone would show a session that looks like it has not started. What the
// DM is actually working from is the block and the record behind it.
func TestContextDumpCarriesTheBlockAndTheArchive(t *testing.T) {
	s := newSession(rosterWorld(t))
	s.turn = 1
	s.ledger.Apply("bar|patrons:scattered", 1, s.isCharacter)
	// The block as the DM had it during the turn, before that turn's report was
	// folded in. A fresh render here would show state it did not yet have.
	s.lastBlock = s.engineState()
	s.archive.Record(archive.Turn{
		Number:      1,
		PlayerInput: "I shoot the fixer",
		Events:      []string{"opposed_attack|samantha|GRAZE"},
		Update:      "bar|patrons:scattered",
		Prose:       "The room empties in a heartbeat.",
	})

	path := filepath.Join(t.TempDir(), "context.txt")
	messages := []llm.Message{{Role: llm.RoleSystem, Content: "system"}, {Role: llm.RoleUser, Content: "briefing"}}
	if err := s.dumpContextToFile(path, &messages); err != nil {
		t.Fatal(err)
	}

	dumped, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(dumped)
	for _, want := range []string{
		"ENGINE STATE",              // the block the DM actually saw
		"id:kyle-katarn",            // roster within it
		"bar|patrons:scattered",     // ledger within it
		"ARCHIVE",                   // the record
		"opposed_attack|samantha",   // engine events, which the block never shows
		"The room empties",          // and the prose, which nothing else keeps
		"MESSAGE CONTEXT", "system", // the messages, still
	} {
		if !strings.Contains(got, want) {
			t.Errorf("context dump is missing %q", want)
		}
	}
}

// The dump must show the block the DM had when it wrote the turn, not the one
// it will get next. Between turns those differ by exactly the turn being
// debugged, which is the worst possible thing to be wrong about.
func TestContextDumpShowsTheBlockTheDMActuallySaw(t *testing.T) {
	dm := &model{reply: resolvingThenNarrating("bar|patrons:scattered")}

	messages, s := gameStart(), newSession(testWorld())
	capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

	path := filepath.Join(t.TempDir(), "context.txt")
	if err := s.dumpContextToFile(path, &messages); err != nil {
		t.Fatal(err)
	}
	dumped, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// The turn reported this, so a freshly rendered block would carry it. The
	// block the DM was working from could not have.
	engineState := string(dumped)[:strings.Index(string(dumped), "=== ARCHIVE ===")]
	if strings.Contains(engineState, "bar|patrons:scattered") {
		t.Errorf("the dump rendered a fresh block instead of the one the DM saw:\n%s", engineState)
	}
	if !strings.Contains(engineState, "ENGINE STATE") {
		t.Error("the dump has no block at all")
	}
}
