package rules

import (
	"fmt"
	"sort"
	"strings"
)

// TableNames lists the rollable tables, sorted so the tool schema built from it
// is stable between runs.
func TableNames() []string {
	names := make([]string, 0, len(tables))
	for name := range tables {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// RollOnTable resolves a rolled value against a named table.
func RollOnTable(name string, roll int) (string, error) {
	t, ok := tables[name]
	if !ok {
		return "", fmt.Errorf("table not found: %s (have %s)", name, strings.Join(TableNames(), ", "))
	}
	for _, entry := range t.Entries {
		if entry.Range.Contains(roll) {
			return entry.Value, nil
		}
	}
	return "", fmt.Errorf("table %q has no result for roll %d", name, roll)
}

// RollTable rolls 4dF against a named table. The modifier covers whatever the
// fiction says should tilt the result — a reputation, a bribe, the season — and
// the table ends are open so it cannot push the roll off the table.
func (w *World) RollTable(name string, modifier int) (int, string, error) {
	if _, ok := tables[name]; !ok {
		return 0, "", fmt.Errorf("table not found: %s (have %s)", name, strings.Join(TableNames(), ", "))
	}
	roll := roll4df(w.Rng)
	value, err := RollOnTable(name, roll+modifier)
	if err != nil {
		return roll, "", err
	}
	return roll, value, nil
}

// difficultyNames maps every accepted spelling of a difficulty onto the trait
// scale. Both registers are accepted: the difficulty a GM would say out loud
// ("very hard") and the trait level it corresponds to ("great").
var difficultyNames = map[string]int{
	"trivially easy": -3, "terrible": -3,
	"very easy": -2, "poor": -2,
	"easy": -1, "mediocre": -1,
	"average": 0, "fair": 0,
	"hard": 1, "good": 1,
	"very hard": 2, "great": 2,
	"extremely hard": 3, "superb": 3,
}

// DifficultyNames lists the difficulties a check may be set to, easiest first.
func DifficultyNames() []string {
	return []string{"trivially easy", "very easy", "easy", "average", "hard", "very hard", "extremely hard"}
}

// DifficultyLevel converts a named difficulty to its value on the trait scale.
// An unknown name is an error rather than a silent Average: a check the caller
// meant to be punishing should not quietly become routine.
func DifficultyLevel(s string) (int, error) {
	level, ok := difficultyNames[strings.ToLower(strings.TrimSpace(s))]
	if !ok {
		return 0, fmt.Errorf("unknown difficulty %q (have %s)", s, strings.Join(DifficultyNames(), ", "))
	}
	return level, nil
}

// severityPoints maps a named wound onto the damage points that inflict it on
// an undamaged character. The lightest value in each band is deliberate: a hit
// should land as the wound the fiction called for and no worse.
var severityPoints = map[string]int{
	"scratch":       1,
	"hurt":          3,
	"very hurt":     5,
	"incapacitated": 7,
	"near death":    9,
}

// SeverityNames lists the wounds a hit may inflict, lightest first.
func SeverityNames() []string {
	return []string{"scratch", "hurt", "very hurt", "incapacitated", "near death"}
}

// DamagePointsForSeverity converts a named wound to damage points. Callers name
// the wound they intend and the engine picks the arithmetic, so the two can
// never disagree.
func DamagePointsForSeverity(s string) (int, error) {
	dp, ok := severityPoints[strings.ToLower(strings.TrimSpace(s))]
	if !ok {
		return 0, fmt.Errorf("unknown severity %q (have %s)", s, strings.Join(SeverityNames(), ", "))
	}
	return dp, nil
}
