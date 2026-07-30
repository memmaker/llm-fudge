package rules

import (
	"fmt"
	"strings"
)

// minimumToHitEqual is the result needed to land a blow on a roughly equal-sized
// opponent (§4.2.2): Poor. Scale shifts it from there.
const minimumToHitEqual = -2

// unarmedDamage is the offensive damage factor of a bare fist (§4.5.4).
const unarmedDamage = -1

// minimumToHit is the result an attacker must reach to touch this defender at
// all, quite apart from winning the Opposed action (§4.2.2). A human needs a
// Poor blow to hit another human; something much smaller takes real precision to
// catch, while a blow aimed at something much larger can hardly miss it.
func minimumToHit(attacker, defender *Character) int {
	// The Scale table runs in steps of two, so half the gap is the number of
	// steps between the two fighters.
	minimum := minimumToHitEqual + (attacker.Scale-defender.Scale)/2
	if minimum < -3 {
		return -3
	}
	if minimum > 2 {
		return 2
	}
	return minimum
}

// AttackRequest is one attack as the DM described it. Weapon is the name the
// weapon is carried under; left out, the attacker is swinging a fist. Thrown
// sends a hand weapon downrange, which takes it off the sheet.
//
// Difficulty and the target trait fields belong to different attacks: an
// unopposed attack rolls against a Difficulty Level, an opposed one against the
// defence the target puts up.
type AttackRequest struct {
	ActorID         string
	ActorTraitType  string
	ActorTraitSlug  string
	ActorModifier   int
	TargetID        string
	TargetTraitType string
	TargetTraitSlug string
	TargetModifier  int
	Difficulty      int
	DifficultyName  string
	Weapon          string
	Thrown          bool
	AddOne          bool
	Reroll          bool
}

// AttackResult is everything one attack settled: the roll, whether it landed,
// how the damage factor was arrived at, and the wound it left. It carries the
// arithmetic rather than just the verdict, so the DM can narrate a blow that
// glanced off armor differently from one that missed.
type AttackResult struct {
	Opposed    bool
	ActorName  string
	TargetName string

	Roll           int
	ActorTotal     int
	TargetTotal    int
	Difficulty     int
	DifficultyName string
	RelativeDegree int
	MinimumToHit   int
	CritSuccess    bool
	CritFailure    bool
	Rerolled       bool
	DiscardedRoll  int

	Hit   bool
	Graze bool
	// Miss says why nothing landed: outrolled, standoff, or a blow too feeble to
	// reach the target at all. Empty when the attack connected.
	Miss string

	WeaponName      string
	WeaponDamage    int
	StrengthBonus   int
	StrengthTrait   string
	ActorScale      int
	ArmorName       string
	ArmorProtection int
	TargetScale     int
	DamageFactor    int

	// Wound is the level the target has been driven to, empty when the blow left
	// no mark. Scratches is where their scratch track stands afterwards.
	Wound     string
	Scratches int
	Penalty   int

	AmmoTracked bool
	AmmoLeft    int
	AmmoMax     int
	// Thrown records that the weapon left the hand. It is still on ThrownBy's
	// sheet: the engine cannot know where it landed, so the move is the DM's,
	// and the rendered result asks for it in as many words.
	Thrown   bool
	ThrownBy string
}

