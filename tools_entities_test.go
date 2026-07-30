package main

import (
	"strings"
	"testing"
)

// The archive is an entity index, and this is what builds the half of it that
// comes from tool calls: if a call's ids are not collected here, every turn it
// appears in becomes unfindable by the entity it was about, which is the
// question recall is for. The other half comes from the state block, where the
// applier already holds the ids and needs no list of argument names at all.
func TestToolEntitiesCollectsIDsFromArguments(t *testing.T) {
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
			name: "an attack names attacker and defender",
			tool: "opposed_attack",
			args: `{"attacker_id":"dust-bandit","defender_id":"kyle-katarn"}`,
			want: []string{"dust-bandit", "kyle-katarn"},
		},
		{
			// The id the DM minted is the id the archive indexes, which is why it
			// is an argument now rather than something read off a reply.
			name: "a new character names the id it was given and where it stands",
			tool: "create_character",
			args: `{"id":"dust-bandit","name":"Dust Bandit","species":"human","location":"saloon"}`,
			want: []string{"dust-bandit", "saloon"},
		},
		{
			name: "a reload names whose weapon it is",
			tool: "reload_weapon",
			args: `{"character_id":"kyle-katarn","weapon":"Peacemaker"}`,
			want: []string{"kyle-katarn"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := toolEntities(call("x", c.tool, c.args))
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

// Prose is not an id. `weapon` is what a thing is called and `name` is what it
// is known as, and indexing either would fill the index with words that address
// nothing — which is the document search this replaced.
func TestToolEntitiesIgnoresProseArguments(t *testing.T) {
	got := toolEntities(call("x", "create_character", `{"id":"barkeep","name":"Ezra Coles","species":"human"}`))

	if len(got) != 1 || got[0] != "barkeep" {
		t.Errorf("collected %v, want just the id", got)
	}
}

// One entity named twice in one call is one entity. The archive dedupes again
// across a turn's events, but a duplicate here would make the per-event list
// misleading on its own.
func TestToolEntitiesDedupes(t *testing.T) {
	got := toolEntities(call("x", "create_character", `{"id":"mira","character_id":"mira"}`))

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

// Ids are matched literally against what the engine holds, so they are folded to
// lower case and trimmed — the same normalisation the narrative store does.
func TestToolEntitiesNormalises(t *testing.T) {
	got := toolEntities(call("x", "unopposed_check", `{"actor_id":"  Kyle-Katarn "}`))

	if len(got) != 1 || got[0] != "kyle-katarn" {
		t.Errorf("collected %v, want [kyle-katarn]", got)
	}
}

// A record thinner than it could be is a far smaller problem than a turn that
// fails over its own index.
func TestToolEntitiesSurvivesMalformedArguments(t *testing.T) {
	for _, args := range []string{
		``,
		`not json at all`,
		`{"actor_id":`,
		`{"actor_id":42}`,
	} {
		t.Run(strings.TrimSpace(args), func(t *testing.T) {
			// The assertion is that it returns at all.
			toolEntities(call("x", "unopposed_check", args))
		})
	}
}
