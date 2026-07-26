package main

import (
	"strings"
	"testing"

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

// The DM can only name a location the engine already holds, so every slug has
// to be readable off the block or it will guess and be refused.
func TestLocationsListEverySlug(t *testing.T) {
	w := mapWorld(t)
	got := locations(w)

	for _, id := range []string{"tavern", "cellar", "alley"} {
		if !strings.Contains(got, id) {
			t.Errorf("%s missing from the map:\n%s", id, got)
		}
	}
}

// Nesting is rendered as nesting: a child sits under its parent and indented
// past it, which is the whole return on holding a parent link at all.
func TestLocationsNestUnderTheirParent(t *testing.T) {
	w := mapWorld(t)
	got := locations(w)

	lines := strings.Split(got, "\n")
	var tavernAt, cellarAt = -1, -1
	for i, line := range lines {
		if strings.Contains(line, "tavern|") {
			tavernAt = i
		}
		if strings.Contains(line, "cellar|") {
			cellarAt = i
		}
	}
	if tavernAt < 0 || cellarAt < 0 {
		t.Fatalf("map is missing a location:\n%s", got)
	}
	// The child follows its parent immediately and sits further in. Where the
	// alley falls is the sort's business, not the tree's.
	if cellarAt != tavernAt+1 {
		t.Errorf("the cellar should follow the tavern it is under:\n%s", got)
	}
	if indent(lines[cellarAt]) <= indent(lines[tavernAt]) {
		t.Errorf("the cellar should be indented past the tavern:\n%s", got)
	}
}

func indent(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

// The scene is expanded because it is the one place the DM is writing about:
// what it is, who is in it, what lies about.
func TestSceneIsMarkedAndExpanded(t *testing.T) {
	w := mapWorld(t)
	cellar := w.Locations["cellar"]
	cellar.AddItem(&rules.Item{Name: "Dagger", Description: "Notched.", Weapon: &rules.Weapon{Damage: 1}}, "quivering in the door frame", 1)
	got := locations(w)

	if !strings.Contains(got, "SCENE") {
		t.Errorf("the current location is not marked:\n%s", got)
	}
	if !strings.Contains(got, "Damp, stacked with casks.") {
		t.Errorf("the scene's description is missing:\n%s", got)
	}
	if !strings.Contains(got, "Kyle Katarn") {
		t.Errorf("who is present is missing:\n%s", got)
	}
	// A room nobody is standing in keeps its description to itself.
	if strings.Contains(got, "Wet brick, no lamps.") {
		t.Errorf("only the scene expands its description:\n%s", got)
	}
}

// An item two rooms away is still an item the DM has to be able to reason
// about — "where did I leave the rope" is the question the map answers.
func TestItemsRenderAtEveryLocation(t *testing.T) {
	w := mapWorld(t)
	w.Locations["alley"].AddItem(&rules.Item{Name: "Rope", Description: "Forty feet."}, "coiled by the gate", 2)
	w.Locations["cellar"].AddItem(&rules.Item{Name: "Dagger", Weapon: &rules.Weapon{Damage: 1}}, "in the door frame", 1)
	got := locations(w)

	if !strings.Contains(got, "Rope") || !strings.Contains(got, "coiled by the gate") {
		t.Errorf("the rope in the alley is missing:\n%s", got)
	}
	// Ratings ride along, so a weapon on the floor can be picked up and swung
	// without the DM having to remember what it hit for.
	if !strings.Contains(got, "dmg+1") {
		t.Errorf("the dagger's rating is missing:\n%s", got)
	}
}

// Every slug has to appear, and whether one did is tracked rather than searched
// for in the rendered text — a crate resting "on the bar" would otherwise pass
// for the location `bar` and suppress the real one.
func TestALocationIsNotHiddenByItsSlugAppearingInProse(t *testing.T) {
	w := mapWorld(t)
	if _, err := w.CreateLocation("Bar", "One long counter.", ""); err != nil {
		t.Fatal(err)
	}
	w.Locations["alley"].AddItem(&rules.Item{Name: "Crate", Description: "Pine."}, "stacked against the bar|side", 1)

	got := locations(w)
	if !strings.Contains(got, "bar|Bar") {
		t.Errorf("the bar is missing from the map:\n%s", got)
	}
	if strings.Contains(got, "parent  is missing") {
		t.Errorf("a rendered location was reported as an orphan:\n%s", got)
	}
}

func TestLocationsBeforeAnyExistIsNotBlank(t *testing.T) {
	w := rosterWorld(t)
	if got := locations(w); strings.TrimSpace(got) == "" {
		t.Error("an empty map must still say so")
	}
}

// The map is re-sent every turn, so its order has to be fixed or the cached
// prefix is rewritten for nothing.
func TestLocationsAreDeterministic(t *testing.T) {
	w := mapWorld(t)
	first := locations(w)
	for i := 0; i < 5; i++ {
		if got := locations(w); got != first {
			t.Fatalf("render %d differs:\n%s\n---\n%s", i, first, got)
		}
	}
}

// The block is the DM's whole world, so a location section has to be in it.
func TestEngineStateCarriesTheMap(t *testing.T) {
	w := mapWorld(t)
	s := newSession(w)
	if got := s.engineState(); !strings.Contains(got, "LOCATIONS") {
		t.Errorf("the block has no LOCATIONS section:\n%s", got)
	}
}
