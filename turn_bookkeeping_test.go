package main

import (
	"math/rand"
	"strings"
	"testing"

	"fudge/rules"

	"github.com/memmaker/gollum/llm"
)

// playedWorld is a world a turn can actually be taken in: somebody to carry
// things, and a forge for the narration to happen in.
func playedWorld(t *testing.T) *rules.World {
	t.Helper()
	w := &rules.World{
		Characters: map[string]*rules.Character{},
		Locations:  map[string]*rules.Location{},
		Rng:        rand.New(rand.NewSource(1)),
		Player:     "catnip",
	}
	w.CreateLocation("Forge", "Hot, dark, ringing with hammer work.", "")
	w.CreateCharacter("catnip", "Catnip", nil, nil)
	return w
}

// The whole point of the second phase: the question "what changed?" is put to
// the DM only once the answer is sitting in front of it. So the round has to
// come after the player has read the scene, it has to carry the prose, and it
// has to have no way left of reaching them.
func TestBookkeepingIsAskedAfterTheProseExists(t *testing.T) {
	dm := &model{reply: resolvingThenNarrating(setOp("forge", "anvil", "cooling"))}

	messages, s := gameStart(), newSession(playedWorld(t))
	capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

	if dm.books != 1 {
		t.Fatalf("the turn ran %d bookkeeping rounds, want exactly one", dm.books)
	}
	asked := dm.seen[len(dm.seen)-1]
	demand := asked[len(asked)-1].Content
	if !strings.Contains(demand, "The smith looks up from the anvil.") {
		t.Errorf("the bookkeeping round was not shown the prose it is meant to read:\n%s", demand)
	}
	// And it was told what the engine had already settled, because recording
	// one of those a second time is the readiest way to get this phase wrong.
	if !strings.Contains(demand, "roll_on_table") {
		t.Errorf("the bookkeeping round was not told what the engine already resolved:\n%s", demand)
	}
	if narrationOffered(dm.tools[len(dm.tools)-1]) {
		t.Error("the bookkeeping round could still have narrated at the player")
	}
}

// The calls it makes are calls like any other: they land on the sheet, and the
// archive keeps the engine's account of them beside the turn's own.
func TestBookkeepingCallsReachTheEngineAndTheRecord(t *testing.T) {
	dm := &model{
		reply: func(round int, tools []llm.Tool) llm.Response {
			return narrationCall(setOp("forge", "handed-over", "lantern"))
		},
		record: func(round int, tools []llm.Tool) llm.Response {
			return llm.Response{Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
				call("i", "add_item", `{"character_id":"catnip","name":"Brass Lantern","description":"Dented, still lit."}`),
			}}}
		},
	}

	w := playedWorld(t)
	messages, s := gameStart(), newSession(w)
	capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

	pc, _ := w.Character("catnip")
	if pc.Inventory["brass-lantern"] == nil {
		t.Fatalf("the lantern the prose handed over never reached the sheet: %v", pc.Inventory)
	}
	recorded := s.archive.Recent(1)
	if len(recorded) != 1 {
		t.Fatal("the turn was not archived")
	}
	var found bool
	for _, event := range recorded[0].Events {
		if event.Tool == "add_item" {
			found = true
		}
	}
	if !found {
		t.Errorf("what the bookkeeping round did is not in the record: %+v", recorded[0].Events)
	}
	// And the ids it named are indexed, which is what makes the record answerable
	// by entity rather than by searching its words.
	var indexed bool
	for _, id := range recorded[0].Entities() {
		if id == "catnip" {
			indexed = true
		}
	}
	if !indexed {
		t.Errorf("the turn is not indexed under the character it was about: %v", recorded[0].Entities())
	}
}

