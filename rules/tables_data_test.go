package rules

import "testing"

func TestRollRangeContains(t *testing.T) {
	tests := []struct {
		name   string
		range_ RollRange
		roll   int
		want   bool
	}{
		{"exact match", ExactRoll(2), 2, true},
		{"exact miss", ExactRoll(2), 1, false},
		{"bounded match", RollRangeBetween(5, 6), 6, true},
		{"bounded miss", RollRangeBetween(5, 6), 7, false},
		{"upper open range", RollAtMost(-3), -10, true},
		{"upper open miss", RollAtMost(-3), -2, false},
		{"lower open range", RollAtLeast(3), 10, true},
		{"lower open miss", RollAtLeast(3), 2, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.range_.Contains(test.roll); got != test.want {
				t.Fatalf("Contains(%d) = %t, want %t", test.roll, got, test.want)
			}
		})
	}
}

func TestRollOnTableUsesEntryRanges(t *testing.T) {
	result, err := RollOnTable("npc-morale", -4)
	if err != nil {
		t.Fatalf("resolve low morale: %v", err)
	}
	if result != "Surrender/Flee" {
		t.Fatalf("low morale result = %q", result)
	}

	result, err = RollOnTable("npc-reaction", 11)
	if err != nil {
		t.Fatalf("resolve npc reaction: %v", err)
	}
	if result != "Neutral/Uncertain" {
		t.Fatalf("npc reaction result = %q", result)
	}

	if _, err := RollOnTable("weather", 5); err == nil {
		t.Fatal("expected an error for an uncovered roll")
	}
}
