package rules

import (
	"strings"
	"testing"
)

func locationWorld() *World {
	return &World{Characters: map[string]*Character{}, Locations: map[string]*Location{}}
}

// drop puts a stack on a location's floor the way the engine does it: with an id
// minted first, because AddItem takes a proto whose identity is already settled.
func drop(w *World, l *Location, proto *Item, where string, count int) *Item {
	proto.ID = w.MintItemID(proto.Name)
	item, err := l.AddItem(proto, where, count)
	if err != nil {
		panic(err)
	}
	return item
}

// carried is a test-only shorthand for what used to be Character.carried: the
// one stack of a name on the sheet, wherever its spot, or nil.
func carried(c *Character, ref string) *Item {
	item, _ := c.FindItem(ref, "")
	return item
}

// container mints a container item — Items initialized rather than left nil —
// and lands it on the given holder, the way mintItem does for `type: container`.
func container(w *World, holder Holder, name, spot string) *Item {
	proto := &Item{ID: w.MintItemID(name), Name: name, Items: map[string]*Item{}}
	item, err := holder.AddItem(proto, spot, 1)
	if err != nil {
		panic(err)
	}
	return item
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
// The spot is what tells them apart — and the pile that grows keeps the id it
// already had, so the second delivery of arrows does not become a second entity.
func TestLocationItemsStackOnNameAndSpot(t *testing.T) {
	w := locationWorld()
	cellar, _ := w.CreateLocation("Cellar", "Damp.", "")

	first := drop(w, cellar, &Item{Name: "Arrow", Description: "Goose-fletched."}, "spilled by the door", 12)
	grown := drop(w, cellar, &Item{Name: "arrow"}, "spilled by the door", 3)
	elsewhere := drop(w, cellar, &Item{Name: "Arrow"}, "under the table", 1)

	if grown.ID != first.ID {
		t.Errorf("the restocked pile got id %q, want the original's %q", grown.ID, first.ID)
	}
	if elsewhere.ID == first.ID {
		t.Errorf("two piles in two spots share the id %q", elsewhere.ID)
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
	w.GiveWeapon(catnip, "Dagger", "Notched at the tip.", 1, Weapon{Damage: 1, DamageAttribute: "strength"})

	if _, err := w.MoveItem(MoveRequest{Item: "Dagger", To: "cellar", Where: "quivering in the door frame"}); err != nil {
		t.Fatal(err)
	}
	if carried(catnip, "dagger") != nil {
		t.Error("the dagger should have left the sheet")
	}

	if _, err := w.MoveItem(MoveRequest{Item: "dagger", To: "catnip"}); err != nil {
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
	w.GiveWeapon(catnip, "Pistol", "A worn revolver.", 2, Weapon{Damage: 2, Ranged: true, AmmoCapacity: 6})

	if _, err := w.MoveItem(MoveRequest{Item: "Pistol", To: "cellar", Where: "on the barrel-head"}); err != nil {
		t.Fatal(err)
	}

	dropped := w.Locations["cellar"].SortedItems()[0]
	if dropped.ID == "" || dropped.ID == carried(catnip, "pistol").ID {
		t.Fatalf("the two halves of the split share the id %q, so they are one entity with two magazines", dropped.ID)
	}
	dropped.Weapon.Ammo = 0
	if got := carried(catnip, "pistol").Weapon.Ammo; got != 6 {
		t.Errorf("the pistol still holstered is down to %d shots: the two stacks share a magazine", got)
	}

	holstered := carried(catnip, "pistol")
	holstered.Weapon.Damage = 99
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
	w.GiveArmor(catnip, "Jerkin", "Boiled leather.", 2, Armor{Protection: 1, Worn: true})

	if _, err := w.MoveItem(MoveRequest{Item: "Jerkin", To: "mira"}); err != nil {
		t.Fatal(err)
	}
	if _, err := mira.EquipArmor("Jerkin", false); err != nil {
		t.Fatal(err)
	}
	if !carried(catnip, "jerkin").Armor.Worn {
		t.Error("mira taking hers off stripped catnip too")
	}
}

// An item on the floor loses its spot the moment somebody picks it up: nothing
// carries the spot across unless the destination is given one of its own.
func TestPickingUpClearsTheSpot(t *testing.T) {
	w := locationWorld()
	w.CreateLocation("Cellar", "Damp.", "")
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)
	w.GiveItem(catnip, "Rope", "Forty feet.", 1)
	w.MoveItem(MoveRequest{Item: "Rope", To: "cellar", Where: "coiled by the stair"})

	if _, err := w.MoveItem(MoveRequest{Item: "Rope", To: "catnip"}); err != nil {
		t.Fatal(err)
	}
	if got := carried(catnip, "rope").Where; got != "" {
		t.Errorf("spot = %q, want it cleared on pickup", got)
	}
}

// Giving is a move like any other, and the count is what moves.
func TestMoveHandsPartOfAStackToAnotherCharacter(t *testing.T) {
	w := locationWorld()
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)
	mira := w.CreateCharacter("mira", "Mira", nil, nil)
	w.GiveItem(catnip, "Ration Pack", "Dried bricks.", 5)

	if _, err := w.MoveItem(MoveRequest{Item: "Ration Pack", To: "mira", Count: 2}); err != nil {
		t.Fatal(err)
	}
	if got := carried(catnip, "ration-pack").Count; got != 3 {
		t.Errorf("catnip keeps %d, want 3", got)
	}
	if got := carried(mira, "ration-pack").Count; got != 2 {
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
	w.GiveItem(catnip, "Dagger", "Notched.", 1)
	w.GiveItem(mira, "Dagger", "Plain.", 1)

	_, err := w.MoveItem(MoveRequest{Item: "Dagger", To: "cellar", Where: "on the floor"})
	if err == nil {
		t.Fatal("an ambiguous name must be refused")
	}
	for _, want := range []string{"catnip", "mira"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want it to name %q", err, want)
		}
	}

	// Naming the source settles it.
	if _, err := w.MoveItem(MoveRequest{Item: "Dagger", From: "mira", To: "cellar", Where: "on the floor"}); err != nil {
		t.Fatal(err)
	}
}

// The spot is part of the identity of a stack, so it disambiguates too.
func TestMoveDisambiguatesBySpot(t *testing.T) {
	w := locationWorld()
	cellar, _ := w.CreateLocation("Cellar", "Damp.", "")
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)
	drop(w, cellar, &Item{Name: "Dagger", Description: "Notched."}, "in the door frame", 1)
	drop(w, cellar, &Item{Name: "Dagger", Description: "Plain."}, "under the table", 1)

	if _, err := w.MoveItem(MoveRequest{Item: "Dagger", From: "cellar", To: "catnip"}); err == nil {
		t.Fatal("two stacks of the same name at one location must be ambiguous")
	}

	if _, err := w.MoveItem(MoveRequest{Item: "Dagger", From: "cellar", FromWhere: "under the table", To: "catnip"}); err != nil {
		t.Fatal(err)
	}
	if got := carried(catnip, "dagger").Description; got != "Plain." {
		t.Errorf("picked up %q, want the one under the table", got)
	}
}

