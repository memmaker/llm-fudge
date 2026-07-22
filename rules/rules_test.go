package rules

import "testing"

func TestSkillChecksAreDeterministic(t *testing.T) {
	first := NewWorld(42)
	first.CreateCharacter("hero", "Hero", nil, map[string]int{"athletics": 2})
	first.CreateCharacter("goblin", "Goblin", nil, map[string]int{"athletics": 1})

	second := NewWorld(42)
	second.CreateCharacter("hero", "Hero", nil, map[string]int{"athletics": 2})
	second.CreateCharacter("goblin", "Goblin", nil, map[string]int{"athletics": 1})

	firstWin, firstTotal, firstRoll, err := first.UnopposedSkillCheck("hero", "Athletics", 1)
	if err != nil {
		t.Fatalf("first unopposed check: %v", err)
	}
	secondWin, secondTotal, secondRoll, err := second.UnopposedSkillCheck("hero", "Athletics", 1)
	if err != nil {
		t.Fatalf("second unopposed check: %v", err)
	}
	if firstWin != secondWin || firstTotal != secondTotal || firstRoll != secondRoll {
		t.Fatalf("same seed produced different checks: (%t, %d, %d) != (%t, %d, %d)", firstWin, firstTotal, firstRoll, secondWin, secondTotal, secondRoll)
	}

	_, firstActor, firstOpponent, firstMargin, err := first.OpposedSkillCheck("hero", "Athletics", "goblin", "Athletics")
	if err != nil {
		t.Fatalf("first opposed check: %v", err)
	}
	_, secondActor, secondOpponent, secondMargin, err := second.OpposedSkillCheck("hero", "Athletics", "goblin", "Athletics")
	if err != nil {
		t.Fatalf("second opposed check: %v", err)
	}
	if firstActor != secondActor || firstOpponent != secondOpponent || firstMargin != secondMargin {
		t.Fatalf("same seed produced different opposed checks")
	}
}

func TestUnopposedSkillCheckRejectsUnknownCharacter(t *testing.T) {
	_, _, _, err := NewWorld(1).UnopposedSkillCheck("missing", "Athletics", 0)
	if err == nil {
		t.Fatal("expected missing character error")
	}
}
