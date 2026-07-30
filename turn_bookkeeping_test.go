package main

import (
	"math/rand"
	"os"
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

// narrating answers every round with the scene and a block, which is the shape
// of a turn that needed nothing resolved.
func narrating(state string) func(round int, tools []llm.Tool) llm.Response {
	return func(round int, tools []llm.Tool) llm.Response { return narration(state) }
}

// The bookkeeping is not a round any more. What justified one was that the
// question "what changed?" needed the prose to exist first — and the block is
// written under the prose, in the same generation, so it does.
func TestAnAcceptedBlockCostsNoExtraRound(t *testing.T) {
	dm := &model{reply: resolvingThenNarrating(setState("forge", "anvil", "cooling"))}

	messages, s := gameStart(), newSession(playedWorld(t))
	capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

	if dm.fixes != 0 {
		t.Errorf("a block the engine took whole still cost %d correction rounds", dm.fixes)
	}
	if dm.rounds != 2 {
		t.Errorf("the turn made %d calls, want one to resolve and one to narrate", dm.rounds)
	}
}

// What the block says lands on the sheet, and the archive keeps the engine's own
// account of it — under the name of the operation the engine derived, so recall
// searches the same vocabulary it always did.
func TestTheBlockReachesTheEngineAndTheRecord(t *testing.T) {
	dm := &model{reply: narrating(`entities:
  brass-lantern:
    type: item
    name: Brass Lantern
    desc: Dented, still lit.
    location: catnip`)}

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
		t.Errorf("what the block did is not in the record: %+v", recorded[0].Events)
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

// A refusal buys exactly one round, and only a refusal does. The scene is
// already read, so the alternative is an engine that quietly disagrees with it.
func TestARefusalBuysOneCorrectionRound(t *testing.T) {
	for _, c := range []struct {
		name  string
		block string
		fixes int
	}{
		{"accepted", "entities:\n  catnip:\n    coins: 5", 0},
		{"refused", "entities:\n  nobody-at-all:\n    coins: 5", 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			dm := &model{reply: narrating(c.block)}

			messages, s := gameStart(), newSession(playedWorld(t))
			capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

			if dm.fixes != c.fixes {
				t.Errorf("the %s block took %d correction rounds, want %d", c.name, dm.fixes, c.fixes)
			}
		})
	}
}

// The correction round has to say the scene is over, or the DM answers a
// question about bookkeeping by narrating again — into a channel that has
// already closed. It also has to name what was actually refused, so the fix is
// one round away rather than a second guess.
func TestTheCorrectionDemandClosesTheSceneAndNamesTheRefusals(t *testing.T) {
	demand := correctionDemand([]string{"state: nobody-at-all does not exist"})

	for _, want := range []string{"the scene is closed", "write no prose at all", "nobody-at-all"} {
		if !strings.Contains(demand, want) {
			t.Errorf("the demand is missing %q:\n%s", want, demand)
		}
	}
}

// What the correction sends lands like anything else.
func TestTheCorrectionRoundIsApplied(t *testing.T) {
	dm := &model{
		reply:   narrating("entities:\n  nobody-at-all:\n    coins: 5"),
		correct: func(round int, tools []llm.Tool) llm.Response { return narration("entities:\n  catnip:\n    coins: 9") },
	}

	w := playedWorld(t)
	messages, s := gameStart(), newSession(w)
	capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

	pc, _ := w.Character("catnip")
	if pc.Coins != 9 {
		t.Errorf("coins = %d, want the correction to have landed", pc.Coins)
	}
}

// A turn nobody read has no prose to square the engine against, so it does not
// pay for a round asking.
func TestNothingIsCorrectedForATurnThePlayerNeverSaw(t *testing.T) {
	dm := &model{reply: func(round int, tools []llm.Tool) llm.Response { return stalling() }}

	messages, s := gameStart(), newSession(playedWorld(t))
	capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

	if dm.fixes != 0 {
		t.Errorf("an unnarrated turn spent %d correction rounds", dm.fixes)
	}
}

// A correction is still part of the turn, so it leaves the history exactly as it
// found it — the demand and the block that answered it go with the rest.
func TestACorrectionLeavesNothingInTheHistory(t *testing.T) {
	dm := &model{
		reply:   narrating("entities:\n  nobody-at-all:\n    coins: 5"),
		correct: func(round int, tools []llm.Tool) llm.Response { return narration("entities:\n  catnip:\n    coins: 9") },
	}

	messages, s := gameStart(), newSession(playedWorld(t))
	before := len(messages)
	capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

	if len(messages) != before {
		t.Fatalf("the correction round left %d messages behind:\n%+v", len(messages)-before, messages[before:])
	}
}

// The miss the prompt was written to catch: a thing that leaves somebody's
// hands. It is not a purchase, not a gift and not a destruction, so it fell
// through every category — the prose said the phone went, the engine went on
// saying it was carried, and nothing disagreed until the sheet was read out loud.
func TestThePromptNamesAThingLeavingAHand(t *testing.T) {
	guide := strings.ToLower(dmGuide(t))

	for _, want := range []string{"dropped", "thrown away", "left behind"} {
		if !strings.Contains(guide, want) {
			t.Errorf("the prompt never mentions %q, so a discard matches none of its categories", want)
		}
	}
	if !strings.Contains(guide, "the ground is somewhere") {
		t.Error("the prompt does not say that a thing on the floor is somewhere")
	}
}

// The other way a turn records nothing: the thing the prose is about was never
// an engine item, so a move has no source, and "create only what never existed"
// reads as ruling it out — the story has been handling the thing for turns.
// Both readings look wrong and the DM writes neither. The test that decides
// between them is a lookup in the engine's own record, and the prompt has to say
// so in those words.
func TestThePromptSettlesExistenceInTheEnginesRecord(t *testing.T) {
	guide := dmGuide(t)

	for _, want := range []string{
		"Look for it by id in WORLD STATE",
		"however many turns the story has",
		"whatever some narrative property says",
		"the ordinary case every",
	} {
		if !strings.Contains(guide, want) {
			t.Errorf("the prompt is missing %q, so a prop the engine never held has nothing that fits", want)
		}
	}
}

// dmGuide is the DM prompt as the model receives it. These two tests are about
// wording the engine cannot enforce, so they read the prompt itself rather than
// any code path.
func dmGuide(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile("system-prompts/dm.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// A block that arrives on a round that resolved something instead of narrating
// is still the DM saying what is true. Dropping it would lose the fact and leave
// the DM believing it was recorded, which is the one failure the engine exists
// to make impossible.
func TestABlockOnAResolvingRoundStillLands(t *testing.T) {
	dm := &model{reply: func(round int, tools []llm.Tool) llm.Response {
		if round == 0 {
			resolving := stalling()
			resolving.Message.Content = "<state>\nentities:\n  catnip:\n    coins: 7\n</state>"
			return resolving
		}
		return narration("")
	}}

	w := playedWorld(t)
	messages, s := gameStart(), newSession(w)
	capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

	pc, _ := w.Character("catnip")
	if pc.Coins != 7 {
		t.Errorf("coins = %d, want the block from the resolving round to have landed", pc.Coins)
	}
}

// Refusals gather across the whole turn rather than being answered where they
// arose. A refusal from a round that resolved something would otherwise reach
// the end of the turn having been put to nobody, because the round it arrived on
// had no correction round to spend.
func TestARefusalFromAnEarlierRoundIsStillCorrected(t *testing.T) {
	dm := &model{
		reply: func(round int, tools []llm.Tool) llm.Response {
			if round == 0 {
				resolving := stalling()
				resolving.Message.Content = "<state>\nentities:\n  nobody-at-all:\n    coins: 5\n</state>"
				return resolving
			}
			return narration("")
		},
		correct: func(round int, tools []llm.Tool) llm.Response {
			return narration("entities:\n  catnip:\n    coins: 9")
		},
	}

	w := playedWorld(t)
	messages, s := gameStart(), newSession(w)
	capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

	if dm.fixes != 1 {
		t.Fatalf("the turn ran %d correction rounds, want the earlier refusal to have bought one", dm.fixes)
	}
	pc, _ := w.Character("catnip")
	if pc.Coins != 9 {
		t.Errorf("coins = %d, want the correction to have landed", pc.Coins)
	}
}
