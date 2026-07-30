package main

import (
	"io"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"fudge/rules"

	"github.com/memmaker/gollum/llm"
)

// model is a scripted DM. It records the tool set it was offered each round, so
// a test can see the moment the turn stopped asking for resolution and started
// demanding the narration.
//
// The two halves of a turn are scripted separately, because they are asked
// different questions: reply answers the resolution rounds, record answers the
// bookkeeping round that follows the prose. Most tests here are about the first
// half and leave record nil, which means "the prose moved nothing" — the phase
// closes on its first round and the turn ends.
type model struct {
	reply  func(round int, tools []llm.Tool) llm.Response
	record func(round int, tools []llm.Tool) llm.Response
	rounds int
	books  int
	tools  [][]llm.Tool
	seen   [][]llm.Message
}

func (m *model) chat(messages []llm.Message, tools []llm.Tool) (llm.Response, error) {
	m.tools = append(m.tools, tools)
	m.seen = append(m.seen, append([]llm.Message(nil), messages...))
	if bookkeeping(tools) {
		m.books++
		if m.record == nil {
			return nothingToRecord(), nil
		}
		return m.record(m.books-1, tools), nil
	}
	m.rounds++
	return m.reply(m.rounds-1, tools), nil
}

func narrationOffered(tools []llm.Tool) bool {
	for _, tool := range tools {
		if tool.Function.Name == "output_narration" {
			return true
		}
	}
	return false
}

// bookkeeping reports the tool set of the round that runs after the player has
// read the scene. It is told apart by what is missing: every resolution round is
// offered output_narration, and the bookkeeping round never is — there is
// nothing left to say to the player by then.
func bookkeeping(tools []llm.Tool) bool {
	return len(tools) > 0 && !narrationOffered(tools)
}

// nothingToRecord is what a DM answers when its prose moved nothing the engine
// does not already hold: no calls, one line saying so.
func nothingToRecord() llm.Response {
	return llm.Response{Message: llm.Message{Role: llm.RoleAssistant, Content: "Nothing moved that the engine does not already hold."}}
}

// onlyNarration reports the tool set of a turn's last round: the narration, and
// create_location beside it, which is there so a scene that moved somewhere new
// can still be narrated. Nothing that resolves anything.
func onlyNarration(tools []llm.Tool) bool {
	if !narrationOffered(tools) {
		return false
	}
	for _, tool := range tools {
		switch tool.Function.Name {
		case "output_narration", "create_location":
		default:
			return false
		}
	}
	return true
}

// setOp builds the one world_update operation most of these tests need: a
// narrative property set on an entity that already exists. The entity has to
// exist — a set against nothing is refused — so it is `forge`, which testWorld
// opens with.
func setOp(entity, key, value string) string {
	return `[{"op":"set","entity":"` + entity + `","properties":{"` + key + `":"` + value + `"}}]`
}

func narrationCall(update string) llm.Response {
	if update == "" {
		update = "[]"
	}
	return llm.Response{Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
		call("n", "output_narration", `{"prose":"The smith looks up from the anvil.","location":"forge","world_update":`+update+`}`),
	}}}
}

// stalling is a response that resolves something and narrates nothing — the shape
// of a DM that would spend a turn forever if the loop let it.
func stalling() llm.Response {
	return llm.Response{Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
		call("t", "roll_on_table", `{"table":"weather"}`),
	}}}
}

// gameStart is the entire history a session carries between turns: the system
// prompt and the opening briefing, and nothing else ever. The player's line is
// not part of it — the turn takes that as an argument and drops it again when it
// folds the turn into the session.
func gameStart() []llm.Message {
	return []llm.Message{
		{Role: llm.RoleSystem, Content: "system"},
		{Role: llm.RoleUser, Content: "briefing"},
	}
}

const playerLine = "I ask the smith about the amulet."

// testWorld opens with a forge already on the map, because every narration has
// to name a location the engine holds and most tests here are about something
// else entirely.
func testWorld() *rules.World {
	w := &rules.World{
		Characters: map[string]*rules.Character{},
		Locations:  map[string]*rules.Location{},
		Rng:        rand.New(rand.NewSource(1)),
	}
	w.CreateLocation("Forge", "Hot, dark, ringing with hammer work.", "")
	return w
}

// narrated reports whether the turn reached the player. The history cannot
// answer that any more — it keeps nothing — so the archive does, which is where
// a turn goes now.
func narrated(s *session) bool {
	recorded := s.archive.Recent(1)
	return len(recorded) == 1 && recorded[0].Prose != ""
}

