package main

import (
	"strings"
	"testing"

	"fudge/rules"

	"github.com/memmaker/gollum/llm"
)

// movingTo is a turn's reply that walks the player somewhere: the scene follows
// their own position, so saying where they are is the whole of saying where the
// scene is.
func movingTo(location string) llm.Response {
	return narration("entities:\n  catnip:\n    location: " + location)
}

// Position is the engine's, and the block is what maintains it. There is no
// separate scene field to keep in step — the scene is wherever the player
// stands.
func TestDeclaringThePlayersLocationMovesTheScene(t *testing.T) {
	w := testWorld()
	w.CreateCharacter("catnip", "Catnip", nil, nil)
	w.CreateLocation("Alley", "Wet brick.", "")
	s := newSession(w)
	messages := gameStart()

	m := &model{reply: func(round int, tools []llm.Tool) llm.Response { return movingTo("alley") }}
	capture(t, func() { s.processGameTurn(m.chat, &messages, playerLine) })

	if got := w.Characters["catnip"].Location; got != "alley" {
		t.Errorf("player is at %q, want alley", got)
	}
	if got := w.Scene(); got != "alley" {
		t.Errorf("scene = %q, want alley", got)
	}
}

// A place the engine does not hold is refused, and the refusal names the places
// that do exist — so the correction round is a correction rather than a second
// guess. The player still read the scene: the prose was never at risk.
func TestMovingToAnUnknownLocationIsRefusedWithTheOnesThatExist(t *testing.T) {
	w := testWorld()
	w.CreateCharacter("catnip", "Catnip", nil, nil)
	s := newSession(w)
	messages := gameStart()

	m := &model{
		reply:   func(round int, tools []llm.Tool) llm.Response { return movingTo("the-back-room") },
		correct: func(round int, tools []llm.Tool) llm.Response { return emptyBlock() },
	}
	out := capture(t, func() { s.processGameTurn(m.chat, &messages, playerLine) })

	if !strings.Contains(out, "The smith looks up") {
		t.Errorf("the player did not read the scene, which was never in doubt:\n%s", out)
	}
	if m.fixes != 1 {
		t.Fatalf("the refusal bought %d correction rounds, want one", m.fixes)
	}
	demand := m.seen[len(m.seen)-1]
	refusal := demand[len(demand)-1].Content
	if !strings.Contains(refusal, "forge") {
		t.Errorf("the demand does not list the locations that exist:\n%s", refusal)
	}
}

// A turn that walks somewhere new creates the place and moves into it in one
// block, in whichever order they were written. That is the point of resolving
// every new id before anything is applied.
func TestALocationCreatedInTheSameBlockCanBeMovedInto(t *testing.T) {
	w := testWorld()
	w.CreateCharacter("catnip", "Catnip", nil, nil)
	s := newSession(w)
	messages := gameStart()

	m := &model{reply: func(round int, tools []llm.Tool) llm.Response {
		return narration(`entities:
  catnip:
    location: back-room
  back-room:
    type: location
    name: Back Room
    desc: Bare boards and a cot.
    parent: forge`)
	}}
	out := capture(t, func() { s.processGameTurn(m.chat, &messages, playerLine) })

	if m.rounds != 1 {
		t.Errorf("rounds = %d, want one: creating and moving belong in the same block", m.rounds)
	}
	if m.fixes != 0 {
		t.Errorf("the block cost %d correction rounds, want none", m.fixes)
	}
	if !strings.Contains(out, "The smith looks up") {
		t.Errorf("the player never read the scene:\n%s", out)
	}
	if got := w.Characters["catnip"].Location; got != "back-room" {
		t.Errorf("player is at %q, want back-room", got)
	}
}

// A turn that says nothing about where anybody is leaves them where they were.
// This is the ordinary case — most turns do not move the scene — and it is why
// the position no longer has to be restated every single turn.
func TestATurnThatSaysNothingLeavesThePlayerWhereTheyWere(t *testing.T) {
	w := testWorld()
	w.CreateCharacter("catnip", "Catnip", nil, nil)
	w.Characters["catnip"].Location = "forge"
	s := newSession(w)
	messages := gameStart()

	m := &model{reply: func(round int, tools []llm.Tool) llm.Response { return narration("") }}
	capture(t, func() { s.processGameTurn(m.chat, &messages, playerLine) })

	if got := w.Scene(); got != "forge" {
		t.Errorf("scene = %q, want the turn to have left it alone", got)
	}
	if m.fixes != 0 {
		t.Errorf("a turn with no block cost %d correction rounds", m.fixes)
	}
}

// The opening turn has no location to name yet, so the briefing has to ask for
// one. If it stopped saying so, turn one would open with the player standing
// nowhere.
func TestTheOpeningBriefingAsksForTheOpeningBlock(t *testing.T) {
	w := testWorld()
	w.AdventureContent = "The party arrives in Dryfall."
	got := openingBriefing(w)

	for _, want := range []string{"<state>", "creates the place", "full baseline"} {
		if !strings.Contains(got, want) {
			t.Errorf("the briefing never mentions %q:\n%s", want, got)
		}
	}
}

// An NPC walks into the scene, so the scene is where they are unless the DM
// says otherwise.
func TestNewCharactersStartWhereTheSceneIs(t *testing.T) {
	w := testWorld()
	player, err := w.CreateCharacterValidated(rules.CharacterRequest{Name: "Catnip", Species: "human"})
	if err != nil {
		t.Fatal(err)
	}
	w.CreateLocation("Forge", "Hot, dark, ringing.", "")
	player.Location = "forge"

	smith, err := w.CreateCharacterValidated(rules.CharacterRequest{Name: "Grimwald", Species: "human"})
	if err != nil {
		t.Fatal(err)
	}
	if smith.Location != "forge" {
		t.Errorf("the smith was created at %q, want the scene", smith.Location)
	}
}

// deadlandsSetting is the setting every hand-built test world carries. A world
// without one has no trait names to validate against, and nothing in a real game
// ever reaches play without a setting loaded — so the helpers give it one rather
// than the engine carrying a guard for a state only a test can produce.
func deadlandsSetting() *rules.SettingInfos {
	setting, err := rules.LoadSetting("settings/deadlands")
	if err != nil {
		panic(err)
	}
	return setting
}