func TestMoveRefusesWhenNothingIsCalledThat(t *testing.T) {
	w := locationWorld()
	w.CreateLocation("Cellar", "Damp.", "")
	w.CreateCharacter("catnip", "Catnip", nil, nil)

	_, err := w.MoveItem(MoveRequest{Item: "Halberd", To: "cellar", Where: "propped up"})
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
	w.GiveItem(catnip, "Rope", "Forty feet.", 1)

	if _, err := w.MoveItem(MoveRequest{Item: "Rope", To: "cellar"}); err == nil {
		t.Fatal("a move onto the ground must name the spot")
	}
}

func TestMoveNeedsADestination(t *testing.T) {
	w := locationWorld()
	w.CreateLocation("Cellar", "Damp.", "")
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)
	w.GiveItem(catnip, "Rope", "Forty feet.", 1)

	if _, err := w.MoveItem(MoveRequest{Item: "Rope"}); err == nil {
		t.Fatal("a move with no destination must be refused")
	}
}

// Kinds do not convert. Dropping a sword and then naming loot after it must not
// quietly turn the sword into gear.
func TestMoveRefusesToMergeDifferentKinds(t *testing.T) {
	w := locationWorld()
	cellar, _ := w.CreateLocation("Cellar", "Damp.", "")
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)
	w.GiveWeapon(catnip, "Dagger", "Notched.", 1, Weapon{Damage: 1})
	drop(w, cellar, &Item{Name: "Dagger", Description: "A wooden prop."}, "on the shelf", 1)

	_, err := w.MoveItem(MoveRequest{Item: "Dagger", From: "catnip", To: "cellar", Where: "on the shelf"})
	if err == nil {
		t.Fatal("merging a weapon into a gear stack must be refused")
	}
}

