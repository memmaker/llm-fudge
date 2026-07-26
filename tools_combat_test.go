package main

import (
	"strings"
	"testing"

	"fudge/rules"

	"github.com/memmaker/gollum/llm"
)

// combatants sets up a gunfight: a capable player character and a bandit to
// point things at.
func combatants(t *testing.T) *rules.World {
	t.Helper()
	w, err := rules.NewWorld(1, "settings/deadlands")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name   string
		skills map[string]int
	}{
		{"Kyle Katarn", map[string]int{"shootin": 3, "fightin": 3}},
		{"Dust Bandit", map[string]int{"shootin": -2, "fightin": -2}},
	} {
		if _, err := w.CreateCharacterValidated(rules.CharacterRequest{
			Name: c.name, Species: "human", Attributes: map[string]int{"grit": 2}, Skills: c.skills,
		}); err != nil {
			t.Fatal(err)
		}
	}
	return w
}

// The one-call promise: an attack rolls, decides, wounds and reports, with the
// ratings read off the sheets rather than supplied by the DM.
func TestOpposedAttackResolvesEverythingInOneCall(t *testing.T) {
	w := combatants(t)
	kyle, _ := w.Character("kyle-katarn")
	bandit, _ := w.Character("dust-bandit")

	for _, c := range []struct{ tool, args string }{
		{"add_weapon", `{"character_id":"kyle-katarn","name":"Peacemaker","description":"A nicked .45.","damage":3,"ranged":true,"ammo_capacity":6,"ammo_item":"Cartridge Box"}`},
		{"add_armor", `{"character_id":"dust-bandit","name":"Duster","description":"Oiled canvas.","protection":1}`},
	} {
		if _, err := executeAction(w, inventoryCall(c.tool, c.args)); err != nil {
			t.Fatalf("%s: %v", c.tool, err)
		}
	}

	// Loaded so heavily in Kyle's favour that the shot cannot miss, which keeps
	// the assertions about what a hit does free of the dice.
	result, err := executeAction(w, inventoryCall("opposed_attack", `{"attacker_id":"kyle-katarn","attacker_trait_type":"skill","attacker_trait_slug":"shootin","attacker_modifier":10,"defender_id":"dust-bandit","defender_trait_type":"skill","defender_trait_slug":"shootin","defender_modifier":-10,"weapon":"Peacemaker"}`))
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"Kyle Katarn", "Dust Bandit", "HIT", "Peacemaker+3", "Duster-1", "ammo:5/6"} {
		if !strings.Contains(result, want) {
			t.Errorf("result missing %q:\n%s", want, result)
		}
	}
	if bandit.Wounds.Level == rules.Undamaged {
		t.Error("a point-blank hit left the bandit undamaged")
	}
	if kyle.Inventory["peacemaker"].Weapon.Ammo != 5 {
		t.Errorf("ammo = %d, want 5 after one shot", kyle.Inventory["peacemaker"].Weapon.Ammo)
	}
	// An attack moves both sheets at once — the shooter's magazine and the
	// target's wounds — so both have to come back, and in that order.
	shooter := strings.Index(result, kyle.ToString())
	wounded := strings.Index(result, bandit.ToString())
	if shooter < 0 || wounded < 0 {
		t.Fatalf("result does not carry both characters' state:\n%s", result)
	}
	if shooter > wounded {
		t.Error("the attacker's state should be reported before the target's")
	}
}

