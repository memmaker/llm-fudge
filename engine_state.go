package main

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"fudge/archive"
	"fudge/narrative"
	"fudge/rules"
)

const (
	// timelineWindow is how many turns of chronology ride in the block. Older
	// turns have already folded into the world, which holds what they left
	// behind; the timeline is there for how the state came about, and that
	// question is almost always asked about the recent past. Anything further
	// back is one recall away.
	timelineWindow = 10
	// proseWindow is how many of those turns keep their narration verbatim.
	// Events preserve facts and lose voice: what the room was already said to
	// look like, the phrasing used for a character, a thread left hanging
	// mid-sentence. Verbatim prose is by far the largest thing in the block, so
	// this is the first knob to turn if it must shrink. One turn is what carrying
	// a voice forward actually needs: the scene just written, which is the one
	// the next scene has to sound like and follow on from. What happened before
	// it is in the timeline as facts, and one recall away as words.
	proseWindow = 1
	// The levels a trait sits at when nothing says otherwise, which is what the
	// rules reference tells the DM to assume and therefore what the block does
	// not have to spell out. Attributes are Fair, skills Poor.
	defaultAttribute = 0
	defaultSkill     = -2
)

// session is everything a game remembers between turns. It exists so that the
// three stores can be handed around as one thing, and because their relationship
// is the point: the world holds what the engine computes, the narrative store
// holds what only the prose knows, the archive holds what happened, and no fact
// belongs to more than one of them.
type session struct {
	world *rules.World
	// narrative is keyed by the same entity ids the world uses, which is what
	// lets one entity render as one record. The ledger this replaced had a
	// namespace of its own, so a character could be spoken about in two places
	// and the whole design went into keeping those two from contradicting
	// each other.
	narrative *narrative.Store
	archive   *archive.Archive
	// turn counts every turn the player has started, narrated or not, and is
	// what the narrative store stamps its entries with. A turn that failed still
	// spent a number: the archive keeps it under that number, and a marker
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
	s := &session{world: w, narrative: narrative.New(), archive: archive.New()}
	// The two stores share one id keyspace, and this is what closes it from the
	// engine's side: without it an item minted from the name "Iron Key" would land
	// on an objective the DM had already created as `iron-key`, and the render
	// would fold two different things into one record.
	w.AlsoTaken = s.narrative.Has
	return s
}

// engineState renders the whole block the DM reads: what is true now, and how it
// came to be. It is rebuilt from the three stores every turn and never stored,
// so it cannot drift from them and cannot go stale the way a message in the
// history did.
// The block is three sections and they are kept apart on purpose: what is true
// now, then what was played, then how the last of it was written. Reading one of
// them is a different question from reading the others, and the interleaved form
// this replaced answered all three at once — a page where a paragraph of
// narration sat between two turns' event lists, so neither the state nor the
// chronology could be scanned without reading past prose.
func (s *session) engineState() string {
	var b strings.Builder

	b.WriteString("=== WORLD STATE — the record. It outranks your memory and anything the prose\n")
	b.WriteString("implied. Everything that exists is here, once, under the id every call must\n")
	b.WriteString("name. `narrative` is yours and @tN marks the turn it was set. Unstated means\n")
	b.WriteString("the default: undamaged, no fudge points, no coins, Fair in an attribute and\n")
	b.WriteString("Poor in a skill. Only what differs is listed. ===\n\n")
	b.WriteString(entities(s.world, s.narrative))

	fmt.Fprintf(&b, "\n\n=== TIMELINE — the last %d turns, oldest first: what the player did and what\n", timelineWindow)
	b.WriteString("the engine resolved. What it changed is above, current. Older turns are in the\n")
	b.WriteString("archive; reach them with recall. ===\n\n")
	b.WriteString(timeline(s.archive))

	if recent := recentProse(s.archive); recent != "" {
		scenes := "scenes"
		if proseWindow == 1 {
			scenes = "scene"
		}
		fmt.Fprintf(&b, "\n\n=== RECENT PROSE — your last %d %s as you wrote them, so you can\n", proseWindow, scenes)
		b.WriteString("carry your own voice forward. ===\n\n")
		b.WriteString(recent)
	}

	// Rebuilt every turn anyway, so repeating the sharpest, most-violated rules
	// here costs nothing in cache and lands as the last thing read before the
	// player's line and the block that follows it — closer to where it is
	// needed than the same rules stated once, in full, back in the system
	// prompt. dm.md is the reference; this is the reminder.
	b.WriteString("\n\n=== WRITING THE <state> BLOCK — closed fields only. `container` is set on\n")
	b.WriteString("the child and only the child; the engine derives everything upward from it,\n")
	b.WriteString("so `contents`, `inventory`, `here` and any other list-of-what's-inside field\n")
	b.WriteString("is refused, not merely unnecessary. Mechanically relevant facts only — do\n")
	b.WriteString("not invent a field to preserve a description your prose already carries. ===")

	return b.String()
}