// Destroying is the only way something leaves the world, and it works wherever
// the thing happens to be.
func TestRemoveItemDestroysAtALocation(t *testing.T) {
	w := locationWorld()
	cellar, _ := w.CreateLocation("Cellar", "Damp.", "")
	drop(w, cellar, &Item{Name: "Torch", Description: "Burning low."}, "in the sconce", 2)

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
	drop(w, cellar, &Item{Name: "Torch", Description: "Burning low."}, "in the sconce", 1)

	if _, _, err := cellar.RemoveItem("Torch", "in the sconce", 4); err == nil {
		t.Fatal("removing more than is there must be refused")
	}
	if got := cellar.SortedItems()[0].Count; got != 1 {
		t.Errorf("count = %d, want the stack untouched", got)
	}
}

// The point of the id: a thing that moves in one piece is the same thing at the
// far end. The old key was built out of where the item was, so a move changed its
// whole address and nothing tied what left to what arrived — which is how a
// session lost track of what the party was carrying.
func TestAFullMoveKeepsTheItemsID(t *testing.T) {
	w := locationWorld()
	cellar, _ := w.CreateLocation("Cellar", "Damp.", "")
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)
	rope, err := w.GiveItem(catnip, "Rope", "Forty feet.", 1)
	if err != nil {
		t.Fatal(err)
	}
	id := rope.ID

	down, err := w.MoveItem(MoveRequest{Item: "Rope", To: "cellar", Where: "coiled by the stair"})
	if err != nil {
		t.Fatal(err)
	}
	if down.Item.ID != id {
		t.Errorf("dropped as %q, want the id it was carried under, %q", down.Item.ID, id)
	}

	up, err := w.MoveItem(MoveRequest{Item: "Rope", To: "catnip"})
	if err != nil {
		t.Fatal(err)
	}
	if up.Item.ID != id {
		t.Errorf("picked up as %q, want %q", up.Item.ID, id)
	}
	if _, err := w.ItemByID(id); err != nil {
		t.Errorf("the rope is no longer addressable by its id: %v", err)
	}
	if len(cellar.Items) != 0 {
		t.Errorf("the cellar still holds %d entries", len(cellar.Items))
	}
}