// A target that cannot resist is a Poor difficulty unless the DM says the shot is
// harder than that (§4.2.2), so the argument can be left off entirely.
func TestUnopposedAttackDefaultsToPoorDifficulty(t *testing.T) {
	w := combatants(t)

	result, err := executeAction(w, inventoryCall("unopposed_attack",
		`{"attacker_id":"kyle-katarn","attacker_trait_type":"skill","attacker_trait_slug":"fightin","target_id":"dust-bandit"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "very easy(-2)") {
		t.Errorf("result does not show the default difficulty:\n%s", result)
	}
	if !strings.Contains(result, "unresisting") {
		t.Errorf("result should say the target put up no defence:\n%s", result)
	}
}

// A weapon whose strength comes from the arm behind it has to name an attribute
// the setting actually has, or every swing would silently lose that arm.
func TestAddWeaponValidatesTheDamageAttribute(t *testing.T) {
	w := combatants(t)

	if _, err := executeAction(w, inventoryCall("add_weapon",
		`{"character_id":"kyle-katarn","name":"Bowie Knife","description":"Heavy.","damage":1,"damage_attribute":"thews"}`)); err == nil {
		t.Error("an attribute the setting does not have should be refused")
	}
	if _, err := executeAction(w, inventoryCall("add_weapon",
		`{"character_id":"kyle-katarn","name":"Bowie Knife","description":"Heavy.","damage":1,"damage_attribute":"grit"}`)); err != nil {
		t.Errorf("a real attribute should be accepted: %v", err)
	}
}

// Ammunition is a supply, not a counter: reloading spends an item off the sheet,
// and a character with no boxes left is out for good.
func TestReloadSpendsAmmunition(t *testing.T) {
	w := combatants(t)
	kyle, _ := w.Character("kyle-katarn")

	for _, c := range []struct{ tool, args string }{
		{"add_weapon", `{"character_id":"kyle-katarn","name":"Peacemaker","description":"A nicked .45.","damage":3,"ranged":true,"ammo_capacity":1,"ammo_item":"Cartridge Box"}`},
		{"add_item", `{"character_id":"kyle-katarn","name":"Cartridge Box","description":"Loose rounds.","count":1}`},
	} {
		if _, err := executeAction(w, inventoryCall(c.tool, c.args)); err != nil {
			t.Fatalf("%s: %v", c.tool, err)
		}
	}
	shoot := inventoryCall("unopposed_attack", `{"attacker_id":"kyle-katarn","attacker_trait_type":"skill","attacker_trait_slug":"shootin","target_id":"dust-bandit","weapon":"Peacemaker"}`)

	if _, err := executeAction(w, shoot); err != nil {
		t.Fatal(err)
	}
	// The chamber is empty now, and that is the end of the shooting until the
	// gun is reloaded.
	if _, err := executeAction(w, shoot); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Errorf("firing an empty revolver: err = %v", err)
	}

	result, err := executeAction(w, inventoryCall("reload_weapon", `{"character_id":"kyle-katarn","weapon":"Peacemaker"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "ammo:1/1") {
		t.Errorf("reload did not fill the chamber:\n%s", result)
	}
	if _, held := kyle.Inventory["cartridge-box"]; held {
		t.Error("the reload should have spent the last box of cartridges")
	}
	if _, err := executeAction(w, shoot); err != nil {
		t.Fatalf("the reloaded gun should fire: %v", err)
	}
	// Out of cartridges: the reload fails and the fiction has to go elsewhere.
	if _, err := executeAction(w, inventoryCall("reload_weapon", `{"character_id":"kyle-katarn","weapon":"Peacemaker"}`)); err == nil {
		t.Error("reloading with no ammunition left should fail")
	}
}

// Armor is worn when it is recorded; taking it off is the exception the fiction
// occasionally turns on.
func TestEquipArmorTogglesProtection(t *testing.T) {
	w := combatants(t)
	bandit, _ := w.Character("dust-bandit")

	if _, err := executeAction(w, inventoryCall("add_armor",
		`{"character_id":"dust-bandit","name":"Steel Breastplate","description":"Dented.","protection":3}`)); err != nil {
		t.Fatal(err)
	}
	if best := bandit.BestWornArmor(); best == nil {
		t.Fatal("armor should be worn from the moment it is added")
	}
	if _, err := executeAction(w, inventoryCall("equip_armor",
		`{"character_id":"dust-bandit","name":"Steel Breastplate","worn":false}`)); err != nil {
		t.Fatal(err)
	}
	if best := bandit.BestWornArmor(); best != nil {
		t.Errorf("armor taken off still protects: %s", best.Name)
	}
}

// Ratings are useless to a DM that cannot see them: the condensed state has to
// say what the gun does and how many shots are left in it.
func TestWeaponAndArmorRatingsReachTheDM(t *testing.T) {
	w := combatants(t)
	kyle, _ := w.Character("kyle-katarn")

	for _, c := range []struct{ tool, args string }{
		{"add_weapon", `{"character_id":"kyle-katarn","name":"Peacemaker","description":"A nicked .45.","damage":3,"ranged":true,"ammo_capacity":6,"ammo":4,"ammo_item":"Cartridge Box"}`},
		{"add_weapon", `{"character_id":"kyle-katarn","name":"Bowie Knife","description":"Heavy.","damage":1,"damage_attribute":"grit"}`},
		{"add_armor", `{"character_id":"kyle-katarn","name":"Duster","description":"Oiled canvas.","protection":1,"worn":false}`},
	} {
		if _, err := executeAction(w, inventoryCall(c.tool, c.args)); err != nil {
			t.Fatalf("%s: %v", c.tool, err)
		}
	}

	state := kyle.ToString()
	for _, want := range []string{
		"Peacemaker|ranged|dmg+3|ammo:4/6|feeds:Cartridge Box",
		"Bowie Knife|melee|dmg+1|+grit",
		"Duster|armor+1|not-worn",
	} {
		if !strings.Contains(state, want) {
			t.Errorf("condensed state missing %q:\n%s", want, state)
		}
	}

	// The player reads the same facts in words on their own sheet.
	sheet := rules.StateFileText(w)
	for _, want := range []string{"ranged, damage +3", "ammo 4/6", "protection +1, carried"} {
		if !strings.Contains(sheet, want) {
			t.Errorf("character sheet missing %q:\n%s", want, sheet)
		}
	}
}

// Recording a sword as plain gear would strip the ratings it fights with, so an
// item already carried as one kind is never quietly converted into another.
func TestItemKindsDoNotCollide(t *testing.T) {
	w := combatants(t)

	if _, err := executeAction(w, inventoryCall("add_weapon",
		`{"character_id":"kyle-katarn","name":"Peacemaker","description":"A nicked .45.","damage":3,"ranged":true}`)); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ tool, args string }{
		{"add_item", `{"character_id":"kyle-katarn","name":"Peacemaker","description":"Just a lump of iron."}`},
		{"add_armor", `{"character_id":"kyle-katarn","name":"Peacemaker","description":"Somehow armor now.","protection":2}`},
	} {
		if _, err := executeAction(w, inventoryCall(c.tool, c.args)); err == nil {
			t.Errorf("%s should not overwrite a weapon of the same name", c.tool)
		}
	}
	kyle, _ := w.Character("kyle-katarn")
	if kyle.Inventory["peacemaker"].Weapon == nil {
		t.Error("the weapon lost its ratings")
	}
}

// Combat is played, not created: the attack calls belong to gameplay, while
// outfitting a character with gear happens in both flows.
func TestCombatToolsOfferedWherePlayed(t *testing.T) {
	offered := func(tools []llm.Tool) map[string]bool {
		names := map[string]bool{}
		for _, tool := range tools {
			names[tool.Function.Name] = true
		}
		return names
	}

	play := offered(gameplayTools())
	for _, name := range []string{"opposed_attack", "unopposed_attack", "reload_weapon"} {
		if !play[name] {
			t.Errorf("gameplay does not offer %s", name)
		}
	}
	creation := offered(characterCreationTools())
	for _, name := range []string{"add_weapon", "add_armor", "equip_armor"} {
		if !creation[name] || !play[name] {
			t.Errorf("%s must be offered in both character creation and gameplay", name)
		}
	}
	// The attack tools would have nothing to fight over during creation.
	if creation["opposed_attack"] {
		t.Error("character creation should not offer attacks")
	}
}
