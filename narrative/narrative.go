// Package narrative holds the facts about the world that only the DM's prose
// knows — a merchant's mood, a rumour's content, who suspects whom — as a
// property map hanging off each entity, merged as the DM reports changes and
// rendered back to it inside that entity's own record.
//
// It replaces the world ledger, which kept the same facts in a namespace of its
// own alongside the engine's. Two namespaces meant one entity could be spoken
// about in two places, and the ledger spent its whole design fighting the
// consequences: a denylist of engine field names, stripped on ingest, so a fact
// the engine already owned could not acquire a second and contradictory home
// here. That denylist survives below, but it is no longer the only defence.
//
// The defence that matters now is structural. Properties written through this
// package can only ever land inside an entity's narrative map — there is no path
// syntax that reaches a top-level engine field, so `location` cannot be
// overwritten by accident or by a model that misread its instructions. What
// structure alone cannot stop is a synonym: nothing about a nested map prevents
// the DM writing `narrative.current_room` and, three hundred turns later,
// reading it back as though it were true. That is what the denylist is for, and
// why both exist. Each catches what the other misses.
package narrative

import (
	"fmt"
	"sort"
	"strings"
)

// reserved names the properties the DM may never write, whatever it calls them.
//
// The first group is the engine's own field names, inherited from the ledger
// that came before: the rules library computes these, renders them every turn,
// and a second copy here would be a second answer ageing out of step with the
// first.
//
// The second group is the part the ledger did not have, and the reason a
// denylist is still worth keeping now that the namespace is structurally
// separate. A model does not need permission to invent a word. Refused
// `location`, it will reach for `where`, then `current_room`, then
// `physical_position` — each one a fact that looks authoritative, is written by
// nobody who can maintain it, and is eventually read back as truth. These are
// not fields the engine uses. They are fields the engine is refusing to let
// become synonyms for the ones it does.
var reserved = map[string]bool{
	// The engine's own, as the ledger held them.
	"at": true, "wounds": true, "scratches": true, "fudge-points": true,
	"coins": true, "carrying": true, "attributes": true, "skills": true,
	"gifts": true, "faults": true, "scale": true, "species": true,
	"id": true, "desc": true, "penalty": true, "cannot-act": true,
	"location": true, "inventory": true, "equipment": true, "type": true,

	// Synonyms for concepts the engine owns. Reserved so they cannot become
	// second answers to a question the engine is already answering.
	"hp": true, "health": true, "where": true, "current_room": true,
	"current-room": true, "position": true, "physical_position": true,
	"physical-position": true, "room": true, "contains": true,
	"contents": true, "container": true, "owner": true, "owns": true,
	"exits": true, "held_by": true, "held-by": true, "worn": true,
	"wielding": true, "items": true, "fudge_points": true,
}

// Op names what an operation does. Three is the whole vocabulary: an entity
// comes into being, its properties are set, or it goes away. There is no
// separate move — position belongs to the engine and is not reachable from
// here — and no separate property removal, because setting a property to null
// says the same thing with one fewer word to learn.
type Op string

const (
	OpCreate Op = "create"
	OpSet    Op = "set"
	OpDelete Op = "delete"
)

// Operation is one instruction in a world_update call.
//
// It arrives as JSON in the tool call's arguments rather than as text to be
// parsed, which is the one structural difference from the ledger's line grammar
// that mattered: a malformed shape is now caught by the provider's own schema
// validation before it ever reaches this package, where the ledger had to parse
// whatever prose arrived and salvage what it could.
type Operation struct {
	Op         Op             `json:"op"`
	Entity     string         `json:"entity"`
	Properties map[string]any `json:"properties,omitempty"`
}

// Entity is one entity's narrative properties.
//
// Property names are held in a slice beside the map because the order the DM
// first mentioned things in is the order that reads best back to it, and because
// rendering in Go's randomised map order would rewrite the cached prompt prefix
// every turn for nothing.
type Entity struct {
	ID string
	// Type is set only for entities that exist nowhere else — a rumour, a
	// faction's standing, a prophecy. An entity the rules library owns takes its
	// type from the engine, and this stays empty.
	Type  string
	names []string
	props map[string]any
	turn  int
}

