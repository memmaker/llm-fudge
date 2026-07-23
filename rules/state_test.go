package rules

import "testing"

func TestWoundStateUsesOneLevel(t *testing.T) {
	state := WoundState{}
	if result := state.ApplyDamage(5); result != "Very Hurt" {
		t.Fatalf("ApplyDamage result = %q", result)
	}
	if state.Level != VeryHurt || state.Scratches != 0 {
		t.Fatalf("wound state = %#v", state)
	}
	if !state.CanAct() || state.Penalty() != -2 {
		t.Fatalf("very hurt state is wrong: canAct=%t penalty=%d", state.CanAct(), state.Penalty())
	}

	state.ApplyDamage(8)
	if state.Level != Incapacitated || state.CanAct() {
		t.Fatalf("incapacitated state is wrong: %#v", state)
	}
	state.Heal(1)
	if state.Level != VeryHurt {
		t.Fatalf("healing should reduce one level, got %s", state.Level)
	}
}

func TestScratchAccumulation(t *testing.T) {
	state := WoundState{}
	if state.Level != Undamaged {
		t.Fatalf("initial level should be Undamaged, got %s", state.Level)
	}
	if state.Penalty() != 0 {
		t.Fatalf("undamaged should have no penalty")
	}

	// Apply damage to get scratches
	state.ApplyDamage(1)
	if state.Level != Scratch || state.Scratches != 1 {
		t.Fatalf("after 1 damage: expected Scratch with 1 scratch, got %s with %d", state.Level, state.Scratches)
	}

	// Apply more damage - should accumulate scratches up to 3
	state.ApplyDamage(1)
	if state.Level != Scratch || state.Scratches != 2 {
		t.Fatalf("after 2 damage: expected Scratch with 2 scratches, got %s with %d", state.Level, state.Scratches)
	}

	state.ApplyDamage(1)
	if state.Level != Hurt || state.Scratches != 0 {
		t.Fatalf("after 3 damage: expected Hurt with 0 scratches, got %s with %d", state.Level, state.Scratches)
	}
}

func TestHealingPreventedWhenDead(t *testing.T) {
	state := WoundState{}
	state.Level = Dead

	healed := state.Heal(1)
	if state.Level != Dead {
		t.Fatalf("dead character should not be healable, got %s", state.Level)
	}
	if healed == "Healed" {
		t.Fatalf("healing dead should not return 'Healed'")
	}
}

func TestDeclareDead(t *testing.T) {
	state := WoundState{}
	state.Level = Hurt
	state.Scratches = 1

	state.DeclareDead()

	if state.Level != Dead {
		t.Fatalf("after declaring dead, level should be Dead, got %s", state.Level)
	}
	if state.Scratches != 0 {
		t.Fatalf("after declaring dead, scratches should be 0, got %d", state.Scratches)
	}
}

func TestHealingWithoutExceedingUndamaged(t *testing.T) {
	state := WoundState{}
	state.Level = Hurt

	state.Heal(10)

	if state.Level != Undamaged {
		t.Fatalf("healing should not exceed undamaged, got %s", state.Level)
	}
	if state.Scratches != 0 {
		t.Fatalf("undamaged should have 0 scratches, got %d", state.Scratches)
	}
}
