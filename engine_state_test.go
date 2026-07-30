package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fudge/archive"
	"fudge/narrative"
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

// One entity, one record. This is the invariant the whole flat map exists for:
// under the three-section block a character was split across all three, and each
// section was somewhere a fact about them could be written.
func TestEveryCharacterAppearsExactlyOnce(t *testing.T) {
	got := rendered(rosterWorld(t))

	for _, id := range []string{"kyle-katarn", "dust-bandit", "preacher-vane"} {
		if n := strings.Count(got, "\n  "+id+":\n"); n != 1 {
			t.Errorf("%s appears %d times, want exactly 1:\n%s", id, n, got)
		}
	}
}

// The map is rendered fresh, so a wound taken twenty turns ago and a wound taken
// this turn read the same way: as the only thing the engine says is true.
func TestTheRenderIsCurrent(t *testing.T) {
	w := rosterWorld(t)
	bandit, err := w.Character("dust-bandit")
	if err != nil {
		t.Fatal(err)
	}
	bandit.Wounds.ApplyDamage(4)

	got := rendered(w)
	if !strings.Contains(got, "wounds: hurt") {
		t.Errorf("the render does not show the wound that was applied:\n%s", got)
	}
	if strings.Count(got, "wounds: undamaged") != 2 {
		t.Errorf("the two unwounded characters are not both reported undamaged:\n%s", got)
	}
}

// The player character leads. Every turn is about them, and the DM should not
// have to hunt for them among the NPCs it invented along the way.
func TestThePlayerLeads(t *testing.T) {
	got := rendered(rosterWorld(t))

	player := strings.Index(got, "\n  kyle-katarn:\n")
	for _, other := range []string{"\n  dust-bandit:\n", "\n  preacher-vane:\n"} {
		if i := strings.Index(got, other); i < player {
			t.Errorf("%s is listed above the player character:\n%s", strings.TrimSpace(other), got)
		}
	}
}

// The dead stay in the map. They are still the reason a body is on the floor,
// and pruning them was considered and deliberately left out.
func TestTheDeadStayInTheMap(t *testing.T) {
	w := rosterWorld(t)
	bandit, err := w.Character("dust-bandit")
	if err != nil {
		t.Fatal(err)
	}
	bandit.Wounds.DeclareDead()

	got := rendered(w)
	if !strings.Contains(got, "\n  dust-bandit:\n") {
		t.Errorf("a dead character fell out of the map:\n%s", got)
	}
	if !strings.Contains(got, "wounds: dead") {
		t.Errorf("the map does not report them as dead:\n%s", got)
	}
}

// An item is one entity with one holder, and a character lists what they carry.
// Both come from the same storage, so they cannot disagree — the second view is
// there because "can they do this with what they have" is the question the DM
// asks most often, and scanning every item in the world for a matching holder is
// exactly the reading a model does badly.
func TestCarriedItemsRenderBothAsEntitiesAndAsInventory(t *testing.T) {
	w := rosterWorld(t)
	if _, err := w.CreateLocation("Saloon", "Sawdust and smoke.", ""); err != nil {
		t.Fatal(err)
	}
	player := w.Characters[w.Player]
	item, err := w.GiveItem(player, "Iron Key", "Cold, heavy.", 1)
	if err != nil {
		t.Fatal(err)
	}

	got := rendered(w)
	if !strings.Contains(got, "\n  "+item.ID+":\n") {
		t.Errorf("the key is not an entity of its own:\n%s", got)
	}
	if !strings.Contains(got, "location: "+w.Player) {
		t.Errorf("the key does not reference who carries it:\n%s", got)
	}
	if !strings.Contains(got, "inventory: ["+item.ID+"]") {
		t.Errorf("the carrier does not list the key by id:\n%s", got)
	}
}

