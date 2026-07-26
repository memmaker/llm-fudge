package main

import (
	"testing"

	"github.com/memmaker/gollum/llm"
)

// toolNames is the set of calls a tool list puts on the table.
func toolNames(tools []llm.Tool) map[string]bool {
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Function.Name] = true
	}
	return names
}

// preconditionTools are the only calls allowed in both halves of a turn, and
// they are there because they are preconditions as well as consequences. All
// five are constructors, and combat reads what they mint: a character must exist
// before a roll can name them, a place must exist before a scene can be narrated
// in it, a weapon and armor must be on the sheet before a blow is worked out
// against them, and a gun must be loaded before it can be fired.
var preconditionTools = []string{"create_character", "create_location", "add_weapon", "add_armor", "reload_weapon"}

// The split is the design. A call either answers a question the DM cannot answer
// itself, or writes down something the prose has already decided — and the ones
// that do both are the three named above and nothing else. A tool that drifts
// into both sets quietly puts the bookkeeping back where it was.
func TestTheTwoPhasesShareOnlyTheirPreconditions(t *testing.T) {
	consult, record := toolNames(consultTools()), toolNames(recordTools())

	shared := map[string]bool{}
	for _, name := range preconditionTools {
		if !consult[name] {
			t.Errorf("%s is a precondition of resolving and is missing from the consult phase", name)
		}
		if !record[name] {
			t.Errorf("%s is a consequence as well and is missing from the record phase", name)
		}
		shared[name] = true
	}

	for name := range consult {
		if record[name] && !shared[name] {
			t.Errorf("%s is offered in both phases; it belongs to exactly one", name)
		}
	}
}

// Consult-first: the dice, the lookups, the constructors and the one call that
// reaches the player. Nothing whose whole job is to write something down — that
// is a prediction of prose the model has not written yet.
func TestConsultPhaseOffersTheEngineAndNotTheBooks(t *testing.T) {
	consult := toolNames(consultTools())

	for _, name := range []string{
		"unopposed_check", "opposed_check", "opposed_attack", "unopposed_attack",
		"roll_on_table", "recall", "output_narration", "user_choice",
	} {
		if !consult[name] {
			t.Errorf("the consult phase does not offer %s", name)
		}
	}
	for _, name := range []string{
		"add_item", "remove_item", "move_item", "move_character", "add_coins",
		"remove_coins", "equip_armor", "apply_damage", "heal", "declare_dead",
	} {
		if consult[name] {
			t.Errorf("%s is bookkeeping and must wait until the prose exists", name)
		}
	}
}

// Record-after: the books, and no way to resolve anything or reach the player.
// A phase that could still roll would be a phase that could decide the outcome
// of a scene already on screen.
func TestRecordPhaseOffersTheBooksAndNothingThatResolves(t *testing.T) {
	record := toolNames(recordTools())

	for _, name := range []string{
		"add_item", "remove_item", "move_item", "move_character", "add_coins",
		"remove_coins", "add_weapon", "add_armor", "equip_armor", "apply_damage",
		"heal", "declare_dead",
	} {
		if !record[name] {
			t.Errorf("the record phase does not offer %s", name)
		}
	}
	for _, name := range []string{
		"output_narration", "user_choice", "unopposed_check", "opposed_check",
		"opposed_attack", "unopposed_attack", "roll_on_table", "recall",
	} {
		if record[name] {
			t.Errorf("%s is still on the table after the player has read the scene", name)
		}
	}
}
