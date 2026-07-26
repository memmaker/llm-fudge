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

// The three calls that hold the map together have to be on the table during
// play, and nowhere else: there is no world to place anything in while a
// character is still being rolled up.
func TestMapToolsAreOfferedDuringPlayOnly(t *testing.T) {
	inPlay := map[string]bool{}
	for _, tool := range gameplayTools() {
		inPlay[tool.Function.Name] = true
	}
	atCreation := map[string]bool{}
	for _, tool := range characterCreationTools() {
		atCreation[tool.Function.Name] = true
	}

	for _, name := range []string{"create_location", "move_item", "move_character"} {
		if !inPlay[name] {
			t.Errorf("gameplay does not offer %s", name)
		}
		if atCreation[name] {
			t.Errorf("character creation should not offer %s", name)
		}
	}
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

// A weapon created on the ground keeps its ratings, so it can be picked up and
// swung without the DM restating what it hits for.
func TestAddWeaponAtALocationKeepsItsRatings(t *testing.T) {
	w := placedWorld(t)

	if _, err := executeAction(w, inventoryCall("add_weapon", `{"location":"cellar","location_description":"propped by the casks","name":"Cavalry Sabre","description":"Notched.","damage":2}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := executeAction(w, inventoryCall("move_item", `{"item":"Cavalry Sabre","to_character":"kyle-katarn"}`)); err != nil {
		t.Fatal(err)
	}

	pc, _ := w.Character("kyle-katarn")
	sabre, err := pc.WeaponItem("Cavalry Sabre")
	if err != nil {
		t.Fatal(err)
	}
	if sabre.Weapon.Damage != 2 {
		t.Errorf("damage = %d, want the rating it was created with", sabre.Weapon.Damage)
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

// The whole round trip the design is for: a knife is thrown, stays on the
// sheet until the DM says where it went, is moved there, and comes back with
// its ratings when it is retrieved.
func TestAThrownKnifeSurvivesTheRoundTrip(t *testing.T) {
	w := placedWorld(t)
	pc, _ := w.Character("kyle-katarn")
	pc.AddWeapon("Throwing Knife", "Balanced for the point.", 1, rules.Weapon{Damage: 1})

	if _, err := executeAction(w, inventoryCall("move_item", `{"item":"Throwing Knife","to_location":"cellar","location_description":"quivering in the door frame"}`)); err != nil {
		t.Fatal(err)
	}
	if _, held := pc.Inventory["throwing-knife"]; held {
		t.Error("the knife should have left the sheet")
	}

	got, err := executeAction(w, inventoryCall("move_item", `{"item":"Throwing Knife","to_character":"kyle-katarn"}`))
	if err != nil {
		t.Fatal(err)
	}
	knife, err := pc.WeaponItem("Throwing Knife")
	if err != nil {
		t.Fatal(err)
	}
	if knife.Weapon.Damage != 1 || knife.Description != "Balanced for the point." {
		t.Errorf("the knife came back changed: %+v", knife)
	}
	// The receiving sheet rides on the result, as every mutating call's does.
	if !strings.Contains(got, "Kyle Katarn|id:kyle-katarn") {
		t.Errorf("result = %q, want the receiving character's state", got)
	}
}

// remove_item destroys. It must not be reachable as a way of moving something,
// which is why its own description says so — but the location half has to work
// too, for a crate that burns.
func TestRemoveItemDestroysAtALocation(t *testing.T) {
	w := placedWorld(t)
	if _, err := executeAction(w, inventoryCall("add_item", `{"location":"cellar","location_description":"by the door","name":"Crate","description":"Pine, banded."}`)); err != nil {
		t.Fatal(err)
	}

	if _, err := executeAction(w, inventoryCall("remove_item", `{"location":"cellar","location_description":"by the door","name":"Crate"}`)); err != nil {
		t.Fatal(err)
	}
	if len(w.Locations["cellar"].Items) != 0 {
		t.Error("the crate survived being destroyed")
	}
}

func TestMoveCharacterPutsAnNPCSomewhere(t *testing.T) {
	w := placedWorld(t)
	w.CreateCharacter("mira", "Mira", nil, nil)

	got, err := executeAction(w, inventoryCall("move_character", `{"character_id":"mira","location":"cellar"}`))
	if err != nil {
		t.Fatal(err)
	}
	if w.Characters["mira"].Location != "cellar" {
		t.Errorf("mira is at %q, want cellar", w.Characters["mira"].Location)
	}
	if !strings.Contains(got, "cellar") {
		t.Errorf("result = %q, want it to state where they are now", got)
	}
}

func TestMoveCharacterRefusesAnUnknownLocation(t *testing.T) {
	w := placedWorld(t)
	w.CreateCharacter("mira", "Mira", nil, nil)

	_, err := executeAction(w, inventoryCall("move_character", `{"character_id":"mira","location":"attic"}`))
	if err == nil {
		t.Fatal("an unknown location must be refused")
	}
	if !strings.Contains(err.Error(), "cellar") {
		t.Errorf("err = %v, want it to list the locations that exist", err)
	}
}

func TestCreateLocationReportsItsID(t *testing.T) {
	w := placedWorld(t)

	got, err := executeAction(w, inventoryCall("create_location", `{"name":"The Wine Cellar","description":"Deeper still.","parent":"cellar"}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"id:the-wine-cellar", "in:cellar"} {
		if !strings.Contains(got, want) {
			t.Errorf("result = %q, want it to carry %q", got, want)
		}
	}
}
