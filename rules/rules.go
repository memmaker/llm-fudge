package rules

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
)

const (
	TraitTypeAttribute = "attribute"
	TraitTypeSkill     = "skill"

	NatCritSuccessRoll = 4
	NatCritFailureRoll = -4
)

type DiceMethod int

const (
	Dice4DF DiceMethod = iota
	Dice1D6
)

func roll4df(rng *rand.Rand) int {
	total := 0
	for i := 0; i < 4; i++ {
		total += roll1df(rng)
	}
	return total
}

func roll1d6f(rng *rand.Rand) int {
	r := rng.Intn(6)
	switch r {
	case 0:
		return -3
	case 1:
		return -2
	case 2:
		return -1
	case 3:
		return 1
	case 4:
		return 2
	default:
		return 3
	}
}

func roll1df(rng *rand.Rand) int {
	r := rng.Intn(3)
	switch {
	case r == 0:
		return -1
	case r == 1:
		return 0
	case r == 2:
		return 1
	}
	return 0
}

func resolveTrait(ch *Character, traitType string, trait string) (int, bool) {
	trait = strings.ToLower(trait)
	switch traitType {
	case "attribute":
		v, ok := ch.Attributes[trait]
		return v, ok
	case "skill":
		v, ok := ch.Skills[trait]
		return v, ok
	default:
		if v, ok := ch.Attributes[trait]; ok {
			return v, true
		}
		if v, ok := ch.Skills[trait]; ok {
			return v, true
		}
		return 0, false
	}
}

// validateCheckInputs checks all inputs before consuming any RNG or resources
func (w *World) validateCheckInputs(actorID string, traitType string, traitSlug string, addOne bool, reroll bool) error {
	if _, err := w.Character(actorID); err != nil {
		return err
	}
	if traitType != TraitTypeAttribute && traitType != TraitTypeSkill {
		return fmt.Errorf("invalid trait type: %s", traitType)
	}
	// Normalize trait slug at input boundary
	traitSlug = strings.ToLower(traitSlug)
	_, ok := resolveTrait(w.Characters[actorID], traitType, traitSlug)
	if !ok {
		return fmt.Errorf("unknown %s: %s", traitType, traitSlug)
	}
	ch := w.Characters[actorID]
	if addOne && ch.FudgePoints < 1 {
		return fmt.Errorf("insufficient Fudge Points for add_one")
	}
	if reroll && ch.FudgePoints < 1 {
		return fmt.Errorf("insufficient Fudge Points for reroll")
	}
	return nil
}

func (w *World) unopposedRoll(ch *Character, traitType string, trait string, mod int, dice DiceMethod) (int, int, int) {
	tv, _ := resolveTrait(ch, traitType, trait)
	var roll int
	switch dice {
	case Dice1D6:
		roll = roll1d6f(w.Rng)
	default:
		roll = roll4df(w.Rng)
	}
	result := tv + roll + mod + ch.Wounds.Penalty()
	return roll, result, tv + mod
}

// CheckResult holds the outcome of a skill check
type CheckResult struct {
	Success     bool
	Total       int
	Roll        int
	CritSuccess bool
	CritFailure bool
}

// unopposedRollEx rolls and returns raw results with critical detection
func (w *World) unopposedRollEx(ch *Character, traitType string, trait string, mod int, addOneBonus int, dice DiceMethod) CheckResult {
	tv, _ := resolveTrait(ch, traitType, trait)
	var roll int
	switch dice {
	case Dice1D6:
		roll = roll1d6f(w.Rng)
	default:
		roll = roll4df(w.Rng)
	}

	// Detect natural criticals before adding modifiers
	critSuccess := roll == NatCritSuccessRoll
	critFailure := roll == NatCritFailureRoll

	// Apply add_one bonus
	roll += addOneBonus

	result := tv + roll + mod + ch.Wounds.Penalty()
	return CheckResult{
		Total:       result,
		Roll:        roll,
		CritSuccess: critSuccess,
		CritFailure: critFailure,
	}
}