// It is one round unless the engine refuses something. A refusal names what
// does exist, so the correction is one call away — and the alternative is an
// engine that quietly disagrees with a scene the player has already read.
func TestBookkeepingTakesASecondRoundOnlyOnARefusal(t *testing.T) {
	for _, c := range []struct {
		name  string
		args  string
		books int
	}{
		{"accepted", `{"character_id":"catnip","name":"Brass Lantern","description":"Dented."}`, 1},
		{"refused", `{"character_id":"nobody-at-all","name":"Brass Lantern","description":"Dented."}`, maxRecordRequests},
	} {
		t.Run(c.name, func(t *testing.T) {
			dm := &model{
				reply: func(round int, tools []llm.Tool) llm.Response { return narrationCall(setOp("forge", "busy", "true")) },
				record: func(round int, tools []llm.Tool) llm.Response {
					return llm.Response{Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
						call("i", "add_item", c.args),
					}}}
				},
			}

			messages, s := gameStart(), newSession(playedWorld(t))
			capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

			if dm.books != c.books {
				t.Errorf("the %s call took %d bookkeeping rounds, want %d", c.name, dm.books, c.books)
			}
		})
	}
}

// The miss this phase was built to catch: a thing that leaves somebody's hands.
// It is not a purchase, not a gift and not a destruction, so it fell through
// every category the demand used to list — the prose said the phone went, the
// engine went on saying it was carried, and nothing disagreed until the sheet
// was read out loud.
func TestTheDemandNamesAThingLeavingAHand(t *testing.T) {
	demand := strings.ToLower(recordDemand(turnLog{shown: []string{"You hurl the phone into the gutter."}}))

	for _, want := range []string{"dropped", "thrown away", "left behind", "move_item"} {
		if !strings.Contains(demand, want) {
			t.Errorf("the demand never mentions %q, so a discard matches none of its categories:\n%s", want, demand)
		}
	}
	// And it says where a discarded thing goes, because "the ground" is the one
	// destination a model does not think of as a destination.
	if !strings.Contains(demand, "the ground is a destination") {
		t.Errorf("the demand does not say that a thing on the floor is somewhere:\n%s", demand)
	}
}

// The other way a turn records nothing: the thing the prose is about was never
// an engine item, so move_item has no source, and "add_item only for what never
// existed before" reads as ruling it out — the story has been handling the thing
// for turns. Both calls look wrong and the DM makes neither. The test that
// decides between them has to be a lookup in the engine's own record, and it has
// to say so in those words.
func TestTheDemandSettlesExistenceInTheEnginesRecord(t *testing.T) {
	demand := recordDemand(turnLog{shown: []string{"Your fingers close around the parchment."}})

	for _, want := range []string{
		"look for the thing by name among the entities above",
		"however many turns your prose has been describing it",
		"A narrative property is not a record of a thing",
		"add_item is not only for things that appear from nowhere",
	} {
		if !strings.Contains(demand, want) {
			t.Errorf("the demand is missing %q, so a prop the engine never held has no call that fits:\n%s", want, demand)
		}
	}
}

// A turn nobody read has no prose to square the engine against, so it does not
// pay for a round asking.
func TestNothingIsRecordedForATurnThePlayerNeverSaw(t *testing.T) {
	dm := &model{reply: func(round int, tools []llm.Tool) llm.Response { return stalling() }}

	messages, s := gameStart(), newSession(playedWorld(t))
	capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

	if dm.books != 0 {
		t.Errorf("an unnarrated turn spent %d bookkeeping rounds", dm.books)
	}
}

// The second phase is still part of the turn, so it leaves the history exactly
// as it found it — the calls, the results and the demand all go with the rest.
func TestBookkeepingLeavesNothingInTheHistory(t *testing.T) {
	dm := &model{
		reply: func(round int, tools []llm.Tool) llm.Response { return narrationCall(setOp("forge", "busy", "true")) },
		record: func(round int, tools []llm.Tool) llm.Response {
			return llm.Response{Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
				call("i", "add_coins", `{"character_id":"catnip","amount":5}`),
			}}}
		},
	}

	messages, s := gameStart(), newSession(playedWorld(t))
	before := len(messages)
	capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

	if len(messages) != before {
		t.Fatalf("the bookkeeping round left %d messages behind:\n%+v", len(messages)-before, messages[before:])
	}
}
