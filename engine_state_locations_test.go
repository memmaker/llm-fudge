package main

import (
	"strings"
	"testing"

	"fudge/narrative"
	"fudge/rules"
)

// mapWorld lays out a tavern with a cellar under it and an alley outside, with
// the player down in the cellar.
func mapWorld(t *testing.T) *rules.World {
	t.Helper()
	w := rosterWorld(t)
	if _, err := w.CreateLocation("Tavern", "Low beams, louder than it looks.", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := w.CreateLocation("Cellar", "Damp, stacked with casks.", "tavern"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.CreateLocation("Alley", "Wet brick, no lamps.", ""); err != nil {
		t.Fatal(err)
	}
	w.Characters[w.Player].Location = "cellar"
	return w
}

// place puts an item on the floor of a location, minting its id the way the tool
// path does. Every item needs one now: an unminted proto is refused, because an
// item nobody assigned an id to could not be addressed by a later call.
func place(t *testing.T, w *rules.World, locationID string, proto *rules.Item, where string, count int) *rules.Item {
	t.Helper()
	proto.ID = w.MintItemID(proto.Name)
	item, err := w.Locations[locationID].AddItem(proto, where, count)
	if err != nil {
		t.Fatal(err)
	}
	return item
}

// rendered is the flat entity map, which is what the DM actually reads.
func rendered(w *rules.World) string {
	return entities(w, narrative.New())
}

// The DM can only name a location the engine already holds, so every slug has
// to be readable off the block or it will guess and be refused.
func TestEveryLocationIsAnEntity(t *testing.T) {
	got := rendered(mapWorld(t))

	for _, id := range []string{"tavern", "cellar", "alley"} {
		if !strings.Contains(got, "\n  "+id+":\n") {
			t.Errorf("%s missing from the entity map:\n%s", id, got)
		}
	}
}

// Nesting is a reference, not a shape. The parent link is still held and still
// rendered — it is how the DM knows a cellar is under a tavern — but a location
// is an entity beside every other entity rather than a branch inside one, which
// is what stops a thing from having two addresses.
func TestNestingIsAReferenceNotAShape(t *testing.T) {
	got := rendered(mapWorld(t))

	if !strings.Contains(got, "parent: tavern") {
		t.Errorf("the cellar does not reference the tavern it is under:\n%s", got)
	}
	// Tree order still decides where a place is listed, so a child reads next to
	// its parent even though it is not inside it.
	tavernAt := strings.Index(got, "\n  tavern:\n")
	cellarAt := strings.Index(got, "\n  cellar:\n")
	if tavernAt < 0 || cellarAt < 0 {
		t.Fatalf("map is missing a location:\n%s", got)
	}
	if cellarAt < tavernAt {
		t.Errorf("the cellar should be listed after the tavern it is under:\n%s", got)
	}
}

// The scene is expanded because it is the one place the DM is writing about:
// what it is and who is in it.
func TestSceneIsMarkedAndExpanded(t *testing.T) {
	w := mapWorld(t)
	got := rendered(w)

	if !strings.Contains(got, "scene: true") {
		t.Errorf("the current location is not marked:\n%s", got)
	}
	if !strings.Contains(got, "Damp, stacked with casks.") {
		t.Errorf("the scene's description is missing:\n%s", got)
	}
	if !strings.Contains(got, "here: [kyle-katarn]") {
		t.Errorf("who is present is missing:\n%s", got)
	}
	// A room nobody is standing in keeps its description to itself: repeating
	// every description every turn would grow the block with the map.
	if strings.Contains(got, "Wet brick, no lamps.") {
		t.Errorf("only the scene expands its description:\n%s", got)
	}
}

// An item two rooms away is still an item the DM has to be able to reason
// about — "where did I leave the rope" is the question this answers — and it is
// answered by the item's own entity, not by a list under the room.
func TestItemsAreEntitiesThatReferenceTheirHolder(t *testing.T) {
	w := mapWorld(t)
	place(t, w, "alley", &rules.Item{Name: "Rope", Description: "Forty feet."}, "coiled by the gate", 2)
	place(t, w, "cellar", &rules.Item{Name: "Dagger", Weapon: &rules.Weapon{Damage: 1}}, "in the door frame", 1)
	got := rendered(w)

	if !strings.Contains(got, "\n  rope:\n") {
		t.Errorf("the rope is not an entity of its own:\n%s", got)
	}
	if !strings.Contains(got, "container: alley") {
		t.Errorf("the rope does not reference the alley it lies in:\n%s", got)
	}
	if !strings.Contains(got, "coiled by the gate") {
		t.Errorf("the spot the rope rests in is missing:\n%s", got)
	}
	if !strings.Contains(got, "count: 2") {
		t.Errorf("the rope's count is missing:\n%s", got)
	}
	// Ratings ride along, so a weapon on the floor can be picked up and swung
	// without the DM having to remember what it hit for.
	if !strings.Contains(got, "damage: +1") {
		t.Errorf("the dagger's rating is missing:\n%s", got)
	}
}

// An item's id is its own, so a spot whose prose happens to contain another
// entity's slug cannot be mistaken for it. Under the old key this was a real
// hazard — identity was built out of the name and the spot — and it is why the
// renderer tracks what it has written rather than searching its own output.
func TestALocationIsNotHiddenByItsSlugAppearingInProse(t *testing.T) {
	w := mapWorld(t)
	if _, err := w.CreateLocation("Bar", "One long counter.", ""); err != nil {
		t.Fatal(err)
	}
	place(t, w, "alley", &rules.Item{Name: "Crate", Description: "Pine."}, "stacked against the bar", 1)

	got := rendered(w)
	if !strings.Contains(got, "\n  bar:\n") {
		t.Errorf("the bar is missing from the entity map:\n%s", got)
	}
	if strings.Contains(got, "is missing") {
		t.Errorf("a rendered location was reported as an orphan:\n%s", got)
	}
}

// A location whose parent went missing would vanish from a walk that only ever
// descends from the roots, and a slug the DM cannot see is a slug it will be
// refused for naming.
func TestAnOrphanedLocationStillRenders(t *testing.T) {
	w := mapWorld(t)
	w.Locations["cellar"].Parent = "nowhere"

	if got := rendered(w); !strings.Contains(got, "\n  cellar:\n") {
		t.Errorf("an orphaned location fell out of the map entirely:\n%s", got)
	}
}

func TestAnEmptyWorldRendersSomething(t *testing.T) {
	w := &rules.World{Characters: map[string]*rules.Character{}, Locations: map[string]*rules.Location{}}
	if got := entities(w, narrative.New()); strings.TrimSpace(got) == "" {
		t.Error("an empty world must still say so, or the block reads as a missing section")
	}
}

// The map is re-sent every turn, so its order has to be fixed or the cached
// prefix is rewritten for nothing.
func TestEntityRenderIsDeterministic(t *testing.T) {
	w := mapWorld(t)
	place(t, w, "alley", &rules.Item{Name: "Rope"}, "coiled by the gate", 2)
	place(t, w, "cellar", &rules.Item{Name: "Dagger", Weapon: &rules.Weapon{Damage: 1}}, "in the door frame", 1)

	first := rendered(w)
	for i := 0; i < 5; i++ {
		if got := rendered(w); got != first {
			t.Fatalf("render %d differs:\n%s\n---\n%s", i, first, got)
		}
	}
}
