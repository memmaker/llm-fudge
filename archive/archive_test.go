package archive

import (
	"strings"
	"testing"
)

// session builds an archive with a few turns worth looking things up in. The
// turns are deliberately unlike each other: one carries prose worth quoting, one
// failed before it was narrated, and one mentions a word that appears nowhere
// else, so a lookup that matches it cannot be matching something incidental.
func session() *Archive {
	a := New()
	a.Record(Turn{
		Number:      1,
		PlayerInput: "I order a drink and listen to the room",
		Events:      []string{"roll_on_table weather|clear"},
		Update:      "old-bartender|disposition:wary; bar|crowded",
		Prose:       "The bartender pours without looking at you.",
	})
	a.Record(Turn{
		Number:      2,
		PlayerInput: "I ask the bartender about the Kade job",
		Events:      []string{"roll_on_table npc-reaction|friendly"},
		Update:      "old-bartender|disposition:friendly; job|giver:kade|reward:5k",
		Prose:       "He wipes the same glass a long while. 'Kade pays. Kade also buries.'",
	})
	a.Record(Turn{
		Number:      3,
		PlayerInput: "I climb the fire escape",
		Update:      "samantha|at:rooftop",
		Prose:       "The rungs are slick with condenser runoff.",
	})
	return a
}

func TestRecordKeepsEveryPart(t *testing.T) {
	a := session()
	if a.Len() != 3 {
		t.Fatalf("Len = %d, want 3", a.Len())
	}

	got := a.Lookup(Query{From: 2, IncludeProse: true})
	for _, want := range []string{
		"2",
		"I ask the bartender about the Kade job",
		"roll_on_table npc-reaction|friendly",
		"job|giver:kade|reward:5k",
		"Kade also buries",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("lookup of turn 2 lost %q:\n%s", want, got)
		}
	}
}

// The DM reaches for a keyword when it only half remembers something, so the
// search has to cover everywhere a half-remembered thing could have been
// written down — what the player typed, what was recorded, and what was said.
func TestKeywordSearchesEveryField(t *testing.T) {
	for _, c := range []struct {
		what    string
		keyword string
		want    string
	}{
		{"player input", "fire escape", "I climb the fire escape"},
		{"recorded changes", "reward:5k", "I ask the bartender"},
		{"prose", "condenser runoff", "I climb the fire escape"},
	} {
		got := session().Lookup(Query{Keyword: c.keyword, IncludeProse: true})
		if !strings.Contains(got, c.want) {
			t.Errorf("keyword %q did not match in %s:\n%s", c.keyword, c.what, got)
		}
	}
}

func TestKeywordIsCaseInsensitive(t *testing.T) {
	got := session().Lookup(Query{Keyword: "KADE JOB"})
	if !strings.Contains(got, "I ask the bartender about the Kade job") {
		t.Errorf("uppercase keyword found nothing:\n%s", got)
	}
}

// Recency ordering is what makes an unbounded lookup useful: the turn the DM
// most likely means is the one it just played, not the one from an hour ago.
func TestKeywordReturnsMostRecentFirst(t *testing.T) {
	got := session().Lookup(Query{Keyword: "bartender"})
	second := strings.Index(got, "I ask the bartender")
	first := strings.Index(got, "I order a drink")
	if second < 0 || first < 0 {
		t.Fatalf("expected both bartender turns:\n%s", got)
	}
	if second > first {
		t.Errorf("older turn came first:\n%s", got)
	}
}

func TestLookupByTurnAndRange(t *testing.T) {
	one := session().Lookup(Query{From: 3})
	if !strings.Contains(one, "I climb the fire escape") {
		t.Errorf("single turn lookup missed:\n%s", one)
	}
	if strings.Contains(one, "I order a drink") {
		t.Errorf("single turn lookup returned neighbours:\n%s", one)
	}

	span := session().Lookup(Query{From: 1, To: 2})
	for _, want := range []string{"I order a drink", "I ask the bartender"} {
		if !strings.Contains(span, want) {
			t.Errorf("range lookup missed %q:\n%s", want, span)
		}
	}
	if strings.Contains(span, "fire escape") {
		t.Errorf("range lookup ran past its end:\n%s", span)
	}
}

