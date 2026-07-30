package narrative

import (
	"strings"
	"testing"
)

// fakeWorld stands in for the rules library: a fixed set of engine-owned ids.
type fakeWorld struct{ ids []string }

func (f fakeWorld) HasEntity(id string) bool {
	for _, existing := range f.ids {
		if existing == id {
			return true
		}
	}
	return false
}

func (f fakeWorld) EntityIDs() []string { return f.ids }

var world = fakeWorld{ids: []string{"merchant", "player", "iron-key", "tavern"}}

// props reads an entity's properties back as a map, for assertions that do not
// care about ordering.
func props(t *testing.T, s *Store, id string) map[string]any {
	t.Helper()
	e := s.Get(id)
	if e == nil {
		t.Fatalf("no narrative entity %q", id)
	}
	out := map[string]any{}
	for _, p := range e.Props() {
		out[p.Name] = p.Value
	}
	return out
}

func TestSetMergesOntoEngineEntity(t *testing.T) {
	s := New()
	notes := s.Apply([]Operation{
		{Op: OpSet, Entity: "merchant", Properties: map[string]any{"mood": "wary"}},
	}, 3, world)
	if len(notes) != 0 {
		t.Fatalf("unexpected notes: %v", notes)
	}

	// A second set must merge, not replace: the field-merge semantics are what
	// make restating an unchanged fact idempotent, which is why the DM is told
	// to write anything it is unsure of.
	s.Apply([]Operation{
		{Op: OpSet, Entity: "merchant", Properties: map[string]any{"suspicion": 0.8}},
	}, 4, world)

	got := props(t, s, "merchant")
	if got["mood"] != "wary" {
		t.Errorf("mood lost on merge: %v", got)
	}
	if got["suspicion"] != 0.8 {
		t.Errorf("suspicion not recorded: %v", got)
	}
	if turn := s.Get("merchant").Turn(); turn != 4 {
		t.Errorf("turn marker = %d, want 4", turn)
	}
}

func TestSetRefusesUnknownEntity(t *testing.T) {
	s := New()
	notes := s.Apply([]Operation{
		{Op: OpSet, Entity: "merchnat", Properties: map[string]any{"mood": "angry"}},
	}, 1, world)

	if len(notes) != 1 {
		t.Fatalf("want one refusal, got %v", notes)
	}
	// The refusal has to name what does exist, or the next turn is another guess.
	if !strings.Contains(notes[0], "merchant") {
		t.Errorf("refusal does not name what exists: %q", notes[0])
	}
	if s.Get("merchnat") != nil {
		t.Error("a typo minted a ghost entity")
	}
}

func TestReservedPropertiesRefused(t *testing.T) {
	// Both halves of the denylist: fields the engine really renders, and the
	// synonyms a model reaches for when refused the real name. The second group
	// is the whole reason the list outlived the ledger it came from.
	for _, name := range []string{"location", "wounds", "coins", "inventory", "where", "current_room", "physical_position", "hp"} {
		t.Run(name, func(t *testing.T) {
			s := New()
			notes := s.Apply([]Operation{
				{Op: OpSet, Entity: "merchant", Properties: map[string]any{name: "somewhere", "mood": "calm"}},
			}, 1, world)

			if len(notes) != 1 {
				t.Fatalf("want one refusal for %q, got %v", name, notes)
			}
			got := props(t, s, "merchant")
			if _, held := got[name]; held {
				t.Errorf("reserved property %q was recorded", name)
			}
			// The rest of the operation still lands: one bad property must not
			// lose the good ones beside it.
			if got["mood"] != "calm" {
				t.Errorf("good property lost alongside a refused one: %v", got)
			}
		})
	}
}

func TestCreateNarrativeEntity(t *testing.T) {
	s := New()
	notes := s.Apply([]Operation{
		{Op: OpCreate, Entity: "rumor-007", Properties: map[string]any{
			"type":        "rumor",
			"content":     "the count murdered his brother",
			"believed_by": []any{"merchant"},
		}},
	}, 12, world)
	if len(notes) != 0 {
		t.Fatalf("unexpected notes: %v", notes)
	}

	e := s.Get("rumor-007")
	if e == nil {
		t.Fatal("entity not created")
	}
	if e.Type != "rumor" {
		t.Errorf("type = %q, want rumor", e.Type)
	}
	// type is consumed as the entity's kind, not stored again as a property —
	// one fact, one home, the rule the whole design turns on.
	if got := props(t, s, "rumor-007"); got["type"] != nil {
		t.Errorf("type duplicated into properties: %v", got)
	}
}