// entities renders the one flat map the DM reads the world off: every character,
// every item, every place, and every narrative-only thing, each exactly once
// under its own id.
//
// Flat is the whole point, and it is worth saying why, because the tree it
// replaced read more naturally to a person. Under the old three-section block a
// single entity was split across all three — a character's traits in the roster,
// their position in the location tree, what the prose knew about them in a ledger
// of its own — and every one of those sections was somewhere a fact about that
// character could be written. Here there is one place, so there is one answer.
//
// The order is fixed rather than incidental: characters (player first), then
// locations in tree order, then items, then narrative-only entities. A block that
// shuffled between turns would rewrite the cached prompt prefix for nothing, and
// the DM would have to re-find everything each time it read one.
func entities(w *rules.World, n *narrative.Store) string {
	if len(w.Characters) == 0 && len(w.Locations) == 0 && len(n.IDs()) == 0 {
		return "entities: {}  # nothing exists yet — your first <state> block creates the world"
	}

	var b strings.Builder
	b.WriteString("entities:\n")
	rendered := map[string]bool{}

	write := func(id string, body func(*strings.Builder)) {
		rendered[id] = true
		fmt.Fprintf(&b, "  %s:\n", id)
		body(&b)
		writeNarrative(&b, n.Get(id))
		b.WriteString("\n")
	}

	// Characters, the player first: they are who the DM is writing about, and
	// what they carry is the fact it gets wrong most often.
	for _, id := range characterOrder(w) {
		write(id, func(b *strings.Builder) { writeCharacter(b, w.Characters[id]) })
	}

	// Locations in tree order, so a place reads next to the places around it even
	// though nothing is nested inside anything.
	for _, loc := range locationOrder(w) {
		write(loc.ID, func(b *strings.Builder) { writeLocation(b, w, loc) })
	}

	// Items last of the engine's own, since they are the most numerous and the
	// least often the subject of a call.
	for _, item := range itemOrder(w) {
		write(item.id, func(b *strings.Builder) { writeItem(b, item) })
	}

	// Narrative-only entities: a rumour, a debt, a job, a faction's standing.
	// Everything the store holds for an entity the engine also holds has already
	// been rendered onto that entity above, so what is left here is what exists
	// nowhere else.
	for _, id := range n.IDs() {
		if rendered[id] {
			continue
		}
		e := n.Get(id)
		write(id, func(b *strings.Builder) {
			kind := e.Type
			if kind == "" {
				kind = "note"
			}
			fmt.Fprintf(b, "    type: %s\n", kind)
		})
	}

	return strings.TrimRight(b.String(), "\n")
}

