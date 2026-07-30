package main

import "testing"

func TestParseResponseSplitsProseFromBlocks(t *testing.T) {
	body := "The door bangs shut behind you.\n\n" +
		"<state>\nentities:\n  gareth:\n    location: cellar\n</state>\n\n" +
		"<img>\nA dim cellar, lit by one lantern.\n</img>\n"

	got := parseResponse(body)

	if got.Prose != "The door bangs shut behind you." {
		t.Errorf("prose = %q, want the narration alone", got.Prose)
	}
	if got.State != "entities:\n  gareth:\n    location: cellar" {
		t.Errorf("state = %q", got.State)
	}
	if got.Img != "A dim cellar, lit by one lantern." {
		t.Errorf("img = %q", got.Img)
	}
}

// An event tag is written where the thing happens, which is usually the middle
// of a sentence. Cutting it must not cut the sentence.
func TestParseResponseCutsInlineEventTagsFromProse(t *testing.T) {
	body := `The blade opens his shoulder.<damage target="gareth" severity="hurt"/> He staggers back.`

	got := parseResponse(body)

	if got.Prose != "The blade opens his shoulder. He staggers back." {
		t.Errorf("prose = %q, want the tag gone and the prose intact", got.Prose)
	}
	if len(got.Events) != 1 {
		t.Fatalf("got %d events, want 1", len(got.Events))
	}
	if got.Events[0] != (stateEvent{Kind: "damage", Target: "gareth", Severity: "hurt"}) {
		t.Errorf("event = %+v", got.Events[0])
	}
}

func TestParseResponseReadsHealAndDeathTags(t *testing.T) {
	body := `She presses the poultice home.<heal target="mira" levels="2"/> The bandit does not get up.<dead target="bandit"/>`

	got := parseResponse(body)

	if len(got.Events) != 2 {
		t.Fatalf("got %d events, want 2", len(got.Events))
	}
	if got.Events[0] != (stateEvent{Kind: "heal", Target: "mira", Levels: 2}) {
		t.Errorf("heal = %+v", got.Events[0])
	}
	if got.Events[1] != (stateEvent{Kind: "dead", Target: "bandit"}) {
		t.Errorf("dead = %+v", got.Events[1])
	}
}

// A model asked for YAML tends to fence it. That is noise, not a mistake worth
// spending a correction round on.
func TestParseResponseUnfencesTheStateBlock(t *testing.T) {
	body := "Prose.\n\n<state>\n```yaml\nentities:\n  gareth:\n    coins: 3\n```\n</state>"

	got := parseResponse(body)

	if got.State != "entities:\n  gareth:\n    coins: 3" {
		t.Errorf("state = %q, want the fence stripped", got.State)
	}
}

// A turn that changed nothing the engine holds is an ordinary turn.
func TestParseResponseAcceptsProseAlone(t *testing.T) {
	got := parseResponse("You wait. Nothing comes.")

	if got.Prose != "You wait. Nothing comes." {
		t.Errorf("prose = %q", got.Prose)
	}
	if got.State != "" || len(got.Events) != 0 || got.Img != "" {
		t.Errorf("expected no blocks, got %+v", got)
	}
}