// prepareAttack settles everything an attack needs before a die is rolled: that
// both characters exist, that the attacker can act and the target can still be
// hurt, and that the weapon is carried and has a shot in it. Nothing here
// consumes a roll, a Fudge Point or a round of ammunition, so an attack that
// cannot happen costs nothing and the DM can narrate the empty chamber instead.
func (w *World) prepareAttack(req AttackRequest) (*Character, *Character, *Item, error) {
	actor, err := w.Character(req.ActorID)
	if err != nil {
		return nil, nil, nil, err
	}
	target, err := w.Character(req.TargetID)
	if err != nil {
		return nil, nil, nil, err
	}
	if actor == target {
		return nil, nil, nil, fmt.Errorf("%s cannot attack themselves", actor.Name)
	}
	if !actor.Wounds.CanAct() {
		return nil, nil, nil, fmt.Errorf("%s is %s and cannot attack", actor.Name, strings.ToLower(actor.Wounds.Level.String()))
	}
	if target.Wounds.Level == Dead {
		return nil, nil, nil, fmt.Errorf("%s is already dead", target.Name)
	}

	if req.Weapon == "" {
		if req.Thrown {
			return nil, nil, nil, fmt.Errorf("nothing to throw: name the weapon being thrown")
		}
		return actor, target, nil, nil
	}
	item, err := actor.WeaponItem(req.Weapon)
	if err != nil {
		return nil, nil, nil, err
	}
	if !item.Weapon.Loaded() {
		return nil, nil, nil, fmt.Errorf("%s is empty (0/%d): reload it before firing", item.Name, item.Weapon.AmmoCapacity)
	}
	return actor, target, item, nil
}

// OpposedAttack resolves an attack the target is actively resisting: the ordinary
// shape of Fudge combat (§4.2.2). The attacker's offence is rolled against
// whatever defence the DM named — parry, dodge, shield — and a blow that wins
// the Opposed action and reaches the minimum needed to hit is put through the
// damage factors in one step.
func (w *World) OpposedAttack(req AttackRequest) (*AttackResult, error) {
	actor, target, weapon, err := w.prepareAttack(req)
	if err != nil {
		return nil, err
	}
	if !target.Wounds.CanAct() {
		return nil, fmt.Errorf("%s is %s and cannot defend: use unopposed_attack", target.Name, strings.ToLower(target.Wounds.Level.String()))
	}

	check, err := w.OpposedCheck(req.ActorID, req.ActorTraitType, req.ActorTraitSlug, req.ActorModifier,
		req.TargetID, req.TargetTraitType, req.TargetTraitSlug, req.TargetModifier, req.AddOne)
	if err != nil {
		return nil, err
	}

	res := &AttackResult{
		Opposed:        true,
		ActorName:      actor.Name,
		TargetName:     target.Name,
		ActorTotal:     check.ActorTotal,
		TargetTotal:    check.OpponentTotal,
		RelativeDegree: check.RelativeDegree,
		MinimumToHit:   minimumToHit(actor, target),
		CritSuccess:    check.ActorCrit,
		CritFailure:    check.OpponentCrit,
	}

	switch {
	case check.RelativeDegree == 0:
		res.Miss = "standoff"
	case !check.ActorWins:
		res.Miss = "outrolled"
	case check.ActorTotal < res.MinimumToHit:
		res.Miss = fmt.Sprintf("too-weak-to-reach(min%+d)", res.MinimumToHit)
	default:
		res.Hit = true
	}

	w.resolveBlow(res, actor, target, weapon, req.Thrown)
	return res, nil
}

// UnopposedAttack resolves an attack nothing is resisting — the target is
// unaware, asleep, bound, cornered or already down — which Fudge treats as an
// Unopposed action against a Difficulty Level, Poor by default (§4.2.2). There is
// no defence roll, so the margin over that difficulty is what stands in for the
// relative degree when the damage is worked out. Ambushes are correspondingly
// brutal, which is the point of them.
func (w *World) UnopposedAttack(req AttackRequest) (*AttackResult, error) {
	actor, target, weapon, err := w.prepareAttack(req)
	if err != nil {
		return nil, err
	}

	check, err := w.UnopposedCheck(req.ActorID, req.ActorTraitType, req.ActorTraitSlug, req.Difficulty, req.ActorModifier, req.AddOne, req.Reroll)
	if err != nil {
		return nil, err
	}

	res := &AttackResult{
		ActorName:      actor.Name,
		TargetName:     target.Name,
		Roll:           check.Roll,
		ActorTotal:     check.Total,
		Difficulty:     req.Difficulty,
		DifficultyName: req.DifficultyName,
		RelativeDegree: check.Total - req.Difficulty,
		MinimumToHit:   minimumToHit(actor, target),
		CritSuccess:    check.CritSuccess,
		CritFailure:    check.CritFailure,
		Rerolled:       check.Rerolled,
		DiscardedRoll:  check.DiscardedRoll,
	}

	switch {
	case !check.Success:
		res.Miss = "failed"
	case check.Total < res.MinimumToHit:
		res.Miss = fmt.Sprintf("too-weak-to-reach(min%+d)", res.MinimumToHit)
	default:
		res.Hit = true
	}

	w.resolveBlow(res, actor, target, weapon, req.Thrown)
	return res, nil
}