// UnopposedCheck resolves an unopposed check with full Fudge Point support
func (w *World) UnopposedCheck(actorID string, traitType string, traitSlug string, difficulty int, situationalModifier int, addOne bool, reroll bool) (*CheckResult, error) {
	// Validate all inputs before consuming resources
	if err := w.validateCheckInputs(actorID, traitType, traitSlug, addOne, reroll); err != nil {
		return nil, err
	}

	ch := w.Characters[actorID]
	if !ch.Wounds.CanAct() {
		return nil, fmt.Errorf("%s is incapacitated and cannot act", ch.Name)
	}

	// All validation passed; now consume resources
	addOneBonus := 0
	if addOne {
		ch.FudgePoints--
		addOneBonus = 1
	}

	if reroll {
		ch.FudgePoints--
	}

	// Execute the roll
	result := w.unopposedRollEx(ch, traitType, traitSlug, situationalModifier, addOneBonus, Dice4DF)
	result.Success = result.Total >= difficulty

	return &result, nil
}

func (w *World) opposedRoll(a *Character, aTraitType string, aTrait string, aMod int, b *Character, bTraitType string, bTrait string, bMod int, dice DiceMethod) (int, int, int, int, int, int) {
	aRoll, aResult, aBase := w.unopposedRoll(a, aTraitType, aTrait, aMod, dice)
	bRoll, bResult, bBase := w.unopposedRoll(b, bTraitType, bTrait, bMod, dice)
	return aRoll, aResult, aBase, bRoll, bResult, bBase
}

// UnopposedSkillCheck resolves a skill against a Fudge difficulty level.
func (w *World) UnopposedSkillCheck(actorID, skill string, difficulty int) (bool, int, int, error) {
	actor, err := w.Character(actorID)
	if err != nil {
		return false, 0, 0, err
	}
	if !actor.Wounds.CanAct() {
		return false, 0, 0, fmt.Errorf("%s is incapacitated and cannot act", actor.Name)
	}
	roll, total, _ := w.unopposedRoll(actor, "skill", skill, 0, Dice4DF)
	return total >= difficulty, total, roll, nil
}

// OpposedSkillCheck resolves two skill checks and returns the winner and relative degree.
func (w *World) OpposedSkillCheck(actorID, actorSkill, opponentID, opponentSkill string) (bool, int, int, int, error) {
	actor, err := w.Character(actorID)
	if err != nil {
		return false, 0, 0, 0, err
	}
	opponent, err := w.Character(opponentID)
	if err != nil {
		return false, 0, 0, 0, err
	}
	if !actor.Wounds.CanAct() || !opponent.Wounds.CanAct() {
		return false, 0, 0, 0, fmt.Errorf("an incapacitated character cannot make an opposed check")
	}
	_, actorTotal, _, _, opponentTotal, _ := w.opposedRoll(actor, "skill", actorSkill, 0, opponent, "skill", opponentSkill, 0, Dice4DF)
	return actorTotal > opponentTotal, actorTotal, opponentTotal, actorTotal - opponentTotal, nil
}

// OpposedResult holds the outcome of an opposed check
type OpposedResult struct {
	ActorWins      bool
	ActorTotal     int
	OpponentTotal  int
	RelativeDegree int
	ActorCrit      bool
	OpponentCrit   bool
}

// OpposedCheck resolves an opposed check with full trait and Fudge Point support
// Only the actor can spend Fudge Points. Ties are not wins; actor must strictly exceed opponent.
func (w *World) OpposedCheck(actorID string, actorTraitType string, actorTraitSlug string, actorMod int,
	opponentID string, opponentTraitType string, opponentTraitSlug string, opponentMod int, addOne bool) (*OpposedResult, error) {

	// Validate all inputs before consuming resources
	if err := w.validateCheckInputs(actorID, actorTraitType, actorTraitSlug, addOne, false); err != nil {
		return nil, err
	}

	opponentTraitSlug = strings.ToLower(opponentTraitSlug)
	if _, err := w.Character(opponentID); err != nil {
		return nil, err
	}
	if opponentTraitType != TraitTypeAttribute && opponentTraitType != TraitTypeSkill {
		return nil, fmt.Errorf("invalid opponent trait type: %s", opponentTraitType)
	}
	_, ok := resolveTrait(w.Characters[opponentID], opponentTraitType, opponentTraitSlug)
	if !ok {
		return nil, fmt.Errorf("unknown opponent %s: %s", opponentTraitType, opponentTraitSlug)
	}

	actor := w.Characters[actorID]
	opponent := w.Characters[opponentID]

	if !actor.Wounds.CanAct() || !opponent.Wounds.CanAct() {
		return nil, fmt.Errorf("an incapacitated character cannot make an opposed check")
	}

	// All validation passed; now consume resources
	addOneBonus := 0
	if addOne {
		actor.FudgePoints--
		addOneBonus = 1
	}

	// Execute rolls
	actorResult := w.unopposedRollEx(actor, actorTraitType, actorTraitSlug, actorMod, addOneBonus, Dice4DF)
	opponentResult := w.unopposedRollEx(opponent, opponentTraitType, opponentTraitSlug, opponentMod, 0, Dice4DF)

	// Actor must strictly exceed opponent; ties are not wins
	actorWins := actorResult.Total > opponentResult.Total

	return &OpposedResult{
		ActorWins:      actorWins,
		ActorTotal:     actorResult.Total,
		OpponentTotal:  opponentResult.Total,
		RelativeDegree: actorResult.Total - opponentResult.Total,
		ActorCrit:      actorResult.CritSuccess,
		OpponentCrit:   opponentResult.CritFailure,
	}, nil
}

