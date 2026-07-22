package rules

import "fmt"

// RollOnTable resolves a rolled value against a named table.
func RollOnTable(name string, roll int) (string, error) {
	t, ok := tables[name]
	if !ok {
		return "", fmt.Errorf("table not found: %s", name)
	}
	for _, entry := range t.Entries {
		if entry.Range.Contains(roll) {
			return entry.Value, nil
		}
	}
	return "", fmt.Errorf("table %q has no result for roll %d", name, roll)
}