func TestCreateRefusals(t *testing.T) {
	tests := []struct {
		name string
		op   Operation
		want string
	}{
		{
			// The boundary that matters most: an engine entity is never minted
			// here, because the rules library computes its defaults.
			name: "engine entity",
			op:   Operation{Op: OpCreate, Entity: "merchant", Properties: map[string]any{"type": "character"}},
			want: "already exists in the engine",
		},
		{
			name: "no type",
			op:   Operation{Op: OpCreate, Entity: "rumor-1", Properties: map[string]any{"content": "something"}},
			want: "needs a type",
		},
		{
			name: "no properties",
			op:   Operation{Op: OpCreate, Entity: "rumor-1"},
			want: "needs properties",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := New()
			notes := s.Apply([]Operation{tc.op}, 1, world)
			if len(notes) != 1 || !strings.Contains(notes[0], tc.want) {
				t.Fatalf("notes = %v, want one containing %q", notes, tc.want)
			}
		})
	}
}

func TestDeleteOnlyReachesNarrativeEntities(t *testing.T) {
	s := New()
	s.Apply([]Operation{
		{Op: OpCreate, Entity: "rumor-007", Properties: map[string]any{"type": "rumor", "content": "x"}},
	}, 1, world)

	if notes := s.Apply([]Operation{{Op: OpDelete, Entity: "rumor-007"}}, 2, world); len(notes) != 0 {
		t.Fatalf("unexpected notes deleting a narrative entity: %v", notes)
	}
	if s.Get("rumor-007") != nil {
		t.Error("narrative entity survived delete")
	}
	if len(s.IDs()) != 0 {
		t.Errorf("deleted id still listed: %v", s.IDs())
	}

	notes := s.Apply([]Operation{{Op: OpDelete, Entity: "merchant"}}, 3, world)
	if len(notes) != 1 || !strings.Contains(notes[0], "cannot be deleted") {
		t.Fatalf("engine entity delete not refused: %v", notes)
	}
}

func TestSetNullRemovesProperty(t *testing.T) {
	s := New()
	s.Apply([]Operation{
		{Op: OpSet, Entity: "merchant", Properties: map[string]any{"mood": "angry", "debt": 40}},
	}, 1, world)
	s.Apply([]Operation{
		{Op: OpSet, Entity: "merchant", Properties: map[string]any{"mood": nil}},
	}, 2, world)

	got := props(t, s, "merchant")
	if _, held := got["mood"]; held {
		t.Errorf("null did not retract the property: %v", got)
	}
	if got["debt"] != 40 {
		t.Errorf("retraction took a neighbouring property with it: %v", got)
	}
}

func TestPropertyOrderIsStable(t *testing.T) {
	// Rendering in Go's randomised map order would rewrite the cached prompt
	// prefix every turn for nothing, so first-seen order is the contract.
	s := New()
	s.Apply([]Operation{
		{Op: OpSet, Entity: "merchant", Properties: map[string]any{"aaa": 1}},
	}, 1, world)
	s.Apply([]Operation{
		{Op: OpSet, Entity: "merchant", Properties: map[string]any{"zzz": 2}},
	}, 2, world)
	s.Apply([]Operation{
		{Op: OpSet, Entity: "merchant", Properties: map[string]any{"mmm": 3}},
	}, 3, world)

	var names []string
	for _, p := range s.Get("merchant").Props() {
		names = append(names, p.Name)
	}
	want := []string{"aaa", "zzz", "mmm"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("property order = %v, want %v", names, want)
		}
	}

	// Re-setting an existing property must not move it to the end: the DM reads
	// these lines back and a line that jumps around is a line it has to re-find.
	s.Apply([]Operation{
		{Op: OpSet, Entity: "merchant", Properties: map[string]any{"aaa": 99}},
	}, 4, world)
	if first := s.Get("merchant").Props()[0]; first.Name != "aaa" || first.Value != 99 {
		t.Errorf("re-set moved or lost the property: %+v", first)
	}
}

func TestBatchContinuesPastARefusal(t *testing.T) {
	// The turn has already happened and the player has already read the prose.
	// One bad operation must not lose the good ones beside it.
	s := New()
	notes := s.Apply([]Operation{
		{Op: OpSet, Entity: "merchant", Properties: map[string]any{"mood": "furious"}},
		{Op: OpSet, Entity: "nobody", Properties: map[string]any{"mood": "lost"}},
		{Op: OpSet, Entity: "iron-key", Properties: map[string]any{"significance": "opens the crypt"}},
	}, 5, world)

	if len(notes) != 1 {
		t.Fatalf("want exactly one refusal, got %v", notes)
	}
	if props(t, s, "merchant")["mood"] != "furious" {
		t.Error("operation before the refusal was lost")
	}
	if props(t, s, "iron-key")["significance"] != "opens the crypt" {
		t.Error("operation after the refusal was lost")
	}
}

func TestUnknownOpIsReported(t *testing.T) {
	s := New()
	notes := s.Apply([]Operation{{Op: "move", Entity: "iron-key", Properties: map[string]any{"to": "player"}}}, 1, world)
	if len(notes) != 1 || !strings.Contains(notes[0], "unknown op") {
		t.Fatalf("notes = %v, want an unknown-op refusal", notes)
	}
	if s.Get("iron-key") != nil {
		t.Error("an unknown op still wrote to the store")
	}
}
