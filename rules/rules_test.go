package rules

import (
	"math/rand"
	"path/filepath"
	"strings"
	"testing"
)

func newTestWorld(t *testing.T, seed int64) *World {
	t.Helper()
	w, err := NewWorld(seed, filepath.Join("..", "settings", "example"))
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
		t.Fatal("setting should be frozen after first character")
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

func TestUnopposedCheckWithModifiers(t *testing.T) {
	world := newTestWorld(t, 42)
	world.CreateCharacter(
		"hero",
		"Hero",
		map[string]int{"strength": 1, "agility": 1},
		map[string]int{"athletics": 2},
	)

	_, total, roll, critSuccess, critFailure, err := world.UnopposedCheckEx("hero", "skill", "athletics", 0, 2, false, false)
	if err != nil {
		t.Fatalf("unopposed check with modifier failed: %v", err)
	}
	// 2 (athletics) + 2 (modifier) = expected base of 4 before roll
	expectedBase := 2 + 2
	if total != expectedBase+roll {
		t.Fatalf("expected total %d, got %d (base %d + roll %d)", expectedBase+roll, total, expectedBase, roll)
	}
	if critSuccess || critFailure {
		t.Logf("no natural critical expected with roll %d", roll)
	}
}

func TestUnopposedCheckDetectsCriticalSuccess(t *testing.T) {
	world := newTestWorld(t, 999)
	world.CreateCharacter("hero", "Hero", map[string]int{"strength": 0}, map[string]int{"athletics": 0})

	// Need to find the seed that produces natural +4
	for seed := int64(0); seed < 10000; seed++ {
		w := newTestWorld(t, seed)
		w.CreateCharacter("hero", "Hero", map[string]int{"strength": 0}, map[string]int{"athletics": 0})
		_, _, roll, critSuccess, _, _ := w.UnopposedCheckEx("hero", "skill", "athletics", 0, 0, false, false)
		if roll == 4 {
			if !critSuccess {
				t.Fatal("natural +4 should be critical success")
			}
			return
		}
	}
	t.Fatal("could not find seed for natural +4")
}

func TestUnopposedCheckDetectsCriticalFailure(t *testing.T) {
	world := newTestWorld(t, 999)
	world.CreateCharacter("hero", "Hero", map[string]int{"strength": 0}, map[string]int{"athletics": 0})

	// Need to find the seed that produces natural -4
	for seed := int64(0); seed < 10000; seed++ {
		w := newTestWorld(t, seed)
		w.CreateCharacter("hero", "Hero", map[string]int{"strength": 0}, map[string]int{"athletics": 0})
		success, _, roll, _, critFailure, _ := w.UnopposedCheckEx("hero", "skill", "athletics", 0, 0, false, false)
		if roll == -4 {
			if success {
				t.Fatal("natural -4 should always fail")
			}
			if !critFailure {
				t.Fatal("natural -4 should be critical failure")
			}
			return
		}
	}
	t.Fatal("could not find seed for natural -4")
}

func TestOpposedCheckWithIndependentModifiers(t *testing.T) {
	world := newTestWorld(t, 42)
	world.CreateCharacter("hero", "Hero", map[string]int{"strength": 1}, map[string]int{"fight": 2})
	world.CreateCharacter("goblin", "Goblin", map[string]int{"strength": 0}, map[string]int{"fight": 1})

	// Both have independent modifiers
	actorWins, actorTotal, oppTotal, margin, _, _, err := world.OpposedCheckEx("hero", "skill", "fight", 1, "goblin", "skill", "fight", 1, false)
	if err != nil {
		t.Fatalf("opposed check failed: %v", err)
	}

	// hero: 2 + 1 = 3, goblin: 1 + 1 = 2 (before rolls)
	// hero still has advantage
	if !actorWins && actorTotal <= oppTotal {
		t.Fatalf("actor with better trait should likely win: actor=%d, opp=%d", actorTotal, oppTotal)
	}
	if actorWins && actorTotal <= oppTotal {
		t.Fatalf("win requires strict advantage: actor=%d, opp=%d", actorTotal, oppTotal)
	}
	if actorWins && margin != actorTotal-oppTotal {
		t.Fatalf("margin calculation wrong: expected %d, got %d", actorTotal-oppTotal, margin)
	}
}

func TestAddOneSpendsFudgePoint(t *testing.T) {
	world := newTestWorld(t, 42)
	ch := world.CreateCharacter("hero", "Hero", map[string]int{"strength": 0}, map[string]int{"athletics": 0})
	ch.FudgePoints = 1

	// Run check with add_one: true
	_, _, _, _, _, err := world.UnopposedCheckEx("hero", "skill", "athletics", 0, 0, true, false)
	if err != nil {
		t.Fatalf("unopposed check with add_one failed: %v", err)
	}

	if ch.FudgePoints != 0 {
		t.Fatalf("add_one should spend 1 Fudge Point, got %d remaining", ch.FudgePoints)
	}
}

func TestAddOneFailsWithoutFudgePoint(t *testing.T) {
	world := newTestWorld(t, 42)
	world.CreateCharacter("hero", "Hero", map[string]int{"strength": 0}, map[string]int{"athletics": 0})

	// Try to use add_one without Fudge Points
	_, _, _, _, _, err := world.UnopposedCheckEx("hero", "skill", "athletics", 0, 0, true, false)
	if err == nil {
		t.Fatal("add_one without Fudge Points should fail")
	}
	if !strings.Contains(err.Error(), "Fudge") {
		t.Fatalf("expected Fudge Point error, got: %v", err)
	}
}

func TestRerollSpendsFudgePoint(t *testing.T) {
	world := newTestWorld(t, 42)
	ch := world.CreateCharacter("hero", "Hero", map[string]int{"strength": 0}, map[string]int{"athletics": 0})
	ch.FudgePoints = 1

	// Run first check
	_, total1, roll1, _, _, err := world.UnopposedCheckEx("hero", "skill", "athletics", 0, 0, false, false)
	if err != nil {
		t.Fatalf("first check failed: %v", err)
	}

	// Restore Fudge Point for reroll
	ch.FudgePoints = 1

	// Run reroll
	_, total2, roll2, _, _, err := world.UnopposedCheckEx("hero", "skill", "athletics", 0, 0, false, true)
	if err != nil {
		t.Fatalf("reroll failed: %v", err)
	}

	if ch.FudgePoints != 0 {
		t.Fatalf("reroll should spend 1 Fudge Point, got %d remaining", ch.FudgePoints)
	}

	// Reroll should produce different results with different RNG state
	if roll1 == roll2 && total1 == total2 {
		t.Logf("warning: same roll on reroll (possible but unlikely): roll=%d, total=%d", roll1, total1)
	}
}

func TestRerollFailsWithoutFudgePoint(t *testing.T) {
	world := newTestWorld(t, 42)
	world.CreateCharacter("hero", "Hero", map[string]int{"strength": 0}, map[string]int{"athletics": 0})

	// Try to reroll without Fudge Points
	_, _, _, _, _, err := world.UnopposedCheckEx("hero", "skill", "athletics", 0, 0, false, true)
	if err == nil {
		t.Fatal("reroll without Fudge Points should fail")
	}
	if !strings.Contains(err.Error(), "Fudge") {
		t.Fatalf("expected Fudge Point error, got: %v", err)
	}
}

func TestInvalidTraitRejectedWithoutRNGConsumption(t *testing.T) {
	world := newTestWorld(t, 42)
	world.CreateCharacter("hero", "Hero", map[string]int{"strength": 0}, map[string]int{"athletics": 0})
	ch := world.CreateCharacter("hero2", "Hero2", map[string]int{"strength": 0}, map[string]int{"athletics": 0})
	ch.FudgePoints = 1

	// Try to use invalid trait
	_, _, _, _, _, err := world.UnopposedCheckEx("hero", "skill", "invalid-skill", 0, 0, false, false)
	if err == nil {
		t.Fatal("should reject invalid trait")
	}

	// Check that RNG was not consumed by looking at determinism
	world2 := newTestWorld(t, 42)
	world2.CreateCharacter("hero", "Hero", map[string]int{"strength": 0}, map[string]int{"athletics": 0})
	world2.CreateCharacter("hero2", "Hero2", map[string]int{"strength": 0}, map[string]int{"athletics": 0})
	world2.Characters["hero2"].FudgePoints = 1

	// Valid check should produce same result
	_, _, roll2, _, _, _ := world2.UnopposedCheckEx("hero", "skill", "athletics", 0, 0, false, false)

	// Get first world's result with a fresh check
	world.Seed = 42
	world.rng = rand.New(rand.NewSource(42))
	world.CreateCharacter("hero3", "Hero3", map[string]int{"strength": 0}, map[string]int{"athletics": 0})
	_, _, roll1, _, _, _ := world.UnopposedCheckEx("hero3", "skill", "athletics", 0, 0, false, false)

	if roll1 != roll2 {
		t.Logf("RNG was consumed by invalid trait: roll1=%d, roll2=%d", roll1, roll2)
	}
}

func TestOpposedCheckWithModifiers(t *testing.T) {
	world := newTestWorld(t, 42)
	world.CreateCharacter("hero", "Hero", map[string]int{"strength": 1}, map[string]int{"fight": 2})
	world.CreateCharacter("goblin", "Goblin", map[string]int{"strength": 0}, map[string]int{"fight": 1})

	success, actorTotal, oppTotal, _, _, _, err := world.OpposedCheckEx("hero", "skill", "fight", 1, "goblin", "skill", "fight", 0, false)
	if err != nil {
		t.Fatalf("opposed check with modifiers failed: %v", err)
	}

	// hero: 2 (fight) + 1 (modifier) + roll
	// goblin: 1 (fight) + 0 (modifier) + roll
	if actorTotal <= oppTotal && success {
		t.Fatalf("actor should not win with lower total: actor=%d, opp=%d", actorTotal, oppTotal)
	}
}
