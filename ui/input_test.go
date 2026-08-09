package ui

import (
	"strings"
	"testing"
)

// The log a debug run writes has to be a script that plays the same session
// back, so the two formats are checked against each other: a run driven by a
// script must write out the script it was driven by. A menu answered by naming
// the option keeps that spelling; a menu answered with free text writes only
// the text, because that is what selects "type your answer" on replay.
func TestInputLogReplaysAsAScript(t *testing.T) {
	const played = `Deadlands
Automatic - I describe my concept, you decide the traits
A gunslinger with a debt
I kick the door in
`
	if err := LoadScript(strings.NewReader(played)); err != nil {
		t.Fatal(err)
	}
	defer func() { script, inputLog = nil, nil }()

	var log strings.Builder
	LogInputs(&log)

	if got, err := ShowChoiceMenu([]string{"Deadlands", "Star Wars"}); err != nil || got != "Deadlands" {
		t.Fatalf("menu = %q, %v", got, err)
	}
	auto := "Automatic - I describe my concept, you decide the traits"
	if got, err := ShowChoiceMenu([]string{auto, "Custom - I want to choose all my traits"}); err != nil || got != auto {
		t.Fatalf("menu = %q, %v", got, err)
	}
	// A concept is not one of the options, so the menu falls through to the
	// free-text branch and the line is answered by the prompt after it.
	if got, err := ShowChoiceMenu([]string{"Bounty hunter", "Preacher"}); err != nil || got != typeYourAnswer {
		t.Fatalf("menu = %q, %v", got, err)
	}
	if got, err := GetInput(nil, "Concept: "); err != nil || got != "A gunslinger with a debt" {
		t.Fatalf("concept = %q, %v", got, err)
	}
	if got, err := GetInput(nil, "> "); err != nil || got != "I kick the door in" {
		t.Fatalf("turn = %q, %v", got, err)
	}

	if log.String() != played {
		t.Errorf("the log does not replay the session\n got:\n%s\nwant:\n%s", log.String(), played)
	}
}
