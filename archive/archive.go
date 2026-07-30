// Package archive is the session's permanent record: every turn as the engine
// played it, kept out of the DM's context entirely and read only when asked for.
//
// It exists because the two things worth remembering longest were the two the
// engine used to throw away fastest. Narration prose was shown to the player and
// dropped the same instant, so what an NPC actually said was unrecoverable an
// hour later. What the engine resolved — the rolls, the wounds, the reaction
// checks — survived only as long as the message history that carried it, which
// is exactly what the context work set out to stop growing.
//
// Keeping all of it costs nothing here. The archive lives in memory and on disk,
// never in a request, and the DM reaches it through a single lookup whose size is
// bounded. That is the trade the whole design turns on: remember everything,
// carry almost none of it.
package archive

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// maxLookupRunes caps a rendered lookup. A lookup that could return a whole
// session would hand back more context than the block it was meant to spare, so
// the ceiling is deliberately far below what the DM could ask for.
const maxLookupRunes = 4000

// defaultLimit is how many turns a keyword lookup returns when the caller names
// no number. Three is enough to see a thread and short enough that a vague
// keyword is not worth much.
//
// A range is not subject to it. Somebody who asked for turns 5 to 15 named the
// turns they wanted, and handing back three of them while reporting nothing
// about the other eight would be the worst kind of quiet answer.
const defaultLimit = 3

// maxLookupTurns is the ceiling on any one lookup, whatever the caller asked
// for. The rune cap below would stop a runaway result eventually, but it would
// stop it mid-sentence; refusing to gather two hundred turns in the first place
// is the better half of the same guarantee.
const maxLookupTurns = 20

// Event is one tool call or world_update operation as the engine ran it: what
// ran, which entities it named, and the engine's own one-line account of the
// result.
//
// Entities cost nothing new to collect. Every call that touches an entity
// already names it by ID to make the call at all — the ID was sitting in the
// argument list the whole time, and the archive used to throw it away and then
// go looking for the word again in the prose. Keeping it is what turns "does
// the word merchant appear here" into "did the merchant take part in this
// turn".
type Event struct {
	Tool     string
	Entities []string
	Headline string
}

// Turn is one played turn as the engine recorded it.
//
// Events are the engine's own headlines rather than the DM's account of them.
// Recording them here is what makes the archive independent of the DM's memory:
// a roll that happened is in the record whether or not the DM thought it worth
// writing down, which is not true of anything it reports itself.
type Turn struct {
	Number      int
	PlayerInput string
	Events      []Event
	Prose       string
	// Unnarrated marks a turn that ran out of requests before it produced
	// narration. It is kept, because a gap in the record is worth explaining,
	// but the player never saw it and it must not be replayed as if they had.
	Unnarrated bool
}

