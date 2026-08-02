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

// validateCheckInputs checks all inputs before consuming any RNG or resources,
// and hands back the actor it resolved: the id a caller passed may be a name or
// a shorthand, so the character found here is the one the roll must use.
func (w *World) validateCheckInputs(actorID string, traitType string, traitSlug string, addOne bool, reroll bool) (*Character, error) {
	ch, err := w.Character(actorID)
	if err != nil {
		return nil, err
	}
	if traitType != TraitTypeAttribute && traitType != TraitTypeSkill {
		return nil, fmt.Errorf("invalid trait type: %s", traitType)
	}
	// Normalize trait slug at input boundary
	traitSlug = strings.ToLower(traitSlug)
	_, ok := resolveTrait(ch, traitType, traitSlug)
	if !ok {
		return nil, fmt.Errorf("unknown %s: %s", traitType, traitSlug)
	}
	// Each option spends its own point, so asking for both costs two. Checked as
	// a total rather than one at a time: a character with a single point left
	// could otherwise buy both and finish the check in debt.
	cost := 0
	if addOne {
		cost++
	}
	if reroll {
		cost++
	}
	if ch.FudgePoints < cost {
		return nil, fmt.Errorf("%s has %d Fudge Points, and this check would spend %d", ch.Name, ch.FudgePoints, cost)
	}
	return ch, nil
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

	// Rerolled records that a Fudge Point bought a second roll, and
	// DiscardedRoll is the one that lost out. The discarded roll is worth
	// reporting: "the shot that would have gone wide" is a better sentence than
	// a number that appeared from nowhere.
	Rerolled      bool
	DiscardedRoll int
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
	ch, err := w.validateCheckInputs(actorID, traitType, traitSlug, addOne, reroll)
	if err != nil {
		return nil, err
	}

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

	// A rerolled check is rolled twice and the better of the two kept. The point
	// is committed before any dice are known — reroll is an argument on the call,
	// not a reaction to the result — and a second roll that simply replaced the
	// first would buy nothing whatever in expectation. Taking the better is what
	// makes a Fudge Point spent blind worth spending, and it is the only reading
	// of "reroll a bad result" (§1.3.6) that survives being declared up front.
	if reroll {
		second := w.unopposedRollEx(ch, traitType, traitSlug, situationalModifier, addOneBonus, Dice4DF)
		discarded := second
		if second.Roll > result.Roll {
			result, discarded = second, result
		}
		result.Rerolled = true
		result.DiscardedRoll = discarded.Roll
	}

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
	actor, err := w.validateCheckInputs(actorID, actorTraitType, actorTraitSlug, addOne, false)
	if err != nil {
		return nil, err
	}

	opponentTraitSlug = strings.ToLower(opponentTraitSlug)
	opponent, err := w.Character(opponentID)
	if err != nil {
		return nil, err
	}
	if opponentTraitType != TraitTypeAttribute && opponentTraitType != TraitTypeSkill {
		return nil, fmt.Errorf("invalid opponent trait type: %s", opponentTraitType)
	}
	_, ok := resolveTrait(opponent, opponentTraitType, opponentTraitSlug)
	if !ok {
		return nil, fmt.Errorf("unknown opponent %s: %s", opponentTraitType, opponentTraitSlug)
	}

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
		for _, it := range sortedInventory(ch.Inventory) {
			fmt.Fprintf(&b, "- %s", itemLabel(it))
			if ratings := itemRatings(it); ratings != "" {
				fmt.Fprintf(&b, " (%s)", ratings)
			}
			if it.Description != "" {
				fmt.Fprintf(&b, " — %s", it.Description)
			}
			fmt.Fprintln(&b)
		}
		fmt.Fprintln(&b)
	}
	return b.String()
}

// itemLabel renders an item as name plus count, dropping the count for a lone
// item so a sheet does not read "Blaster Pistol x1".
func itemLabel(it *Item) string {
	if it.Count > 1 {
		return fmt.Sprintf("%s x%d", it.Name, it.Count)
	}
	return it.Name
}

const (
	Creset  = "\033[0m"
	Cbold   = "\033[1m"
	Cdim    = "\033[2m"
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

// ToString renders a character as YAML: the id as a key, then the engine-owned
// fields. It is the engine's answer to "what is true about this character right
// now", and it is what the DM is handed instead of the character sheet — which
// it cannot read — and instead of its own notes, which drift as a session runs
// long.
//
// It is byte-for-byte the shape the world state block renders, because it is the
// same function: WriteYAML does the work and both call it. That matters. A tool
// result and the block are the same character described twice, and when they were
// two renderers in two notations, the DM had to learn both and trust they agreed.
//
// Position is among the fields, and used not to be. Location was once written
// at creation and never moved — there was no call that did — so the engine
// would have been asserting a position that stopped being true the first time
// the fiction walked anybody anywhere, and where a character stood was left to
// the DM's own narrative properties instead. It is stated here now because the
// engine finally tracks it: every narration names the location it happened in, and
// move_character moves anyone the scene leaves behind. A fact the engine keeps
// current is a fact the engine owns.
func (c *Character) ToString() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s:\n", c.ID)
	c.WriteYAML(&b, "  ")
	return strings.TrimRight(b.String(), "\n")
}