// Source and destination can be the same room. A whole stack keeps its id, so the
// arriving stack is written into the very map slot the departing one occupied, and
// the emptied original must not then delete its own replacement — that would lose
// the dagger for good.
func TestAFullMoveWithinOneLocationKeepsTheItem(t *testing.T) {
	w := locationWorld()
	cellar, _ := w.CreateLocation("Cellar", "Damp.", "")
	dagger := drop(w, cellar, &Item{Name: "Dagger", Description: "Notched."}, "in the door frame", 1)

	res, err := w.MoveItem(MoveRequest{Item: "Dagger", From: "cellar", To: "cellar", Where: "on the table"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cellar.Items) != 1 {
		t.Fatalf("the cellar holds %d entries, want the one dagger", len(cellar.Items))
	}
	if res.Item.ID != dagger.ID {
		t.Errorf("the dagger came to rest as %q, want %q", res.Item.ID, dagger.ID)
	}
	moved := cellar.SortedItems()[0]
	if moved.ID != dagger.ID || moved.Where != "on the table" || moved.Count != 1 {
		t.Errorf("stack = %+v, want one dagger on the table under its original id", moved)
	}
}

// A partial move genuinely creates a second thing: the two halves are counted,
// moved and destroyed separately from here on, so they cannot share an id. What
// stays behind is still the original and keeps the original's.
func TestAPartialMoveMintsANewIDForWhatLeft(t *testing.T) {
	w := locationWorld()
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)
	w.CreateCharacter("mira", "Mira", nil, nil)
	packs, err := w.GiveItem(catnip, "Ration Pack", "Dried bricks.", 5)
	if err != nil {
		t.Fatal(err)
	}
	id := packs.ID

	res, err := w.MoveItem(MoveRequest{Item: "Ration Pack", To: "mira", Count: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got := carried(catnip, "ration-pack").ID; got != id {
		t.Errorf("what stayed behind became %q, want the original %q", got, id)
	}
	if res.Item.ID == "" {
		t.Fatal("the half that moved got no id at all")
	}
	if res.Item.ID == id {
		t.Errorf("both halves answer to %q", id)
	}
	for _, want := range []struct {
		id    string
		count int
	}{{id, 3}, {res.Item.ID, 2}} {
		got, err := w.ItemByID(want.id)
		if err != nil {
			t.Fatalf("%s is not addressable: %v", want.id, err)
		}
		if got.Count != want.count {
			t.Errorf("%s holds %d, want %d", want.id, got.Count, want.count)
		}
	}
}

// Landing on a matching stack merges into one entity, so one of the two ids has
// to go. The stack that was already there keeps its own — it is the thing the DM
// has been reading about — and the absorbed id leaves the world with the stack it
// belonged to.
func TestMergingKeepsTheDestinationStacksID(t *testing.T) {
	w := locationWorld()
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)
	mira := w.CreateCharacter("mira", "Mira", nil, nil)
	hers, err := w.GiveItem(mira, "Ration Pack", "Dried bricks.", 1)
	if err != nil {
		t.Fatal(err)
	}
	his, err := w.GiveItem(catnip, "Ration Pack", "Dried bricks.", 2)
	if err != nil {
		t.Fatal(err)
	}
	if his.ID == hers.ID {
		t.Fatalf("two piles in two pockets share the id %q", his.ID)
	}

	res, err := w.MoveItem(MoveRequest{Item: "Ration Pack", From: "catnip", To: "mira", Count: 2})
	if err != nil {
		t.Fatal(err)
	}
	if res.Item.ID != hers.ID {
		t.Errorf("the merged pile answers to %q, want mira's own %q", res.Item.ID, hers.ID)
	}
	if res.Item.Count != 3 {
		t.Errorf("mira holds %d, want all 3", res.Item.Count)
	}
	if len(mira.Inventory) != 1 {
		t.Errorf("mira has %d entries, want the one merged pile", len(mira.Inventory))
	}
	if w.HasEntity(his.ID) {
		t.Errorf("the absorbed id %q is still somewhere in the world", his.ID)
	}
}

// One global keyspace means the minter dodges every entity there is, not just the
// items. The DM addresses a thing by bare id, so a character and an item both
// answering to `lantern` would make every call about either a coin toss.
func TestMintItemIDAvoidsTheWholeKeyspace(t *testing.T) {
	w := locationWorld()
	w.CreateCharacter("lantern", "Lantern", nil, nil)
	w.CreateLocation("Rope", "A street named for the ropewalk.", "")
	cellar, _ := w.CreateLocation("Cellar", "Damp.", "")
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)

	if got := w.MintItemID("Lantern"); got == "lantern" {
		t.Error("an item was minted onto a character's id")
	}
	if got := w.MintItemID("Rope"); got == "rope" {
		t.Error("an item was minted onto a location's id")
	}

	// The suffix path: each new torch steps past every torch already minted,
	// wherever it happens to be.
	first := drop(w, cellar, &Item{Name: "Torch"}, "in the sconce", 1)
	second := drop(w, cellar, &Item{Name: "Torch"}, "on the step", 1)
	third, err := w.GiveItem(catnip, "Torch", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, it := range []*Item{first, second, third} {
		if it.ID == "" {
			t.Fatal("a torch was minted with no id")
		}
		if seen[it.ID] {
			t.Errorf("two torches share the id %q", it.ID)
		}
		seen[it.ID] = true
	}
	if got := w.MintItemID("Torch"); w.HasEntity(got) {
		t.Errorf("minted %q, which is already taken", got)
	}
}

// Two iron keys in two places are two things, and each has to be reachable on its
// own — the name they share is exactly what could not tell them apart before.
func TestTwoItemsOfOneNameStayIndependentlyAddressable(t *testing.T) {
	w := locationWorld()
	cellar, _ := w.CreateLocation("Cellar", "Damp.", "")
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)

	pocketed, err := w.GiveItem(catnip, "Iron Key", "Long-shanked.", 1)
	if err != nil {
		t.Fatal(err)
	}
	lying := drop(w, cellar, &Item{Name: "Iron Key", Description: "Stubby, and green with age."}, "on the ledge", 1)

	if pocketed.ID == lying.ID {
		t.Fatalf("both keys answer to %q", pocketed.ID)
	}
	for _, want := range []*Item{pocketed, lying} {
		got, err := w.ItemByID(want.ID)
		if err != nil {
			t.Fatalf("%s is not addressable: %v", want.ID, err)
		}
		if got != want {
			t.Errorf("%s resolved to the other key: %q", want.ID, got.Description)
		}
	}

	// A miss names what exists, the way a character miss names the roster.
	_, err = w.ItemByID("brass-key")
	if err == nil {
		t.Fatal("an unknown item id must be an error")
	}
	for _, want := range []string{"brass-key", pocketed.ID, lying.ID} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want it to mention %q", err, want)
		}
	}
}

