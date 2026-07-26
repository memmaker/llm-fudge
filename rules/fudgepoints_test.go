package rules

import (
	"math/rand"
	"strings"
	"testing"
)

func pointSpender(points int) (*World, *Character) {
	w := &World{Characters: map[string]*Character{}, Rng: rand.New(rand.NewSource(4))}
	ch := w.CreateCharacter("kyle", "Kyle", map[string]int{"grit": 1}, map[string]int{"shootin": 1})
	ch.FudgePoints = points
	return w, ch
}

func (w *World) shoot(addOne, reroll bool) (*CheckResult, error) {
	return w.UnopposedCheck("kyle", TraitTypeSkill, "shootin", 0, 0, addOne, reroll)
}

// A Fudge Point spent on a reroll has to buy something. The DM commits it before
// the dice are known, so the engine rolls twice and keeps the better roll — a
// second roll that merely replaced the first would leave the point paying for
// nothing at all.
func TestRerollKeepsTheBetterRoll(t *testing.T) {
	w, ch := pointSpender(500)

	var differed int
	for i := 0; i < 250; i++ {
		result, err := w.shoot(false, true)
		if err != nil {
			t.Fatal(err)
		}
		if !result.Rerolled {
			t.Fatal("a rerolled check must report itself as rerolled")
		}
		if result.Roll < result.DiscardedRoll {
			t.Fatalf("kept roll %+d is worse than the discarded %+d", result.Roll, result.DiscardedRoll)
		}
		if result.Roll != result.DiscardedRoll {
			differed++
		}
		// The reported total belongs to the roll that was kept, trait plus roll,
		// and not to the one thrown away.
		if want := ch.Skills["shootin"] + result.Roll; result.Total != want {
			t.Fatalf("total = %d, want %d from the kept roll %+d", result.Total, want, result.Roll)
		}
	}
	// Guards the assertion above against passing vacuously: over 250 rerolls the
	// two rolls have to disagree sometimes, or nothing was ever compared.
	if differed == 0 {
		t.Error("the two rolls never differed; the comparison is untested")
	}
}

// Each option is a separate point, so the arithmetic has to be exact — a reroll
// that quietly charged twice, or not at all, is the whole bug this pins down.
func TestFudgePointCosts(t *testing.T) {
	for _, c := range []struct {
		what           string
		addOne, reroll bool
		cost           int
	}{
		{"a plain check", false, false, 0},
		{"add_one alone", true, false, 1},
		{"a reroll alone", false, true, 1},
		{"both at once", true, true, 2},
	} {
		w, ch := pointSpender(4)
		if _, err := w.shoot(c.addOne, c.reroll); err != nil {
			t.Fatalf("%s: %v", c.what, err)
		}
		if spent := 4 - ch.FudgePoints; spent != c.cost {
			t.Errorf("%s spent %d Fudge Points, want %d", c.what, spent, c.cost)
		}
	}
}

// A character cannot finish a check in debt. Asking for both options on a single
// point is refused outright rather than half-granted, and a refused check spends
// nothing and rolls nothing.
func TestCheckRefusedWhenPointsCannotCoverIt(t *testing.T) {
	w, ch := pointSpender(1)

	if _, err := w.shoot(true, true); err == nil || !strings.Contains(err.Error(), "Fudge Points") {
		t.Fatalf("err = %v, want a complaint about Fudge Points", err)
	}
	if ch.FudgePoints != 1 {
		t.Errorf("fudge points = %d: a refused check must spend none", ch.FudgePoints)
	}

	// One option on one point is affordable, and leaves them empty.
	if _, err := w.shoot(false, true); err != nil {
		t.Fatal(err)
	}
	if ch.FudgePoints != 0 {
		t.Errorf("fudge points = %d, want 0", ch.FudgePoints)
	}
	if _, err := w.shoot(false, true); err == nil {
		t.Error("a character with no points left cannot reroll")
	}
}

// A check nobody paid for says nothing about rerolls, so the DM has no discarded
// roll to narrate and no reason to think a point went anywhere.
func TestPlainCheckIsNotFlaggedAsRerolled(t *testing.T) {
	w, _ := pointSpender(3)

	result, err := w.shoot(false, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Rerolled || result.DiscardedRoll != 0 {
		t.Errorf("a plain check reported a reroll: %+v", result)
	}
}

// An ambush is exactly where a player wants to spend a point, so the unopposed
// attack passes the reroll through and reports the roll it threw away.
func TestUnopposedAttackCanReroll(t *testing.T) {
	w, ch := pointSpender(2)
	target := w.CreateCharacter("bandit", "Bandit", nil, nil)

	res, err := w.UnopposedAttack(AttackRequest{
		ActorID: "kyle", ActorTraitType: TraitTypeSkill, ActorTraitSlug: "shootin",
		TargetID: "bandit", Difficulty: -2, DifficultyName: "very easy", Reroll: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Rerolled {
		t.Fatal("the attack did not reroll")
	}
	if res.Roll < res.DiscardedRoll {
		t.Errorf("kept roll %+d is worse than the discarded %+d", res.Roll, res.DiscardedRoll)
	}
	if ch.FudgePoints != 1 {
		t.Errorf("fudge points = %d, want 1 after one reroll", ch.FudgePoints)
	}
	if !strings.Contains(res.String(), "rerolled") {
		t.Errorf("the result line does not mention the reroll:\n%s", res.String())
	}
	if target.Name == "" {
		t.Error("the target went missing")
	}
}
