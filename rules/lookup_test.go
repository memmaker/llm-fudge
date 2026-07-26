package rules

import (
	"math/rand"
	"strings"
	"testing"
)

// scene is a world holding the characters a lookup has to pick between.
func scene(names ...string) *World {
	w := &World{Characters: map[string]*Character{}, Rng: rand.New(rand.NewSource(1))}
	for _, name := range names {
		w.CreateCharacter("", name, map[string]int{"grit": 1}, map[string]int{"brawlin": 1})
	}
	return w
}

// Ids are exact, so a miss has to be correctable. The DM writes its next response
// from this error and nothing else, which means the error carries the roster: a
// bare "not found" leaves it guessing at the same wrong id again.
func TestCharacterNotFoundListsTheRoster(t *testing.T) {
	w := scene("Catnip", "Grimwald the Smith")

	if _, err := w.Character("grimwald"); err == nil {
		t.Fatal("a shortened id must not resolve")
	}

	_, err := w.Character("grimwald")
	for _, want := range []string{"catnip", "grimwald-the-smith"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error must list %s: %v", want, err)
		}
	}

	// An empty world has no roster to offer, so it says what to do instead.
	_, err = scene().Character("mayor")
	if err == nil {
		t.Fatal("an unknown character must not resolve")
	}
	if !strings.Contains(err.Error(), "create") {
		t.Errorf("an empty world must point at creation: %v", err)
	}
}

// A check rolls the character the lookup returned. The two used to be separate —
// validation resolved the character and then the roll indexed the map again with
// the caller's string — and only one of them can be the source of truth.
func TestChecksRunOnTheLookedUpCharacter(t *testing.T) {
	w := scene("Grimwald the Smith", "Catnip")
	smith, err := w.Character("grimwald-the-smith")
	if err != nil {
		t.Fatal(err)
	}
	smith.FudgePoints = 1

	if _, err := w.UnopposedCheck("grimwald-the-smith", TraitTypeSkill, "brawlin", 0, 0, true, false); err != nil {
		t.Fatalf("unopposed check: %v", err)
	}
	if smith.FudgePoints != 0 {
		t.Errorf("fudge points = %d, want the point spent on the character who rolled", smith.FudgePoints)
	}

	if _, err := w.OpposedCheck("catnip", TraitTypeSkill, "brawlin", 0, "grimwald-the-smith", TraitTypeAttribute, "grit", 0, false); err != nil {
		t.Fatalf("opposed check: %v", err)
	}
	if _, err := w.OpposedCheck("catnip", TraitTypeSkill, "brawlin", 0, "grimwald", TraitTypeAttribute, "grit", 0, false); err == nil {
		t.Fatal("an opponent id that does not exist must fail the check")
	}
}
