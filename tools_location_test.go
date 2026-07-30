package main

import (
	"strings"
	"testing"

	"fudge/rules"
)

func placedWorld(t *testing.T) *rules.World {
	t.Helper()
	w := equippedWorld(t)
	if _, err := w.CreateLocation("Cellar", "Damp, stacked with casks.", ""); err != nil {
		t.Fatal(err)
	}
	return w
}

// An item can be created straight onto the ground: a crate already in the
// cellar, gear on a body, a sword in a stone.
func TestAddItemCanCreateAtALocation(t *testing.T) {
	w := placedWorld(t)

	got, err := executeAction(w, inventoryCall("add_item", `{"location":"cellar","location_description":"under the stair","name":"Crowbar","description":"Rusted but sound."}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "under the stair") {
		t.Errorf("result = %q, want it to state the spot", got)
	}
	if len(w.Locations["cellar"].Items) != 1 {
		t.Fatalf("the cellar holds %d entries, want 1", len(w.Locations["cellar"].Items))
	}
	// And it did not quietly land in the player's pockets, which is where a
	// missing character_id would otherwise send it.
	pc, _ := w.Character("kyle-katarn")
	if len(pc.Inventory) != 0 {
		t.Errorf("the crowbar reached the player's sheet: %+v", pc.Inventory)
	}
}

// A location destination needs its spot, or the engine would be recording an
// item as lying nowhere in particular.
func TestAddItemAtALocationNeedsASpot(t *testing.T) {
	w := placedWorld(t)

	if _, err := executeAction(w, inventoryCall("add_item", `{"location":"cellar","name":"Crowbar","description":"Rusted."}`)); err == nil {
		t.Fatal("a location destination without a spot must be refused")
	}
}

func TestItemToolsRefuseTwoHolders(t *testing.T) {
	w := placedWorld(t)

	if _, err := executeAction(w, inventoryCall("add_item", `{"character_id":"kyle-katarn","location":"cellar","location_description":"here","name":"Crowbar","description":"Rusted."}`)); err == nil {
		t.Fatal("naming both a character and a location must be refused")
	}
}

// The whole round trip the design is for, now stated rather than called for: a
// knife is thrown, is given the place the prose left it, and comes back with its
// ratings when it is picked up again. Two declarations, and the engine works out
// that both of them are moves.
func TestAThrownKnifeSurvivesTheRoundTrip(t *testing.T) {
	s := newSession(placedWorld(t))
	pc, _ := s.world.Character("kyle-katarn")
	s.world.GiveWeapon(pc, "Throwing Knife", "Balanced for the point.", 1, rules.Weapon{Damage: 1})

	apply(t, s, `
entities:
  throwing-knife:
    location: cellar
    spot: quivering in the door frame
`)
	if _, held := pc.Inventory["throwing-knife"]; held {
		t.Error("the knife should have left the sheet")
	}

	apply(t, s, "entities:\n  throwing-knife:\n    location: kyle-katarn\n")

	knife, err := pc.WeaponItem("Throwing Knife")
	if err != nil {
		t.Fatal(err)
	}
	if knife.Weapon.Damage != 1 || knife.Description != "Balanced for the point." {
		t.Errorf("the knife came back changed: %+v", knife)
	}
	// A carried thing lies nowhere, so the spot it had on the floor is gone.
	if knife.Where != "" {
		t.Errorf("the knife is still recorded as lying %q", knife.Where)
	}
}

// A weapon created on the ground keeps its ratings, so it can be picked up and
// swung without the DM restating what it hits for.
func TestAWeaponCreatedOnTheGroundKeepsItsRatings(t *testing.T) {
	s := newSession(placedWorld(t))

	apply(t, s, `
entities:
  cavalry-sabre:
    type: item
    name: Cavalry Sabre
    desc: Notched.
    location: cellar
    spot: propped by the casks
    weapon: {damage: 2}
`)
	apply(t, s, "entities:\n  cavalry-sabre:\n    location: kyle-katarn\n")

	pc, _ := s.world.Character("kyle-katarn")
	sabre, err := pc.WeaponItem("Cavalry Sabre")
	if err != nil {
		t.Fatal(err)
	}
	if sabre.Weapon.Damage != 2 {
		t.Errorf("damage = %d, want the rating it was created with", sabre.Weapon.Damage)
	}
}

// Moving somebody is a declaration like any other, and an unknown destination is
// refused with the places that do exist.
func TestDeclaringACharactersLocationMovesThem(t *testing.T) {
	s := newSession(placedWorld(t))
	s.world.CreateCharacter("mira", "Mira", nil, nil)

	apply(t, s, "entities:\n  mira:\n    location: cellar\n")
	if s.world.Characters["mira"].Location != "cellar" {
		t.Errorf("mira is at %q, want cellar", s.world.Characters["mira"].Location)
	}

	notes := refusals(t, s, "entities:\n  mira:\n    location: attic\n")
	if !strings.Contains(notes, "cellar") {
		t.Errorf("notes = %v, want the locations that exist", notes)
	}
}
