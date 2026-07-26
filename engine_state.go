package main

import (
	"fmt"
	"sort"
	"strings"

	"fudge/archive"
	"fudge/ledger"
	"fudge/rules"
)

const (
	// timelineWindow is how many turns of chronology ride in the block. Older
	// turns have already folded into the ledger, which holds what they left
	// behind; the timeline is there for how the state came about, and that
	// question is almost always asked about the recent past. Anything further
	// back is one recall away.
	timelineWindow = 10
	// proseWindow is how many of those turns keep their narration verbatim.
	// Deltas preserve facts and lose voice: what the room was already said to
	// look like, the phrasing used for a character, a thread left hanging
	// mid-sentence. Three turns is a scene rather than a beat, and it is by far
	// the largest thing in the block — the first knob to turn if it must shrink.
	proseWindow = 3
)

// session is everything a game remembers between turns. It exists so that the
// three stores can be handed around as one thing, and because their relationship
// is the point: the world holds what the engine computes, the ledger holds what
// only the prose knows, the archive holds what happened, and no fact belongs to
// more than one of them.
type session struct {
	world   *rules.World
	ledger  *ledger.Ledger
	archive *archive.Archive
	// turn counts every turn the player has started, narrated or not, and is
	// what the ledger stamps its entries with. A turn that failed still spent a
	// number: the archive keeps it under that number, and a marker in the ledger
	// pointing at a turn the DM can recall and find unnarrated says more than a
	// marker pointing at a turn that silently became somebody else's.
	turn int
	// lastBlock is the block as the DM actually last received it. Kept only so
	// the context dump can show that, rather than rendering a fresh one: between
	// turns a fresh render is the *next* turn's block, already carrying what the
	// turn just folded in, and somebody debugging a bad narration would be
	// reading state the DM did not have when it wrote it.
	lastBlock string
}

func newSession(w *rules.World) *session {
	return &session{world: w, ledger: ledger.New(), archive: archive.New()}
}

// isCharacter reports whether a ledger key names somebody the engine holds, and
// is what decides that a reported fact is the engine's to state rather than the
// DM's. Passed to the ledger rather than looked up inside it, because the ledger
// has no business knowing what a character is.
func (s *session) isCharacter(key string) bool {
	_, err := s.world.Character(key)
	return err == nil
}

// engineState renders the whole block the DM reads: what is true now, and how it
// came to be. It is rebuilt from the three stores every turn and never stored,
// so it cannot drift from them and cannot go stale the way a message in the
// history did.
func (s *session) engineState() string {
	var b strings.Builder

	b.WriteString("=== ENGINE STATE ===\n")
	b.WriteString("This is the record. It outranks your memory and anything the prose implied.\n\n")

	b.WriteString("ROSTER — every character the engine holds, as they stand right now.\n")
	b.WriteString("These facts are the engine's: wounds, Fudge Points, coins, possessions, traits.\n")
	b.WriteString("You do not write them, and what is absent here is not carried or not true.\n\n")
	b.WriteString(roster(s.world))

	b.WriteString("\n\nWORLD — what only your prose knows, merged from every world_update you have\n")
	b.WriteString("sent. @tN marks the turn an entry was last set; recall it to see how it got\n")
	b.WriteString("that way. Where characters are standing lives here, not in the roster.\n\n")
	if rendered := s.ledger.Render(); rendered != "" {
		b.WriteString(rendered)
	} else {
		b.WriteString("(nothing recorded yet)")
	}

	b.WriteString("\n\nTIMELINE — the last ")
	fmt.Fprintf(&b, "%d turns in order, oldest first. The most recent %d keep the\n", timelineWindow, proseWindow)
	b.WriteString("prose you wrote, so you can carry your own voice forward. Older turns are in\n")
	b.WriteString("the archive; reach them with recall.\n\n")
	b.WriteString(timeline(s.archive))

	return b.String()
}

// roster renders every character in the world, the player first and the rest by
// id. Order is fixed rather than incidental: a block that shuffled between turns
// would rewrite the cached prefix for nothing, and the DM would have to re-find
// everybody each time it read one.
func roster(w *rules.World) string {
	if len(w.Characters) == 0 {
		return "(no characters exist yet)"
	}

	ids := make([]string, 0, len(w.Characters))
	for id := range w.Characters {
		if id != w.Player {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if _, err := w.Character(w.Player); err == nil {
		ids = append([]string{w.Player}, ids...)
	}

	blocks := make([]string, 0, len(ids))
	for _, id := range ids {
		blocks = append(blocks, w.Characters[id].ToString())
	}
	return strings.Join(blocks, "\n\n")
}

// timeline renders the recent past as the player and the DM made it: what was
// typed, what it changed, and lately what was said about it.
func timeline(a *archive.Archive) string {
	turns := a.Recent(timelineWindow)
	if len(turns) == 0 {
		return "(play has not started)"
	}

	var b strings.Builder
	for i, t := range turns {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "t%d player: %s\n", t.Number, t.PlayerInput)
		if t.Update != "" {
			fmt.Fprintf(&b, "    changes: %s\n", t.Update)
		}
		// Prose rides on the newest turns only, counted from the end so the
		// window slides rather than sticking to the turns that happen to be
		// first in this render.
		if len(turns)-i <= proseWindow && t.Prose != "" {
			fmt.Fprintf(&b, "    prose: %s\n", t.Prose)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