// Narrative properties hang off the entity they are about, under a key of their
// own, so there is no second namespace in which the same entity can be described.
func TestNarrativePropertiesRenderOnTheirEntity(t *testing.T) {
	w := rosterWorld(t)
	n := narrative.New()
	n.Apply([]narrative.Operation{
		{Op: narrative.OpSet, Entity: "dust-bandit", Properties: map[string]any{"mood": "cornered"}},
	}, 7, w)

	got := entities(w, n)
	bandit := got[strings.Index(got, "\n  dust-bandit:\n"):]
	if !strings.Contains(bandit, "narrative: {mood: cornered}") {
		t.Errorf("the property is not on the entity it is about:\n%s", bandit)
	}
	if !strings.Contains(bandit, "@t7") {
		t.Errorf("the turn marker is missing, so the fact cannot be traced:\n%s", bandit)
	}
}

// A narrative-only entity has no engine half at all, and still gets a record.
func TestNarrativeOnlyEntitiesRender(t *testing.T) {
	w := rosterWorld(t)
	n := narrative.New()
	n.Apply([]narrative.Operation{
		{Op: narrative.OpCreate, Entity: "rumor-crypt", Properties: map[string]any{
			"type": "rumor", "content": "the count murdered his brother",
		}},
	}, 9, w)

	got := entities(w, n)
	if !strings.Contains(got, "\n  rumor-crypt:\n") {
		t.Errorf("the rumor has no record:\n%s", got)
	}
	if !strings.Contains(got, "type: rumor") {
		t.Errorf("the rumor does not say what it is:\n%s", got)
	}
}

// One id means one entity, across both stores. The engine and the narrative
// store share a single keyspace, and it has to be guarded from both sides: the
// store refuses to create onto an id the engine holds, and minting refuses to
// land an item on an id the store holds.
//
// The second half is the one that was missing. A DM creating the objective
// `iron-key`, then a scene producing an item named "Iron Key", used to put two
// different things under one id — and the render folded them into a single
// record, which is precisely the two-truths defect this design exists to end.
func TestOneIDMeansOneEntityAcrossBothStores(t *testing.T) {
	s := newSession(rosterWorld(t))
	if _, err := s.world.CreateLocation("Saloon", "Dust.", ""); err != nil {
		t.Fatal(err)
	}

	if notes := s.narrative.Apply([]narrative.Operation{
		{Op: narrative.OpCreate, Entity: "iron-key", Properties: map[string]any{
			"type": "objective", "why": "opens the crypt",
		}},
	}, 1, s.world); len(notes) != 0 {
		t.Fatalf("creating a narrative entity was refused: %v", notes)
	}

	item, err := s.world.GiveItem(s.world.Characters[s.world.Player], "Iron Key", "Cold, heavy.", 1)
	if err != nil {
		t.Fatal(err)
	}
	if item.ID == "iron-key" {
		t.Fatalf("the item was minted onto the objective's id: %q", item.ID)
	}

	// And both are in the block, separately, each saying what it is.
	docEntities := entitiesDoc(t, s.engineState())
	objective, held := docEntities["iron-key"].(map[string]any)
	if !held {
		t.Fatalf("the objective is gone from the block: %v", keysOf(docEntities))
	}
	if objective["type"] != "objective" {
		t.Errorf("the objective was overwritten by the item: %#v", objective)
	}
	if _, held := docEntities[item.ID]; !held {
		t.Errorf("the item %q is missing from the block: %v", item.ID, keysOf(docEntities))
	}
}

// The other direction, which was already guarded: an entity the engine holds is
// never minted here, because the rules library computes its defaults.
func TestNarrativeCannotCreateOntoAnEngineID(t *testing.T) {
	s := newSession(rosterWorld(t))

	notes := s.narrative.Apply([]narrative.Operation{
		{Op: narrative.OpCreate, Entity: "kyle-katarn", Properties: map[string]any{"type": "rumor"}},
	}, 1, s.world)
	if len(notes) != 1 || !strings.Contains(notes[0], "already exists in the engine") {
		t.Fatalf("creating onto a character's id was not refused: %v", notes)
	}
}