// Prose dominates the size of any result, which is the whole reason it is asked
// for rather than given. A lookup that did not ask must not pay for it.
func TestProseIsOptIn(t *testing.T) {
	without := session().Lookup(Query{From: 2})
	if strings.Contains(without, "Kade also buries") {
		t.Errorf("prose returned without being asked for:\n%s", without)
	}
	with := session().Lookup(Query{From: 2, IncludeProse: true})
	if !strings.Contains(with, "Kade also buries") {
		t.Errorf("prose withheld when asked for:\n%s", with)
	}
}

func TestLimitCapsTurns(t *testing.T) {
	got := session().Lookup(Query{Keyword: "I", Limit: 1})
	if n := strings.Count(got, "turn "); n != 1 {
		t.Errorf("limit 1 returned %d turns:\n%s", n, got)
	}
}

// wordy is an archive of 200 near-identical turns, each carrying a paragraph of
// narration — a keyword that matches all of them is the shape of lookup that
// could flood the context this design exists to keep small.
func wordy() *Archive {
	a := New()
	for i := 1; i <= 200; i++ {
		a.Record(Turn{
			Number:      i,
			PlayerInput: "I search the ruins again",
			Prose:       strings.Repeat("A long paragraph of narration. ", 40),
		})
	}
	return a
}

// A caller who names no limit still gets a bounded answer. This is the first of
// two independent ceilings, and the one that usually binds.
func TestLookupIsBoundedWithoutALimit(t *testing.T) {
	got := wordy().Lookup(Query{Keyword: "ruins", IncludeProse: true})
	if len([]rune(got)) > maxLookupRunes+200 {
		t.Errorf("unbounded lookup returned %d runes", len([]rune(got)))
	}
	if strings.Count(got, "turn ") > defaultLimit {
		t.Errorf("unbounded lookup returned more than the default %d turns:\n%s", defaultLimit, got)
	}
}

// The second ceiling catches what the first cannot: few enough turns to pass the
// limit, each long enough that together they would still swamp the request. It
// has to announce itself, because a turn cut off mid-sentence otherwise reads as
// the whole of what was recorded.
func TestOversizedLookupIsTruncatedVisibly(t *testing.T) {
	got := wordy().Lookup(Query{Keyword: "ruins", IncludeProse: true, Limit: 10})
	if len([]rune(got)) > maxLookupRunes+200 {
		t.Errorf("oversized lookup returned %d runes", len([]rune(got)))
	}
	if !strings.Contains(got, "truncated") {
		t.Errorf("truncation was silent:\n%s", got[:200])
	}
}

// An ambiguous empty answer is worse than no answer: it reads as permission to
// invent the history that was not found.
func TestEmptyResultSaysSoPlainly(t *testing.T) {
	got := session().Lookup(Query{Keyword: "dragon"})
	if !strings.Contains(strings.ToLower(got), "no archived turn") {
		t.Errorf("empty result is not unmistakable:\n%q", got)
	}
	if strings.Contains(got, "turn 1") {
		t.Errorf("empty result returned turns anyway:\n%s", got)
	}
}

// A turn that died before it was narrated is part of the record — it explains a
// gap — but the player never saw it, so it must not reach the timeline and
// become something later turns narrate around.
func TestUnnarratedTurnsAreRecordedButNotRecent(t *testing.T) {
	a := session()
	a.Record(Turn{Number: 4, PlayerInput: "I do something the DM never resolved", Unnarrated: true})

	if a.Len() != 4 {
		t.Errorf("Len = %d, want the failed turn recorded", a.Len())
	}
	for _, turn := range a.Recent(10) {
		if turn.Number == 4 {
			t.Error("an unnarrated turn reached Recent")
		}
	}
	if got := a.Lookup(Query{From: 4}); !strings.Contains(got, "never resolved") {
		t.Errorf("an unnarrated turn is still part of the record:\n%s", got)
	}
}

