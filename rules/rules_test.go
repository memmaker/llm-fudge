package rules

import (
	"path/filepath"
	"testing"
)

func newTestWorld(t *testing.T, seed int64) *World {
	t.Helper()
	w, err := NewWorld(seed, filepath.Join("..", "rulesets", "example"))
	if err != nil {
		t.Fatalf("NewWorld returned error: %v", err)
	}
	return w
}

func TestSkillChecksAreDeterministic(t *testing.T) {
	first := newTestWorld(t, 42)
	first.CreateCharacter(
		"hero",
		"Hero",
		map[string]int{"strength": 1, "agility": 1, "perception": 0},
		map[string]int{"athletics": 2, "stealth": 1, "fight": 1},
	)
	first.CreateCharacter(
		"goblin",
		"Goblin",
		map[string]int{"strength": 0, "agility": 1, "perception": 0},
		map[string]int{"athletics": 1, "stealth": 0, "fight": 1},
	)

	second := newTestWorld(t, 42)
	second.CreateCharacter(
		"hero",
		"Hero",
		map[string]int{"strength": 1, "agility": 1, "perception": 0},
		map[string]int{"athletics": 2, "stealth": 1, "fight": 1},
	)
	second.CreateCharacter(
		"goblin",
		"Goblin",
		map[string]int{"strength": 0, "agility": 1, "perception": 0},
		map[string]int{"athletics": 1, "stealth": 0, "fight": 1},
	)

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
	world := newTestWorld(t, 1)
	_, _, _, err := world.UnopposedSkillCheck("missing", "Athletics", 0)
	if err == nil {
		t.Fatal("expected missing character error")
	}
}
