package ledger

import (
	"strings"
	"testing"
)

// charSet builds the isCharacter predicate from a list of ids. The ledger asks
// the rules library exactly one question — does this key name a character it
// owns — so a set is the whole of what the tests need to stand in for it.
func charSet(ids ...string) func(string) bool {
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}
	return func(key string) bool { return set[key] }
}

// apply runs a sequence of reports, one per turn starting at turn 1, so a case
// can state its turn markers without arithmetic. Everything a test asserts on
// comes back out of Render: the rendered text is this package's observable
// behaviour, and the shape behind it has to stay free to change.
func apply(l *Ledger, chars []string, updates ...string) []string {
	var notes []string
	for i, update := range updates {
		notes = append(notes, l.Apply(update, i+1, charSet(chars...))...)
	}
	return notes
}

func TestMergeGrammar(t *testing.T) {
	cases := []struct {
		name    string
		chars   []string
		updates []string
		want    string
	}{
		{
			name:    "a keyed field replaces only itself and leaves the rest of the entry standing",
			updates: []string{"vance-kade|mood:sour|disposition:hostile", "vance-kade|mood:cheerful"},
			want:    "vance-kade|mood:cheerful|disposition:hostile @t2",
		},
		{
			name:    "a new field joins an entry without disturbing what is already on it",
			updates: []string{"vance-kade|mood:sour|disposition:hostile", "vance-kade|holding:knife"},
			want:    "vance-kade|mood:sour|disposition:hostile|holding:knife @t2",
		},
		{
			name:    "a bare flag is added once however often it is reported",
			updates: []string{"bar|dim", "bar|dim|crowded", "bar|dim"},
			want:    "bar|dim|crowded @t3",
		},
		{
			name:    "a retraction removes a field",
			updates: []string{"door|locked:yes|heavy", "door|!locked"},
			want:    "door|heavy @t2",
		},
		{
			name:    "a retraction removes a flag",
			updates: []string{"door|locked|heavy", "door|!locked"},
			want:    "door|heavy @t2",
		},
		{
			name:    "retracting something the entry never held changes nothing",
			updates: []string{"door|heavy", "door|!locked"},
			want:    "door|heavy @t2",
		},
		{
			name:    "GONE drops the whole entry and leaves its neighbours alone",
			updates: []string{"door|locked;bar|dim", "door|GONE"},
			want:    "bar|dim @t1",
		},
		{
			name:    "GONE is honoured whatever case the DM writes it in",
			updates: []string{"door|locked;bar|dim", "door|gone"},
			want:    "bar|dim @t1",
		},
		{
			name:    "GONE on an entry that was never there is not an error",
			updates: []string{"bar|dim", "cellar|GONE"},
			want:    "bar|dim @t1",
		},
		{
			name:    "a key reported again after GONE starts a fresh entry",
			updates: []string{"door|locked:yes|heavy", "door|GONE", "door|open"},
			want:    "door|open @t3",
		},
		{
			name:    "only the first colon separates a field from its value",
			updates: []string{"clock|reads:12:45"},
			want:    "clock|reads:12:45 @t1",
		},
		{
			name:    "a flag that later takes a value becomes a field in the same place",
			updates: []string{"guard|alert|at:gate", "guard|alert:very"},
			want:    "guard|alert:very|at:gate @t2",
		},
		{
			name:    "a bare flag never overwrites the value a field of that name already holds",
			updates: []string{"guard|alert:very", "guard|alert"},
			want:    "guard|alert:very @t2",
		},
		{
			name:    "a replaced value keeps the field's original position",
			updates: []string{"bar|a:1|b:2|c:3", "bar|b:9"},
			want:    "bar|a:1|b:9|c:3 @t2",
		},
		{
			name:    "a field re-added after retraction is first-seen again and goes last",
			updates: []string{"bar|a:1|b:2", "bar|!a", "bar|a:3"},
			want:    "bar|b:2|a:3 @t3",
		},
		{
			name:    "entries render in the order they were first seen, not alphabetically",
			updates: []string{"zeb|here;alma|there", "moss|elsewhere"},
			want:    "zeb|here @t1\nalma|there @t1\nmoss|elsewhere @t2",
		},
		{
			name:    "semicolons and newlines both separate entries",
			updates: []string{"bar|dim;cellar|damp\nalley|dark"},
			want:    "bar|dim @t1\ncellar|damp @t1\nalley|dark @t1",
		},
		{
			name:    "whitespace around separators is insignificant and keys are lowercased",
			updates: []string{"  Bar | dim  ;  VANCE-KADE | mood : sour  "},
			want:    "bar|dim @t1\nvance-kade|mood:sour @t1",
		},
		{
			name:    "the same key reported twice in one report merges in reading order",
			updates: []string{"bar|dim;bar|!dim|bright"},
			want:    "bar|bright @t1",
		},
		{
			name:    "a bare key records that the entity exists",
			updates: []string{"cellar"},
			want:    "cellar @t1",
		},
		{
			name:    "an empty report changes nothing",
			updates: []string{"bar|dim", "", "   ", ";;"},
			want:    "bar|dim @t1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := New()
			apply(l, tc.chars, tc.updates...)
			if got := l.Render(); got != tc.want {
				t.Errorf("Render() =\n%s\nwant\n%s", got, tc.want)
			}
		})
	}
}