// resolveBlow finishes an attack that has already been rolled: it totals the
// damage factors, wounds the target, and spends whatever the swing cost. The
// ammunition and the thrown knife go regardless of whether the blow landed — a
// shot that misses is still a shot fired.
func (w *World) resolveBlow(res *AttackResult, actor, target *Character, weapon *Item, thrown bool) {
	res.ActorScale = actor.Scale
	res.TargetScale = target.Scale

	offense := unarmedDamage
	if weapon != nil {
		wp := weapon.Weapon
		res.WeaponName = weapon.Name
		res.WeaponDamage = wp.Damage
		offense = wp.Damage
		// A muscle-powered weapon carries the arm behind it into the damage
		// factor; a weapon with its own power source does not care who fires it.
		if wp.DamageAttribute != "" {
			bonus, _ := resolveTrait(actor, TraitTypeAttribute, wp.DamageAttribute)
			res.StrengthTrait = wp.DamageAttribute
			res.StrengthBonus = bonus
			offense += bonus
		}
	}
	// Scale enters as the attacker's Strength Scale against the defender's Mass
	// Scale (§4.5.8): the gap between a leprechaun and a giant dwarfs any weapon.
	offense += actor.Scale

	defense := target.Scale
	if armor := target.BestWornArmor(); armor != nil {
		res.ArmorName = armor.Name
		res.ArmorProtection = armor.Armor.Protection
		defense += armor.Armor.Protection
	}

	if res.Hit {
		res.DamageFactor = res.RelativeDegree + offense - defense
		// A blow that only just won is a graze at most, however heavy the weapon
		// behind it (§4.5.6).
		res.Graze = res.RelativeDegree == 1
		if points := woundPoints(res.DamageFactor, res.Graze); points > 0 {
			res.Wound = strings.ToLower(target.Wounds.ApplyDamage(points))
		}
		res.Scratches = target.Wounds.Scratches
		res.Penalty = target.Wounds.Penalty()
	}

	w.spendShot(res, actor, weapon, thrown)
}

// spendShot takes what the swing cost off the sheet: the round out of the
// magazine.
//
// A thrown weapon is reported and not removed. It has certainly left the hand,
// but where it came to rest is a fact only the prose holds — buried in the
// target, skidding under a table, out of a window into the alley — and the
// engine learns that when the DM moves it, one call, in the response that
// narrates the throw. Taking it off the sheet here would mean guessing a
// destination, and an item this engine cannot place is an item it would have to
// destroy.
func (w *World) spendShot(res *AttackResult, actor *Character, weapon *Item, thrown bool) {
	if weapon == nil {
		return
	}
	if weapon.Weapon.TracksAmmo() {
		weapon.Weapon.Ammo--
		res.AmmoTracked = true
		res.AmmoLeft = weapon.Weapon.Ammo
		res.AmmoMax = weapon.Weapon.AmmoCapacity
	}
	if thrown {
		res.Thrown = true
		res.ThrownBy = actor.Name
	}
}

// woundPoints converts a total damage factor into the damage points the wound
// track is scored with (§4.5.5). A graze reads off its own gentler scale
// (§4.5.6): it can never do worse than a light wound no matter how hard the blow
// behind it was. Zero means the blow left no mark at all — armor turned it, or
// the sheer size of the target shrugged it off.
func woundPoints(factor int, graze bool) int {
	if graze {
		switch {
		case factor < 0:
			return 0
		case factor <= 4:
			return 1 // Scratch
		default:
			return 3 // Hurt
		}
	}
	if factor < 1 {
		return 0
	}
	return factor
}

