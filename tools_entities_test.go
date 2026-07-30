package main

import (
	"strings"
	"testing"
)

// The archive is an entity index now, and this is the whole of what builds it:
// if a call's ids are not collected here, every turn it appears in becomes
// unfindable by the entity it was about, which is the question recall is for.
func TestCallEntitiesCollectsIDsFromArguments(t *testing.T) {
	for _, c := range []struct {
		name string
		tool string
		args string
		want []string
	}{
		{
			name: "a check names its actor",
			tool: "unopposed_check",
			args: `{"actor_id":"kyle-katarn","trait_type":"skill","trait_slug":"shootin","difficulty":"average"}`,
			want: []string{"kyle-katarn"},
		},
		{
			name: "an opposed check names both sides",
			tool: "opposed_check",
			args: `{"actor_id":"kyle-katarn","opponent_id":"dust-bandit"}`,
			want: []string{"kyle-katarn", "dust-bandit"},
		},
		{
			// The call the whole identity change was for: what moved, and both
			// ends of the move.
			name: "a move names the item and both holders",
			tool: "move_item",
			args: `{"item":"iron-key","from_character":"mira","to_location":"cellar","location_description":"on the top shelf"}`,
			want: []string{"iron-key", "mira", "cellar"},
		},
		{
			name: "an attack names attacker and defender",
			tool: "opposed_attack",
			args: `{"attacker_id":"dust-bandit","defender_id":"kyle-katarn"}`,
			want: []string{"dust-bandit", "kyle-katarn"},
		},
		{
			name: "a narration names its location",
			tool: "output_narration",
			args: `{"prose":"The room empties.","location":"saloon"}`,
			want: []string{"saloon"},
		},
		{
			// The ids the DM itself chose to talk about, which are the most worth
			// indexing of the lot.
			name: "world_update names every entity it writes to",
			tool: "output_narration",
			args: `{"prose":"They back away.","location":"saloon","world_update":[` +
				`{"op":"set","entity":"mira","properties":{"mood":"wary"}},` +
				`{"op":"create","entity":"rumor-crypt","properties":{"type":"rumor"}}]}`,
			want: []string{"saloon", "mira", "rumor-crypt"},
		},
		{
			name: "a new location names its parent",
			tool: "create_location",
			args: `{"name":"Cellar","description":"Damp.","parent":"saloon"}`,
			want: []string{"saloon"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := callEntities(call("x", c.tool, c.args))
			if len(got) != len(c.want) {
				t.Fatalf("collected %v, want %v", got, c.want)
			}
			for _, want := range c.want {
				var found bool
				for _, id := range got {
					if id == want {
						found = true
					}
				}
				if !found {
					t.Errorf("%q is missing from %v", want, got)
				}
			}
		})
	}
}

// Prose is not an id. `where` describes a shelf, `name` is what a thing is
// called, and indexing either would fill the index with words that address
// nothing — which is the document search this replaced.
func TestCallEntitiesIgnoresProseArguments(t *testing.T) {
	got := callEntities(call("x", "add_item", `{"name":"Brass Lantern","description":"Dented.","where":"on the bar","location":"saloon"}`))

	if len(got) != 1 || got[0] != "saloon" {
		t.Errorf("collected %v, want just the location id", got)
	}
}

// One entity named twice in one call is one entity. The archive dedupes again
// across a turn's events, but a duplicate here would make the per-event list
// misleading on its own.
func TestCallEntitiesDedupes(t *testing.T) {
	got := callEntities(call("x", "move_item", `{"item":"iron-key","from_character":"mira","to_character":"mira"}`))

	var mira int
	for _, id := range got {
		if id == "mira" {
			mira++
		}
	}
	if mira != 1 {
		t.Errorf("mira appears %d times in %v", mira, got)
	}
}

// Ids are matched literally against what the engine minted, so they are folded
// to lower case and trimmed — the same normalisation the narrative store does.
func TestCallEntitiesNormalises(t *testing.T) {
	got := callEntities(call("x", "unopposed_check", `{"actor_id":"  Kyle-Katarn "}`))

	if len(got) != 1 || got[0] != "kyle-katarn" {
		t.Errorf("collected %v, want [kyle-katarn]", got)
	}
}

// This runs while a turn is being recorded, after the player has already read
// the prose. A record thinner than it could be is a far smaller problem than a
// turn that fails over its own bookkeeping.
func TestCallEntitiesSurvivesMalformedArguments(t *testing.T) {
	for _, args := range []string{
		``,
		`not json at all`,
		`{"actor_id":`,
		`{"actor_id":42}`,
		`{"world_update":"a string where the array should be"}`,
		`{"world_update":[null,{"entity":7},{"op":"set"}]}`,
	} {
		t.Run(strings.TrimSpace(args), func(t *testing.T) {
			// The assertion is that it returns at all.
			callEntities(call("x", "output_narration", args))
		})
	}
}