func TestEmptyLedgerRendersNothing(t *testing.T) {
	if got := New().Render(); got != "" {
		t.Errorf("Render() = %q, want empty", got)
	}
}

// Everything the rules library computes is rendered from the roster every turn.
// A copy of it in the ledger is a second answer to the same question that ages
// out of step with the first, which is the whole defect this package exists to
// close, so those fields never get in.
func TestEngineOwnedFieldsAreStrippedFromCharacterEntries(t *testing.T) {
	owned := []string{
		"wounds", "scratches", "fudge-points", "coins", "carrying", "attributes",
		"skills", "gifts", "faults", "scale", "species", "id", "desc", "penalty",
		"cannot-act",
	}

	for _, field := range owned {
		t.Run(field, func(t *testing.T) {
			l := New()
			l.Apply("vance-kade|mood:sour", 1, charSet("vance-kade"))
			notes := l.Apply("vance-kade|"+field+":something", 2, charSet("vance-kade"))

			if got, want := l.Render(), "vance-kade|mood:sour @t1"; got != want {
				t.Errorf("Render() = %q, want %q", got, want)
			}
			if len(notes) != 1 {
				t.Fatalf("notes = %v, want exactly one discard note", notes)
			}
			if !strings.Contains(notes[0], field) || !strings.Contains(notes[0], "vance-kade") {
				t.Errorf("note %q must name both the entry and the discarded field", notes[0])
			}
		})
	}
}

func TestEngineOwnedNamesAreStrippedAsFlagsAndRetractionsToo(t *testing.T) {
	cases := []struct{ name, update string }{
		{"bare flag", "vance-kade|cannot-act"},
		{"flag sharing a field name", "vance-kade|wounds"},
		{"retraction", "vance-kade|!fudge-points"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := New()
			l.Apply("vance-kade|mood:sour", 1, charSet("vance-kade"))
			notes := l.Apply(tc.update, 2, charSet("vance-kade"))

			if got, want := l.Render(), "vance-kade|mood:sour @t1"; got != want {
				t.Errorf("Render() = %q, want %q", got, want)
			}
			if len(notes) != 1 {
				t.Errorf("notes = %v, want exactly one discard note", notes)
			}
		})
	}
}

func TestStrippingLeavesTheRestOfTheLineIntact(t *testing.T) {
	l := New()
	notes := l.Apply("vance-kade|wounds:scratch|mood:sour|hostile|coins:12", 3, charSet("vance-kade"))

	if got, want := l.Render(), "vance-kade|mood:sour|hostile @t3"; got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
	if len(notes) != 2 {
		t.Errorf("notes = %v, want one per discarded token", notes)
	}
}