// Entities is the set of entity IDs that took part in this turn, deduped and
// sorted.
//
// It is computed on every call and deliberately not stored on Turn. A turn's
// participants are already written down once, on its events; a second copy on
// the turn would be a fact with two homes, and a fact with two homes is exactly
// the class of bug the flat entity model was built to remove — the two drift,
// and nothing says which one is lying. Recomputing a union over a handful of
// events is not a cost worth buying that risk back with.
func (t Turn) Entities() []string {
	var ids []string
	for _, e := range t.Events {
		for _, id := range e.Entities {
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
	}
	slices.Sort(ids)
	return ids
}

// Archive holds every turn of a session in the order they were played.
type Archive struct {
	turns []Turn
}

func New() *Archive {
	return &Archive{}
}

// Record files a turn. Turns arrive in order and are kept that way, so nothing
// here sorts or dedupes: the engine plays a turn once.
func (a *Archive) Record(t Turn) {
	a.turns = append(a.turns, t)
}

// Len reports how many turns have been recorded, failed ones included.
func (a *Archive) Len() int {
	return len(a.turns)
}

// Recent returns up to n of the most recently narrated turns, oldest first,
// which is the order the timeline reads them in — a chronology is followed
// forward, not backward.
//
// Unnarrated turns are left out. They are half turns whose outcome nobody ever
// read, and putting one in front of the DM would have it narrating around a
// scene the player was never shown.
func (a *Archive) Recent(n int) []Turn {
	if n <= 0 {
		return nil
	}
	narrated := make([]Turn, 0, len(a.turns))
	for _, t := range a.turns {
		if !t.Unnarrated {
			narrated = append(narrated, t)
		}
	}
	if len(narrated) > n {
		narrated = narrated[len(narrated)-n:]
	}
	return narrated
}

// Query selects the turns a lookup hands back. Whatever it names — an entity, a
// keyword, a range — is an AND: each field narrows the last.
//
// Entity, Keyword and From/To are three ways at the same archive, and they
// answer different questions. An entity ID is the exact one, and the one to
// reach for whenever the question is about a thing: "has the player met this
// merchant" is a lookup on the merchant's ID, not a hope that the narration
// happened to use the word. A keyword is for what is not tied to one entity at
// all — "what did anyone say about ghosts". A turn number goes straight to the
// source of a fact the DM is already looking at.
type Query struct {
	// Entity matches a turn whose events name this exact ID. It is never a
	// substring or a case-fold: `key` must not answer for `iron-key`, and two
	// entities that differ only in case are two entities.
	Entity  string
	Keyword string
	// From and To select an inclusive range of turn numbers. From alone selects
	// a single turn. Zero means unset.
	From, To     int
	IncludeProse bool
	Limit        int
}

// Lookup renders the turns matching q as text for the DM to read.
func (a *Archive) Lookup(q Query) string {
	matches := a.match(q)

	// Keyword matches are ranked by recency, since the turn the DM means is
	// usually the last one that mentioned the thing. A range was asked for
	// explicitly and is handed back in the order it was played.
	byRange := q.From > 0

	limit := q.Limit
	if limit <= 0 {
		limit = defaultLimit
		if byRange {
			limit = len(matches)
		}
	}
	if limit > maxLookupTurns {
		limit = maxLookupTurns
	}

	if !byRange {
		sort.SliceStable(matches, func(i, j int) bool {
			return matches[i].Number > matches[j].Number
		})
	}
	if len(matches) > limit {
		matches = matches[:limit]
	}

	if len(matches) == 0 {
		return a.nothingFound(q)
	}

	var b strings.Builder
	for i, t := range matches {
		if i > 0 {
			b.WriteString("\n")
		}
		writeTurn(&b, t, q.IncludeProse)
	}
	return truncate(b.String())
}

// Dump renders the whole archive, prose and all, with none of the caps a lookup
// is subject to. Those caps exist because a lookup lands in the DM's context and
// has to be bounded; a dump lands in a file a person reads while working out why
// a turn went wrong, and a record truncated at the interesting part is no record.
func (a *Archive) Dump() string {
	if len(a.turns) == 0 {
		return "(no turns recorded)"
	}
	var b strings.Builder
	for i, t := range a.turns {
		if i > 0 {
			b.WriteString("\n")
		}
		writeTurn(&b, t, true)
	}
	return b.String()
}

// match collects every turn q selects, before any ranking or capping.
func (a *Archive) match(q Query) []Turn {
	var matches []Turn
	for _, t := range a.turns {
		if q.From > 0 {
			to := q.To
			if to == 0 {
				to = q.From
			}
			if t.Number < q.From || t.Number > to {
				continue
			}
		}
		if q.Entity != "" && !slices.Contains(t.Entities(), q.Entity) {
			continue
		}
		if q.Keyword != "" && !mentions(t, q.Keyword) {
			continue
		}
		matches = append(matches, t)
	}
	return matches
}

// mentions reports whether a turn talks about keyword anywhere the DM could
// have written it: what the player typed, what the engine reported back, what
// was narrated.
func mentions(t Turn, keyword string) bool {
	needle := strings.ToLower(keyword)
	fields := []string{t.PlayerInput, t.Prose}
	for _, e := range t.Events {
		fields = append(fields, e.Headline)
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), needle) {
			return true
		}
	}
	return false
}

