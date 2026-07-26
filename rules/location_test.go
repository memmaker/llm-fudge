package rules

import (
	"strings"
	"testing"
)

func locationWorld() *World {
	return &World{Characters: map[string]*Character{}, Locations: map[string]*Location{}}
}

// A location is minted from its name the same way a character is, so the DM can
// read an id off a result instead of deriving one.
func TestCreateLocationMintsASlug(t *testing.T) {
	w := locationWorld()

	loc, err := w.CreateLocation("The Wine Cellar", "Damp, low-ceilinged, stacked with casks.", "")
	if err != nil {
		t.Fatal(err)
	}
	if loc.ID != "the-wine-cellar" {
		t.Errorf("id = %q, want the-wine-cellar", loc.ID)
	}
	if w.Locations["the-wine-cellar"] != loc {
		t.Error("the location is not in the world under its id")
	}
}

// Two rooms with one name would give the DM two slugs for one place, which is
// the drift the registry exists to stop.
func TestCreateLocationRefusesDuplicates(t *testing.T) {
	w := locationWorld()
	if _, err := w.CreateLocation("Cellar", "Damp.", ""); err != nil {
		t.Fatal(err)
	}

	_, err := w.CreateLocation("cellar", "Damp again.", "")
	if err == nil {
		t.Fatal("a duplicate slug must be refused")
	}
	if !strings.Contains(err.Error(), "cellar") {
		t.Errorf("err = %v, want it to name the slug", err)
	}
}

// A parent must already exist. Requiring it is also what makes a cycle
// impossible: a location can only point at one that was created before it.
func TestCreateLocationRequiresAnExistingParent(t *testing.T) {
	w := locationWorld()

	if _, err := w.CreateLocation("Cellar", "Damp.", "tavern"); err == nil {
		t.Fatal("an unknown parent must be refused")
	}

	if _, err := w.CreateLocation("Tavern", "Loud.", ""); err != nil {
		t.Fatal(err)
	}
	cellar, err := w.CreateLocation("Cellar", "Damp.", "tavern")
	if err != nil {
		t.Fatal(err)
	}
	if cellar.Parent != "tavern" {
		t.Errorf("parent = %q, want tavern", cellar.Parent)
	}
}

// The same correction the character lookup offers: a miss names what does
// exist, so the retry picks a real slug instead of guessing twice.
func TestUnknownLocationNamesTheOnesThatExist(t *testing.T) {
	w := locationWorld()
	w.CreateLocation("Tavern", "Loud.", "")
	w.CreateLocation("Cellar", "Damp.", "tavern")

	_, err := w.Location("taproom")
	if err == nil {
		t.Fatal("an unknown location must be an error")
	}
	for _, want := range []string{"taproom", "tavern", "cellar"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want it to mention %q", err, want)
		}
	}
}

func TestUnknownLocationSaysSoWhenThereAreNone(t *testing.T) {
	w := locationWorld()

	_, err := w.Location("tavern")
	if err == nil {
		t.Fatal("an unknown location must be an error")
	}
	if !strings.Contains(err.Error(), "no locations exist yet") {
		t.Errorf("err = %v, want it to say the world has no locations", err)
	}
}

// Two daggers in two corners are two things. One pile of arrows is one thing.
// The spot is what tells them apart.
func TestLocationItemsStackOnNameAndSpot(t *testing.T) {
	w := locationWorld()
	cellar, _ := w.CreateLocation("Cellar", "Damp.", "")

	if _, err := cellar.AddItem(&Item{Name: "Arrow", Description: "Goose-fletched."}, "spilled by the door", 12); err != nil {
		t.Fatal(err)
	}
	if _, err := cellar.AddItem(&Item{Name: "arrow"}, "spilled by the door", 3); err != nil {
		t.Fatal(err)
	}
	if _, err := cellar.AddItem(&Item{Name: "Arrow"}, "under the table", 1); err != nil {
		t.Fatal(err)
	}

	if len(cellar.Items) != 2 {
		t.Fatalf("location holds %d entries, want 2", len(cellar.Items))
	}
	stacks := cellar.SortedItems()
	if stacks[0].Where != "spilled by the door" || stacks[0].Count != 15 {
		t.Errorf("first stack = %+v, want 15 by the door", stacks[0])
	}
	if stacks[1].Where != "under the table" || stacks[1].Count != 1 {
		t.Errorf("second stack = %+v, want 1 under the table", stacks[1])
	}
}