// Position used to be the exception that motivated the exception list, because
// Character.Location was written once at creation and never updated. The engine
// maintains it now — narration states the scene, move_character moves everyone
// else — so it is stripped like any other fact the roster states.
func TestPositionIsEngineOwned(t *testing.T) {
	l := New()
	notes := l.Apply("vance-kade|at:alley", 1, charSet("vance-kade"))

	if got := l.Render(); got != "" {
		t.Errorf("Render() = %q, want position discarded", got)
	}
	if len(notes) != 1 {
		t.Errorf("notes = %v, want one discard note", notes)
	}
}

// Only for characters, though. A place has no sheet, so a report about where a
// location sits is the ledger's to keep.
func TestPositionOfANonCharacterSurvives(t *testing.T) {
	l := New()
	l.Apply("bar|at:dock-road", 1, charSet("vance-kade"))

	if got, want := l.Render(), "bar|at:dock-road @t1"; got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

// Stripping keys off the entry, not off the field name: a tavern with a coin
// jar and a wound in its floorboards owns both of those facts, because no
// character sheet is holding them.
func TestNonCharacterEntriesKeepNamesThatWouldBeStrippedFromACharacter(t *testing.T) {
	l := New()
	notes := l.Apply("bar|wounds:gouged-bartop|coins:till-full|cannot-act", 1, charSet("vance-kade"))

	if got, want := l.Render(), "bar|wounds:gouged-bartop|coins:till-full|cannot-act @t1"; got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
	if len(notes) != 0 {
		t.Errorf("notes = %v, want none", notes)
	}
}

// A line whose every token was engine-owned has nothing left to record, and an
// empty entry for a character the roster already renders in full would be the
// duplicate the stripping is there to prevent.
func TestALineOfNothingButEngineOwnedFieldsCreatesNoEntry(t *testing.T) {
	l := New()
	l.Apply("vance-kade|wounds:scratch", 1, charSet("vance-kade"))

	if got := l.Render(); got != "" {
		t.Errorf("Render() = %q, want empty", got)
	}
}

func TestNilPredicateTreatsNothingAsACharacter(t *testing.T) {
	l := New()
	l.Apply("vance-kade|wounds:scratch", 1, nil)

	if got, want := l.Render(), "vance-kade|wounds:scratch @t1"; got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestTurnMarkers(t *testing.T) {
	cases := []struct {
		name    string
		chars   []string
		updates map[int]string
		want    string
	}{
		{
			name:    "an entry nobody touched keeps the turn it was last set",
			updates: map[int]string{4: "bar|dim", 9: "alley|dark"},
			want:    "bar|dim @t4\nalley|dark @t9",
		},
		{
			name:    "any change to an entry moves its marker to the current turn",
			updates: map[int]string{4: "bar|dim", 9: "bar|crowded"},
			want:    "bar|dim|crowded @t9",
		},
		{
			name:    "restating a fact that has not changed still marks the turn it was restated",
			updates: map[int]string{4: "bar|dim", 9: "bar|dim"},
			want:    "bar|dim @t9",
		},
		{
			name:    "a retraction marks the turn as well",
			updates: map[int]string{4: "bar|dim|loud", 9: "bar|!dim"},
			want:    "bar|loud @t9",
		},
		{
			name:  "a line the stripper empties leaves the marker where it was",
			chars: []string{"vance-kade"},
			updates: map[int]string{
				4: "vance-kade|mood:sour",
				9: "vance-kade|wounds:scratch",
			},
			want: "vance-kade|mood:sour @t4",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := New()
			for _, turn := range []int{4, 9} {
				if update, ok := tc.updates[turn]; ok {
					l.Apply(update, turn, charSet(tc.chars...))
				}
			}
			if got := l.Render(); got != tc.want {
				t.Errorf("Render() =\n%s\nwant\n%s", got, tc.want)
			}
		})
	}
}

// Dropping a fact silently is the failure this design exists to prevent, so a
// line the grammar cannot read is kept in whatever form survives rather than
// thrown away, and it must not reach into the entries around it.
func TestMalformedLinesPreserveTheirFactWithoutCorruptingNeighbours(t *testing.T) {
	cases := []struct {
		name      string
		updates   []string
		want      string
		wantNotes int
	}{
		{
			name:      "a line with no key is kept verbatim under a bucket of its own",
			updates:   []string{"|orphan:fact"},
			want:      "unparsed||orphan:fact @t1",
			wantNotes: 1,
		},
		{
			name:      "a keyless line does not disturb the entries reported beside it",
			updates:   []string{"bar|dim;|orphan:fact;alley|dark"},
			want:      "bar|dim @t1\nunparsed||orphan:fact @t1\nalley|dark @t1",
			wantNotes: 1,
		},
		{
			name:      "a token with no field name is kept as written",
			updates:   []string{"bar|:value|dim"},
			want:      "bar|:value|dim @t1",
			wantNotes: 1,
		},
		{
			name:      "a retraction naming nothing is kept rather than acted on",
			updates:   []string{"bar|dim|!"},
			want:      "bar|dim|! @t1",
			wantNotes: 1,
		},
		{
			name:      "empty tokens and empty entries are skipped, since they assert nothing",
			updates:   []string{"bar||dim|;;alley|dark"},
			want:      "bar|dim @t1\nalley|dark @t1",
			wantNotes: 0,
		},
		{
			name:      "a field left without a value keeps the fact that the field was named",
			updates:   []string{"bar|mood:"},
			want:      "bar|mood: @t1",
			wantNotes: 0,
		},
		{
			name:      "junk piles up in the bucket instead of breeding entries",
			updates:   []string{"|first", "|second"},
			want:      "unparsed||first||second @t2",
			wantNotes: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := New()
			notes := apply(l, nil, tc.updates...)
			if got := l.Render(); got != tc.want {
				t.Errorf("Render() =\n%s\nwant\n%s", got, tc.want)
			}
			if len(notes) != tc.wantNotes {
				t.Errorf("notes = %v, want %d", notes, tc.wantNotes)
			}
		})
	}
}