// noBackoff runs the retry path at full speed: the pause is there for a network,
// and a test has no network to be polite to.
func noBackoff(t *testing.T) {
	t.Helper()
	previous := requestRetryDelay
	requestRetryDelay = time.Millisecond
	t.Cleanup(func() { requestRetryDelay = previous })
}

func capture(t *testing.T, run func()) string {
	t.Helper()
	stdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		out, _ := io.ReadAll(r)
		done <- string(out)
	}()
	run()
	os.Stdout = stdout
	w.Close()
	return <-done
}

// A DM that keeps resolving does not get to spend the turn doing it. The last
// call of the budget has the engine taken off the table, so the turn still ends
// on a narration rather than on the budget running out.
func TestLastCallIsSpentOnTheNarration(t *testing.T) {
	dm := &model{reply: func(round int, tools []llm.Tool) llm.Response {
		if onlyNarration(tools) {
			return narrationCall(setOp("forge", "anvil", "cooling"))
		}
		return stalling()
	}}

	messages, s := gameStart(), newSession(testWorld())
	capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

	if !narrated(s) {
		t.Fatal("the turn ended without narration")
	}
	if dm.rounds != maxTurnRequests {
		t.Errorf("the turn made %d calls, want the narration demanded on call %d", dm.rounds, maxTurnRequests)
	}
	for i, tools := range dm.tools[:maxTurnRequests-1] {
		if len(tools) < 2 {
			t.Errorf("call %d was denied the toolkit while the turn could still resolve", i+1)
		}
	}
	if !onlyNarration(dm.tools[maxTurnRequests-1]) {
		t.Error("the last call still offered tools other than the narration")
	}

	// The demand is made in words, not only by withdrawing the tools: a DM whose
	// calls silently vanish reads that as a fault and retries them.
	forced := dm.seen[maxTurnRequests-1]
	if !strings.Contains(forced[len(forced)-1].Content, "output_narration") {
		t.Error("the DM was not told why its tools were gone")
	}
}

// The prose the player read is also on disk, where anything watching the game
// from outside the terminal can pick it up. The turn writes it, not the archive:
// the file is the scene on screen right now.
func TestTurnWritesTheNarrationToFile(t *testing.T) {
	t.Chdir(t.TempDir())

	dm := &model{reply: func(round int, tools []llm.Tool) llm.Response {
		return narrationCall(setOp("forge", "anvil", "cooling"))
	}}

	messages, s := gameStart(), newSession(testWorld())
	capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

	written, err := os.ReadFile("narration.txt")
	if err != nil {
		t.Fatalf("the turn narrated but wrote no narration.txt: %v", err)
	}
	if got := string(written); got != "The smith looks up from the anvil." {
		t.Errorf("narration.txt holds %q, want the prose the player was shown", got)
	}
}

// The budget is the whole budget: a turn that has not been narrated by the last
// call is over, and it fails loudly rather than leaving the player waiting on a
// scene that is not coming.
func TestTurnErrorsOutWhenTheBudgetRunsOut(t *testing.T) {
	dm := &model{reply: func(round int, tools []llm.Tool) llm.Response {
		return stalling()
	}}

	messages, s := gameStart(), newSession(testWorld())
	before := len(messages)
	out := capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

	if dm.rounds != maxTurnRequests {
		t.Errorf("the turn made %d calls, want it capped at %d", dm.rounds, maxTurnRequests)
	}
	if !strings.Contains(out, "did not narrate this turn") {
		t.Errorf("the failure was not reported to the player: %q", out)
	}
	if len(messages) != before {
		t.Errorf("the failed turn left %d messages behind, want none", len(messages)-before)
	}
}

// A hiccup with budget left is worth a second try: the call is spent, but the
// turn is not, and the player still gets their scene.
func TestHiccupIsRetriedWhileBudgetRemains(t *testing.T) {
	noBackoff(t)

	var attempts int
	chat := func(messages []llm.Message, tools []llm.Tool) (llm.Response, error) {
		attempts++
		if attempts == 1 {
			return llm.Response{}, io.ErrUnexpectedEOF
		}
		return narrationCall(setOp("forge", "door", "open")), nil
	}

	messages, s := gameStart(), newSession(testWorld())
	capture(t, func() { s.processGameTurn(chat, &messages, playerLine) })

	if !narrated(s) {
		t.Fatal("a turn was lost to a hiccup that cleared on the next call")
	}
}