// writeCharacter renders one character's engine-owned fields.
//
// Maps and lists go in YAML's flow style — `{agility: 1}` rather than a block of
// indented lines — because this is valid YAML either way and the block form costs
// a line per trait, every turn, for a set of numbers the DM reads as a group.
func writeCharacter(b *strings.Builder, c *rules.Character) {
	b.WriteString("    type: character\n")
	fmt.Fprintf(b, "    name: %s\n", yamlString(c.Name))
	if c.Location != "" {
		fmt.Fprintf(b, "    location: %s\n", c.Location)
	}
	if c.Species != "" {
		fmt.Fprintf(b, "    species: %s\n", c.Species)
	}
	if c.Scale != 0 {
		fmt.Fprintf(b, "    scale: %+d\n", c.Scale)
	}
	if c.Description != "" {
		fmt.Fprintf(b, "    desc: %s\n", yamlString(c.Description))
	}

	// Wounds, points and coins are stated only when they are not at the value the
	// header already declares. Every character in a scene carries all three, and
	// on most of them all three are the default: a cast of five NPCs spent five
	// lines a turn, every turn, to say that nobody is hurt and nobody has any
	// money. Absence says it in none.
	if c.Wounds.Level != rules.Undamaged {
		fmt.Fprintf(b, "    wounds: %s\n", strings.ToLower(c.Wounds.Level.String()))
	}
	if c.Wounds.Scratches > 0 {
		fmt.Fprintf(b, "    scratches: %d/3\n", c.Wounds.Scratches)
	}
	if p := c.Wounds.Penalty(); p != 0 {
		fmt.Fprintf(b, "    penalty: %+d  # already applied to rolls\n", p)
	}
	if !c.Wounds.CanAct() {
		b.WriteString("    cannot_act: true\n")
	}
	if c.FudgePoints != 0 {
		fmt.Fprintf(b, "    fudge_points: %d\n", c.FudgePoints)
	}
	if c.Coins != 0 {
		fmt.Fprintf(b, "    coins: %d\n", c.Coins)
	}

	writeTraits(b, "attributes", c.Attributes, defaultAttribute)
	writeTraits(b, "skills", c.Skills, defaultSkill)
	if len(c.Gifts) > 0 {
		fmt.Fprintf(b, "    gifts: [%s]\n", strings.Join(c.Gifts, ", "))
	}
	if len(c.Faults) > 0 {
		fmt.Fprintf(b, "    faults: [%s]\n", strings.Join(c.Faults, ", "))
	}

	// The ids of what they carry, which is the same single fact as each item's
	// own `container` — one storage, two views. It is worth the second view: "can
	// they do this with what they have" is the question the DM asks most, and
	// answering it by scanning every item in the world for a matching container is
	// exactly the sort of reading a model does badly.
	//
	// Marked read-only right here, not just in the prompt's rules, for the same
	// reason `here` is on the location render: a list of ids on a character reads
	// exactly like something to hand-edit, and moving an item is a container on
	// the item, never a line here.
	if ids := carriedIDs(c); len(ids) > 0 {
		fmt.Fprintf(b, "    inventory: [%s]  # derived, don't write this back — set container on the item instead\n", strings.Join(ids, ", "))
	}
}

// writeLocation renders one place. Only the scene spells out who is standing in
// it: that list is derived from the characters' own `location` fields, and
// repeating it under every place in the world would grow the block with the map
// for a reading nobody does anywhere but the scene.
func writeLocation(b *strings.Builder, w *rules.World, loc *rules.Location) {
	b.WriteString("    type: location\n")
	fmt.Fprintf(b, "    name: %s\n", yamlString(loc.Name))
	if loc.Parent != "" {
		fmt.Fprintf(b, "    parent: %s\n", loc.Parent)
	}
	if loc.ID == w.Scene() {
		b.WriteString("    scene: true\n")
		if loc.Description != "" {
			fmt.Fprintf(b, "    desc: %s\n", yamlString(loc.Description))
		}
		// Marked read-only at the point of rendering rather than only in the
		// prompt's rules: this is a list of ids sitting on a location exactly
		// where a real field would, and "write the world back in the shape you
		// read it" is the DM's own governing instruction — so of everything
		// this line could do wrong, copying it back into the block is the one
		// the prompt itself invites.
		if here := w.CharactersAt(loc.ID); len(here) > 0 {
			fmt.Fprintf(b, "    here: [%s]  # derived, don't write this back — move the character instead\n", strings.Join(here, ", "))
		}
	}
}

