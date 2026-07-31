package main

import (
	"strings"
	"testing"

	"fudge/rules"
)

// Every tool that can change a character reports that character back. The DM
// tracks nothing itself — if a call moves a wound, a Fudge Point, a coin or an
// item, the result has to say what the character looks like afterwards.
func TestMutatingToolsReportCharacterState(t *testing.T) {
	w, err := rules.NewWorld(1, "settings/deadlands")
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"Kyle Katarn", "Dust Bandit"} {
		if _, err := w.CreateCharacterValidated(rules.CharacterRequest{
			Name:       name,
			Species:    "human",
			Attributes: map[string]int{"grit": 1},
			Skills:     map[string]int{"shootin": 1},
		}); err != nil {
			t.Fatal(err)
		}
	}
	pc, _ := w.Character("kyle-katarn")
	pc.FudgePoints = 3

	for _, c := range []struct {
		tool    string
		args    string
		subject string
	}{
		{"create_character", `{"name":"Preacher Vane","species":"human"}`, "preacher-vane"},
		{"add_item", `{"name":"Peacemaker","description":"A nicked .45."}`, "kyle-katarn"},
		{"add_coins", `{"amount":40}`, "kyle-katarn"},
		{"unopposed_check", `{"actor_id":"kyle-katarn","trait_type":"skill","trait_slug":"shootin","difficulty":"average","add_one":true}`, "kyle-katarn"},
		{"opposed_check", `{"actor_id":"kyle-katarn","actor_trait_type":"skill","actor_trait_slug":"shootin","opponent_id":"dust-bandit","opponent_trait_type":"attribute","opponent_trait_slug":"grit","add_one":true}`, "kyle-katarn"},
	} {
		result, err := executeAction(w, inventoryCall(c.tool, c.args))
		if err != nil {
			t.Fatalf("%s: %v", c.tool, err)
		}
		ch, err := w.Character(c.subject)
		if err != nil {
			t.Fatalf("%s: %v", c.tool, err)
		}
		// The state must be the one left behind by this very call, so it is
		// compared against the character as they stand now, not merely sniffed
		// for a stray field.
		if !strings.HasSuffix(result, ch.ToString()) {
			t.Errorf("%s result does not end with %s's state:\n%s", c.tool, c.subject, result)
		}
	}
}