// The whole reason moves beat remove-then-add: a thrown weapon comes back with
// the ratings it was fighting with, not with whatever the DM remembers.
func TestMoveCarriesRatingsIntact(t *testing.T) {
	w := locationWorld()
	cellar, _ := w.CreateLocation("Cellar", "Damp.", "")
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)
	catnip.AddWeapon("Dagger", "Notched at the tip.", 1, Weapon{Damage: 1, DamageAttribute: "strength"})

	if _, err := w.MoveItem(MoveRequest{Item: "Dagger", ToLocation: "cellar", Where: "quivering in the door frame"}); err != nil {
		t.Fatal(err)
	}
	if _, held := catnip.Inventory["dagger"]; held {
		t.Error("the dagger should have left the sheet")
	}

	if _, err := w.MoveItem(MoveRequest{Item: "dagger", ToCharacter: "catnip"}); err != nil {
		t.Fatal(err)
	}
	back, err := catnip.WeaponItem("Dagger")
	if err != nil {
		t.Fatal(err)
	}
	if back.Weapon.Damage != 1 || back.Weapon.DamageAttribute != "strength" {
		t.Errorf("ratings = %+v, want damage 1 off strength", back.Weapon)
	}
	if back.Description != "Notched at the tip." {
		t.Errorf("description = %q, want the original", back.Description)
	}
	if len(cellar.Items) != 0 {
		t.Errorf("the cellar still holds %d entries", len(cellar.Items))
	}
}

// Moving part of a stack splits it into two independent things. They were one
// entry a moment ago, so it is easy for them to go on sharing a magazine — and
// then a shot fired from the pistol on the floor empties the one in the holster.
func TestASplitStackDoesNotShareItsRatings(t *testing.T) {
	w := locationWorld()
	w.CreateLocation("Cellar", "Damp.", "")
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)
	catnip.AddWeapon("Pistol", "A worn revolver.", 2, Weapon{Damage: 2, Ranged: true, AmmoCapacity: 6})

	if _, err := w.MoveItem(MoveRequest{Item: "Pistol", ToLocation: "cellar", Where: "on the barrel-head"}); err != nil {
		t.Fatal(err)
	}

	dropped := w.Locations["cellar"].SortedItems()[0]
	dropped.Weapon.Ammo = 0
	if got := catnip.Inventory["pistol"].Weapon.Ammo; got != 6 {
		t.Errorf("the pistol still holstered is down to %d shots: the two stacks share a magazine", got)
	}

	carried := catnip.Inventory["pistol"]
	carried.Weapon.Damage = 99
	if dropped.Weapon.Damage != 2 {
		t.Error("the dropped pistol's rating moved with the carried one")
	}
}

// The same for armor, where sharing would mean taking off one jerkin and
// leaving somebody else's open too.
func TestASplitArmorStackDoesNotShareItsWorn(t *testing.T) {
	w := locationWorld()
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)
	mira := w.CreateCharacter("mira", "Mira", nil, nil)
	catnip.AddArmor("Jerkin", "Boiled leather.", 2, Armor{Protection: 1, Worn: true})

	if _, err := w.MoveItem(MoveRequest{Item: "Jerkin", ToCharacter: "mira"}); err != nil {
		t.Fatal(err)
	}
	if _, err := mira.EquipArmor("Jerkin", false); err != nil {
		t.Fatal(err)
	}
	if !catnip.Inventory["jerkin"].Armor.Worn {
		t.Error("mira taking hers off stripped catnip too")
	}
}

// An item on the floor loses its spot the moment somebody picks it up: a spot
// describes where a thing lies, and a carried thing lies nowhere.
func TestPickingUpClearsTheSpot(t *testing.T) {
	w := locationWorld()
	w.CreateLocation("Cellar", "Damp.", "")
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)
	catnip.AddItem("Rope", "Forty feet.", 1)
	w.MoveItem(MoveRequest{Item: "Rope", ToLocation: "cellar", Where: "coiled by the stair"})

	if _, err := w.MoveItem(MoveRequest{Item: "Rope", ToCharacter: "catnip"}); err != nil {
		t.Fatal(err)
	}
	if got := catnip.Inventory["rope"].Where; got != "" {
		t.Errorf("spot = %q, want it cleared on pickup", got)
	}
}

// Giving is a move like any other, and the count is what moves.
func TestMoveHandsPartOfAStackToAnotherCharacter(t *testing.T) {
	w := locationWorld()
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)
	mira := w.CreateCharacter("mira", "Mira", nil, nil)
	catnip.AddItem("Ration Pack", "Dried bricks.", 5)

	if _, err := w.MoveItem(MoveRequest{Item: "Ration Pack", ToCharacter: "mira", Count: 2}); err != nil {
		t.Fatal(err)
	}
	if got := catnip.Inventory["ration-pack"].Count; got != 3 {
		t.Errorf("catnip keeps %d, want 3", got)
	}
	if got := mira.Inventory["ration-pack"].Count; got != 2 {
		t.Errorf("mira received %d, want 2", got)
	}
}

// With the same name in two places, the engine will not guess. It says where
// both are and lets the DM name the one it meant.
func TestAmbiguousMoveNamesEveryHolder(t *testing.T) {
	w := locationWorld()
	w.CreateLocation("Cellar", "Damp.", "")
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)
	mira := w.CreateCharacter("mira", "Mira", nil, nil)
	catnip.AddItem("Dagger", "Notched.", 1)
	mira.AddItem("Dagger", "Plain.", 1)

	_, err := w.MoveItem(MoveRequest{Item: "Dagger", ToLocation: "cellar", Where: "on the floor"})
	if err == nil {
		t.Fatal("an ambiguous name must be refused")
	}
	for _, want := range []string{"catnip", "mira"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want it to name %q", err, want)
		}
	}

	// Naming the source settles it.
	if _, err := w.MoveItem(MoveRequest{Item: "Dagger", FromCharacter: "mira", ToLocation: "cellar", Where: "on the floor"}); err != nil {
		t.Fatal(err)
	}
}

