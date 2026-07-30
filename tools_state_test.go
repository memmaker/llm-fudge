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