// Turn reports when this entity's narrative was last written to, so a fact can
// be traced back to the turn that produced it and recalled from the archive.
func (e *Entity) Turn() int { return e.turn }

// Props returns the entity's properties in the order they were first set.
func (e *Entity) Props() []Prop {
	out := make([]Prop, 0, len(e.names))
	for _, name := range e.names {
		out = append(out, Prop{Name: name, Value: e.props[name]})
	}
	return out
}

// Prop is one narrative property, rendered in the order it was first set.
type Prop struct {
	Name  string
	Value any
}

// Store holds the narrative properties of every entity that has any. The zero
// value is not usable; call New.
type Store struct {
	ids      []string
	entities map[string]*Entity
}

func New() *Store {
	return &Store{entities: map[string]*Entity{}}
}

// Get returns an entity's narrative properties, or nil if it has none. The flat
// renderer asks this of every engine entity in turn, which is why a miss is
// nil rather than an error: most entities have no narrative at any given moment
// and that is not a condition worth reporting.
func (s *Store) Get(id string) *Entity {
	return s.entities[id]
}

// IDs lists every entity holding narrative properties, in the order they were
// first written to.
func (s *Store) IDs() []string {
	return append([]string(nil), s.ids...)
}

// World is what this package needs to know about the engine's own entities:
// whether an id is already taken, and what exists if it is not. It is an
// interface rather than a *rules.World so that the ownership boundary runs both
// ways — this package cannot reach into the engine's state even by accident,
// and cannot be tempted to write to it.
type World interface {
	HasEntity(id string) bool
	EntityIDs() []string
}

// Apply runs a batch of operations, recording turn against every entity it
// touches, and returns a note for each thing it refused.
//
// Nothing here returns an error. The turn has already happened, the prose has
// already been shown to the player, and there is nobody left to ask for a
// correction — so a refused operation is reported and the rest of the batch
// still runs. A world_update that is wrong in one line should not lose the four
// lines that were right.
func (s *Store) Apply(ops []Operation, turn int, w World) []string {
	var notes []string
	for _, op := range ops {
		notes = append(notes, s.apply(op, turn, w)...)
	}
	return notes
}

func (s *Store) apply(op Operation, turn int, w World) []string {
	id := strings.ToLower(strings.TrimSpace(op.Entity))
	if id == "" {
		return []string{fmt.Sprintf("world_update: %q operation names no entity, skipped", op.Op)}
	}

	switch op.Op {
	case OpCreate:
		return s.create(id, op, turn, w)
	case OpSet:
		return s.set(id, op, turn, w)
	case OpDelete:
		return s.remove(id, op, w)
	default:
		return []string{fmt.Sprintf("world_update: %s: unknown op %q — use create, set or delete", id, op.Op)}
	}
}

// create mints a narrative-only entity: a rumour, a faction's standing, a debt
// nobody has written down. It is the one operation that brings something into
// existence here, and it is deliberately unable to bring a character, an item or
// a location into existence — those are minted by their own typed tools so the
// rules library computes their defaults, rather than a model inventing a number
// with no relationship to the wound tiers or the conservation rules it will be
// measured against later.
func (s *Store) create(id string, op Operation, turn int, w World) []string {
	if w != nil && w.HasEntity(id) {
		return []string{fmt.Sprintf("world_update: %s already exists in the engine — use set to add narrative properties to it, not create", id)}
	}
	if _, held := s.entities[id]; held {
		return []string{fmt.Sprintf("world_update: %s already exists — use set to change it", id)}
	}
	if len(op.Properties) == 0 {
		return []string{fmt.Sprintf("world_update: create %s needs properties, including a type", id)}
	}

	kind, _ := op.Properties["type"].(string)
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return []string{fmt.Sprintf("world_update: create %s needs a type property saying what it is (rumor, faction, prophecy, ...)", id)}
	}

	e := s.touch(id, turn)
	e.Type = kind
	return s.merge(e, op.Properties)
}