// Failed calls come out of the same budget as successful ones — a request that
// errored cost the same round trip — so an outage ends the turn instead of being
// retried through it.
func TestFailedCallsSpendTheBudget(t *testing.T) {
	noBackoff(t)

	var attempts int
	chat := func(messages []llm.Message, tools []llm.Tool) (llm.Response, error) {
		attempts++
		return llm.Response{}, io.ErrUnexpectedEOF
	}

	messages, s := gameStart(), newSession(testWorld())
	before := len(messages)
	out := capture(t, func() { s.processGameTurn(chat, &messages, playerLine) })

	if attempts != maxTurnRequests {
		t.Errorf("made %d calls, want the budget of %d", attempts, maxTurnRequests)
	}
	if !strings.Contains(out, "did not narrate this turn") {
		t.Errorf("the failure was not reported to the player: %q", out)
	}
	if len(messages) != before {
		t.Errorf("the failed turn left %d messages behind, want none", len(messages)-before)
	}
}

// The wait between retries grows, then stops growing.
func TestRetryDelayBacksOffToACeiling(t *testing.T) {
	if retryDelay(1) >= retryDelay(2) {
		t.Error("the wait must grow with consecutive failures")
	}
	if got := retryDelay(1000); got != maxRetryDelay {
		t.Errorf("retryDelay(1000) = %s, want the ceiling %s", got, maxRetryDelay)
	}
}

// Prose written outside the call reaches nobody. The DM is told so and asked
// again, and the player is shown the turn only once it arrives through
// output_narration.
func TestProseWrittenOutsideTheCallIsNotATurn(t *testing.T) {
	dm := &model{reply: func(round int, tools []llm.Tool) llm.Response {
		if round == 0 {
			return llm.Response{Message: llm.Message{Role: llm.RoleAssistant, Content: "The smith sets down his hammer."}}
		}
		return narrationCall(setOp("forge", "anvil", "cooling"))
	}}

	messages, s := gameStart(), newSession(testWorld())
	out := capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

	if strings.Contains(out, "sets down his hammer") {
		t.Error("text written outside the call was shown to the player")
	}
	if !strings.Contains(out, "looks up from the anvil") {
		t.Errorf("the narration never reached the player: %q", out)
	}
	// The correction goes back to the DM rather than the same context a second
	// time: a request that has not changed has no reason to answer differently.
	second := dm.seen[1]
	if !strings.Contains(second[len(second)-1].Content, "output_narration") {
		t.Error("the DM was not told its reply went nowhere")
	}
}

// A reply with neither calls nor text costs the turn a call like any other, and
// gets told what was missing.
func TestEmptyRepliesAreAnsweredAndCharged(t *testing.T) {
	dm := &model{reply: func(round int, tools []llm.Tool) llm.Response {
		if round == 0 {
			return llm.Response{Message: llm.Message{Role: llm.RoleAssistant}}
		}
		return narrationCall(setOp("forge", "door", "open"))
	}}

	messages, s := gameStart(), newSession(testWorld())
	capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

	if !narrated(s) {
		t.Fatal("the turn ended unnarrated")
	}
	if dm.rounds != 2 {
		t.Errorf("made %d calls, want the empty reply to have cost one", dm.rounds)
	}
	second := dm.seen[1]
	if !strings.Contains(second[len(second)-1].Content, "empty") {
		t.Error("the DM was not told its reply was empty")
	}
}

// blockOf returns the WORLD STATE message from a request the DM was shown, and
// says where it sat. Position matters as much as presence: the two messages
// above it are what stays byte-identical and cached all session.
func blockOf(t *testing.T, request []llm.Message) (string, int) {
	t.Helper()
	for i, msg := range request {
		if msg.Role == llm.RoleSystem && strings.Contains(msg.Content, "WORLD STATE") {
			return msg.Content, i
		}
	}
	t.Fatalf("no WORLD STATE block in a request of %d messages", len(request))
	return "", -1
}

// The block reaches the DM as a system message pinned directly after the
// briefing, with the turn's live messages below it.
func TestBlockIsPinnedAfterTheBriefing(t *testing.T) {
	dm := &model{reply: resolvingThenNarrating(setOp("forge", "door", "open"))}

	messages, s := gameStart(), newSession(testWorld())
	capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

	for round, request := range dm.seen {
		block, at := blockOf(t, request)
		if at != pinnedMessages {
			t.Errorf("round %d: block sits at index %d, want %d", round+1, at, pinnedMessages)
		}
		if request[0].Content != "system" || request[1].Content != "briefing" {
			t.Errorf("round %d: the cached prefix was disturbed: %q, %q", round+1, request[0].Content, request[1].Content)
		}
		if !strings.Contains(block, "entities:") || !strings.Contains(block, "TIMELINE") {
			t.Errorf("round %d: block is missing a section:\n%s", round+1, block)
		}
	}
}

