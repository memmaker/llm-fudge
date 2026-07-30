package main

import (
	"strings"
	"testing"

	"fudge/rules"

	"github.com/memmaker/gollum/llm"
)

func inventoryCall(name, args string) llm.ToolCall {
	return llm.ToolCall{Type: "function", Function: llm.ToolCallFunction{Name: name, Arguments: args}}
}

func equippedWorld(t *testing.T) *rules.World {
	t.Helper()
	w := &rules.World{Characters: map[string]*rules.Character{}, Player: "kyle-katarn", Setting: deadlandsSetting()}
	w.CreateCharacter("kyle-katarn", "Kyle Katarn", nil, nil)
	return w
}

// The four calls are the model's only way to move gear and money, so each has
// to land on the character sheet exactly as asked.
func TestInventoryToolsMoveGoods(t *testing.T) {
	w := equippedWorld(t)
	pc, _ := w.Character("kyle-katarn")

	for _, c := range []struct {
		tool string
		args string
	}{
		{"add_item", `{"character_id":"kyle-katarn","name":"Power Cell","description":"Spares.","count":4}`},
		{"add_coins", `{"character_id":"kyle-katarn","amount":50}`},
	} {
		if _, err := executeAction(w, inventoryCall(c.tool, c.args)); err != nil {
			t.Fatalf("%s: %v", c.tool, err)
		}
	}

	if got := pc.Inventory["power-cell"].Count; got != 4 {
		t.Errorf("power cells = %d, want 4", got)
	}
	if pc.Coins != 50 {
		t.Errorf("coins = %d, want 50", pc.Coins)
	}
}

// A count the model left out means one — the common case is a single object,
// and omitting the field must not silently move nothing.
func TestAddItemDefaultsToOne(t *testing.T) {
	w := equippedWorld(t)
	pc, _ := w.Character("kyle-katarn")

	if _, err := executeAction(w, inventoryCall("add_item", `{"name":"Blaster Pistol","description":"A worn DL-44."}`)); err != nil {
		t.Fatal(err)
	}
	if got := pc.Inventory["blaster-pistol"].Count; got != 1 {
		t.Errorf("count = %d, want 1", got)
	}
}

// During character creation the model has not seen the generated id yet, so an
// omitted character_id has to reach the player character.
func TestInventoryToolsDefaultToPlayer(t *testing.T) {
	w := equippedWorld(t)
	pc, _ := w.Character("kyle-katarn")

	if _, err := executeAction(w, inventoryCall("add_coins", `{"amount":40}`)); err != nil {
		t.Fatal(err)
	}
	if pc.Coins != 40 {
		t.Errorf("coins = %d, want 40 on the player character", pc.Coins)
	}

	// With no player yet, the same call has to say so rather than panic.
	empty := &rules.World{Characters: map[string]*rules.Character{}}
	_, err := executeAction(empty, inventoryCall("add_coins", `{"amount":40}`))
	if err == nil || !strings.Contains(err.Error(), "character_id") {
		t.Errorf("err = %v, want a complaint about character_id", err)
	}
}

// Failures come back as errors so the model reads them and narrates around
// them; nothing may change on the sheet when a call fails.
func TestInventoryToolsRejectImpossibleMoves(t *testing.T) {
	w := equippedWorld(t)
	pc, _ := w.Character("kyle-katarn")
	pc.AddCoins(10)
	w.GiveItem(pc, "Ration Pack", "Dried bricks.", 1)

	for _, c := range []struct{ tool, args string }{
		{"remove_coins", `{"amount":11}`},
		{"remove_item", `{"name":"Ration Pack","count":2}`},
		{"remove_item", `{"name":"Lightsaber","count":1}`},
		{"add_coins", `{"amount":-5}`},
		{"add_item", `{"name":"","description":"Nothing."}`},
	} {
		if _, err := executeAction(w, inventoryCall(c.tool, c.args)); err == nil {
			t.Errorf("%s %s should have failed", c.tool, c.args)
		}
	}

	if pc.Coins != 10 || pc.Inventory["ration-pack"].Count != 1 {
		t.Errorf("failed calls changed state: %d coins, inventory %v", pc.Coins, pc.Inventory)
	}
}

// Character creation keeps these calls because it runs before there is a world
// to declare anything against: no scene, no ids in a block, nobody to read a
// state block back. During play the same facts are stated rather than called for.
func TestInventoryToolsBelongToCharacterCreation(t *testing.T) {
	offered := toolNames(characterCreationTools())
	for _, name := range []string{"add_item", "add_coins", "add_weapon", "add_armor"} {
		if !offered[name] {
			t.Errorf("character creation does not offer %s", name)
		}
	}

	inPlay := toolNames(queryTools())
	for _, name := range []string{"add_item", "add_coins"} {
		if inPlay[name] {
			t.Errorf("%s is still offered during play; the state block states it now", name)
		}
	}
}

// The DM never saw character creation and cannot read the character sheet, so
// the whole character has to reach it somehow. It is the entity map that carries
// them, rebuilt every turn — not the briefing, which is written once and would
// be contradicting the map by the second turn.
func TestTheOpeningBlockCarriesCharacterState(t *testing.T) {
	w := equippedWorld(t)
	w.AdventureContent = "Dust and trouble in Dryfall."
	pc, _ := w.Character("kyle-katarn")
	w.GiveItem(pc, "Blaster Pistol", "A worn DL-44 with a scored barrel.", 1)
	w.GiveItem(pc, "Power Cell", "Spares for the blaster.", 4)
	pc.AddCoins(75)

	block := newSession(w).engineState()
	for _, want := range []string{
		"kyle-katarn:",
		"coins: 75",
		"inventory: [blaster-pistol, power-cell]",
		// Each item is its own entity, with the ratings and wording it was
		// recorded with, so a thing can be reasoned about wherever it is.
		"count: 4",
		"A worn DL-44 with a scored barrel.",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("opening block missing %q:\n%s", want, block)
		}
	}

	// And the briefing does not, because a second copy would be the one that
	// goes stale the moment they spend a coin.
	if strings.Contains(openingBriefing(w), "coins") {
		t.Error("the briefing still embeds a snapshot of the character")
	}
}