// The engine owns wounds and renders them itself. A DM that reports one anyway
// must not get it into the narrative map, or the block starts disagreeing with
// itself again — which is the defect this whole design was built to end.
func TestEngineOwnedFactsNeverReachTheNarrativeMap(t *testing.T) {
	w := rosterWorld(t)
	n := narrative.New()
	n.Apply([]narrative.Operation{
		{Op: narrative.OpSet, Entity: "kyle-katarn", Properties: map[string]any{
			"wounds": "hurt", "disposition": "rattled",
		}},
	}, 1, w)

	got := entities(w, n)
	if strings.Contains(got, "narrative: {wounds") || strings.Contains(got, "wounds: hurt") {
		t.Errorf("an engine-owned fact reached the narrative map:\n%s", got)
	}
	if !strings.Contains(got, "disposition: rattled") {
		t.Errorf("the fiction fact in the same operation was lost with it:\n%s", got)
	}
}

// A synonym is the failure a namespace alone cannot stop: refused `location`, a
// model reaches for `where`, and three hundred turns later reads it back as
// though the engine had put it there.
func TestSynonymsForEngineFactsAreRefusedToo(t *testing.T) {
	w := rosterWorld(t)
	n := narrative.New()
	n.Apply([]narrative.Operation{
		{Op: narrative.OpSet, Entity: "kyle-katarn", Properties: map[string]any{
			"where": "the cellar", "current_room": "cellar", "mood": "wary",
		}},
	}, 1, w)

	got := entities(w, n)
	for _, unwanted := range []string{"where:", "current_room:"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("a synonym for an engine fact was recorded (%s):\n%s", unwanted, got)
		}
	}
	if !strings.Contains(got, "mood: wary") {
		t.Errorf("the good property was lost alongside the refused ones:\n%s", got)
	}
}