// The rendered block is part of a cached prompt prefix, so a ledger that
// rendered its entries in map order would rewrite the prefix on every turn for
// no reason, quite apart from making the tests above meaningless.
func TestRenderIsDeterministic(t *testing.T) {
	report := "bar|dim|crowded|at:dock-road;vance-kade|mood:sour|disposition:hostile|armed;" +
		"combat|samantha-vs-vance-kade|engaged;alley|dark|wet;dock-road|busy;" +
		"harbour|fogbound;rumour|shipment-tonight;watch|patrolling;innkeeper|nervous"

	first := New()
	first.Apply(report, 1, charSet("vance-kade"))
	want := first.Render()

	for i := 0; i < 100; i++ {
		if got := first.Render(); got != want {
			t.Fatalf("render %d differs:\n%s\nwant\n%s", i, got, want)
		}
		other := New()
		other.Apply(report, 1, charSet("vance-kade"))
		if got := other.Render(); got != want {
			t.Fatalf("ledger %d differs:\n%s\nwant\n%s", i, got, want)
		}
	}
}

// A report captured from a live session, which is the shape this has to survive:
// a character line carrying a wound the engine owns, an entry whose only token is
// a comma-run the grammar treats as one flag, and a keyed relation.
func TestLiveSessionReport(t *testing.T) {
	l := New()
	notes := l.Apply(
		"vance-kade|wounds:scratch;bar|patrons-scattering,space-clearing,table-displaced;combat|samantha-vs-vance-kade|engaged",
		12, charSet("vance-kade", "samantha"))

	want := "bar|patrons-scattering,space-clearing,table-displaced @t12\n" +
		"combat|samantha-vs-vance-kade|engaged @t12"
	if got := l.Render(); got != want {
		t.Errorf("Render() =\n%s\nwant\n%s", got, want)
	}
	if len(notes) != 1 {
		t.Errorf("notes = %v, want the one wound discard", notes)
	}
}
