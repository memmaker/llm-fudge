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

// The rule that decides what is still a tool: a call exists during play if and
// only if a query depends on the state it sets. Everything else is declared in
// the state block, where it is a fact rather than a command.
//
// Two calls pass that test. A character must exist before a roll can name them,
// and a gun must be loaded before it can be fired — both are state a check reads
// in the same response it is set in, which a block written under the prose is
// far too late for.
var midTurnConstructors = []string{"create_character", "reload_weapon"}

// Queries answer something the DM cannot answer itself. Nothing that merely
// writes something down is on the list any more: a declarative block has no way
// to hand a result back, which is exactly why these are what stayed.
func TestPlayOffersQueriesAndNothingThatOnlyRecords(t *testing.T) {
	play := toolNames(queryTools())

	for _, name := range []string{
		"unopposed_check", "opposed_check", "opposed_attack", "unopposed_attack",
		"roll_on_table", "recall", "user_choice",
	} {
		if !play[name] {
			t.Errorf("play does not offer %s", name)
		}
	}
	for _, name := range midTurnConstructors {
		if !play[name] {
			t.Errorf("%s is a precondition of resolving and must be on the table", name)
		}
	}
}

// The bookkeeping calls are gone, not moved. A tool that comes back is the whole
// design coming back with it: fourteen calls the DM has to remember to make,
// beside a block that says the same things declaratively.
func TestTheBookkeepingCallsAreGoneFromPlay(t *testing.T) {
	play := toolNames(queryTools())

	for _, name := range []string{
		"output_narration", "create_location", "move_item", "move_character",
		"add_item", "remove_item", "add_coins", "remove_coins", "add_weapon",
		"add_armor", "equip_armor", "apply_damage", "heal", "declare_dead",
	} {
		if play[name] {
			t.Errorf("%s is still offered during play; the state block records it now", name)
		}
	}
}

// Character creation happens before there is a world to declare anything
// against, so it keeps its own calls — and none of them are the ones that
// resolve a scene.
func TestCharacterCreationKeepsItsOutfittingCalls(t *testing.T) {
	creation := toolNames(characterCreationTools())

	for _, name := range []string{"create_character", "add_item", "add_weapon", "add_armor", "add_coins", "equip_armor"} {
		if !creation[name] {
			t.Errorf("character creation cannot outfit anybody without %s", name)
		}
	}
	for _, name := range []string{"opposed_attack", "unopposed_check", "recall"} {
		if creation[name] {
			t.Errorf("character creation should not offer %s — there is no scene yet", name)
		}
	}
}
