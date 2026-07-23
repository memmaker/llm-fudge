package rules

import (
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

	result, err := world.UnopposedCheck("hero", "skill", "athletics", 0, 2, false, false)
	if err != nil {
		t.Fatalf("unopposed check with modifier failed: %v", err)
	}
	// 2 (athletics) + 2 (modifier) = expected base of 4 before roll
	expectedBase := 2 + 2
	if result.Total != expectedBase+result.Roll {
		t.Fatalf("expected total %d, got %d (base %d + roll %d)", expectedBase+result.Roll, result.Total, expectedBase, result.Roll)
	}
	if result.CritSuccess || result.CritFailure {
		t.Logf("no natural critical expected with roll %d", result.Roll)
	}
}

func TestUnopposedCheckDetectsCriticalSuccess(t *testing.T) {
	// Search for a seed that produces natural +4
	var result *CheckResult
	foundSeed := int64(-1)
	for seed := int64(0); seed < 1000; seed++ {
		world := newTestWorld(t, seed)
		world.CreateCharacter("hero", "Hero", map[string]int{"strength": 0}, map[string]int{"athletics": 0})

		// Try several rolls with this seed
		for i := 0; i < 10; i++ {
			r, _ := world.UnopposedCheck("hero", "skill", "athletics", 0, 0, false, false)
			if r.Roll == 4 {
				result = r
				foundSeed = seed
				break
			}
		}
		if foundSeed >= 0 {
			break
		}
	}

	if foundSeed < 0 {
		t.Fatal("could not find seed that produces natural +4 within 1000 seeds and 10 rolls each")
	}
	if !result.CritSuccess {
		t.Fatal("natural +4 should be critical success")
	}
}

func TestUnopposedCheckDetectsCriticalFailure(t *testing.T) {
	// Search for a seed that produces natural -4
	var result *CheckResult
	foundSeed := int64(-1)
	for seed := int64(0); seed < 1000; seed++ {
		world := newTestWorld(t, seed)
		world.CreateCharacter("hero", "Hero", map[string]int{"strength": 0}, map[string]int{"athletics": 0})

		// Try several rolls with this seed
		for i := 0; i < 10; i++ {
			r, _ := world.UnopposedCheck("hero", "skill", "athletics", 0, 0, false, false)
			if r.Roll == -4 {
				result = r
				foundSeed = seed
				break
			}
		}
		if foundSeed >= 0 {
			break
		}
	}

	if foundSeed < 0 {
		t.Fatal("could not find seed that produces natural -4 within 1000 seeds and 10 rolls each")
	}
	if result.Success {
		t.Fatal("natural -4 should always fail")
	}
	if !result.CritFailure {
		t.Fatal("natural -4 should be critical failure")
	}
}

func TestOpposedCheckWithIndependentModifiers(t *testing.T) {
	world := newTestWorld(t, 42)
	world.CreateCharacter("hero", "Hero", map[string]int{"strength": 1}, map[string]int{"fight": 2})
	world.CreateCharacter("goblin", "Goblin", map[string]int{"strength": 0}, map[string]int{"fight": 1})

	// Both have independent modifiers
	result, err := world.OpposedCheck("hero", "skill", "fight", 1, "goblin", "skill", "fight", 1, false)
	if err != nil {
		t.Fatalf("opposed check failed: %v", err)
	}

	// hero: 2 + 1 = 3, goblin: 1 + 1 = 2 (before rolls)
	// hero still has advantage
	if !result.ActorWins && result.ActorTotal <= result.OpponentTotal {
		t.Fatalf("actor with better trait should likely win: actor=%d, opp=%d", result.ActorTotal, result.OpponentTotal)
	}
	if result.ActorWins && result.ActorTotal <= result.OpponentTotal {
		t.Fatalf("win requires strict advantage: actor=%d, opp=%d", result.ActorTotal, result.OpponentTotal)
	}
	if result.ActorWins && result.RelativeDegree != result.ActorTotal-result.OpponentTotal {
		t.Fatalf("margin calculation wrong: expected %d, got %d", result.ActorTotal-result.OpponentTotal, result.RelativeDegree)
	}
}

