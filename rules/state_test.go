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