// The spot is part of the identity of a stack, so it disambiguates too.
func TestMoveDisambiguatesBySpot(t *testing.T) {
	w := locationWorld()
	cellar, _ := w.CreateLocation("Cellar", "Damp.", "")
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)
	cellar.AddItem(&Item{Name: "Dagger", Description: "Notched."}, "in the door frame", 1)
	cellar.AddItem(&Item{Name: "Dagger", Description: "Plain."}, "under the table", 1)

	if _, err := w.MoveItem(MoveRequest{Item: "Dagger", FromLocation: "cellar", ToCharacter: "catnip"}); err == nil {
		t.Fatal("two stacks of the same name at one location must be ambiguous")
	}

	if _, err := w.MoveItem(MoveRequest{Item: "Dagger", FromLocation: "cellar", FromWhere: "under the table", ToCharacter: "catnip"}); err != nil {
		t.Fatal(err)
	}
	if got := catnip.Inventory["dagger"].Description; got != "Plain." {
		t.Errorf("picked up %q, want the one under the table", got)
	}
}

func TestMoveRefusesWhenNothingIsCalledThat(t *testing.T) {
	w := locationWorld()
	w.CreateLocation("Cellar", "Damp.", "")
	w.CreateCharacter("catnip", "Catnip", nil, nil)

	_, err := w.MoveItem(MoveRequest{Item: "Halberd", ToLocation: "cellar", Where: "propped up"})
	if err == nil {
		t.Fatal("moving something nobody has must be refused")
	}
	if !strings.Contains(err.Error(), "Halberd") {
		t.Errorf("err = %v, want it to name the item", err)
	}
}

// A move to a location has to say where the thing comes to rest, because that
// is the one fact the engine cannot work out for itself.
func TestMoveToALocationRequiresASpot(t *testing.T) {
	w := locationWorld()
	w.CreateLocation("Cellar", "Damp.", "")
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)
	catnip.AddItem("Rope", "Forty feet.", 1)

	if _, err := w.MoveItem(MoveRequest{Item: "Rope", ToLocation: "cellar"}); err == nil {
		t.Fatal("a move onto the ground must name the spot")
	}
}

func TestMoveNeedsExactlyOneDestination(t *testing.T) {
	w := locationWorld()
	w.CreateLocation("Cellar", "Damp.", "")
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)
	catnip.AddItem("Rope", "Forty feet.", 1)

	if _, err := w.MoveItem(MoveRequest{Item: "Rope"}); err == nil {
		t.Fatal("a move with no destination must be refused")
	}
	if _, err := w.MoveItem(MoveRequest{Item: "Rope", ToCharacter: "catnip", ToLocation: "cellar", Where: "here"}); err == nil {
		t.Fatal("a move to two destinations must be refused")
	}
}

// Kinds do not convert. Dropping a sword and then naming loot after it must not
// quietly turn the sword into gear.
func TestMoveRefusesToMergeDifferentKinds(t *testing.T) {
	w := locationWorld()
	cellar, _ := w.CreateLocation("Cellar", "Damp.", "")
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)
	catnip.AddWeapon("Dagger", "Notched.", 1, Weapon{Damage: 1})
	cellar.AddItem(&Item{Name: "Dagger", Description: "A wooden prop."}, "on the shelf", 1)

	_, err := w.MoveItem(MoveRequest{Item: "Dagger", FromCharacter: "catnip", ToLocation: "cellar", Where: "on the shelf"})
	if err == nil {
		t.Fatal("merging a weapon into a gear stack must be refused")
	}
}

// Destroying is the only way something leaves the world, and it works wherever
// the thing happens to be.
func TestRemoveItemDestroysAtALocation(t *testing.T) {
	w := locationWorld()
	cellar, _ := w.CreateLocation("Cellar", "Damp.", "")
	cellar.AddItem(&Item{Name: "Torch", Description: "Burning low."}, "in the sconce", 2)

	if _, _, err := cellar.RemoveItem("Torch", "in the sconce", 2); err != nil {
		t.Fatal(err)
	}
	if len(cellar.Items) != 0 {
		t.Errorf("the cellar still holds %d entries", len(cellar.Items))
	}
}

func TestLocationRemoveRefusesMoreThanIsThere(t *testing.T) {
	w := locationWorld()
	cellar, _ := w.CreateLocation("Cellar", "Damp.", "")
	cellar.AddItem(&Item{Name: "Torch", Description: "Burning low."}, "in the sconce", 1)

	if _, _, err := cellar.RemoveItem("Torch", "in the sconce", 4); err == nil {
		t.Fatal("removing more than is there must be refused")
	}
	if got := cellar.SortedItems()[0].Count; got != 1 {
		t.Errorf("count = %d, want the stack untouched", got)
	}
}