// played fills an archive with n turns, each one distinguishable from the others
// in input, events and prose.
func played(n int) *archive.Archive {
	a := archive.New()
	for i := 1; i <= n; i++ {
		a.Record(archive.Turn{
			Number:      i,
			PlayerInput: fmt.Sprintf("player line <%d>", i),
			Events: []archive.Event{{
				Tool:     "world_update",
				Entities: []string{"thing"},
				Headline: fmt.Sprintf("set thing state: %d", i),
			}},
			Prose: fmt.Sprintf("narration <%d>", i),
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

// The block is the DM's entire memory, so both parts have to be in it and each
// has to say what it is authoritative for.
func TestBlockCarriesTheWorldAndTheTimeline(t *testing.T) {
	s := newSession(rosterWorld(t))
	s.narrative.Apply([]narrative.Operation{
		{Op: narrative.OpSet, Entity: "dust-bandit", Properties: map[string]any{"patrons": "scattered"}},
	}, 1, s.world)
	s.archive.Record(archive.Turn{Number: 1, PlayerInput: "I shoot the fixer", Prose: "The room empties."})

	got := s.engineState()
	for _, want := range []string{
		"WORLD STATE", "entities:", "TIMELINE",
		"kyle-katarn",        // an engine entity
		"patrons: scattered", // a narrative property on one
		"I shoot the fixer",  // timeline
		"The room empties.",  // prose of the newest turn
	} {
		if !strings.Contains(got, want) {
			t.Errorf("block is missing %q:\n%s", want, got)
		}
	}
}

// The block is YAML the DM reads as one document, so prose the engine did not
// write — a description, the spot a thing rests in — must not be able to break
// its shape.
func TestProseWithYAMLPunctuationIsQuoted(t *testing.T) {
	w := rosterWorld(t)
	if _, err := w.CreateLocation("Cellar", "Damp: stacked with casks, and #cold", ""); err != nil {
		t.Fatal(err)
	}
	w.Characters[w.Player].Location = "cellar"

	got := rendered(w)
	if !strings.Contains(got, `desc: "Damp: stacked with casks, and #cold"`) {
		t.Errorf("a description containing YAML punctuation was left unquoted:\n%s", got)
	}
}

// Between turns the message array holds two messages and nothing else, so a dump
// of it alone would show a session that looks like it has not started. What the
// DM is actually working from is the block and the record behind it.
func TestContextDumpCarriesTheBlockAndTheArchive(t *testing.T) {
	s := newSession(rosterWorld(t))
	s.turn = 1
	s.narrative.Apply([]narrative.Operation{
		{Op: narrative.OpSet, Entity: "dust-bandit", Properties: map[string]any{"patrons": "scattered"}},
	}, 1, s.world)
	// The block as the DM had it during the turn, before that turn's report was
	// folded in. A fresh render here would show state it did not yet have.
	s.lastBlock = s.engineState()
	s.archive.Record(archive.Turn{
		Number:      1,
		PlayerInput: "I shoot the fixer",
		Events:      []archive.Event{{Tool: "opposed_attack", Entities: []string{"samantha"}, Headline: "GRAZE"}},
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
		"WORLD STATE",               // the block the DM actually saw
		"kyle-katarn",               // an entity within it
		"patrons: scattered",        // a narrative property within it
		"ARCHIVE",                   // the record
		"opposed_attack",            // engine events, which the block never shows
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
	dm := &model{reply: resolvingThenNarrating(setState("forge", "patrons", "scattered"))}

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
	if strings.Contains(engineState, "patrons: scattered") {
		t.Errorf("the dump rendered a fresh block instead of the one the DM saw:\n%s", engineState)
	}
	if !strings.Contains(engineState, "WORLD STATE") {
		t.Error("the dump has no block at all")
	}
}

// recall is the only way the archive reaches the DM, and an entity lookup is the
// call its own tool description tells the DM to prefer. It shipped unreachable
// once — the schema advertised `entity`, the archive matched on it, and the
// session in between never read the argument, so every entity lookup came back
// "recall needs either a query or a turn number". Nothing failed, because nothing
// exercised recall end to end. This does.
func TestRecallAnswersByEntity(t *testing.T) {
	s := newSession(rosterWorld(t))
	s.archive.Record(archive.Turn{
		Number:      4,
		PlayerInput: "I ask the bandit who sent him",
		Events: []archive.Event{{
			Tool:     "world_update",
			Entities: []string{"dust-bandit"},
			Headline: "set dust-bandit mood: cornered",
		}},
		Prose: "He looks at the door, not at you.",
	})
	s.archive.Record(archive.Turn{
		Number:      5,
		PlayerInput: "I look out the window",
		Events: []archive.Event{{
			Tool:     "output_narration",
			Entities: []string{"preacher-vane"},
			Headline: "narration shown to player",
		}},
		Prose: "Rain, and nobody in the street.",
	})

	got := s.recall(call("r", "recall", `{"entity":"dust-bandit"}`))
	if !strings.Contains(got, "turn 4") {
		t.Errorf("an entity lookup did not find the turn that entity took part in:\n%s", got)
	}
	if strings.Contains(got, "turn 5") {
		t.Errorf("an entity lookup returned a turn that entity had no part in:\n%s", got)
	}

	// The id is matched exactly, so a name is not an id — but an unrecognised id
	// must say that it is unrecognised rather than that nothing happened.
	unknown := s.recall(call("r", "recall", `{"entity":"Dust Bandit"}`))
	if !strings.Contains(unknown, "check the id") {
		t.Errorf("an unrecognised id was reported as proof nothing happened:\n%s", unknown)
	}

	// Case and stray space are normalised, since ids are minted lowercase.
	if got := s.recall(call("r", "recall", `{"entity":" Dust-Bandit "}`)); !strings.Contains(got, "turn 4") {
		t.Errorf("an id differing only in case did not resolve:\n%s", got)
	}

	// And a lookup naming nothing at all is still refused, now mentioning all three.
	empty := s.recall(call("r", "recall", `{}`))
	for _, want := range []string{"entity", "query", "turn"} {
		if !strings.Contains(empty, want) {
			t.Errorf("the refusal for an empty recall does not mention %q:\n%s", want, empty)
		}
	}
}