// A proto with no id is a caller that forgot to mint, and the entry it would open
// could never be addressed again. A loud refusal here beats a ghost item in play.
func TestAnUnmintedProtoIsRefused(t *testing.T) {
	w := locationWorld()
	cellar, _ := w.CreateLocation("Cellar", "Damp.", "")
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)

	if _, err := cellar.AddItem(&Item{Name: "Torch"}, "in the sconce", 1); err == nil {
		t.Error("a floor must refuse an item with no id")
	}
	if _, err := catnip.AddItem(&Item{Name: "Torch"}, "", 1); err == nil {
		t.Error("an inventory must refuse an item with no id")
	}
}

// The id is what the world state hands the DM and what it is told every call
// names, so a call naming one has to resolve — including after the thing has
// moved, which is the point of the id existing at all.
func TestAnItemIsFoundByItsID(t *testing.T) {
	w := locationWorld()
	cellar, _ := w.CreateLocation("Cellar", "Damp.", "")
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)
	key, err := w.GiveItem(catnip, "Iron Key", "Cold.", 1)
	if err != nil {
		t.Fatal(err)
	}

	// Dropped by id, from a character.
	if _, err := w.MoveItem(MoveRequest{Item: key.ID, To: cellar.ID, Where: "under a cask"}); err != nil {
		t.Fatalf("move by id from a character: %v", err)
	}
	// And picked up by the same id, now that it is on a floor instead.
	if _, err := w.MoveItem(MoveRequest{Item: key.ID, To: "catnip"}); err != nil {
		t.Fatalf("move by id from a location: %v", err)
	}
	if carried(catnip, key.ID) == nil {
		t.Error("the key is not addressable by its id after the round trip")
	}
}

// Accepting ids must not cost the ambiguity refusal. With two stacks of one
// name, the first holds the id minted from that name, so an id-first lookup
// would have resolved the bare name to one of the two and guessed.
func TestAnIDMatchDoesNotSuppressAmbiguity(t *testing.T) {
	w := locationWorld()
	cellar, _ := w.CreateLocation("Cellar", "Damp.", "")
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)
	first := drop(w, cellar, &Item{Name: "Dagger", Description: "Notched."}, "in the door frame", 1)
	second := drop(w, cellar, &Item{Name: "Dagger", Description: "Plain."}, "under the table", 1)

	if first.ID != "dagger" {
		t.Fatalf("expected the first stack to hold the bare id, got %q", first.ID)
	}
	if _, err := w.MoveItem(MoveRequest{Item: "Dagger", From: cellar.ID, To: "catnip"}); err == nil {
		t.Error("the bare name matched an id and skipped the ambiguity refusal")
	}

	// The suffixed id is unambiguous, and is how the DM gets out of it.
	if _, err := w.MoveItem(MoveRequest{Item: second.ID, From: cellar.ID, To: "catnip"}); err != nil {
		t.Fatalf("the second stack's own id did not resolve: %v", err)
	}
	if got := carried(catnip, second.ID); got == nil || got.Description != "Plain." {
		t.Errorf("the wrong stack moved: %+v", got)
	}
}

// One id means one entity across every type, so a location cannot open on an id
// a character or an item already holds.
func TestCreateLocationRefusesATakenID(t *testing.T) {
	w := locationWorld()
	w.CreateCharacter("cellar", "Cellar", nil, nil)

	if _, err := w.CreateLocation("Cellar", "Damp.", ""); err == nil {
		t.Error("a location opened on an id a character already holds")
	}
}

// A container is a Holder like any other, so moving something into one is the
// same MoveItem every other move is — and the DM never has to know the
// destination happens to be an item rather than a place.
func TestItemMovesIntoAContainer(t *testing.T) {
	w := locationWorld()
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)
	backpack := container(w, catnip, "Backpack", "")
	w.GiveItem(catnip, "Iron Coin", "Worn smooth.", 1)

	if _, err := w.MoveItem(MoveRequest{Item: "Iron Coin", To: backpack.ID, Where: "inside"}); err != nil {
		t.Fatal(err)
	}
	if holder, _ := w.ItemHolder("iron-coin"); holder != backpack.ID {
		t.Errorf("holder = %q, want the backpack", holder)
	}
	if len(backpack.Items) != 1 {
		t.Fatalf("the backpack holds %d entries, want 1", len(backpack.Items))
	}
}

