package main

import (
	"strings"
	"testing"

	"github.com/memmaker/gollum/llm"
)

func call(id, name, arguments string) llm.ToolCall {
	return llm.ToolCall{ID: id, Type: "function", Function: llm.ToolCallFunction{Name: name, Arguments: arguments}}
}

// resolvingThenNarrating is the ordinary shape of a turn: one round that settles
// what happened, one that tells the player about it.
func resolvingThenNarrating(update string) func(round int, tools []llm.Tool) llm.Response {
	return func(round int, tools []llm.Tool) llm.Response {
		if round == 0 {
			return llm.Response{Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
				call("t", "roll_on_table", `{"table":"weather"}`),
			}}}
		}
		return narrationCall(update)
	}
}

// The point of the whole change: a finished turn leaves the history exactly as
// it found it. Everything it produced has a home elsewhere — the world, the
// narrative store, the archive — and a fourth copy in the messages is the one
// that used to grow without bound.
func TestAFinishedTurnLeavesNothingBehind(t *testing.T) {
	dm := &model{reply: resolvingThenNarrating(setOp("forge", "anvil", "cooling"))}

	messages, s := gameStart(), newSession(testWorld())
	before := len(messages)
	capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

	if len(messages) != before {
		t.Fatalf("the turn left %d messages behind, want none:\n%+v", len(messages)-before, messages[before:])
	}
	for _, msg := range messages {
		if msg.Role == llm.RoleAssistant || msg.Role == llm.RoleTool {
			t.Errorf("a %s message survived the turn: %+v", msg.Role, msg)
		}
	}
}

// Ten turns cost what one turn costs. This is the growth curve the design set
// out to flatten, asserted directly rather than inferred from the parts.
func TestHistoryDoesNotGrowWithTurns(t *testing.T) {
	dm := &model{reply: resolvingThenNarrating(setOp("forge", "door", "open"))}

	messages, s := gameStart(), newSession(testWorld())
	before := len(messages)
	for i := 0; i < 10; i++ {
		capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })
	}

	if len(messages) != before {
		t.Errorf("history grew to %d messages over ten turns", len(messages))
	}
	if s.archive.Len() != 10 {
		t.Errorf("the archive holds %d turns, want all 10", s.archive.Len())
	}
}

// Nothing survives in the messages, so what the DM knows next turn is whatever
// the block says. Everything the turn established has to be in it.
func TestWhatTheTurnEstablishedReachesTheNextBlock(t *testing.T) {
	dm := &model{reply: resolvingThenNarrating(setOp("forge", "mood", "wary"))}

	messages, s := gameStart(), newSession(testWorld())
	capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

	block := s.engineState()
	for _, want := range []string{
		"mood: wary",                         // the narrative store took the report
		playerLine,                           // the timeline took the input
		"The smith looks up from the anvil.", // and the prose
	} {
		if !strings.Contains(block, want) {
			t.Errorf("the next turn's block lost %q:\n%s", want, block)
		}
	}
}

// The engine's own account of what it resolved is kept by the engine, so a roll
// is in the record whether or not the DM chose to write it down. This is why the
// archive does not depend on the DM's diligence the way the narrative store does.
func TestTheEngineRecordsItsOwnEvents(t *testing.T) {
	dm := &model{reply: resolvingThenNarrating(setOp("forge", "door", "open"))}

	messages, s := gameStart(), newSession(testWorld())
	capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

	recorded := s.archive.Recent(1)
	if len(recorded) != 1 {
		t.Fatal("the turn was not archived")
	}
	var rolled bool
	for _, event := range recorded[0].Events {
		if event.Tool == "roll_on_table" {
			rolled = true
		}
	}
	if !rolled {
		t.Errorf("the roll the engine resolved is not in the record: %+v", recorded[0].Events)
	}
}

// A turn the player never saw is kept as part of the record — a gap is worth
// explaining — but must not reach the timeline, where it would become something
// later turns narrate around.
func TestAFailedTurnIsArchivedButNotShown(t *testing.T) {
	dm := &model{reply: func(round int, tools []llm.Tool) llm.Response { return stalling() }}

	messages, s := gameStart(), newSession(testWorld())
	before := len(messages)
	capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

	if len(messages) != before {
		t.Errorf("the failed turn left %d messages behind", len(messages)-before)
	}
	if s.archive.Len() != 1 {
		t.Error("the failed turn was not recorded at all")
	}
	if len(s.archive.Recent(5)) != 0 {
		t.Error("an unnarrated turn reached the timeline")
	}
	if strings.Contains(s.engineState(), playerLine) {
		t.Error("the block shows a turn the player was never given")
	}
}

// The prose the player read is the one thing that used to be thrown away
// outright. It is kept now, out of context and behind a lookup.
func TestProseIsRecoverableAfterTheTurn(t *testing.T) {
	dm := &model{reply: resolvingThenNarrating(setOp("forge", "door", "open"))}

	messages, s := gameStart(), newSession(testWorld())
	capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

	got := s.recall(call("r", "recall", `{"turn":1,"include_prose":true}`))
	if !strings.Contains(got, "The smith looks up from the anvil.") {
		t.Errorf("recall cannot reach the prose that was shown:\n%s", got)
	}
}