// WriteYAML writes a character's engine-owned fields as YAML at the given
// indent, without the id key above them. Anything empty is left out — a field
// the engine has nothing to say about is better absent than stated as blank —
// except what a character carries, where silence would read as "the engine did
// not say" rather than "they have none".
func (c *Character) WriteYAML(b *strings.Builder, indent string) {
	Field(b, indent, "type", "character")
	Field(b, indent, "name", c.Name)
	if c.Location != "" {
		Field(b, indent, "location", c.Location)
	}
	if c.Species != "" {
		Field(b, indent, "species", c.Species)
	}
	if c.Scale != 0 {
		fmt.Fprintf(b, "%sscale: %+d\n", indent, c.Scale)
	}
	if c.Description != "" {
		Field(b, indent, "desc", c.Description)
	}

	Field(b, indent, "wounds", strings.ToLower(c.Wounds.Level.String()))
	if c.Wounds.Scratches > 0 {
		fmt.Fprintf(b, "%sscratches: %d/3\n", indent, c.Wounds.Scratches)
	}
	if p := c.Wounds.Penalty(); p != 0 {
		fmt.Fprintf(b, "%spenalty: %+d  # already applied to rolls\n", indent, p)
	}
	if !c.Wounds.CanAct() {
		Field(b, indent, "cannot_act", true)
	}
	Field(b, indent, "fudge_points", c.FudgePoints)
	Field(b, indent, "coins", c.Coins)

	writeTraitsYAML(b, indent, "attributes", c.Attributes)
	writeTraitsYAML(b, indent, "skills", c.Skills)
	if len(c.Gifts) > 0 {
		fmt.Fprintf(b, "%sgifts: [%s]\n", indent, strings.Join(c.Gifts, ", "))
	}
	if len(c.Faults) > 0 {
		fmt.Fprintf(b, "%sfaults: [%s]\n", indent, strings.Join(c.Faults, ", "))
	}

	// The ids of what they carry. Each item is also an entity of its own in the
	// world state, naming this character as its location — one storage, two
	// views — but "can they do this with what they have" is the question the DM
	// asks most, and it should not require reading the whole world to answer.
	if len(c.Inventory) == 0 {
		fmt.Fprintf(b, "%sinventory: []  # carrying nothing\n", indent)
		return
	}
	ids := make([]string, 0, len(c.Inventory))
	for _, it := range sortedInventory(c.Inventory) {
		ids = append(ids, it.ID)
	}
	fmt.Fprintf(b, "%sinventory: [%s]\n", indent, strings.Join(ids, ", "))
}

// writeTraitsYAML renders a trait group in flow style. A line per trait would be
// twenty lines per character in a block that is re-sent every turn, and the DM
// reads a trait group across rather than down.
func writeTraitsYAML(b *strings.Builder, indent, label string, traits map[string]int) {
	if len(traits) == 0 {
		return
	}
	parts := make([]string, 0, len(traits))
	for _, name := range sortedKeys(traits) {
		parts = append(parts, fmt.Sprintf("%s: %+d", name, traits[name]))
	}
	fmt.Fprintf(b, "%s%s: {%s}\n", indent, label, strings.Join(parts, ", "))
}

// sortedInventory orders an inventory by item name so the sheet reads the same
// way every time it is rewritten. It is sortedItems (holder.go) under its old
// name at the two call sites — the character sheet and BestWornArmor — that
// predate spot mattering on a character and have no reason to know it now.
func sortedInventory(inv map[string]*Item) []*Item {
	return sortedItems(inv)
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
	fmt.Fprintf(&b, "%s\u25c6 Location:%s %s\n\n", Ccyan, Creset, w.LocationName(ch.Location))

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
		fmt.Fprintf(&b, "%s\u25b8 Inventory%s\n", Cbold, Creset)
		for _, it := range sortedInventory(ch.Inventory) {
			fmt.Fprintf(&b, "  \u2022 %s", itemLabel(it))
			if ratings := itemRatings(it); ratings != "" {
				fmt.Fprintf(&b, " %s(%s)%s", Ccyan, ratings, Creset)
			}
			fmt.Fprintln(&b)
			if it.Description != "" {
				fmt.Fprintf(&b, "    %s%s%s\n", Cdim, it.Description, Creset)
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