func writeTurn(b *strings.Builder, t Turn, includeProse bool) {
	fmt.Fprintf(b, "turn %d\n", t.Number)
	if t.Unnarrated {
		b.WriteString("unnarrated: this turn ran out of requests and the player was shown nothing\n")
	}
	if t.PlayerInput != "" {
		fmt.Fprintf(b, "player: %s\n", t.PlayerInput)
	}
	// One line per event, tool first, entities last in brackets. Every
	// character of this lands in the DM's context and counts against
	// maxLookupRunes, so the entities are bare IDs rather than a labelled
	// list: the DM already knows an ID when it sees one, having passed it in.
	for _, e := range t.Events {
		fmt.Fprintf(b, "events: %s", e.Tool)
		if e.Headline != "" {
			fmt.Fprintf(b, ": %s", e.Headline)
		}
		if len(e.Entities) > 0 {
			fmt.Fprintf(b, " [%s]", strings.Join(e.Entities, " "))
		}
		b.WriteString("\n")
	}
	if includeProse && t.Prose != "" {
		fmt.Fprintf(b, "prose: %s\n", t.Prose)
	}
}

// nothingFound says so in as many words. An empty or ambiguous answer is the
// one result worth being careful about: the DM asked because it did not know,
// and a blank reply reads as licence to invent whatever it needed.
func (a *Archive) nothingFound(q Query) string {
	// The clauses are listed rather than switched over, because with three
	// fields that compose freely a switch is eight cases to keep in step with
	// match(), and match() will grow a fourth field before this does.
	var clauses []string
	if q.Entity != "" {
		clauses = append(clauses, "entity "+q.Entity)
	}
	if q.Keyword != "" {
		clauses = append(clauses, fmt.Sprintf("%q", q.Keyword))
	}
	if q.From > 0 {
		clauses = append(clauses, "turns "+rangeLabel(q))
	}
	asked := "that lookup"
	if len(clauses) > 0 {
		asked = strings.Join(clauses, " and ")
	}

	// An id that appears in no turn at all is a different answer from a real id
	// with nothing recorded against it, and the two must not read alike. The
	// closing sentence is deliberately absolute — a blank reply reads to a model as
	// licence to invent — but attached to a lookup that merely failed to spell the
	// id right it would turn a typo into proof. So the unrecognised case says which
	// it is, and still refuses the invention.
	unrecognised := ""
	if q.Entity != "" && !a.knows(q.Entity) {
		unrecognised = fmt.Sprintf(" No entity with the id %q appears anywhere in the record, so check the id against WORLD STATE before concluding anything from this — an id that is merely misspelled proves nothing either way.", q.Entity)
	}
	return fmt.Sprintf("no archived turn matches %s.%s %d turns are recorded. Nothing here is something you have forgotten — do not invent it.", asked, unrecognised, len(a.turns))
}

// knows reports whether an id has ever taken part in a recorded turn. The archive
// does not hold the world, so this is the only sense in which it can recognise an
// id: something addressed by that name has happened at least once.
func (a *Archive) knows(entity string) bool {
	for _, t := range a.turns {
		for _, event := range t.Events {
			for _, id := range event.Entities {
				if id == entity {
					return true
				}
			}
		}
	}
	return false
}

func rangeLabel(q Query) string {
	if q.To == 0 || q.To == q.From {
		return fmt.Sprintf("%d", q.From)
	}
	return fmt.Sprintf("%d-%d", q.From, q.To)
}

// truncate holds a lookup under the ceiling, and says that it did. Cutting
// quietly would leave the DM reading a turn that stops mid-sentence and taking
// it for the whole of what was recorded.
func truncate(s string) string {
	runes := []rune(s)
	if len(runes) <= maxLookupRunes {
		return s
	}
	return string(runes[:maxLookupRunes]) + "\n…lookup truncated. Ask again with a turn number or a narrower keyword.\n"
}
