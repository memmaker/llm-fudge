package rules

import (
	"math/rand"
	"strings"
	"testing"
)

func combatWorld() *World {
	return &World{Characters: map[string]*Character{}, Rng: rand.New(rand.NewSource(1))}
}

// fighter builds a combatant directly, so a test can state the one thing it is
// about — a Scale, a strength, an armor rating — without a setting behind it.
func fighter(w *World, id, name string, scale int, attrs, skills map[string]int) *Character {
	ch := w.CreateCharacter(id, name, attrs, skills)
	ch.Scale = scale
	return ch
}

// landed rigs an attack that has already been rolled and won by the given
// margin, so the damage arithmetic can be checked without fighting the dice.
func (w *World) landed(actor, target *Character, weapon *Item, degree int) *AttackResult {
	res := &AttackResult{
		ActorName:      actor.Name,
		TargetName:     target.Name,
		Hit:            true,
		RelativeDegree: degree,
		MinimumToHit:   minimumToHit(actor, target),
	}
	w.resolveBlow(res, actor, target, weapon, false)
	return res
}

// The whole point of the attack tools: the margin of the winning roll, the
// weapon, the arm swinging it and the armor in the way are totalled in one step
// (§4.5.5), and the wound that falls out is applied.
func TestDamageFactorTotalsEveryFactor(t *testing.T) {
	w := combatWorld()
	kyle := fighter(w, "kyle", "Kyle", 0, map[string]int{"strength": 2}, nil)
	bandit := fighter(w, "bandit", "Bandit", 0, nil, nil)

	sword, err := w.GiveWeapon(kyle, "Sword", "Notched.", 1, Weapon{Damage: 1, DamageAttribute: "strength"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.GiveArmor(bandit, "Leather Jerkin", "Cracked.", 1, Armor{Protection: 2, Worn: true}); err != nil {
		t.Fatal(err)
	}

	// degree 3 + sword 1 + strength 2 - jerkin 2 = 4, which is a Hurt.
	res := w.landed(kyle, bandit, sword, 3)
	if res.DamageFactor != 4 {
		t.Errorf("damage factor = %d, want 4", res.DamageFactor)
	}
	if res.Wound != "hurt" {
		t.Errorf("wound = %q, want hurt", res.Wound)
	}
	if bandit.Wounds.Level != Hurt {
		t.Errorf("the wound was not applied: %s", bandit.Wounds.Level)
	}
	if res.Penalty != -1 {
		t.Errorf("penalty = %d, want -1", res.Penalty)
	}
	// The line the DM reads has to carry the arithmetic, not just the verdict:
	// a blow blunted by armor is narrated differently from a clean one.
	for _, want := range []string{"HIT", "dmg+4", "deg+3", "Sword+1", "strength+2", "Leather Jerkin-2", "Bandit:hurt"} {
		if !strings.Contains(res.String(), want) {
			t.Errorf("result line missing %q:\n%s", want, res.String())
		}
	}
}

// A weapon with its own power source hits the same however weak the hand holding
// it; only muscle-powered weapons name an attribute to add.
func TestOnlyMusclePoweredWeaponsAddStrength(t *testing.T) {
	w := combatWorld()
	kyle := fighter(w, "kyle", "Kyle", 0, map[string]int{"strength": 3}, nil)
	bandit := fighter(w, "bandit", "Bandit", 0, nil, nil)

	blaster, _ := w.GiveWeapon(kyle, "Blaster", "Scored barrel.", 1, Weapon{Damage: 3, Ranged: true})
	res := w.landed(kyle, bandit, blaster, 0)
	if res.DamageFactor != 3 {
		t.Errorf("damage factor = %d, want 3 (strength must not count)", res.DamageFactor)
	}
	if res.StrengthTrait != "" {
		t.Errorf("strength trait = %q, want none", res.StrengthTrait)
	}
}

// A bare fist is a damage factor of its own (§4.5.4), and a weak one.
func TestUnarmedAttack(t *testing.T) {
	w := combatWorld()
	kyle := fighter(w, "kyle", "Kyle", 0, nil, nil)
	bandit := fighter(w, "bandit", "Bandit", 0, nil, nil)

	res := w.landed(kyle, bandit, nil, 2)
	if res.DamageFactor != 1 {
		t.Errorf("damage factor = %d, want 1 (degree 2 less an unarmed -1)", res.DamageFactor)
	}
	if !strings.Contains(res.String(), "unarmed-1") {
		t.Errorf("result should name the empty hand:\n%s", res.String())
	}
}

// §4.5.6: a blow that only just won is a graze, and a graze can never do worse
// than a light wound however heavy the thing swung.
func TestGrazeIsCappedNoMatterTheWeapon(t *testing.T) {
	w := combatWorld()
	kyle := fighter(w, "kyle", "Kyle", 0, nil, nil)

	for _, c := range []struct {
		what   string
		damage int
		want   WoundLevel
	}{
		{"a cannon that merely grazed", 20, Hurt},
		{"a knife that merely grazed", 1, Scratch},
	} {
		bandit := fighter(w, "bandit-"+c.what, "Bandit", 0, nil, nil)
		weapon, _ := w.GiveWeapon(kyle, c.what, "Rigged for the test.", 1, Weapon{Damage: c.damage})
		res := w.landed(kyle, bandit, weapon, 1)
		if !res.Graze {
			t.Errorf("%s: degree +1 should be a graze", c.what)
		}
		if bandit.Wounds.Level != c.want {
			t.Errorf("%s: wound = %s, want %s", c.what, bandit.Wounds.Level, c.want)
		}
	}
}

// Armor protects the person wearing it and nobody else. A breastplate hauled in
// a pack is cargo.
func TestOnlyWornArmorProtects(t *testing.T) {
	w := combatWorld()
	kyle := fighter(w, "kyle", "Kyle", 0, nil, nil)
	bandit := fighter(w, "bandit", "Bandit", 0, nil, nil)
	knife, _ := w.GiveWeapon(kyle, "Knife", "Short.", 1, Weapon{Damage: 0})

	if _, err := w.GiveArmor(bandit, "Breastplate", "Looted, still bloody.", 1, Armor{Protection: 4, Worn: false}); err != nil {
		t.Fatal(err)
	}
	if res := w.landed(kyle, bandit, knife, 3); res.DamageFactor != 3 {
		t.Errorf("carried armor protected the carrier: damage factor = %d, want 3", res.DamageFactor)
	}

	if _, err := bandit.EquipArmor("Breastplate", true); err != nil {
		t.Fatal(err)
	}
	res := w.landed(kyle, bandit, knife, 3)
	if res.DamageFactor != -1 {
		t.Errorf("worn armor did not protect: damage factor = %d, want -1", res.DamageFactor)
	}
	if res.Wound != "" {
		t.Errorf("the plate should have turned the blade entirely, got %q", res.Wound)
	}
	if !strings.Contains(res.String(), "Bandit:unhurt") {
		t.Errorf("a blow stopped by armor must say so:\n%s", res.String())
	}
}

// Fudge totals one armor rating, not a wardrobe: the better piece takes the blow.
func TestBestWornArmorTakesTheBlow(t *testing.T) {
	w := combatWorld()
	bandit := fighter(w, "bandit", "Bandit", 0, nil, nil)
	w.GiveArmor(bandit, "Padded Coat", "Filthy.", 1, Armor{Protection: 1, Worn: true})
	w.GiveArmor(bandit, "Mail Shirt", "Rusted rings.", 1, Armor{Protection: 2, Worn: true})
	w.GiveArmor(bandit, "Plate", "In the cart.", 1, Armor{Protection: 4, Worn: false})

	best := bandit.BestWornArmor()
	if best == nil || best.Name != "Mail Shirt" {
		t.Fatalf("best worn armor = %v, want the Mail Shirt", best)
	}
}

// §4.2.2 and §4.5.8: a size gap dominates a fight. It sets how good a blow must
// be to connect at all, and it swamps the weapon once one does.
func TestScaleGoverningHitAndDamage(t *testing.T) {
	for _, c := range []struct {
		what               string
		attacker, defender int
		wantMinimum        int
	}{
		{"human vs human", 0, 0, -2},
		{"human vs halfling", 0, -2, -1},
		{"human vs cat", 0, -6, 1},
		{"human vs pixie", 0, -10, 2},
		{"human vs dragon", 0, 8, -3},
		{"dragon vs human", 8, 0, 2},
	} {
		w := combatWorld()
		a := fighter(w, "a", "Attacker", c.attacker, nil, nil)
		d := fighter(w, "d", "Defender", c.defender, nil, nil)
		if got := minimumToHit(a, d); got != c.wantMinimum {
			t.Errorf("%s: minimum to hit = %+d, want %+d", c.what, got, c.wantMinimum)
		}
	}

	// A fist against something eight Scale steps up barely registers.
	w := combatWorld()
	wilbur := fighter(w, "wilbur", "Wilbur", 0, nil, nil)
	dragon := fighter(w, "dragon", "Dragon", 8, nil, nil)
	if res := w.landed(wilbur, dragon, nil, 4); res.Wound != "" {
		t.Errorf("a punch wounded a dragon: %s", res.String())
	}
	// And the dragon's answer is not survivable in armor.
	if res := w.landed(dragon, wilbur, nil, 1); res.Wound == "" {
		t.Errorf("a dragon's swipe left no mark: %s", res.String())
	}
}

// Every shot fired counts down the magazine, and a miss is still a shot fired.
func TestAmmunitionIsSpentHitOrMiss(t *testing.T) {
	w := combatWorld()
	kyle := fighter(w, "kyle", "Kyle", 0, nil, nil)
	bandit := fighter(w, "bandit", "Bandit", 0, nil, nil)
	blaster, err := w.GiveWeapon(kyle, "Blaster", "Scored barrel.", 1, Weapon{Damage: 3, Ranged: true, AmmoCapacity: 2, AmmoItem: "Power Cell"})
	if err != nil {
		t.Fatal(err)
	}
	// A magazine the DM did not fill starts full: a gun handed over empty is
	// almost never what was meant.
	if blaster.Weapon.Ammo != 2 {
		t.Fatalf("ammo = %d, want a full magazine of 2", blaster.Weapon.Ammo)
	}

	w.landed(kyle, bandit, blaster, 2)
	missed := &AttackResult{ActorName: kyle.Name, TargetName: bandit.Name, Miss: "outrolled"}
	w.resolveBlow(missed, kyle, bandit, blaster, false)

	if blaster.Weapon.Ammo != 0 {
		t.Errorf("ammo = %d, want 0: both the hit and the miss cost a shot", blaster.Weapon.Ammo)
	}
	if !strings.Contains(missed.String(), "ammo:0/2 EMPTY") {
		t.Errorf("an emptied weapon must say so:\n%s", missed.String())
	}
}

// An empty weapon does not fire, and finding that out costs nothing: no roll, no
// Fudge Point, no wound. The DM narrates the dead trigger instead.
func TestEmptyWeaponCannotFire(t *testing.T) {
	w := combatWorld()
	kyle := fighter(w, "kyle", "Kyle", 0, nil, map[string]int{"shooting": 3})
	kyle.FudgePoints = 1
	bandit := fighter(w, "bandit", "Bandit", 0, nil, map[string]int{"dodge": 0})
	blaster, _ := w.GiveWeapon(kyle, "Blaster", "Scored barrel.", 1, Weapon{Damage: 3, Ranged: true, AmmoCapacity: 1})
	blaster.Weapon.Ammo = 0

	_, err := w.OpposedAttack(AttackRequest{
		ActorID: "kyle", ActorTraitType: TraitTypeSkill, ActorTraitSlug: "shooting",
		TargetID: "bandit", TargetTraitType: TraitTypeSkill, TargetTraitSlug: "dodge",
		Weapon: "Blaster", AddOne: true,
	})
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("err = %v, want a complaint that the blaster is empty", err)
	}
	if kyle.FudgePoints != 1 {
		t.Errorf("fudge points = %d: a refused attack must cost nothing", kyle.FudgePoints)
	}
	if bandit.Wounds.Level != Undamaged {
		t.Error("a refused attack must not wound anybody")
	}
}

// A thrown weapon leaves the hand, but the engine does not decide where it
// lands: it may be buried in the target, skittering under a table or out of a
// window into the alley, and only the prose knows which. So the engine reports
// the throw and leaves the sheet alone, and the DM moves it once it has read
// the result and knows what it is narrating.
func TestThrowingIsReportedButLeavesTheSheetToTheDM(t *testing.T) {
	w := combatWorld()
	kyle := fighter(w, "kyle", "Kyle", 0, map[string]int{"strength": 1}, nil)
	bandit := fighter(w, "bandit", "Bandit", 0, nil, nil)
	knives, _ := w.GiveWeapon(kyle, "Throwing Knife", "A matched pair.", 2, Weapon{Damage: 0, DamageAttribute: "strength"})

	res := &AttackResult{ActorName: kyle.Name, TargetName: bandit.Name, Hit: true, RelativeDegree: 3}
	w.resolveBlow(res, kyle, bandit, knives, true)

	if got := kyle.carried("throwing-knife").Count; got != 2 {
		t.Errorf("knives left = %d, want both still on the sheet", got)
	}
	if !res.Thrown {
		t.Errorf("the throw was not recorded: %+v", res)
	}
}

// The result line is the text the DM reads at the moment it is about to
// narrate, so it states the outstanding move rather than a bare fact it would
// have to draw the conclusion from.
func TestThrownResultAsksForTheMove(t *testing.T) {
	w := combatWorld()
	kyle := fighter(w, "kyle", "Kyle", 0, nil, nil)
	bandit := fighter(w, "bandit", "Bandit", 0, nil, nil)
	knife, _ := w.GiveWeapon(kyle, "Throwing Knife", "Balanced.", 1, Weapon{Damage: 0})

	res := &AttackResult{ActorName: kyle.Name, TargetName: bandit.Name, Hit: true, RelativeDegree: 3}
	w.resolveBlow(res, kyle, bandit, knife, true)

	line := res.String()
	for _, want := range []string{"Throwing Knife", "move_item", "Kyle"} {
		if !strings.Contains(line, want) {
			t.Errorf("result = %q, want it to mention %q", line, want)
		}
	}
}

// Death is never arithmetic. However savage the blow, a character stops at Near
// Death and is killed only by an explicit declare_dead.
func TestAttacksNeverKill(t *testing.T) {
	w := combatWorld()
	kyle := fighter(w, "kyle", "Kyle", 0, nil, nil)
	bandit := fighter(w, "bandit", "Bandit", 0, nil, nil)
	cannon, _ := w.GiveWeapon(kyle, "Deck Gun", "Absurd.", 1, Weapon{Damage: 20, Ranged: true})

	w.landed(kyle, bandit, cannon, 4)
	if bandit.Wounds.Level != NearDeath {
		t.Fatalf("wound level = %s, want near death", bandit.Wounds.Level)
	}
	w.landed(kyle, bandit, cannon, 4)
	if bandit.Wounds.Level == Dead {
		t.Error("an attack killed a character; only declare_dead may do that")
	}
}

// The attack tools refuse what the fiction cannot support, before any dice are
// rolled, so the DM reads the reason and narrates around it.
func TestAttacksRefuseTheImpossible(t *testing.T) {
	w := combatWorld()
	kyle := fighter(w, "kyle", "Kyle", 0, nil, map[string]int{"fighting": 1})
	bandit := fighter(w, "bandit", "Bandit", 0, nil, map[string]int{"fighting": 1})
	w.GiveItem(kyle, "Lucky Coin", "Not a weapon.", 1)

	swing := func(weapon string) error {
		_, err := w.OpposedAttack(AttackRequest{
			ActorID: "kyle", ActorTraitType: TraitTypeSkill, ActorTraitSlug: "fighting",
			TargetID: "bandit", TargetTraitType: TraitTypeSkill, TargetTraitSlug: "fighting",
			Weapon: weapon,
		})
		return err
	}

	if err := swing("Vibroblade"); err == nil || !strings.Contains(err.Error(), "does not carry") {
		t.Errorf("swinging a weapon they do not carry: err = %v", err)
	}
	if err := swing("Lucky Coin"); err == nil || !strings.Contains(err.Error(), "not a weapon") {
		t.Errorf("swinging plain gear: err = %v", err)
	}

	// A defender who cannot act cannot defend: that attack is unopposed, and the
	// error says so rather than rolling a defence nobody could make.
	bandit.Wounds.ApplyDamage(7)
	if err := swing(""); err == nil || !strings.Contains(err.Error(), "unopposed_attack") {
		t.Errorf("attacking a downed defender: err = %v", err)
	}
	if _, err := w.UnopposedAttack(AttackRequest{
		ActorID: "kyle", ActorTraitType: TraitTypeSkill, ActorTraitSlug: "fighting",
		TargetID: "bandit", Difficulty: -2,
	}); err != nil {
		t.Errorf("an unopposed attack on a downed target should work: %v", err)
	}

	bandit.Wounds.DeclareDead()
	if err := swing(""); err == nil || !strings.Contains(err.Error(), "already dead") {
		t.Errorf("attacking a corpse: err = %v", err)
	}

	kyle.Wounds.ApplyDamage(9)
	if err := swing(""); err == nil || !strings.Contains(err.Error(), "cannot attack") {
		t.Errorf("attacking while down: err = %v", err)
	}
}

// An unresisted blow is rolled against a difficulty, and the margin over it does
// the work the relative degree does in an opposed fight (§4.2.2). Ambushes are
// meant to be brutal.
func TestUnopposedAttackUsesTheMarginOverTheDifficulty(t *testing.T) {
	w := combatWorld()
	kyle := fighter(w, "kyle", "Kyle", 0, nil, map[string]int{"fighting": 3})
	sleeper := fighter(w, "sleeper", "Sleeper", 0, nil, nil)
	w.GiveWeapon(kyle, "Dagger", "Thin.", 1, Weapon{Damage: 0})

	res, err := w.UnopposedAttack(AttackRequest{
		ActorID: "kyle", ActorTraitType: TraitTypeSkill, ActorTraitSlug: "fighting",
		TargetID: "sleeper", Difficulty: -2, DifficultyName: "very easy", Weapon: "Dagger",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Opposed {
		t.Error("an unopposed attack must not report itself as opposed")
	}
	if res.RelativeDegree != res.ActorTotal-(-2) {
		t.Errorf("degree %+d is not the margin over the difficulty (total %d)", res.RelativeDegree, res.ActorTotal)
	}
	// Superb fighting against a Poor difficulty cannot fail on 4dF.
	if !res.Hit {
		t.Fatalf("a Superb attack on a sleeping target missed: %s", res.String())
	}
	if res.Wound == "" {
		t.Errorf("a knife in a sleeper's ribs left no wound: %s", res.String())
	}
	if !strings.Contains(res.String(), "unresisting") {
		t.Errorf("the line should say the target put up no defence:\n%s", res.String())
	}
	if sleeper.Wounds.Level == Undamaged {
		t.Error("the wound was not applied to the sleeper")
	}
}

// The wound track is read off the total damage factor (§4.5.5). Nothing at or
// below zero leaves a mark at all — armor turned it, or the target is simply too
// big to notice.
func TestWoundPointsFollowTheTrack(t *testing.T) {
	for _, c := range []struct {
		factor int
		graze  bool
		want   int
	}{
		{-3, false, 0}, {0, false, 0}, {1, false, 1}, {4, false, 4}, {9, false, 9},
		{-1, true, 0}, {0, true, 1}, {4, true, 1}, {5, true, 3}, {20, true, 3},
	} {
		if got := woundPoints(c.factor, c.graze); got != c.want {
			t.Errorf("woundPoints(%d, graze=%v) = %d, want %d", c.factor, c.graze, got, c.want)
		}
	}
}
