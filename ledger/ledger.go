// Package ledger holds the fiction facts the rules library cannot compute —
// dispositions, positions, scene details, open threads — as one entry per
// entity, merged as the DM reports changes and rendered back to it as dense
// text.
//
// It exists because the DM's reports used to accumulate in the message history
// as separate deltas. Three reports about one NPC meant three answers to the
// question "how hurt is he", two of them stale and contradicting the third, and
// the prompt's longest section was spent teaching a probabilistic system the
// delta discipline that would have avoided it. Merging in Go moves that from a
// system that sometimes obeys to one that always does, and it makes restating
// an unchanged fact idempotent, so the DM can be told to write anything it is
// unsure of instead of being told to hold back.
package ledger

import (
	"fmt"
	"strings"
)

// unparsedKey is the entry every line that cannot be keyed is filed under. A
// keyless line still carries a fact, and the one thing this package must never
// do is drop one; collecting them all under a single visible entry keeps the
// fact in front of the DM while stopping malformed input from breeding entries.
const unparsedKey = "unparsed"

// engineOwned names the fields and flags the rules library computes and renders
// itself, every turn, from the world. When one of these arrives on a line keyed
// to a character it is discarded rather than merged: the roster is already
// stating it, and a second copy here would be a second answer that ages out of
// step with the first. Making the overlap impossible by construction is the
// point — measured sessions show the DM reporting wounds it does not own, and
// no amount of prompt wording stopped it.
//
// `at` is deliberately absent. Character.Location is written once at creation
// and never updated, so the engine is not in a position to own position; that
// fact belongs here.
var engineOwned = map[string]bool{
	"wounds":       true,
	"scratches":    true,
	"fudge-points": true,
	"coins":        true,
	"carrying":     true,
	"attributes":   true,
	"skills":       true,
	"gifts":        true,
	"faults":       true,
	"scale":        true,
	"species":      true,
	"id":           true,
	"desc":         true,
	"penalty":      true,
	"cannot-act":   true,
}

type tokenKind int

const (
	// kindField is `k:v`, kindFlag a bare word, kindRemove a `!name` retraction,
	// and kindVerbatim a token the grammar could not read, carried through
	// unchanged so its text survives even though its meaning did not.
	kindField tokenKind = iota
	kindFlag
	kindRemove
	kindVerbatim
)

type token struct {
	kind  tokenKind
	name  string
	value string
}

func (t token) render() string {
	if t.kind == kindField {
		return t.name + ":" + t.value
	}
	return t.name
}

// entry is one entity's current truth. Names are held in a slice beside the map
// because the order the DM first mentioned things in is the order that reads
// best back to it, and because rendering out of map order would rewrite the
// cached prompt prefix every turn for no reason.
type entry struct {
	key    string
	names  []string
	tokens map[string]token
	turn   int
}

// Ledger is the merged world state. The zero value is not usable; call New.
type Ledger struct {
	keys    []string
	entries map[string]*entry
}

func New() *Ledger {
	return &Ledger{entries: map[string]*entry{}}
}

// Apply merges one report into the ledger, recording turn as the turn each
// touched entry was last set. isCharacter reports whether a key names a
// character the rules library owns, and may be nil, in which case nothing is.
// Returns human-readable notes about anything it discarded, for debug output.
func (l *Ledger) Apply(update string, turn int, isCharacter func(key string) bool) []string {
	var notes []string
	for _, line := range splitEntries(update) {
		notes = append(notes, l.applyLine(line, turn, isCharacter)...)
	}
	return notes
}