// writeItem renders one item wherever it lies. `container` is the id of
// whoever or whatever holds it — a character, a place, or another item that
// is itself a container — and it is the item's single home: this is the field
// whose absence, when identity was a key built out of position, made an item
// impossible to follow across a move.
func writeItem(b *strings.Builder, it heldItem) {
	if it.item.IsContainer() {
		b.WriteString("    type: container\n")
	} else {
		b.WriteString("    type: item\n")
	}
	fmt.Fprintf(b, "    name: %s\n", yamlString(it.item.Name))
	fmt.Fprintf(b, "    container: %s\n", it.holder)
	if it.item.Count > 1 {
		fmt.Fprintf(b, "    count: %d\n", it.item.Count)
	}
	if it.item.Where != "" {
		fmt.Fprintf(b, "    spot: %s\n", yamlString(it.item.Where))
	}
	if it.item.Description != "" {
		fmt.Fprintf(b, "    desc: %s\n", yamlString(it.item.Description))
	}
	if wp := it.item.Weapon; wp != nil {
		fmt.Fprintf(b, "    weapon: {damage: %+d", wp.Damage)
		if wp.Ranged {
			b.WriteString(", ranged: true")
		}
		if wp.DamageAttribute != "" {
			fmt.Fprintf(b, ", adds: %s", wp.DamageAttribute)
		}
		if wp.TracksAmmo() {
			fmt.Fprintf(b, ", ammo: %d/%d", wp.Ammo, wp.AmmoCapacity)
			if wp.AmmoItem != "" {
				fmt.Fprintf(b, ", takes: %s", wp.AmmoItem)
			}
		}
		b.WriteString("}\n")
	}
	if ar := it.item.Armor; ar != nil {
		fmt.Fprintf(b, "    armor: {protection: %+d, worn: %t}\n", ar.Protection, ar.Worn)
	}
	// Read-only, same as a character's `inventory` or a location's `here`: this
	// is the same fact as each contained item's own `container`, repeated here
	// because "what does this hold right now" is the question asked of a
	// container, and answering it by scanning every item in the world is
	// exactly the reading a model does badly.
	if ids := containedIDs(it.item); len(ids) > 0 {
		fmt.Fprintf(b, "    contents: [%s]  # derived, don't write this back — set container on the item instead\n", strings.Join(ids, ", "))
	}
}

// writeNarrative renders an entity's narrative map, or nothing at all when it has
// none. The turn marker rides as a comment so the properties stay ordinary YAML
// values: a fact and a note about when it was written are different kinds of
// thing, and mixing them would have the DM reading `@t12` as part of the world.
func writeNarrative(b *strings.Builder, e *narrative.Entity) {
	if e == nil {
		return
	}
	props := e.Props()
	if len(props) == 0 {
		return
	}
	parts := make([]string, 0, len(props))
	for _, p := range props {
		parts = append(parts, yamlFlowScalar(p.Name)+": "+yamlFlowValue(p.Value))
	}
	fmt.Fprintf(b, "    narrative: {%s}  # @t%d\n", strings.Join(parts, ", "), e.Turn())
}

