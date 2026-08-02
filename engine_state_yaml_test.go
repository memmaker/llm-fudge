package main

import (
	"strings"
	"testing"

	"fudge/narrative"
	"fudge/rules"

	"gopkg.in/yaml.v3"
)

// The block tells the DM it is reading YAML, and the whole flat entity design
// rests on that being true. Every other test here asserts that some fact reached
// the block; these assert the block is still a document, which is the one failure
// that would make every fact in it unreadable at once.
//
// The values worth attacking are the ones the engine did not compose: a
// character's description, the spot an item rests in, and above all a narrative
// property, which is whatever JSON the DM sent and is usually prose. A comma or a
// colon in a flow mapping ends a value early, and both of those turned up in the
// first real render.
func hostileWorld(t *testing.T) *session {
	t.Helper()
	w, err := rules.NewWorld(1, "settings/deadlands")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.CreateLocation("Dryfall Saloon", "Sawdust, smoke: low beams #warped", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := w.CreateLocation("Cellar", "Damp: stacked with casks, mostly", "dryfall-saloon"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Kyle Katarn", "Dust Bandit"} {
		if _, err := w.CreateCharacterValidated(rules.CharacterRequest{
			Name: name, Species: "human",
			Attributes:  map[string]int{"grit": 1},
			Skills:      map[string]int{"shootin": 2},
			Description: `Wiry; restless - and "quick": {mostly}`,
		}); err != nil {
			t.Fatal(err)
		}
	}
	pc, _ := w.Character("kyle-katarn")
	pc.Location = "cellar"
	pc.AddCoins(75)
	pc.Wounds.ApplyDamage(4)
	if _, err := w.GiveWeapon(pc, "Peacemaker", "A nicked .45.", 1, rules.Weapon{
		Damage: 3, Ranged: true, AmmoCapacity: 6, Ammo: 4, AmmoItem: "Cartridge Box",
	}); err != nil {
		t.Fatal(err)
	}
	dagger := &rules.Item{Name: "Dagger", Weapon: &rules.Weapon{Damage: 1}}
	dagger.ID = w.MintItemID(dagger.Name)
	if _, err := w.Locations["cellar"].AddItem(dagger, "quivering in the door frame: deep, and stuck", 1); err != nil {
		t.Fatal(err)
	}

	s := newSession(w)
	s.narrative.Apply([]narrative.Operation{
		{Op: narrative.OpSet, Entity: "dust-bandit", Properties: map[string]any{
			"mood":      "cornered",
			"suspicion": 0.83,
			"wants":     "the cargo, and no witnesses",
			"said":      "he told them: run",
			"armed":     true,
			"allies":    []any{"preacher-vane", "the barman, maybe"},
			"debts":     map[string]any{"to": "mira", "amount": "40 coins, overdue"},
			"nothing":   nil,
		}},
		{Op: narrative.OpCreate, Entity: "rumor-crypt", Properties: map[string]any{
			"type":        "rumor",
			"content":     "the count murdered his brother: they say, anyway",
			"believed_by": []any{"dust-bandit"},
		}},
	}, 12, w)
	return s
}

// entitiesDoc pulls the YAML half out of the block and parses it.
func entitiesDoc(t *testing.T, block string) map[string]any {
	t.Helper()
	start := strings.Index(block, "entities:")
	if start < 0 {
		t.Fatalf("block has no entities document:\n%s", block)
	}
	document := block[start:]
	if end := strings.Index(document, "\n=== TIMELINE"); end >= 0 {
		document = document[:end]
	}

	var parsed struct {
		Entities map[string]any `yaml:"entities"`
	}
	if err := yaml.Unmarshal([]byte(document), &parsed); err != nil {
		t.Fatalf("the block is not valid YAML: %v\n\n%s", err, document)
	}
	if parsed.Entities == nil {
		t.Fatalf("entities parsed as nothing:\n%s", document)
	}
	return parsed.Entities
}

func TestTheBlockIsValidYAML(t *testing.T) {
	entitiesDoc(t, hostileWorld(t).engineState())
}

// Parsing is not enough on its own: a value cut short by a comma can still leave
// a document that parses, just one that says something else. So the awkward
// values are read back and compared with what went in.
func TestHostileValuesSurviveTheRoundTrip(t *testing.T) {
	docEntities := entitiesDoc(t, hostileWorld(t).engineState())

	bandit, ok := docEntities["dust-bandit"].(map[string]any)
	if !ok {
		t.Fatalf("dust-bandit did not parse as a mapping: %#v", docEntities["dust-bandit"])
	}
	narr, ok := bandit["narrative"].(map[string]any)
	if !ok {
		t.Fatalf("narrative did not parse as a mapping: %#v", bandit["narrative"])
	}

	if got := narr["wants"]; got != "the cargo, and no witnesses" {
		t.Errorf("a comma truncated the value: %#v", got)
	}
	if got := narr["said"]; got != "he told them: run" {
		t.Errorf("a colon truncated the value: %#v", got)
	}
	if got := narr["suspicion"]; got != 0.83 {
		t.Errorf("a number did not survive as a number: %#v", got)
	}
	if got := narr["armed"]; got != true {
		t.Errorf("a bool did not survive as a bool: %#v", got)
	}
	// A list and a nested map arrive as whatever JSON the DM sent, and both have
	// to come back out shaped the way they went in.
	allies, ok := narr["allies"].([]any)
	if !ok || len(allies) != 2 || allies[1] != "the barman, maybe" {
		t.Errorf("a list did not survive: %#v", narr["allies"])
	}
	debts, ok := narr["debts"].(map[string]any)
	if !ok || debts["amount"] != "40 coins, overdue" {
		t.Errorf("a nested map did not survive: %#v", narr["debts"])
	}

	// The engine's own prose fields are just as capable of breaking the document.
	player, _ := docEntities["kyle-katarn"].(map[string]any)
	if got := player["desc"]; got != `Wiry; restless - and "quick": {mostly}` {
		t.Errorf("a description with YAML punctuation did not survive: %#v", got)
	}
	dagger, _ := docEntities["dagger"].(map[string]any)
	if got := dagger["spot"]; got != "quivering in the door frame: deep, and stuck" {
		t.Errorf("a spot with YAML punctuation did not survive: %#v", got)
	}
	cellar, _ := docEntities["cellar"].(map[string]any)
	if got := cellar["desc"]; got != "Damp: stacked with casks, mostly" {
		t.Errorf("a location description did not survive: %#v", got)
	}
}

// Every entity is one key in one mapping. This is the invariant the flat model
// exists for, asserted against the parsed document rather than the text: an
// entity that appeared twice would have silently collapsed to one key here, so
// the count is taken from the render and compared with what parsed.
func TestEveryEntityIsExactlyOneKey(t *testing.T) {
	s := hostileWorld(t)
	docEntities := entitiesDoc(t, s.engineState())

	want := []string{
		"kyle-katarn", "dust-bandit", // characters
		"dryfall-saloon", "cellar", // locations
		"peacemaker", "dagger", // items
		"rumor-crypt", // narrative-only
	}
	for _, id := range want {
		if _, held := docEntities[id]; !held {
			t.Errorf("%s is not in the parsed document: %v", id, keysOf(docEntities))
		}
	}
	if len(docEntities) != len(want) {
		t.Errorf("document holds %d entities, want %d: %v", len(docEntities), len(want), keysOf(docEntities))
	}

	// Every entity says what it is, which is the other half of the kernel.
	for id, entity := range docEntities {
		fields, ok := entity.(map[string]any)
		if !ok {
			t.Fatalf("%s did not parse as a mapping: %#v", id, entity)
		}
		if fields["type"] == nil || fields["type"] == "" {
			t.Errorf("%s has no type", id)
		}
	}
}

// Every reference points at an entity that exists. Under the old key an item's
// address changed when it moved, so this could not have been asserted at all.
func TestEveryReferenceResolves(t *testing.T) {
	docEntities := entitiesDoc(t, hostileWorld(t).engineState())

	for id, entity := range docEntities {
		fields := entity.(map[string]any)
		for _, field := range []string{"location", "parent"} {
			target, held := fields[field]
			if !held {
				continue
			}
			if _, exists := docEntities[target.(string)]; !exists {
				t.Errorf("%s.%s points at %q, which is not an entity", id, field, target)
			}
		}
		// An inventory lists ids, and each has to be one.
		if inventory, held := fields["inventory"].([]any); held {
			for _, itemID := range inventory {
				if _, exists := docEntities[itemID.(string)]; !exists {
					t.Errorf("%s carries %q, which is not an entity", id, itemID)
				}
			}
		}
	}
}

// An item's inventory entry and its own location field are two views of one
// storage, so they can never disagree — but that is a property of the code, not
// of the render, and the render is what the DM reads.
func TestInventoryAndLocationAgree(t *testing.T) {
	docEntities := entitiesDoc(t, hostileWorld(t).engineState())

	for id, entity := range docEntities {
		inventory, held := entity.(map[string]any)["inventory"].([]any)
		if !held {
			continue
		}
		for _, itemID := range inventory {
			item := docEntities[itemID.(string)].(map[string]any)
			if item["container"] != id {
				t.Errorf("%s carries %s, but %s says it is at %v", id, itemID, itemID, item["container"])
			}
		}
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