// splitEntries cuts a report into lines. Both separators are accepted because
// the DM writes both: `;` when it packs a report onto one line and newlines when
// it lays one out, and which of the two it reaches for is not worth a retry.
func splitEntries(update string) []string {
	raw := strings.FieldsFunc(update, func(r rune) bool {
		return r == ';' || r == '\n' || r == '\r'
	})
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func (l *Ledger) applyLine(line string, turn int, isCharacter func(key string) bool) []string {
	parts := strings.Split(line, "|")
	key := strings.ToLower(strings.TrimSpace(parts[0]))
	if key == "" {
		// Nothing here can be keyed, so the text goes into the bucket as it
		// arrived. Rewriting it into something well-formed would be guessing at
		// what the DM meant, and the DM can read its own words back.
		l.touch(unparsedKey, turn).set(token{kind: kindVerbatim, name: line})
		return []string{fmt.Sprintf("ledger: line has no key, kept verbatim under %s: %q", unparsedKey, line)}
	}

	var notes []string
	character := isCharacter != nil && isCharacter(key)

	// GONE settles the line on its own: whatever else it says, the entity is out
	// of play, and there is nothing to merge into an entry that no longer exists.
	for _, raw := range parts[1:] {
		if strings.EqualFold(strings.TrimSpace(raw), "gone") {
			l.drop(key)
			if len(parts) > 2 {
				notes = append(notes, fmt.Sprintf("ledger: %s|GONE dropped the entry, ignoring %d other token(s) on the line", key, len(parts)-2))
			}
			return notes
		}
	}

	kept := make([]token, 0, len(parts)-1)
	for _, raw := range parts[1:] {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		tok, ok := parseToken(raw)
		if !ok {
			notes = append(notes, fmt.Sprintf("ledger: %s: token %q is not `k:v`, `flag` or `!name`, kept verbatim", key, raw))
		}
		if character && engineOwned[tok.name] {
			notes = append(notes, fmt.Sprintf("ledger: %s: discarded %q — the rules library owns %s and renders it in the roster", key, raw, tok.name))
			continue
		}
		kept = append(kept, tok)
	}

	// A line the stripper emptied has said nothing this ledger is allowed to
	// record, so it must not create an entry or move a turn marker. A bare key
	// with no tokens at all is different: it asserts that the entity exists, and
	// that is a fact worth holding.
	if len(kept) == 0 && len(parts) > 1 {
		return notes
	}

	e := l.touch(key, turn)
	for _, tok := range kept {
		if tok.kind == kindRemove {
			e.remove(tok.name)
			continue
		}
		e.set(tok)
	}
	return notes
}

// parseToken reads one token, reporting whether the grammar could account for
// it. An unreadable token is not an error the caller can act on — there is
// nobody to ask for a correction and the turn has already happened — so it comes
// back as verbatim text that will render exactly as it arrived.
func parseToken(raw string) (token, bool) {
	if name, found := strings.CutPrefix(raw, "!"); found {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			return token{kind: kindVerbatim, name: raw}, false
		}
		return token{kind: kindRemove, name: name}, true
	}
	if name, value, found := strings.Cut(raw, ":"); found {
		// Only the first colon separates: a value is free to contain more of
		// them, which timestamps, ratios and the DM's own notation all do.
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			return token{kind: kindVerbatim, name: raw}, false
		}
		return token{kind: kindField, name: name, value: strings.TrimSpace(value)}, true
	}
	return token{kind: kindFlag, name: strings.ToLower(raw)}, true
}

// touch returns the entry for key, creating it if it is new, and marks it as set
// this turn. Only callers that are about to record something call this, so the
// marker always answers "when was this last actually stated".
func (l *Ledger) touch(key string, turn int) *entry {
	e, ok := l.entries[key]
	if !ok {
		e = &entry{key: key, tokens: map[string]token{}}
		l.entries[key] = e
		l.keys = append(l.keys, key)
	}
	e.turn = turn
	return e
}

func (l *Ledger) drop(key string) {
	if _, ok := l.entries[key]; !ok {
		return
	}
	delete(l.entries, key)
	for i, k := range l.keys {
		if k == key {
			l.keys = append(l.keys[:i], l.keys[i+1:]...)
			return
		}
	}
}

// set merges one token into the entry. A value replaces whatever the name held
// before and stays where it was, so a line the DM reads back keeps the shape it
// had; a bare flag whose name already carries a value leaves that value alone,
// because "add the flag if absent" is not licence to delete a fact the DM did
// not ask to retract.
func (e *entry) set(tok token) {
	existing, held := e.tokens[tok.name]
	if held && tok.kind != kindField && existing.kind == kindField {
		return
	}
	if !held {
		e.names = append(e.names, tok.name)
	}
	e.tokens[tok.name] = tok
}

func (e *entry) remove(name string) {
	if _, held := e.tokens[name]; !held {
		return
	}
	delete(e.tokens, name)
	for i, n := range e.names {
		if n == name {
			e.names = append(e.names[:i], e.names[i+1:]...)
			return
		}
	}
}

// Render returns the ledger as dense text for the DM to read: one entry per
// line, its tokens in the order they were first seen, and a marker for the turn
// the entry was last set so any fact can be traced back to where it came from.
func (l *Ledger) Render() string {
	var b strings.Builder
	for i, key := range l.keys {
		e := l.entries[key]
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(e.key)
		for _, name := range e.names {
			b.WriteByte('|')
			b.WriteString(e.tokens[name].render())
		}
		fmt.Fprintf(&b, " @t%d", e.turn)
	}
	return b.String()
}