// yamlFlowValue renders one narrative property value inside a flow mapping.
//
// These values are the only part of the block the engine did not compose: they
// arrive as whatever JSON the DM sent, and they are prose — "the cargo, and no
// witnesses", "he said: run". A comma or a colon in a flow mapping ends the
// value early, so this is where the block stops being YAML if nothing quotes.
func yamlFlowValue(value any) string {
	switch v := value.(type) {
	case nil:
		return "null"
	case bool:
		return fmt.Sprintf("%t", v)
	case float64, float32, int, int64:
		return fmt.Sprintf("%v", v)
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			parts = append(parts, yamlFlowValue(item))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any:
		// Sorted: Go randomises map order, and a block that reshuffled between
		// turns would rewrite the cached prefix for nothing.
		parts := make([]string, 0, len(v))
		for _, name := range slices.Sorted(maps.Keys(v)) {
			parts = append(parts, yamlFlowScalar(name)+": "+yamlFlowValue(v[name]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case string:
		return yamlFlowScalar(v)
	default:
		return yamlFlowScalar(fmt.Sprintf("%v", v))
	}
}

// yamlFlowScalar quotes anything that is not unambiguously a plain scalar.
//
// The test is an allowlist rather than a list of troublesome characters, because
// guessing which punctuation matters is exactly how the comma in "the cargo, no
// witnesses" and the colon in "he said: run" got through and left the block
// unparseable. Interior spaces are fine — a plain scalar may hold them — so most
// prose still renders unquoted; anything else pays two characters and is right.
func yamlFlowScalar(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	plain := s != "" && strings.TrimSpace(s) == s && strings.IndexFunc(s, func(r rune) bool {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return false
		case r == '-', r == '_', r == '.', r == '+', r == '/', r == ' ':
			return false
		}
		return true
	}) < 0
	if plain {
		return s
	}
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

// writeTraits renders only the traits that are not at the level the rules give
// them by default — Fair for an attribute, Poor for an untrained skill.
//
// A cast of NPCs is mostly default by construction: a bystander is Fair at
// everything and Poor at every skill, and spelling that out cost two full lines
// each, every turn, to say nothing. What is left is what actually distinguishes
// the character, which is the reading the DM wanted from these lines anyway.
// The world still holds every trait, so a check may name one that is not
// printed and it resolves at its default exactly as before.
func writeTraits(b *strings.Builder, label string, traits map[string]int, def int) {
	parts := make([]string, 0, len(traits))
	for _, name := range slices.Sorted(maps.Keys(traits)) {
		if traits[name] == def {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %+d", name, traits[name]))
	}
	if len(parts) == 0 {
		return
	}
	fmt.Fprintf(b, "    %s: {%s}\n", label, strings.Join(parts, ", "))
}

// characterOrder puts the player first and everybody else in id order.
func characterOrder(w *rules.World) []string {
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
	return ids
}

// locationOrder walks the map from its roots so a place reads next to the places
// around it.
//
// An orphan — a location whose parent is somehow missing — is appended rather
// than lost. Nothing should reach that: a parent must exist before its child is
// created and locations are never deleted. But this render is the DM's only view
// of what slugs exist, and a slug it cannot see is a slug it will be refused for
// naming.
func locationOrder(w *rules.World) []*rules.Location {
	out := make([]*rules.Location, 0, len(w.Locations))
	seen := map[string]bool{}
	var walk func(parent string)
	walk = func(parent string) {
		for _, loc := range w.Children(parent) {
			seen[loc.ID] = true
			out = append(out, loc)
			walk(loc.ID)
		}
	}
	walk("")
	for _, id := range w.LocationIDs() {
		if !seen[id] {
			out = append(out, w.Locations[id])
		}
	}
	return out
}

// heldItem is one item and the id of whatever holds it, which is the pair the
// flat render needs and neither the character, the location nor a container
// item knows on its own.
type heldItem struct {
	id     string
	holder string
	item   *rules.Item
}

// itemOrder lists every item in the world in a fixed order: by holder, then by
// id, walking into a container's own contents wherever one is found — so a
// coin in a pocket in a carried jacket still renders once, flat, alongside
// everything else. The top level follows characterOrder and locationOrder
// (player first, then tree order) rather than World.WalkItems' own plain
// alphabetical scan, because this order is what the rest of the block already
// renders characters and locations in, and a mismatch would rewrite the
// cached prompt prefix for nothing.
func itemOrder(w *rules.World) []heldItem {
	var out []heldItem
	var walk func(holder rules.Holder)
	walk = func(holder rules.Holder) {
		for _, it := range holder.SortedItems() {
			out = append(out, heldItem{id: it.ID, holder: holder.HolderID(), item: it})
			if it.IsContainer() {
				walk(it)
			}
		}
	}
	for _, id := range characterOrder(w) {
		walk(w.Characters[id])
	}
	for _, loc := range locationOrder(w) {
		walk(loc)
	}
	return out
}

// carriedIDs lists the ids of what a character directly carries, in a fixed
// order. Nested items — inside a container the character carries — are not
// here: they render under that container's own `contents`, one level down,
// the same as the character's own inventory renders one level down from
// nothing.
func carriedIDs(c *rules.Character) []string {
	return slices.Sorted(maps.Keys(c.Inventory))
}

// containedIDs lists the ids directly inside a container item, in a fixed
// order — the same derived, read-only view carriedIDs gives a character.
func containedIDs(it *rules.Item) []string {
	return slices.Sorted(maps.Keys(it.Items))
}

// yamlString quotes a value only when leaving it bare would change what the YAML
// means. The DM's prose arrives here — descriptions, the spot a thing rests in —
// and it contains colons, hashes and leading quotes often enough to matter, but
// quoting everything would add two characters to most lines in the block for
// nothing.
func yamlString(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	needsQuote := s == "" ||
		strings.ContainsAny(s, ":#'\"[]{}|>&*!%@`,") ||
		strings.TrimSpace(s) != s ||
		strings.HasPrefix(s, "-")
	if !needsQuote {
		return s
	}
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

// resolved names the operations whose outcome nothing else in the block can
// show: what the dice said, what a table gave, what a blow came to. Everything
// left out of it is bookkeeping — a thing created, moved, renamed, re-rated —
// and bookkeeping is exactly what the entity list above already states, in its
// current form, under the id. Repeating it here said the same fact twice a turn,
// and the copy in the timeline was the stale one.
//
// An allowlist rather than a list of what to drop, so a bookkeeping operation
// added later stays out of the block by default instead of quietly appearing in
// it. Nothing is lost either way: the archive keeps every event, and `recall`
// reaches them all.
var resolved = map[string]bool{
	"unopposed_check":  true,
	"opposed_check":    true,
	"opposed_attack":   true,
	"unopposed_attack": true,
	"roll_on_table":    true,
	"reload_weapon":    true,
	"apply_damage":     true,
	"heal":             true,
	"declare_dead":     true,
}

// timeline renders the recent past as facts: what was typed and what the engine
// resolved, one turn after another with nothing between them. The narration that
// went with it is recentProse's, below, and keeping the two apart is what lets
// this be skimmed — it is a chronology, and a chronology broken up by paragraphs
// of prose is one nobody reads to the end.
func timeline(a *archive.Archive) string {
	turns := a.Recent(timelineWindow)
	if len(turns) == 0 {
		return "(play has not started)"
	}

	var b strings.Builder
	for _, t := range turns {
		fmt.Fprintf(&b, "t%d player: %s\n", t.Number, t.PlayerInput)
		for _, event := range t.Events {
			if event.Headline == "" || !resolved[event.Tool] {
				continue
			}
			fmt.Fprintf(&b, "    %s: %s\n", event.Tool, event.Headline)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// recentProse renders the newest turns' narration verbatim, oldest first. The
// window is counted from the end of the archive so it slides with play rather
// than sticking to whichever turns happen to be first in this render.
func recentProse(a *archive.Archive) string {
	turns := a.Recent(proseWindow)

	var b strings.Builder
	for _, t := range turns {
		if t.Prose == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "t%d: %s", t.Number, t.Prose)
	}
	return b.String()
}