// Recent feeds the timeline, which reads oldest first because it is a
// chronology: the DM follows it forward to see how the state came about.
func TestRecentReturnsOldestFirst(t *testing.T) {
	got := session().Recent(2)
	if len(got) != 2 {
		t.Fatalf("Recent(2) returned %d turns", len(got))
	}
	if got[0].Number != 2 || got[1].Number != 3 {
		t.Errorf("Recent(2) = turns %d,%d; want 2,3 oldest first", got[0].Number, got[1].Number)
	}
}

func TestRecentHandlesFewerTurnsThanAsked(t *testing.T) {
	if got := session().Recent(50); len(got) != 3 {
		t.Errorf("Recent(50) returned %d turns, want all 3", len(got))
	}
	if got := New().Recent(10); len(got) != 0 {
		t.Errorf("Recent on an empty archive returned %d turns", len(got))
	}
}

func TestRenderingIsDeterministic(t *testing.T) {
	q := Query{Keyword: "bartender", IncludeProse: true}
	first := session().Lookup(q)
	for i := 0; i < 5; i++ {
		if got := session().Lookup(q); got != first {
			t.Fatalf("lookup differs between runs:\n%s\n---\n%s", first, got)
		}
	}
}

// Somebody who names a range named the turns they want. Handing back three of
// fifteen while saying nothing about the rest is the quiet answer this package
// is otherwise careful to avoid.
func TestRangeLookupReturnsTheWholeRange(t *testing.T) {
	a := New()
	for i := 1; i <= 15; i++ {
		a.Record(Turn{Number: i, PlayerInput: "I press on"})
	}

	got := a.Lookup(Query{From: 5, To: 15})
	if n := strings.Count(got, "turn "); n != 11 {
		t.Errorf("range 5-15 returned %d turns, want 11:\n%s", n, got)
	}
}

// Whatever the caller asks for, a lookup stays bounded. The rune cap would stop
// a runaway eventually, but only mid-sentence.
func TestLookupTurnCountIsClamped(t *testing.T) {
	a := New()
	for i := 1; i <= 200; i++ {
		a.Record(Turn{Number: i, PlayerInput: "I press on"})
	}

	for _, q := range []Query{
		{Keyword: "press", Limit: 1000},
		{From: 1, To: 200},
		{From: 1, To: 200, Limit: 500},
	} {
		if n := strings.Count(a.Lookup(q), "turn "); n > maxLookupTurns {
			t.Errorf("%+v returned %d turns, want at most %d", q, n, maxLookupTurns)
		}
	}
}

// The dump is read by a person in a file, not by the DM in a request, so none of
// the caps that keep a lookup small apply to it.
func TestDumpIsCompleteAndUntruncated(t *testing.T) {
	a := New()
	for i := 1; i <= 60; i++ {
		a.Record(Turn{
			Number:      i,
			PlayerInput: "I search the ruins again",
			Prose:       strings.Repeat("A long paragraph of narration. ", 20),
		})
	}

	got := a.Dump()
	if n := strings.Count(got, "turn "); n != 60 {
		t.Errorf("dump holds %d turns, want all 60", n)
	}
	if strings.Contains(got, "truncated") {
		t.Error("the dump was truncated")
	}
	if !strings.Contains(got, "A long paragraph") {
		t.Error("the dump dropped the prose")
	}
}

func TestDumpOfAnEmptyArchiveIsNotBlank(t *testing.T) {
	if strings.TrimSpace(New().Dump()) == "" {
		t.Error("an empty dump renders as nothing at all")
	}
}
