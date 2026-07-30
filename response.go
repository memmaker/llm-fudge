package main

import (
	"regexp"
	"strconv"
	"strings"
)

// A turn's reply is one document, not a list of calls. The DM writes the scene
// as ordinary prose, marks the mechanical transitions inside it, and closes with
// the state the prose left the world in. This file takes that document apart;
// worldstate.go decides what the state half means.
//
// Prose is the body itself rather than an argument of a call, so there is no
// escaping layer between what the model writes and what the player reads: a
// narration is written the way any text is written, and the blocks that follow
// it are the only parts the parser has to find.

var (
	stateBlockPattern = regexp.MustCompile(`(?is)<state>(.*?)</state>`)
	imgBlockPattern   = regexp.MustCompile(`(?is)<img>(.*?)</img>`)
	// Event tags are written inline, at the moment in the prose where the thing
	// happens, so they are matched as empty elements anywhere in the text and
	// cut out of what the player is shown.
	eventTagPattern = regexp.MustCompile(`(?is)<(damage|heal|dead)\b([^>]*?)/?>`)
	attributePatter = regexp.MustCompile(`([a-z_]+)\s*=\s*"([^"]*)"`)
	// A model that has been asked for YAML often fences it, and a fence inside
	// the block is noise rather than an error worth a round trip.
	fencePattern = regexp.MustCompile("(?s)^\\s*```[a-z]*\\s*\n(.*?)\n?\\s*```\\s*$")
	// Blank lines left behind where a block was cut out. Prose keeps its own
	// paragraph breaks; it should not inherit the ones that only ever separated
	// the machinery underneath it.
	blankRunPattern = regexp.MustCompile(`\n{3,}`)
)

// dmResponse is one reply taken apart: what the player reads, the mechanical
// transitions the prose marked, the state it left behind, and the image prompt.
type dmResponse struct {
	Prose  string
	Events []stateEvent
	// State is the raw YAML of the <state> block, empty when the reply carried
	// none. A turn that changed nothing the engine holds is a normal turn, so an
	// absent block is not a failure.
	State string
	Img   string
}

// stateEvent is one mechanical transition marked inline in the prose.
//
// These exist for exactly the changes whose declared end state would
// under-determine the rules outcome — health, where what a wound does depends on
// the wounds already carried. Everything else is declared in the state block and
// the engine derives the operation from the change, so this vocabulary is
// deliberately three tags and not a grammar.
type stateEvent struct {
	Kind   string // damage, heal, dead
	Target string
	// Severity is the wound a damage tag inflicts; Levels is how many a heal
	// tag mends. Each is meaningless to the other kind.
	Severity string
	Levels   int
}

// parseResponse splits a reply into the part the player reads and the parts the
// engine does.
//
// Nothing here refuses anything. A reply with no state block, no events and no
// prose is returned as it is, and what to do about that is the turn loop's
// question — this only ever reports what arrived.
func parseResponse(body string) dmResponse {
	var out dmResponse

	if m := stateBlockPattern.FindStringSubmatch(body); m != nil {
		out.State = unfence(m[1])
	}
	if m := imgBlockPattern.FindStringSubmatch(body); m != nil {
		out.Img = strings.TrimSpace(m[1])
	}

	prose := stateBlockPattern.ReplaceAllString(body, "")
	prose = imgBlockPattern.ReplaceAllString(prose, "")

	for _, m := range eventTagPattern.FindAllStringSubmatch(prose, -1) {
		out.Events = append(out.Events, parseEventTag(m[1], m[2]))
	}
	// Cut after collecting, so an event tag mid-sentence leaves the sentence
	// joined up rather than leaving a hole in what the player reads.
	prose = eventTagPattern.ReplaceAllString(prose, "")

	out.Prose = strings.TrimSpace(blankRunPattern.ReplaceAllString(prose, "\n\n"))
	return out
}

func parseEventTag(kind, attributes string) stateEvent {
	e := stateEvent{Kind: strings.ToLower(kind)}
	for _, a := range attributePatter.FindAllStringSubmatch(attributes, -1) {
		switch strings.ToLower(a[1]) {
		case "target", "character", "id":
			e.Target = strings.TrimSpace(a[2])
		case "severity":
			e.Severity = strings.ToLower(strings.TrimSpace(a[2]))
		case "levels":
			e.Levels, _ = strconv.Atoi(strings.TrimSpace(a[2]))
		}
	}
	return e
}

// unfence strips a code fence the model wrapped the block in, and is a no-op on
// a block that arrived bare.
func unfence(s string) string {
	if m := fencePattern.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(m[1])
	}
	return strings.TrimSpace(s)
}
