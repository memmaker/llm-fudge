package rules

import (
	"path/filepath"
	"strings"
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

func TestCreateCharacterRejectsDuplicateSlug(t *testing.T) {
	world := newTestWorld(t, 1)

	req1 := CharacterRequest{
		Name:    "Hero",
		Species: "human",
	}
	ch1, err := world.CreateCharacterValidated(req1)
	if err != nil {
		t.Fatalf("first character creation failed: %v", err)
	}
	if ch1.ID != "hero" {
		t.Fatalf("expected slug 'hero', got %q", ch1.ID)
	}

	if !world.Ruleset.Frozen {
		t.Fatal("ruleset should be frozen after first character")
	}

	req2 := CharacterRequest{
		Slug:    "hero",
		Name:    "AnotherHero",
		Species: "human",
	}
	_, err = world.CreateCharacterValidated(req2)
	if err == nil {
		t.Fatal("expected error when creating character with duplicate slug")
	}
	if !strings.Contains(err.Error(), "frozen") && !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateCharacterValidatesGiftsAndFaults(t *testing.T) {
	world := newTestWorld(t, 2)

	req := CharacterRequest{
		Name:    "Hero",
		Species: "human",
		Gifts:   []string{"night-eyes"},
		Faults:  []string{"hot-headed"},
	}
	ch, err := world.CreateCharacterValidated(req)
	if err != nil {
		t.Fatalf("character creation failed: %v", err)
	}
	if len(ch.Gifts) != 1 || ch.Gifts[0] != "night-eyes" {
		t.Fatalf("gifts not set correctly: %v", ch.Gifts)
	}
	if len(ch.Faults) != 1 || ch.Faults[0] != "hot-headed" {
		t.Fatalf("faults not set correctly: %v", ch.Faults)
	}
}

func TestCreateCharacterRejectsInvalidGiftOrFault(t *testing.T) {
	world := newTestWorld(t, 3)

	req := CharacterRequest{
		Name:    "Hero",
		Species: "human",
		Gifts:   []string{"invalid-gift"},
	}
	_, err := world.CreateCharacterValidated(req)
	if err == nil {
		t.Fatal("expected error for invalid gift")
	}
	if !strings.Contains(err.Error(), "gift") {
		t.Fatalf("unexpected error: %v", err)
	}
}
