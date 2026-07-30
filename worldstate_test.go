package main

import (
	"strings"
	"testing"

	"fudge/rules"
)

// stateSession is a game far enough along to have a place, a person and a thing
// in it — the state most blocks are written against.
func stateSession(t *testing.T) *session {
	t.Helper()
	w, err := rules.NewWorld(1, "settings/deadlands")
	if err != nil {
		t.Fatal(err)
	}
	s := newSession(w)
	if _, err := w.CreateLocation("Saloon", "Sawdust and lamp smoke.", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := w.CreateCharacterValidated(rules.CharacterRequest{Name: "Kyle Katarn", Species: "human", Location: "saloon"}); err != nil {
		t.Fatal(err)
	}
	return s
}

// apply runs a block and fails the test on any refusal, for the cases that are
// about what landed rather than about what was refused.
func apply(t *testing.T, s *session, block string) {
	t.Helper()
	_, notes := s.applyState(block)
	if len(notes) > 0 {
		t.Fatalf("block was refused: %s", strings.Join(notes, "; "))
	}
}

// refusals runs a block that is expected to be wrong and hands back what the DM
// would be told.
func refusals(t *testing.T, s *session, block string) string {
	t.Helper()
	_, notes := s.applyState(block)
	if len(notes) == 0 {
		t.Fatal("expected the block to be refused, it was accepted")
	}
	return strings.Join(notes, "\n")
}

// The whole point of the block: an entity that is not in the world is created,
// one that is gets merged, and the DM never says which it meant.
func TestPresenceDecidesCreateOrUpdate(t *testing.T) {
	s := stateSession(t)

	apply(t, s, `
entities:
  cellar:
    type: location
    name: The Cellar
    desc: Damp, stacked with casks.
    parent: saloon
  kyle-katarn:
    location: cellar
`)

	loc, err := s.world.Location("cellar")
	if err != nil {
		t.Fatalf("cellar was not created: %v", err)
	}
	if loc.Name != "The Cellar" || loc.Parent != "saloon" {
		t.Errorf("cellar = %+v", loc)
	}
	pc, _ := s.world.Character("kyle-katarn")
	if pc.Location != "cellar" {
		t.Errorf("player is at %q, want cellar", pc.Location)
	}
	if s.world.Scene() != "cellar" {
		t.Errorf("scene = %q, want cellar — it follows the player", s.world.Scene())
	}
}

// Nothing in the block has to be written in dependency order. The barkeep is
// declared before the room he stands in and the bottle before the barkeep.
func TestOrderInsideTheBlockDoesNotMatter(t *testing.T) {
	s := stateSession(t)

	apply(t, s, `
entities:
  whiskey-bottle:
    type: item
    name: Bottle of Whiskey
    location: barkeep
  barkeep:
    type: character
    name: Ezra Coles
    species: human
    location: back-room
  back-room:
    type: location
    name: The Back Room
    parent: saloon
`)

	ch, err := s.world.Character("barkeep")
	if err != nil {
		t.Fatalf("barkeep: %v", err)
	}
	if ch.Location != "back-room" {
		t.Errorf("barkeep is at %q, want back-room", ch.Location)
	}
	if _, held := ch.Inventory["whiskey-bottle"]; !held {
		t.Errorf("the bottle did not reach the barkeep: %v", ch.Inventory)
	}
}

// An item declaring a new holder is a move, and the engine runs the same
// conservation it always ran — the thing crosses over rather than being
// destroyed and rebuilt.
func TestDeclaringALocationMovesTheItem(t *testing.T) {
	s := stateSession(t)
	apply(t, s, `
entities:
  bowie-knife:
    type: item
    name: Bowie Knife
    location: kyle-katarn
    weapon: {damage: 2}
`)

	apply(t, s, `
entities:
  bowie-knife:
    location: saloon
    spot: buried in the bar top
`)

	item, err := s.world.ItemByID("bowie-knife")
	if err != nil {
		t.Fatalf("the knife was lost in the move: %v", err)
	}
	holder, _ := s.world.ItemHolder("bowie-knife")
	if holder != "saloon" {
		t.Errorf("the knife is held by %q, want saloon", holder)
	}
	if item.Weapon == nil || item.Weapon.Damage != 2 {
		t.Errorf("the knife lost its rating in the move: %+v", item.Weapon)
	}
	if item.Where != "buried in the bar top" {
		t.Errorf("spot = %q", item.Where)
	}
}

// count: 0 is the one spelling for a thing ending, and it retires rather than
// erases: the id is spent forever so the archive's references keep meaning what
// they meant.
func TestCountZeroRetiresTheItemAndSpendsItsID(t *testing.T) {
	s := stateSession(t)
	apply(t, s, `
entities:
  cartridges:
    type: item
    name: Cartridges
    location: kyle-katarn
    count: 6
`)

	apply(t, s, "entities:\n  cartridges:\n    count: 2\n")
	item, _ := s.world.ItemByID("cartridges")
	if item.Count != 2 {
		t.Fatalf("count = %d, want 2", item.Count)
	}

	apply(t, s, "entities:\n  cartridges:\n    count: 0\n")
	if _, err := s.world.ItemByID("cartridges"); err == nil {
		t.Error("the empty stack is still in the world")
	}

	notes := refusals(t, s, `
entities:
  cartridges:
    type: item
    name: More Cartridges
    location: kyle-katarn
`)
	if !strings.Contains(notes, "never reused") {
		t.Errorf("a spent id was handed out again: %s", notes)
	}
}

// A purse is stated, not adjusted. The engine works out the payment and refuses
// a total it cannot reach.
func TestCoinsAreStatedAsATotal(t *testing.T) {
	s := stateSession(t)
	apply(t, s, "entities:\n  kyle-katarn:\n    coins: 40\n")
	pc, _ := s.world.Character("kyle-katarn")
	if pc.Coins != 40 {
		t.Fatalf("coins = %d, want 40", pc.Coins)
	}

	apply(t, s, "entities:\n  kyle-katarn:\n    coins: 15\n")
	if pc.Coins != 15 {
		t.Fatalf("coins = %d, want 15", pc.Coins)
	}

	if notes := refusals(t, s, "entities:\n  kyle-katarn:\n    coins: -5\n"); !strings.Contains(notes, "coins") {
		t.Errorf("a negative purse was not refused: %s", notes)
	}
}

// The one open namespace. Everything else the DM writes is a known field.
func TestNarrativePropertiesMergeOntoAnyEntity(t *testing.T) {
	s := stateSession(t)

	apply(t, s, `
entities:
  kyle-katarn:
    narrative: {mood: wary, owes: the barkeep}
  broken-oath:
    type: oath
    narrative: {sworn_to: kyle-katarn}
`)

	apply(t, s, "entities:\n  kyle-katarn:\n    narrative: {mood: furious}\n")

	props := s.narrative.Get("kyle-katarn").Props()
	found := map[string]any{}
	for _, p := range props {
		found[p.Name] = p.Value
	}
	if found["mood"] != "furious" {
		t.Errorf("mood = %v, want furious", found["mood"])
	}
	if found["owes"] != "the barkeep" {
		t.Errorf("an unmentioned property was lost: %v", found)
	}
	if s.narrative.Get("broken-oath") == nil {
		t.Error("the narrative-only entity was not created")
	}
}

// The closed field set is what replaces the old denylist: a word nobody thought
// to forbid is refused because it is not a field, not because it was listed.
func TestInventedFieldsAreRefused(t *testing.T) {
	s := stateSession(t)

	notes := refusals(t, s, "entities:\n  kyle-katarn:\n    current_room: saloon\n")
	if !strings.Contains(notes, "current_room") || !strings.Contains(notes, "location") {
		t.Errorf("the refusal should name the invention and the real fields: %s", notes)
	}
}

// A derived field is not a mistaken word but a mistaken direction, so it is
// answered with the fact to state instead.
func TestDerivedFieldsAreRefusedWithTheFactToStateInstead(t *testing.T) {
	s := stateSession(t)

	for field, want := range map[string]string{
		"inventory": "move the item",
		"here":      "move the character",
		"penalty":   "follows from the wound",
	} {
		notes := refusals(t, s, "entities:\n  kyle-katarn:\n    "+field+": [a]\n")
		if !strings.Contains(notes, want) {
			t.Errorf("%s: refusal was %q, want it to mention %q", field, notes, want)
		}
	}
}

// Health under-determines its own outcome, so it has no field at all — and the
// refusal points at the tags that do work.
func TestHealthCannotBeWrittenAsState(t *testing.T) {
	s := stateSession(t)

	notes := refusals(t, s, "entities:\n  kyle-katarn:\n    wounds: hurt\n")
	if !strings.Contains(notes, "<damage") {
		t.Errorf("the refusal should point at the event tag: %s", notes)
	}
	pc, _ := s.world.Character("kyle-katarn")
	if pc.Wounds.Level != 0 {
		t.Errorf("the wound was written anyway: %v", pc.Wounds.Level)
	}
}

// An id the model minted is held to the shape it reads everywhere else.
func TestMalformedIDsAreRefused(t *testing.T) {
	s := stateSession(t)

	for _, id := range []string{"Iron Key", "-leading", "double--hyphen", "trailing-"} {
		notes := refusals(t, s, "entities:\n  \""+id+"\":\n    type: location\n    name: A Place\n")
		if !strings.Contains(notes, "usable id") {
			t.Errorf("%q was not refused as an id: %s", id, notes)
		}
	}
}

// One id means one entity, across engine and narrative alike.
func TestIDCollisionsAreRefused(t *testing.T) {
	s := stateSession(t)

	notes := refusals(t, s, `
entities:
  saloon:
    type: character
    name: Somebody Else
    species: human
`)
	if !strings.Contains(notes, "saloon") {
		t.Errorf("a taken id was not refused: %s", notes)
	}
	if _, err := s.world.Location("saloon"); err != nil {
		t.Error("the existing location was clobbered")
	}
}

// A refused entity takes its dependents with it rather than leaving them
// half-applied against something that does not exist.
func TestDependentsOfARefusedEntityAreSkipped(t *testing.T) {
	s := stateSession(t)

	notes := refusals(t, s, `
entities:
  drifter:
    type: character
    name: The Drifter
    species: jackalope
    location: saloon
  drifters-hat:
    type: item
    name: Battered Hat
    location: drifter
  kyle-katarn:
    coins: 12
`)

	if !strings.Contains(notes, "drifters-hat") {
		t.Errorf("the dependent item was not reported as skipped: %s", notes)
	}
	if _, err := s.world.ItemByID("drifters-hat"); err == nil {
		t.Error("the hat landed on a character who was never created")
	}
	// The lines that were right still applied.
	pc, _ := s.world.Character("kyle-katarn")
	if pc.Coins != 12 {
		t.Errorf("coins = %d — one bad entity lost a good one", pc.Coins)
	}
}

// Engine entities are never deleted. The refusal says what to do instead.
func TestDeleteRefusesEngineEntities(t *testing.T) {
	s := stateSession(t)

	notes := refusals(t, s, "entities: {}\ndelete: [kyle-katarn]\n")
	if !strings.Contains(notes, "count: 0") {
		t.Errorf("the refusal should name the real ending: %s", notes)
	}
	if _, err := s.world.Character("kyle-katarn"); err != nil {
		t.Error("a character was deleted")
	}
}

func TestDeleteRetiresNarrativeEntities(t *testing.T) {
	s := stateSession(t)
	apply(t, s, "entities:\n  bar-rumor:\n    type: rumor\n    narrative: {about: the mine}\n")

	apply(t, s, "entities: {}\ndelete: [bar-rumor]\n")

	if s.narrative.Has("bar-rumor") {
		t.Error("the rumor is still in the store")
	}
	if !s.world.Retired["bar-rumor"] {
		t.Error("the id was freed rather than retired")
	}
}

// Traits merge field by field against the setting; a trait the setting does not
// have is refused rather than invented onto the sheet the dice read from.
func TestTraitsMergeAndAreCheckedAgainstTheSetting(t *testing.T) {
	s := stateSession(t)

	apply(t, s, "entities:\n  kyle-katarn:\n    skills: {shootin: 2}\n")
	pc, _ := s.world.Character("kyle-katarn")
	if pc.Skills["shootin"] != 2 {
		t.Fatalf("shootin = %d, want 2", pc.Skills["shootin"])
	}
	if pc.Skills["fightin"] != -2 {
		t.Errorf("an unmentioned skill changed: %d", pc.Skills["fightin"])
	}

	if notes := refusals(t, s, "entities:\n  kyle-katarn:\n    skills: {podracing: 3}\n"); !strings.Contains(notes, "podracing") {
		t.Errorf("an invented skill was accepted: %s", notes)
	}
	if notes := refusals(t, s, "entities:\n  kyle-katarn:\n    skills: {shootin: 9}\n"); !strings.Contains(notes, "-3 and 3") {
		t.Errorf("an out-of-range trait was accepted: %s", notes)
	}
}

// A block the engine cannot read at all is one note, not silence.
func TestUnreadableBlockIsReported(t *testing.T) {
	s := stateSession(t)
	if notes := refusals(t, s, "entities:\n  - this is a list\n"); !strings.Contains(notes, "entities") {
		t.Errorf("note = %s", notes)
	}
	if notes := refusals(t, s, "characters:\n  kyle-katarn: {}\n"); !strings.Contains(notes, "entities") {
		t.Errorf("a top-level key that is not `entities` should be refused: %s", notes)
	}
}

// Health moves through the tags, and the engine works out what the wound comes
// to — which is the reason it is a tag and not a field.
func TestEventTagsMoveHealth(t *testing.T) {
	s := stateSession(t)
	pc, _ := s.world.Character("kyle-katarn")

	_, notes := s.applyEvents([]stateEvent{{Kind: "damage", Target: "kyle-katarn", Severity: "hurt"}})
	if len(notes) > 0 {
		t.Fatalf("refused: %v", notes)
	}
	if pc.Wounds.Level != rules.Hurt {
		t.Fatalf("wounds = %v, want Hurt", pc.Wounds.Level)
	}

	if _, notes := s.applyEvents([]stateEvent{{Kind: "heal", Target: "kyle-katarn", Levels: 1}}); len(notes) > 0 {
		t.Fatalf("refused: %v", notes)
	}
	if pc.Wounds.Level != rules.Undamaged {
		t.Errorf("wounds = %v, want Undamaged", pc.Wounds.Level)
	}

	if _, notes := s.applyEvents([]stateEvent{{Kind: "dead", Target: "kyle-katarn"}}); len(notes) > 0 {
		t.Fatalf("refused: %v", notes)
	}
	if pc.Wounds.Level != rules.Dead {
		t.Errorf("wounds = %v, want Dead", pc.Wounds.Level)
	}
}

// Scratches accumulate, and accumulation is the whole reason the DM cannot
// simply declare the resulting level.
func TestRepeatedScratchesAccumulateThroughTheTags(t *testing.T) {
	s := stateSession(t)
	pc, _ := s.world.Character("kyle-katarn")

	for range 3 {
		if _, notes := s.applyEvents([]stateEvent{{Kind: "damage", Target: "kyle-katarn", Severity: "scratch"}}); len(notes) > 0 {
			t.Fatalf("refused: %v", notes)
		}
	}
	if pc.Wounds.Level != rules.Hurt {
		t.Errorf("three scratches came to %v, want Hurt", pc.Wounds.Level)
	}
}

func TestEventTagsReportAnUnknownTarget(t *testing.T) {
	s := stateSession(t)
	_, notes := s.applyEvents([]stateEvent{{Kind: "damage", Target: "nobody", Severity: "hurt"}})
	if len(notes) != 1 || !strings.Contains(notes[0], "nobody") {
		t.Errorf("notes = %v", notes)
	}
}

// The archive is indexed exactly as it was when every change was a tool call:
// the applier logs the operation it derived, under the same vocabulary recall
// already searches.
func TestAppliedChangesReachTheArchiveAsOperations(t *testing.T) {
	s := stateSession(t)
	events, notes := s.applyState(`
entities:
  cellar:
    type: location
    name: The Cellar
  kyle-katarn:
    location: cellar
`)
	if len(notes) > 0 {
		t.Fatalf("refused: %v", notes)
	}

	seen := map[string]bool{}
	for _, e := range events {
		seen[e.Tool] = true
		if len(e.Entities) == 0 {
			t.Errorf("event %q names no entity, so recall cannot find it", e.Tool)
		}
	}
	for _, op := range []string{"create_location", "move_character"} {
		if !seen[op] {
			t.Errorf("no %s event was recorded: %v", op, seen)
		}
	}
}