// Built once and held. A block rebuilt per round would move the cached prefix
// on every request of a turn, and tell the DM nothing the tool results below it
// are not already telling it.
func TestBlockIsStableAcrossTheRoundsOfATurn(t *testing.T) {
	dm := &model{reply: resolvingThenNarrating(setOp("forge", "door", "open"))}

	messages, s := gameStart(), newSession(testWorld())
	capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

	if dm.rounds < 2 {
		t.Fatalf("wanted a turn of at least two resolution rounds, got %d", dm.rounds)
	}
	first, _ := blockOf(t, dm.seen[0])
	for round, request := range dm.seen[1:dm.rounds] {
		if got, _ := blockOf(t, request); got != first {
			t.Errorf("the block changed on round %d of the same turn", round+2)
		}
	}
}

// The bookkeeping round is the exception, and has to be: the turn's own calls
// have moved the roster since the resolution rounds read it, and this is the
// round that names items and characters off it.
func TestTheBookkeepingRoundGetsAFreshBlock(t *testing.T) {
	dm := &model{reply: resolvingThenNarrating(setOp("forge", "patrons", "scattered"))}

	messages, s := gameStart(), newSession(testWorld())
	capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

	if dm.books != 1 {
		t.Fatalf("the turn ran %d bookkeeping rounds, want exactly one", dm.books)
	}
	resolving, _ := blockOf(t, dm.seen[0])
	recording, _ := blockOf(t, dm.seen[len(dm.seen)-1])
	if recording == resolving {
		t.Error("the bookkeeping round read the block the turn started with")
	}
	if !strings.Contains(recording, "patrons: scattered") {
		t.Errorf("the bookkeeping block does not carry what the narration just established:\n%s", recording)
	}
}

// Between turns it is rebuilt, and that is where last turn's report shows up.
func TestBlockIsRebuiltBetweenTurns(t *testing.T) {
	dm := &model{reply: resolvingThenNarrating(setOp("forge", "patrons", "scattered"))}

	messages, s := gameStart(), newSession(testWorld())
	capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })
	firstTurnRounds := len(dm.seen)
	capture(t, func() { s.processGameTurn(dm.chat, &messages, "I look around") })

	opening, _ := blockOf(t, dm.seen[0])
	later, _ := blockOf(t, dm.seen[firstTurnRounds])
	if opening == later {
		t.Error("the block did not change between turns")
	}
	if !strings.Contains(later, "patrons: scattered") {
		t.Errorf("what turn one established is not in turn two's block:\n%s", later)
	}
}

// recall is an ordinary gameplay call, and like every other it is off the table
// on the round that has to end the turn.
func TestRecallIsOfferedDuringPlayButNotOnTheLastRound(t *testing.T) {
	offers := func(tools []llm.Tool, name string) bool {
		for _, tool := range tools {
			if tool.Function.Name == name {
				return true
			}
		}
		return false
	}

	dm := &model{reply: func(round int, tools []llm.Tool) llm.Response {
		if onlyNarration(tools) {
			return narrationCall(setOp("forge", "door", "open"))
		}
		return stalling()
	}}

	messages, s := gameStart(), newSession(testWorld())
	capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

	if !offers(dm.tools[0], "recall") {
		t.Error("recall was not offered during play")
	}
	if offers(dm.tools[maxTurnRequests-1], "recall") {
		t.Error("recall was still on the table on the round that had to narrate")
	}
}

// A recall spends a round like anything else. The budget exists to bound what
// one line of player input can cost, and a lookup is a round trip like any other.
func TestRecallCostsARoundOfTheBudget(t *testing.T) {
	dm := &model{reply: func(round int, tools []llm.Tool) llm.Response {
		if round == 0 {
			return llm.Response{Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
				call("r", "recall", `{"query":"amulet"}`),
			}}}
		}
		return narrationCall(setOp("forge", "door", "open"))
	}}

	messages, s := gameStart(), newSession(testWorld())
	capture(t, func() { s.processGameTurn(dm.chat, &messages, playerLine) })

	if dm.rounds != 2 {
		t.Errorf("the turn made %d calls, want the recall to have cost one", dm.rounds)
	}
	if !narrated(s) {
		t.Fatal("the turn did not survive a recall")
	}
	// The answer was handed back in the round that asked, and went no further.
	answered := dm.seen[1]
	if !strings.Contains(answered[len(answered)-1].Content, "no archived turn") {
		t.Errorf("the recall was not answered in the round that asked:\n%+v", answered[len(answered)-1])
	}
	if len(messages) != 2 {
		t.Errorf("the recall answer outlived its turn: %d messages", len(messages))
	}
}