func characterMarkdown(ch *Character) string {
	var b strings.Builder
	fmt.Fprintf(&b, "### %s\n\n", ch.Name)
	fmt.Fprintf(&b, "**Scale:** %d\n\n", ch.Scale)
	fmt.Fprintf(&b, "**Location:** %s\n\n", ch.Location)
	if ch.Description != "" {
		fmt.Fprintf(&b, "*%s*\n\n", ch.Description)
	}
	fmt.Fprintf(&b, "#### Attributes\n\n")
	for _, k := range sortedKeys(ch.Attributes) {
		fmt.Fprintf(&b, "- %s: **%s** (%+d)\n", strings.Title(k), levelString(ch.Attributes[k]), ch.Attributes[k])
	}
	fmt.Fprintf(&b, "\n#### Skills\n\n")
	for _, k := range sortedKeys(ch.Skills) {
		fmt.Fprintf(&b, "- %s: **%s** (%+d)\n", strings.Title(k), levelString(ch.Skills[k]), ch.Skills[k])
	}
	fmt.Fprintf(&b, "\n#### Wounds\n\n")
	fmt.Fprintf(&b, "- Scratches: %d/3\n", ch.Wounds.Scratches)
	fmt.Fprintf(&b, "- Level: %s\n", ch.Wounds.Level)
	fmt.Fprintf(&b, "\n**Wound Penalty:** %+d\n\n", ch.Wounds.Penalty())
	if len(ch.Gifts) > 0 {
		fmt.Fprintf(&b, "#### Gifts\n\n")
		for _, g := range ch.Gifts {
			fmt.Fprintf(&b, "- %s\n", g)
		}
		fmt.Fprintln(&b)
	}
	if len(ch.Faults) > 0 {
		fmt.Fprintf(&b, "#### Faults\n\n")
		for _, f := range ch.Faults {
			fmt.Fprintf(&b, "- %s\n", f)
		}
		fmt.Fprintln(&b)
	}
	fmt.Fprintf(&b, "\n**Fudge Points:** %d\n", ch.FudgePoints)
	fmt.Fprintf(&b, "**Coins:** %d\n\n", ch.Coins)
	if len(ch.Inventory) > 0 {
		fmt.Fprintf(&b, "#### Inventory\n\n")
		for _, it := range sortedKeys(ch.Inventory) {
			n := ch.Inventory[it]
			if n > 1 {
				fmt.Fprintf(&b, "- %s x%d\n", it, n)
			} else {
				fmt.Fprintf(&b, "- %s\n", it)
			}
		}
		fmt.Fprintln(&b)
	}
	return b.String()
}

const (
	Creset  = "\033[0m"
	Cbold   = "\033[1m"
	Cred    = "\033[31m"
	Cgreen  = "\033[32m"
	Cyellow = "\033[33m"
	Cblue   = "\033[34m"
	Ccyan   = "\033[36m"
)

