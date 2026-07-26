package main

import (
	"strings"
	"testing"

	"fudge/rules"

	"github.com/memmaker/gollum/llm"
)

// narrationAt is a narration that says where it happened, which every narration
// must.
func narrationAt(location string) string {
	return `{"prose":"The smith looks up from the anvil.","location":"` + location + `","world_update":"smith|busy"}`
}

// The location is how the engine learns where the player is standing, so a
// narration that names a place the engine does not hold is refused before the
// player reads a word of it — and the refusal names the places that do exist,
// so the retry is a correction rather than a second guess.
func TestNarrationAtAnUnknownLocationIsRefused(t *testing.T) {
	w := testWorld()
	w.CreateLocation("Forge", "Hot, dark, ringing.", "")
	s := newSession(w)
	messages := gameStart()

	m := &model{reply: func(round int, tools []llm.Tool) llm.Response {
		if round == 0 {
			return llm.Response{Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
				call("n", "output_narration", narrationAt("the-back-room")),
			}}}
		}
		return llm.Response{Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			call("n", "output_narration", narrationAt("forge")),
		}}}
	}}

	out := capture(t, func() { s.processGameTurn(m.chat, &messages, playerLine) })

	if m.rounds != 2 {
		t.Errorf("rounds = %d, want the bad location to cost one and the retry to land", m.rounds)
	}
	if strings.Count(out, "The smith looks up") != 1 {
		t.Errorf("the player should have read the scene exactly once:\n%s", out)
	}
	// The correction has to reach the DM, listing what it could have said.
	var refusal string
	for _, msg := range m.seen[1] {
		if msg.Role == llm.RoleTool && strings.Contains(msg.Content, "location not found") {
			refusal = msg.Content
		}
	}
	if refusal == "" {
		t.Fatalf("the DM was never told the location was wrong: %+v", m.seen[1])
	}
	if !strings.Contains(refusal, "forge") {
		t.Errorf("refusal = %q, want it to list the locations that exist", refusal)
	}
}

// Position is the engine's now, and this is the call that maintains it.
func TestNarrationMovesThePlayerToTheScene(t *testing.T) {
	w := testWorld()
	w.CreateCharacter("catnip", "Catnip", nil, nil)
	w.CreateLocation("Forge", "Hot, dark, ringing.", "")
	w.CreateLocation("Alley", "Wet brick.", "")
	s := newSession(w)
	messages := gameStart()

	m := &model{reply: func(round int, tools []llm.Tool) llm.Response {
		return llm.Response{Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			call("n", "output_narration", narrationAt("alley")),
		}}}
	}}
	capture(t, func() { s.processGameTurn(m.chat, &messages, playerLine) })

	if got := w.Characters["catnip"].Location; got != "alley" {
		t.Errorf("player is at %q, want alley", got)
	}
	if got := w.Scene(); got != "alley" {
		t.Errorf("scene = %q, want alley", got)
	}
}

// A turn that walks somewhere new creates the place and narrates it in one
// response. That only works if the engine runs the other calls before it reads
// the narration, whatever order the model emitted them in — so this response
// emits them backwards.
func TestALocationCreatedInTheSameResponseCanBeNarratedIn(t *testing.T) {
	w := testWorld()
	w.CreateCharacter("catnip", "Catnip", nil, nil)
	w.CreateLocation("Forge", "Hot, dark, ringing.", "")
	s := newSession(w)
	messages := gameStart()

	m := &model{reply: func(round int, tools []llm.Tool) llm.Response {
		return llm.Response{Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			call("n", "output_narration", narrationAt("back-room")),
			call("c", "create_location", `{"name":"Back Room","description":"Bare boards and a cot.","parent":"forge"}`),
		}}}
	}}
	out := capture(t, func() { s.processGameTurn(m.chat, &messages, playerLine) })

	if m.rounds != 1 {
		t.Errorf("rounds = %d, want one: creating and narrating belong in the same response", m.rounds)
	}
	if !strings.Contains(out, "The smith looks up") {
		t.Errorf("the player never read the scene:\n%s", out)
	}
	if got := w.Characters["catnip"].Location; got != "back-room" {
		t.Errorf("player is at %q, want back-room", got)
	}
}

// The last round has the engine taken off the table, but not create_location:
// a turn whose scene has moved somewhere new would otherwise have to name a
// slug that does not exist, be refused, and be lost for want of a call it was
// not allowed to make.
func TestTheLastRoundCanStillCreateTheSceneItNarrates(t *testing.T) {
	w := testWorld()
	w.CreateCharacter("catnip", "Catnip", nil, nil)
	s := newSession(w)
	messages := gameStart()

	m := &model{reply: func(round int, tools []llm.Tool) llm.Response {
		if !onlyNarration(tools) {
			return stalling()
		}
		return llm.Response{Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			call("c", "create_location", `{"name":"Back Room","description":"Bare boards and a cot."}`),
			call("n", "output_narration", narrationAt("back-room")),
		}}}
	}}
	out := capture(t, func() { s.processGameTurn(m.chat, &messages, playerLine) })

	if !narrated(s) {
		t.Fatalf("the turn was lost on its last round:\n%s", out)
	}
	if got := w.Characters["catnip"].Location; got != "back-room" {
		t.Errorf("player is at %q, want back-room", got)
	}
	// Nothing that resolves anything came back to the table.
	for _, tool := range m.tools[maxTurnRequests-1] {
		switch tool.Function.Name {
		case "output_narration", "create_location":
		default:
			t.Errorf("the last round still offered %s", tool.Function.Name)
		}
	}
}

// A narration with no location at all is as unusable as one with no prose.
func TestNarrationWithoutALocationIsRefused(t *testing.T) {
	w := testWorld()
	w.CreateLocation("Forge", "Hot, dark, ringing.", "")
	s := newSession(w)
	messages := gameStart()

	m := &model{reply: func(round int, tools []llm.Tool) llm.Response {
		if round == 0 {
			return llm.Response{Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
				call("n", "output_narration", `{"prose":"The smith looks up.","world_update":"smith|busy"}`),
			}}}
		}
		return llm.Response{Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			call("n", "output_narration", narrationAt("forge")),
		}}}
	}}
	out := capture(t, func() { s.processGameTurn(m.chat, &messages, playerLine) })

	if m.rounds != 2 {
		t.Errorf("rounds = %d, want the locationless narration refused and retried", m.rounds)
	}
	if strings.Count(out, "The smith looks up") != 1 {
		t.Errorf("the player should have read one scene:\n%s", out)
	}
}

// The opening turn has no location to name yet, so the briefing has to ask for
// one to be created. If it stopped saying so, turn one would be refused by
// construction and every game would open on a wasted round.
func TestTheOpeningBriefingAsksForALocation(t *testing.T) {
	w := testWorld()
	w.AdventureContent = "The party arrives in Dryfall."
	got := openingBriefing(w)

	for _, want := range []string{"create_location", "location"} {
		if !strings.Contains(got, want) {
			t.Errorf("the briefing never mentions %q:\n%s", want, got)
		}
	}
}

// An NPC walks into the scene, so the scene is where they are unless the DM
// says otherwise.
func TestNewCharactersStartWhereTheSceneIs(t *testing.T) {
	w := testWorld()
	w.Setting = deadlands(t)
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

func deadlands(t *testing.T) *rules.SettingInfos {
	t.Helper()
	setting, err := rules.LoadSetting("settings/deadlands")
	if err != nil {
		t.Fatal(err)
	}
	return setting
}