// Moving a container moves everything inside it for free: contents reference
// the container by id, not by a copy of its position, so nothing about them
// has to be restated when the container itself changes hands.
func TestMovingAContainerCarriesItsContents(t *testing.T) {
	w := locationWorld()
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)
	w.CreateCharacter("mira", "Mira", nil, nil)
	backpack := container(w, catnip, "Backpack", "")
	w.GiveItem(catnip, "Iron Coin", "Worn smooth.", 1)
	if _, err := w.MoveItem(MoveRequest{Item: "Iron Coin", To: backpack.ID, Where: "inside"}); err != nil {
		t.Fatal(err)
	}

	if _, err := w.MoveItem(MoveRequest{Item: "Backpack", To: "mira"}); err != nil {
		t.Fatal(err)
	}
	if holder, _ := w.ItemHolder(backpack.ID); holder != "mira" {
		t.Errorf("the backpack is held by %q, want mira", holder)
	}
	if holder, _ := w.ItemHolder("iron-coin"); holder != backpack.ID {
		t.Errorf("the coin's holder changed to %q; it should still be the backpack", holder)
	}
	coin, err := w.ItemByID("iron-coin")
	if err != nil || coin.Count != 1 {
		t.Errorf("the coin is no longer addressable after its container moved: %v, %+v", err, coin)
	}
}

// A backpack inside a backpack inside a backpack, moved once, moves the
// whole chain — this is the nested case the flat container/spot design is
// for.
func TestContainersNestArbitrarily(t *testing.T) {
	w := locationWorld()
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)
	outer := container(w, catnip, "Rucksack", "")
	inner := container(w, outer, "Pouch", "inside")
	w.GiveItem(catnip, "Bead", "Glass, blue.", 1)
	if _, err := w.MoveItem(MoveRequest{Item: "Bead", To: inner.ID, Where: "inside"}); err != nil {
		t.Fatal(err)
	}

	if holder, _ := w.ItemHolder("bead"); holder != inner.ID {
		t.Errorf("the bead is held by %q, want the pouch", holder)
	}
	if holder, _ := w.ItemHolder(inner.ID); holder != outer.ID {
		t.Errorf("the pouch is held by %q, want the rucksack", holder)
	}
}

// Putting a container inside its own contents would be a cycle nothing could
// ever render or move again — the DM asking for it is refused rather than
// silently corrupting the world.
func TestMoveRefusesACycle(t *testing.T) {
	w := locationWorld()
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)
	outer := container(w, catnip, "Rucksack", "")
	inner := container(w, outer, "Pouch", "inside")

	if _, err := w.MoveItem(MoveRequest{Item: outer.Name, To: inner.ID, Where: "inside"}); err == nil {
		t.Fatal("moving the rucksack into its own pouch must be refused")
	}
}

// An ordinary item is not a valid destination — only a character, a location,
// or something explicitly minted as a container can hold anything.
func TestMoveRefusesANonContainerDestination(t *testing.T) {
	w := locationWorld()
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)
	w.GiveItem(catnip, "Dagger", "Notched.", 1)
	w.GiveItem(catnip, "Coin", "Worn.", 1)

	if _, err := w.MoveItem(MoveRequest{Item: "Coin", To: "dagger"}); err == nil {
		t.Fatal("a plain item is not a container and must refuse a move into it")
	}
}

// Two containers never merge, even sharing a name and a spot: each is its own
// thing with its own contents, unlike two piles of the same coin.
func TestContainersNeverStack(t *testing.T) {
	w := locationWorld()
	catnip := w.CreateCharacter("catnip", "Catnip", nil, nil)
	first := container(w, catnip, "Pouch", "")
	second := container(w, catnip, "Pouch", "")

	if first.ID == second.ID {
		t.Fatalf("two pouches merged into one id %q", first.ID)
	}
	count := 0
	for _, it := range catnip.SortedItems() {
		if it.Name == "Pouch" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("catnip holds %d pouches, want 2 independent entries", count)
	}
}