func woundColor(ch *Character) string {
	switch ch.Wounds.Level {
	case Dead, NearDeath, Incapacitated:
		return Cred
	case VeryHurt:
		return Cred
	case Hurt:
		return Cyellow
	}
	return Cgreen
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func fmtTrait(name string, val int) string {
	c := Cgreen
	if val < 0 {
		c = Cred
	}
	return fmt.Sprintf("  %s%s%s %s(%+d)%s", c, name, Creset, levelString(val), val, Creset)
}

func fmtWound(w WoundState) string {
	var parts []string
	boxes := []struct {
		filled bool
		label  string
		color  string
	}{
		{w.Scratches > 0, "Scratch", Cyellow},
		{w.Level == Hurt, "Hurt", Cred},
		{w.Level == VeryHurt, "Very Hurt", Cred},
		{w.Level == Incapacitated, "Incap.", Cred},
		{w.Level == NearDeath, "Near Death", Cred},
		{w.Level == Dead, "Dead", Cred},
	}
	for _, b := range boxes {
		if b.filled {
			parts = append(parts, fmt.Sprintf("%s\u25c9%s %s", b.color, Creset, b.label))
		} else {
			parts = append(parts, fmt.Sprintf("\u25cb %s", b.label))
		}
	}
	if w.Scratches > 0 && w.Scratches < 3 {
		parts = append(parts, fmt.Sprintf("%sx%d%s", Cyellow, w.Scratches, Creset))
	}
	return strings.Join(parts, " | ")
}

func StateFileText(w *World) string {
	if w.Player == "" {
		return "No player character set."
	}
	ch := w.Characters[w.Player]
	if ch == nil {
		return "Player character not found."
	}
	var b strings.Builder

	// Header bar
	fmt.Fprintf(&b, "%s%s\u2500\u2500\u2500 CHARACTER SHEET: %s %s\u2500\u2500\u2500%s\n",
		Cbold, Ccyan, ch.Name, Ccyan, Creset)
	fmt.Fprintf(&b, "%s\u25c6 Location:%s %s\n\n", Ccyan, Creset, ch.Location)

	// Attributes
	fmt.Fprintf(&b, "%s\u25b8 Attributes%s\n", Cbold, Creset)
	attrs := sortedKeys(ch.Attributes)
	for _, k := range attrs {
		fmt.Fprintf(&b, "  %s\n", fmtTrait(strings.Title(k), ch.Attributes[k]))
	}

	// Skills
	fmt.Fprintf(&b, "\n%s\u25b8 Skills%s\n", Cbold, Creset)
	skills := sortedKeys(ch.Skills)
	for _, k := range skills {
		fmt.Fprintf(&b, "  %s\n", fmtTrait(strings.Title(k), ch.Skills[k]))
	}

	// Wounds
	fmt.Fprintf(&b, "\n%s\u25b8 Wounds%s\n", Cbold, Creset)
	fmt.Fprintf(&b, "  %s\n", fmtWound(ch.Wounds))
	p := ch.Wounds.Penalty()
	if p < 0 {
		fmt.Fprintf(&b, "  %sPenalty: %d%s\n", Cred, p, Creset)
	}
	fmt.Fprintln(&b)

	// Gifts & Faults
	if len(ch.Gifts) > 0 {
		fmt.Fprintf(&b, "%s\u25b8 Gifts%s\n", Cbold, Creset)
		for _, g := range ch.Gifts {
			fmt.Fprintf(&b, "  %s\u2713%s %s\n", Cgreen, Creset, g)
		}
		fmt.Fprintln(&b)
	}
	if len(ch.Faults) > 0 {
		fmt.Fprintf(&b, "%s\u25b8 Faults%s\n", Cbold, Creset)
		for _, f := range ch.Faults {
			fmt.Fprintf(&b, "  %s\u2717%s %s\n", Cred, Creset, f)
		}
		fmt.Fprintln(&b)
	}

	// Fudge Points
	fpColor := Cgreen
	if ch.FudgePoints <= 0 {
		fpColor = Cred
	}
	fmt.Fprintf(&b, "%s\u25b8 Fudge Points:%s %s%d%s\n", Cbold, Creset, fpColor, ch.FudgePoints, Creset)
	fmt.Fprintf(&b, "%s\u25b8 Coins:%s %d\n\n", Cbold, Creset, ch.Coins)

	// Inventory
	if len(ch.Inventory) > 0 {
		fmt.Fprintf(&b, "\n%s\u25b8 Inventory%s\n", Cbold, Creset)
		for _, it := range sortedKeys(ch.Inventory) {
			n := ch.Inventory[it]
			if n > 1 {
				fmt.Fprintf(&b, "  \u2022 %s x%d\n", it, n)
			} else {
				fmt.Fprintf(&b, "  \u2022 %s\n", it)
			}
		}
	}

	// Present NPCs
	var present []string
	for k, c := range w.Characters {
		if c.Location == ch.Location && k != w.Player {
			present = append(present, c.Name)
		}
	}
	sort.Strings(present)
	if len(present) > 0 {
		fmt.Fprintf(&b, "\n%s\u25c6 Present:%s %s\n", Ccyan, Creset, strings.Join(present, ", "))
	}

	// Footer
	fmt.Fprintf(&b, "%s%s%s%s\n", Ccyan, Cbold, strings.Repeat("\u2500", 42), Creset)
	return b.String()
}
