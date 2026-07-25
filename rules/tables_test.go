package rules

import "testing"

// Every table is rolled with 4dF (-4..+4) and takes a modifier the GM chooses,
// documented as roughly -4 to +4. Any combination has to land on an entry, so
// the tables need open ends rather than an exact entry at each extreme.
func TestEveryTableCoversEveryRoll(t *testing.T) {
	for _, name := range TableNames() {
		for roll := -8; roll <= 8; roll++ {
			if _, err := RollOnTable(name, roll); err != nil {
				t.Errorf("table %q roll %+d: %v", name, roll, err)
			}
		}
	}
}

func TestDifficultyLevel(t *testing.T) {
	// Both registers resolve to the same value: the difficulty a GM says out
	// loud, and the trait level it corresponds to.
	for _, c := range []struct {
		in   string
		want int
	}{
		{"trivially easy", -3}, {"average", 0}, {"Hard", 1}, {"  very hard  ", 2},
		{"extremely hard", 3}, {"superb", 3}, {"fair", 0},
	} {
		got, err := DifficultyLevel(c.in)
		if err != nil {
			t.Errorf("DifficultyLevel(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("DifficultyLevel(%q) = %d, want %d", c.in, got, c.want)
		}
	}

	// An unrecognised difficulty must not silently resolve to Average.
	if _, err := DifficultyLevel("impossible"); err == nil {
		t.Error("DifficultyLevel(\"impossible\") succeeded, want error")
	}

	// Every name offered in the tool schema has to parse.
	for _, name := range DifficultyNames() {
		if _, err := DifficultyLevel(name); err != nil {
			t.Errorf("advertised difficulty %q does not parse: %v", name, err)
		}
	}
}

// Naming a severity has to produce exactly that wound on an undamaged
// character, or the DM's intent and the engine's arithmetic have drifted apart.
func TestSeverityInflictsNamedWound(t *testing.T) {
	want := map[string]string{
		"scratch":       "Scratch",
		"hurt":          "Hurt",
		"very hurt":     "Very Hurt",
		"incapacitated": "Incapacitated",
		"near death":    "Near Death",
	}
	for _, name := range SeverityNames() {
		dp, err := DamagePointsForSeverity(name)
		if err != nil {
			t.Errorf("advertised severity %q does not parse: %v", name, err)
			continue
		}
		var ws WoundState
		if got := ws.ApplyDamage(dp); got != want[name] {
			t.Errorf("severity %q (%d dp) inflicted %q, want %q", name, dp, got, want[name])
		}
	}

	if _, err := DamagePointsForSeverity("obliterated"); err == nil {
		t.Error("DamagePointsForSeverity(\"obliterated\") succeeded, want error")
	}
}