// String renders the attack in the condensed notation the DM reads, one line: the
// roll, the verdict, the damage arithmetic that produced the wound, and what the
// swing cost. The arithmetic is spelled out on purpose — a blow stopped by a
// breastplate and a blow that never arrived are narrated differently.
func (r *AttackResult) String() string {
	outcome := "MISS"
	if r.Hit {
		if r.Graze {
			outcome = "GRAZE"
		} else {
			outcome = "HIT"
		}
	}

	parts := []any{
		"actor", r.ActorName,
		"actor_total", r.ActorTotal,
	}

	if r.Opposed {
		parts = append(parts,
			"defender", r.TargetName,
			"defender_total", r.TargetTotal,
		)
	} else {
		roll := r.Roll
		if r.Rerolled {
			parts = append(parts,
				"roll", roll,
				"rerolled", r.DiscardedRoll,
			)
		} else {
			parts = append(parts, "roll", roll)
		}
		parts = append(parts,
			"vs", r.DifficultyName,
			"target", "unresisting",
		)
	}

	parts = append(parts,
		"degree", fmt.Sprintf("deg%+d", r.RelativeDegree),
		"result", outcome,
	)

	if r.CritSuccess {
		parts = append(parts, "crit", "actor")
	} else if r.CritFailure {
		parts = append(parts, "crit", "defender")
	}

	if !r.Hit {
		parts = append(parts, "reason", r.Miss)
	} else {
		parts = append(parts,
			"damage", fmt.Sprintf("dmg%+d(%s)", r.DamageFactor, r.damageTerms()),
		)
		if r.Wound == "" {
			parts = append(parts, "target_state", "unhurt")
		} else {
			parts = append(parts, "target_state", r.Wound)
			if r.Scratches > 0 {
				parts = append(parts, "scratches", fmt.Sprintf("%d/3", r.Scratches))
			}
			parts = append(parts, "penalty", fmt.Sprintf("pen%+d", r.Penalty))
		}
	}

	if r.AmmoTracked {
		ammo := fmt.Sprintf("%d/%d", r.AmmoLeft, r.AmmoMax)
		if r.AmmoLeft == 0 {
			ammo += " EMPTY"
		}
		parts = append(parts, "ammo", ammo)
	}

	if r.Thrown {
		parts = append(parts,
			"thrown", r.WeaponName,
			"thrown_by", r.ThrownBy,
			"note", "STILL ON SHEET — move_item to where prose says it landed",
		)
	}

	return Flow(parts...)
}

// damageTerms lists the factors that made up the damage, dropping the ones that
// contributed nothing so the line stays readable.
func (r *AttackResult) damageTerms() string {
	terms := []string{fmt.Sprintf("deg%+d", r.RelativeDegree)}
	if r.WeaponName == "" {
		terms = append(terms, fmt.Sprintf("unarmed%+d", unarmedDamage))
	} else {
		terms = append(terms, fmt.Sprintf("%s%+d", r.WeaponName, r.WeaponDamage))
	}
	if r.StrengthTrait != "" {
		terms = append(terms, fmt.Sprintf("%s%+d", r.StrengthTrait, r.StrengthBonus))
	}
	if r.ActorScale != 0 {
		terms = append(terms, fmt.Sprintf("scale%+d", r.ActorScale))
	}
	if r.ArmorName != "" {
		terms = append(terms, fmt.Sprintf("%s%+d", r.ArmorName, -r.ArmorProtection))
	}
	if r.TargetScale != 0 {
		terms = append(terms, fmt.Sprintf("target-scale%+d", -r.TargetScale))
	}
	if r.Graze {
		terms = append(terms, "graze-capped")
	}
	return strings.Join(terms, ",")
}