// set merges properties into whatever the id names — an engine entity's
// narrative half, or a narrative-only entity's whole. An id matching nothing is
// refused rather than quietly created: a set against a typo would otherwise mint
// a ghost that reads exactly like a real entity for the rest of the session.
func (s *Store) set(id string, op Operation, turn int, w World) []string {
	_, held := s.entities[id]
	engine := w != nil && w.HasEntity(id)
	if !held && !engine {
		return []string{fmt.Sprintf("world_update: %s does not exist. %s Use create if it is new and purely narrative, or the typed tool if it is a character, item or location", id, existing(w))}
	}
	if len(op.Properties) == 0 {
		return []string{fmt.Sprintf("world_update: set %s carries no properties, nothing to record", id)}
	}
	return s.merge(s.touch(id, turn), op.Properties)
}

// remove drops a narrative-only entity. An engine entity cannot be deleted from
// here: the thing itself is not this package's to destroy, and its narrative
// half going away should not read like the entity did.
func (s *Store) remove(id string, op Operation, w World) []string {
	var notes []string
	if len(op.Properties) > 0 {
		notes = append(notes, fmt.Sprintf("world_update: delete %s ignores the properties it was given — delete removes the whole entity", id))
	}
	if w != nil && w.HasEntity(id) {
		return append(notes, fmt.Sprintf("world_update: %s is an engine entity and cannot be deleted here — set its narrative properties instead, or use the typed tool that owns it", id))
	}
	if _, held := s.entities[id]; !held {
		return append(notes, fmt.Sprintf("world_update: %s does not exist, nothing to delete", id))
	}
	delete(s.entities, id)
	for i, existing := range s.ids {
		if existing == id {
			s.ids = append(s.ids[:i], s.ids[i+1:]...)
			break
		}
	}
	return notes
}

// merge writes properties onto an entry, refusing the reserved ones. A null
// value removes the property: retraction is rare enough not to deserve an
// operation of its own, and "set it to nothing" is what a model reaches for
// unprompted anyway.
func (s *Store) merge(e *Entity, props map[string]any) []string {
	var notes []string
	// Sorted so a batch that trips several refusals reports them the same way
	// every run. Map order here would make the debug output nondeterministic for
	// no reason.
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" {
			notes = append(notes, fmt.Sprintf("world_update: %s: a property with no name was skipped", e.ID))
			continue
		}
		if key == "type" {
			// Consumed by create, and immutable afterwards. An entity that could
			// change type mid-session is one the DM would have to re-learn.
			continue
		}
		if reserved[key] {
			notes = append(notes, fmt.Sprintf("world_update: %s: %q is the engine's to state and was discarded — it is already rendered in this entity's record, and a second copy here would be a second answer", e.ID, key))
			continue
		}
		if props[name] == nil {
			e.unset(key)
			continue
		}
		e.set(key, props[name])
	}
	return notes
}

func (s *Store) touch(id string, turn int) *Entity {
	e, held := s.entities[id]
	if !held {
		e = &Entity{ID: id, props: map[string]any{}}
		s.entities[id] = e
		s.ids = append(s.ids, id)
	}
	e.turn = turn
	return e
}

func (e *Entity) set(name string, value any) {
	if _, held := e.props[name]; !held {
		e.names = append(e.names, name)
	}
	e.props[name] = value
}

func (e *Entity) unset(name string) {
	if _, held := e.props[name]; !held {
		return
	}
	delete(e.props, name)
	for i, existing := range e.names {
		if existing == name {
			e.names = append(e.names[:i], e.names[i+1:]...)
			return
		}
	}
}

// existing names what the engine holds, so a refusal for a name that does not
// exist can be corrected on the next turn rather than guessed at again. It is
// the same courtesy the rules library's own errors extend.
func existing(w World) string {
	if w == nil {
		return ""
	}
	ids := w.EntityIDs()
	if len(ids) == 0 {
		return "Nothing exists yet."
	}
	const most = 30
	if len(ids) > most {
		return fmt.Sprintf("What exists: %s, and %d more.", strings.Join(ids[:most], ", "), len(ids)-most)
	}
	return fmt.Sprintf("What exists: %s.", strings.Join(ids, ", "))
}
