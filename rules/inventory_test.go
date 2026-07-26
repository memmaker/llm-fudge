package rules

import (
	"strings"
	"testing"
)

// Items are keyed by the slugified name so the DM naming the same thing twice
// adds to one stack instead of opening a second entry the player then sees
// twice on their sheet.
func TestAddItemStacksByName(t *testing.T) {
	ch := &Character{Name: "Kyle"}

	if _, err := ch.AddItem("Ration Pack", "Dried protein bricks.", 2); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.AddItem("ration pack", "", 3); err != nil {
		t.Fatal(err)
	}

	if len(ch.Inventory) != 1 {
		t.Fatalf("inventory has %d entries, want 1", len(ch.Inventory))
	}
	item := ch.Inventory["ration-pack"]
	if item.Count != 5 {
		t.Errorf("count = %d, want 5", item.Count)
	}
	// The restock carried no description, so the original wording survives.
	if item.Description != "Dried protein bricks." {
		t.Errorf("description = %q, want the original", item.Description)
	}
	if item.Name != "Ration Pack" {
		t.Errorf("name = %q, want the name it was first added under", item.Name)
	}
}

func TestRemoveItem(t *testing.T) {
	ch := &Character{Name: "Kyle"}
	if _, err := ch.AddItem("Thermal Detonator", "Scavenged, and touchy.", 2); err != nil {
		t.Fatal(err)
	}

	// More than is carried fails outright rather than clamping: the DM needs to
	// learn they cannot spend it, so the scene can go the other way.
	if _, _, err := ch.RemoveItem("Thermal Detonator", 3); err == nil {
		t.Error("removing more than carried should fail")
	}
	if ch.Inventory["thermal-detonator"].Count != 2 {
		t.Error("a failed removal must not change the stack")
	}

	if _, left, err := ch.RemoveItem("thermal detonator", 1); err != nil || left != 1 {
		t.Errorf("RemoveItem = %d, %v; want 1, nil", left, err)
	}
	// The last one taken drops the entry, so the sheet does not list "x0".
	if _, left, err := ch.RemoveItem("Thermal Detonator", 1); err != nil || left != 0 {
		t.Errorf("RemoveItem = %d, %v; want 0, nil", left, err)
	}
	if _, held := ch.Inventory["thermal-detonator"]; held {
		t.Error("emptied stack should be dropped from the inventory")
	}
	if _, _, err := ch.RemoveItem("Thermal Detonator", 1); err == nil {
		t.Error("removing an item not carried should fail")
	}
}

// A purse never goes negative — a character who cannot afford something simply
// does not pay.
func TestCoins(t *testing.T) {
	ch := &Character{Name: "Kyle"}

	if balance, err := ch.AddCoins(40); err != nil || balance != 40 {
		t.Fatalf("AddCoins = %d, %v; want 40, nil", balance, err)
	}
	if _, err := ch.RemoveCoins(41); err == nil {
		t.Error("overspending should fail")
	}
	if ch.Coins != 40 {
		t.Errorf("a failed debit must not change the purse: %d", ch.Coins)
	}
	if balance, err := ch.RemoveCoins(40); err != nil || balance != 0 {
		t.Errorf("RemoveCoins = %d, %v; want 0, nil", balance, err)
	}
	if _, err := ch.AddCoins(0); err == nil {
		t.Error("a zero credit should fail; the sign is the tool's, not the amount's")
	}
}

// The sheet is the player's only view of what they carry, so it has to name
// every item, its count and the description the DM wrote for it.
func TestStateFileTextListsInventory(t *testing.T) {
	w := &World{Characters: map[string]*Character{}, Player: "kyle-katarn"}
	ch := w.CreateCharacter("kyle-katarn", "Kyle Katarn", nil, nil)
	if _, err := ch.AddItem("Blaster Pistol", "A worn DL-44 with a scored barrel.", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.AddItem("Power Cell", "Spares for the blaster.", 4); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.AddCoins(75); err != nil {
		t.Fatal(err)
	}

	sheet := StateFileText(w)
	for _, want := range []string{
		"Inventory",
		"Blaster Pistol",
		"A worn DL-44 with a scored barrel.",
		"Power Cell x4",
		"Spares for the blaster.",
		"Coins:",
	} {
		if !strings.Contains(sheet, want) {
			t.Errorf("sheet missing %q:\n%s", want, sheet)
		}
	}
	// A lone item reads as itself, not as a stack of one.
	if strings.Contains(sheet, "Blaster Pistol x1") {
		t.Error("single items should carry no count")
	}
}