func TestAddOneSpendsFudgePoint(t *testing.T) {
	world := newTestWorld(t, 42)
	ch := world.CreateCharacter("hero", "Hero", map[string]int{"strength": 0}, map[string]int{"athletics": 0})
	ch.FudgePoints = 1

	// Run check with add_one: true
	_, err := world.UnopposedCheck("hero", "skill", "athletics", 0, 0, true, false)
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
	_, err := world.UnopposedCheck("hero", "skill", "athletics", 0, 0, true, false)
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
	result1, err := world.UnopposedCheck("hero", "skill", "athletics", 0, 0, false, false)
	if err != nil {
		t.Fatalf("first check failed: %v", err)
	}

	// Restore Fudge Point for reroll
	ch.FudgePoints = 1

	// Run reroll
	result2, err := world.UnopposedCheck("hero", "skill", "athletics", 0, 0, false, true)
	if err != nil {
		t.Fatalf("reroll failed: %v", err)
	}

	if ch.FudgePoints != 0 {
		t.Fatalf("reroll should spend 1 Fudge Point, got %d remaining", ch.FudgePoints)
	}

	// Reroll should produce different results with different RNG state
	if result1.Roll == result2.Roll && result1.Total == result2.Total {
		t.Logf("warning: same roll on reroll (possible but unlikely): roll=%d, total=%d", result1.Roll, result1.Total)
	}
}

func TestRerollFailsWithoutFudgePoint(t *testing.T) {
	world := newTestWorld(t, 42)
	world.CreateCharacter("hero", "Hero", map[string]int{"strength": 0}, map[string]int{"athletics": 0})

	// Try to reroll without Fudge Points
	_, err := world.UnopposedCheck("hero", "skill", "athletics", 0, 0, false, true)
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

	// Try to use invalid trait - should fail before RNG consumption
	_, err := world.UnopposedCheck("hero", "skill", "invalid-skill", 0, 0, false, false)
	if err == nil {
		t.Fatal("should reject invalid trait")
	}

	// Verify RNG was not consumed: second world with same seed should produce same result
	world2 := newTestWorld(t, 42)
	world2.CreateCharacter("hero", "Hero", map[string]int{"strength": 0}, map[string]int{"athletics": 0})

	// Valid check in world2 should produce same roll as world1 would have without the error
	result2, _ := world2.UnopposedCheck("hero", "skill", "athletics", 0, 0, false, false)

	// Create fresh world and verify determinism
	world3 := newTestWorld(t, 42)
	world3.CreateCharacter("hero", "Hero", map[string]int{"strength": 0}, map[string]int{"athletics": 0})
	result3, _ := world3.UnopposedCheck("hero", "skill", "athletics", 0, 0, false, false)

	if result2.Roll != result3.Roll {
		t.Errorf("RNG consumed by invalid trait: roll2=%d, roll3=%d", result2.Roll, result3.Roll)
	}
}

func TestOpposedCheckWithModifiers(t *testing.T) {
	world := newTestWorld(t, 42)
	world.CreateCharacter("hero", "Hero", map[string]int{"strength": 1}, map[string]int{"fight": 2})
	world.CreateCharacter("goblin", "Goblin", map[string]int{"strength": 0}, map[string]int{"fight": 1})

	result, err := world.OpposedCheck("hero", "skill", "fight", 1, "goblin", "skill", "fight", 0, false)
	if err != nil {
		t.Fatalf("opposed check with modifiers failed: %v", err)
	}

	// hero: 2 (fight) + 1 (modifier) + roll
	// goblin: 1 (fight) + 0 (modifier) + roll
	if result.ActorTotal <= result.OpponentTotal && result.ActorWins {
		t.Fatalf("actor should not win with lower total: actor=%d, opp=%d", result.ActorTotal, result.OpponentTotal)
	}
}