// The two checks can spend a Fudge Point, so the state they report has to show
// the point already gone — that is the whole reason they report it.
func TestChecksReportSpentFudgePoints(t *testing.T) {
	w, err := rules.NewWorld(1, "settings/deadlands")
	if err != nil {
		t.Fatal(err)
	}
	pc, err := w.CreateCharacterValidated(rules.CharacterRequest{
		Name: "Kyle Katarn", Species: "human", Skills: map[string]int{"shootin": 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	pc.FudgePoints = 2

	result, err := executeAction(w, inventoryCall("unopposed_check",
		`{"actor_id":"kyle-katarn","trait_type":"skill","trait_slug":"shootin","difficulty":"average","add_one":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if pc.FudgePoints != 1 {
		t.Fatalf("fudge points = %d, want 1 after add_one", pc.FudgePoints)
	}
	if !strings.Contains(result, "fudge_points: 1") {
		t.Errorf("result does not show the point as spent:\n%s", result)
	}
}

// ToString is the DM's whole view of a character, so it has to carry every
// field that changes how a scene is narrated.
func TestCharacterToString(t *testing.T) {
	ch := &rules.Character{
		ID: "kyle-katarn", Name: "Kyle Katarn", Species: "human", Scale: 0,
		Location: "dryfall", Description: "Rebel saboteur.",
		Attributes: map[string]int{"grit": 1}, Skills: map[string]int{"shootin": 2},
		Gifts: []string{"quick-draw"}, Faults: []string{"wanted"}, FudgePoints: 3,
	}
	ch.Wounds.ApplyDamage(4) // Hurt, -1
	// Items are minted into a world's id keyspace now, so one has to exist even
	// for a character assembled by hand.
	w := &rules.World{Characters: map[string]*rules.Character{ch.ID: ch}, Locations: map[string]*rules.Location{}}
	if _, err := w.GiveItem(ch, "Peacemaker", "A nicked .45.", 2); err != nil {
		t.Fatal(err)
	}
	ch.AddCoins(40)

	got := ch.ToString()
	for _, want := range []string{
		"Kyle Katarn", "kyle-katarn:", "human", "desc: Rebel saboteur",
		"wounds: hurt", "penalty: -1", "fudge_points: 3",
		"grit: +1", "shootin: +2", "quick-draw", "wanted",
		"coins: 40", "peacemaker",
		// Position is stated now that the engine maintains it: the narration
		// names the scene every turn and move_character moves everybody else.
		"location: dryfall",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ToString missing %q:\n%s", want, got)
		}
	}

	// An incapacitated character cannot act at all, and the DM has to know that
	// before it narrates them doing something.
	ch.Wounds.ApplyDamage(8)
	if !strings.Contains(ch.ToString(), "cannot_act") {
		t.Errorf("downed character not flagged:\n%s", ch.ToString())
	}
}

// Empty fields are left out rather than written as blanks — except possessions,
// where silence reads as "the engine did not say" instead of "they have none".
func TestCharacterToStringOmitsEmptyFields(t *testing.T) {
	bare := &rules.Character{ID: "drifter", Name: "Drifter"}

	got := bare.ToString()
	for _, unwanted := range []string{"desc:", "gifts:", "faults:", "attributes:", "skills:"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("ToString should omit empty %q:\n%s", unwanted, got)
		}
	}
	for _, want := range []string{"wounds: undamaged", "fudge_points: 0", "coins: 0", "inventory: []"} {
		if !strings.Contains(got, want) {
			t.Errorf("ToString missing %q:\n%s", want, got)
		}
	}
	// An undamaged character carries no penalty, so none is reported.
	if strings.Contains(got, "penalty:") {
		t.Errorf("undamaged character should report no penalty:\n%s", got)
	}
}

// The counterpart rule: a call that moved nothing says nothing. Restating
// unchanged state would burn context and leave the DM two copies to choose
// between, so silence means "as you were last told".
func TestUnchangedStateIsNotRepeated(t *testing.T) {
	w, err := rules.NewWorld(1, "settings/deadlands")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Kyle Katarn", "Dust Bandit"} {
		if _, err := w.CreateCharacterValidated(rules.CharacterRequest{
			Name: name, Species: "human",
			Attributes: map[string]int{"grit": 1}, Skills: map[string]int{"shootin": 1},
		}); err != nil {
			t.Fatal(err)
		}
	}
	pc, _ := w.Character("kyle-katarn")
	pc.FudgePoints = 2

	for _, c := range []struct {
		what string
		tool string
		args string
	}{
		{"a check that spends no Fudge Point", "unopposed_check",
			`{"actor_id":"kyle-katarn","trait_type":"skill","trait_slug":"shootin","difficulty":"average"}`},
		{"an opposed check that spends no Fudge Point", "opposed_check",
			`{"actor_id":"kyle-katarn","actor_trait_type":"skill","actor_trait_slug":"shootin","opponent_id":"dust-bandit","opponent_trait_type":"attribute","opponent_trait_slug":"grit"}`},
	} {
		result, err := executeAction(w, inventoryCall(c.tool, c.args))
		if err != nil {
			t.Fatalf("%s: %v", c.tool, err)
		}
		if strings.Contains(result, "\n") {
			t.Errorf("%s reported state anyway:\n%s", c.what, result)
		}
	}

	if pc.FudgePoints != 2 {
		t.Errorf("fudge points = %d; the checks above were meant to spend none", pc.FudgePoints)
	}
}

// create_character answers with the character and nothing else. The id the DM
// needs for every later call is the first field of it, so a headline repeating
// that id would be the only thing the extra line said.
func TestCreateCharacterReturnsCharacterState(t *testing.T) {
	w, err := rules.NewWorld(1, "settings/deadlands")
	if err != nil {
		t.Fatal(err)
	}

	result, err := executeAction(w, inventoryCall("create_character",
		`{"name":"Preacher Vane","species":"human","description":"Buries the dead, badly.","skills":{"occult":2}}`))
	if err != nil {
		t.Fatal(err)
	}

	ch, err := w.Character("preacher-vane")
	if err != nil {
		t.Fatal(err)
	}
	if result != ch.ToString() {
		t.Errorf("result is not the character's state:\n%s", result)
	}
	// The id is the handle for every call that follows; losing it would strand
	// the DM with a character it cannot name.
	if !strings.Contains(result, "preacher-vane:") {
		t.Errorf("result does not carry the generated id:\n%s", result)
	}
}

// Outfitting somebody used to be a call each for the knife, the apron, the kit
// and the purse, every one of them waiting on the id the first call returned.
// Mid-turn it could not be done at all: add_weapon is not on the table during
// play, so an NPC created for a fight fought bare-handed however the prose
// described him. One call now, and the ratings have to survive it — an item
// created here is one an attack reads.
func TestCreateCharacterOutfitsInOneCall(t *testing.T) {
	w, err := rules.NewWorld(1, "settings/deadlands")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := executeAction(w, inventoryCall("create_character", `{
		"name":"Mackie","species":"human","description":"A sharp-eyed knife sharpener.",
		"coins":18,
		"inventory":[
			{"name":"Slim Work Knife","description":"A balanced utility blade.","weapon":{"damage":0,"adds":"grit"}},
			{"name":"Worn Leather Apron","description":"Stiff with filings and oil.","armor":{"protection":1}},
			{"name":"Whetstones","description":"Fine stones in a wooden box.","count":3}
		]}`)); err != nil {
		t.Fatal(err)
	}

	ch, err := w.Character("mackie")
	if err != nil {
		t.Fatal(err)
	}
	if ch.Coins != 18 {
		t.Errorf("purse = %d, want 18", ch.Coins)
	}
	if len(ch.Inventory) != 3 {
		t.Fatalf("carrying %d items, want 3", len(ch.Inventory))
	}

	knife, ok := ch.Inventory["slim-work-knife"]
	if !ok || knife.Weapon == nil {
		t.Fatalf("the knife is not carried as a weapon: %+v", ch.Inventory)
	}
	// A weapon whose rating did not survive the call is gear that cannot be
	// swung, which is the whole failure this replaces.
	if knife.Weapon.DamageAttribute != "grit" {
		t.Errorf("the arm behind the knife was lost: %+v", knife.Weapon)
	}

	apron, ok := ch.Inventory["worn-leather-apron"]
	if !ok || apron.Armor == nil {
		t.Fatalf("the apron is not carried as armor: %+v", ch.Inventory)
	}
	// Worn by default, as add_armor has always had it: armor recorded as cargo
	// protects nobody, and nothing would ever say so.
	if !apron.Armor.Worn {
		t.Error("armor created without an explicit `worn` is not being worn")
	}
	if apron.Armor.Protection != 1 {
		t.Errorf("protection = %d, want 1", apron.Armor.Protection)
	}

	if stones := ch.Inventory["whetstones"]; stones == nil || stones.Count != 3 {
		t.Errorf("count did not survive: %+v", stones)
	}
}

// A rating the setting cannot honour is refused, and refused before the
// character lands: a half-outfitted NPC is one somebody has to notice and
// repair, and nothing downstream is looking.
func TestCreateCharacterRefusesBadGearWithoutCreatingAnybody(t *testing.T) {
	w, err := rules.NewWorld(1, "settings/deadlands")
	if err != nil {
		t.Fatal(err)
	}

	_, err = executeAction(w, inventoryCall("create_character", `{
		"name":"Mackie","species":"human",
		"inventory":[{"name":"Odd Blade","description":"Wrong.","weapon":{"damage":1,"adds":"strength"}}]}`))
	if err == nil {
		t.Fatal("an attribute the setting does not have was accepted")
	}
	if !strings.Contains(err.Error(), "strength") {
		t.Errorf("the error does not name the bad attribute: %v", err)
	}
	if _, err := w.Character("mackie"); err == nil {
		t.Error("the character was created despite the refusal")
	}
}
