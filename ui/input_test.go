package ui

import (
	markterm "github.com/memmaker/gollum/mark-term"
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
	if got, err := ShowChoiceMenu([]string{"Bounty hunter", "Preacher"}); err != nil || got != TypeYourAnswer {
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

// The menu counts the lines it drew so it can erase exactly those. A wrap that
// returned a trailing newline or an escape sequence would put the count out and
// the redraw would eat the line above.
func TestWrapFitsAndCountsStraight(t *testing.T) {
	long := strings.Repeat("word ", 30)
	got := markterm.Wrap(long, 20)
	if strings.HasSuffix(got, "\n") {
		t.Fatalf("trailing newline: %q", got)
	}
	if strings.Contains(got, "\033") {
		t.Fatal("wrap introduced an escape sequence")
	}
	for _, line := range strings.Split(got, "\n") {
		if markterm.StringWidth(line) > 20 {
			t.Fatalf("line over width: %q", line)
		}
	}
	if markterm.Wrap("short", 20) != "short" {
		t.Fatal("a short option was not left alone")
	}
}
