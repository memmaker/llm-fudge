package main

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"fudge/archive"
	"fudge/narrative"
	"fudge/rules"
)

// The state block is the DM's account of what its prose left true, written in
// the same shape it reads the world in. It states facts, not instructions: the
// engine works out which operation each changed field corresponds to and runs
// exactly the rules that operation always ran. `location: gareth` on an item is
// still MoveItem, with conservation, stack splitting and id minting intact — the
// DM simply no longer has to know that.
//
// What it cannot say is anything whose end state would under-determine the rules
// outcome. Health is the whole of that category: a scratch on a twice-scratched
// character is Hurt and on a fresh one is a Scratch, so naming the resulting
// level means performing the wound rules. `wounds` and `scratches` are therefore
// not fields here at all — they move through the event tags in the prose — and
// because they cannot be written, they cannot be written wrongly.

// worldDiff is one state block. Entities are keyed by id and merged into the
// world; presence in the world is what decides between creating and updating, so
// the DM never states which it meant.
type worldDiff struct {
	Entities map[string]map[string]any `yaml:"entities"`
	// Delete retires narrative-only entities whose thread is finished. Engine
	// entities are refused here: an item stops existing by reaching count 0, and
	// a character never stops existing at all.
	Delete []string `yaml:"delete"`
}

// writableFields is the closed field set, per type. A key that is not in it is
// refused rather than merged, which is what stops the DM growing a second,
// competing vocabulary for a fact the engine already holds — `current_room`
// beside `location` — and then reading its own invention back as true. A
// denylist could only ever refuse the synonyms somebody thought to list first.
var writableFields = map[string]map[string]bool{
	"character": {"type": true, "name": true, "species": true, "desc": true, "location": true, "coins": true, "attributes": true, "skills": true, "gifts": true, "faults": true, "narrative": true},
	"item":      {"type": true, "name": true, "desc": true, "container": true, "count": true, "spot": true, "weapon": true, "armor": true, "narrative": true},
	// A container is an item that can itself hold items — same fields, one
	// more capability. See newKind/mint and mintItem's container branch.
	"container": {"type": true, "name": true, "desc": true, "container": true, "count": true, "spot": true, "weapon": true, "armor": true, "narrative": true},
	"location":  {"type": true, "name": true, "desc": true, "parent": true, "narrative": true},
	"narrative": {"type": true, "narrative": true},
}

// derivedFields are rendered in the block but stored nowhere: each is a second
// view of a fact that lives on another entity. They get a refusal of their own
// rather than the generic unknown-field one, because the DM writing `inventory`
// has not invented a word — it has mistaken a view for a fact, and the fix is to
// state the fact instead.
var derivedFields = map[string]string{
	"inventory":    "an inventory follows from each item's own `container` — move the item instead",
	"contents":     "a container's contents follow from each item's own `container` — move the item instead",
	"here":         "who is present follows from each character's own `location` — move the character instead",
	"scene":        "the scene follows from where the player character stands",
	"penalty":      "a wound penalty follows from the wound",
	"cannot_act":   "whether somebody can act follows from the wound",
	"wounds":       "health is not written here: mark it in the prose with <damage target=\"id\" severity=\"...\"/>, <heal target=\"id\" levels=\"n\"/> or <dead target=\"id\"/>",
	"scratches":    "health is not written here: mark it in the prose with <damage target=\"id\" severity=\"...\"/>",
	"fudge_points": "fudge points are spent through the checks that spend them",
	"scale":        "scale follows from species",
	"id":           "an entity's id is the key it is written under",
}

// applyState folds one state block into the world, and reports what it refused.
//
// Nothing here returns an error. The prose has already reached the player, so a
// refusal is a note the DM is asked to correct in one follow-up round, not a
// reason to lose the facts that were right. This is the rule the narrative store
// has always followed, applied to the whole block.
func (s *session) applyState(raw string) ([]archive.Event, []string) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}

	var diff worldDiff
	decoder := yaml.NewDecoder(strings.NewReader(raw))
	// Closed at the top level by the decoder, and per entity by writableFields
	// below. The two halves are separate because an entity's legal fields depend
	// on its type, which is inside the document rather than known to the decoder.
	decoder.KnownFields(true)
	if err := decoder.Decode(&diff); err != nil {
		return nil, []string{fmt.Sprintf("state: could not read the block (%v). Send it again as YAML with a top-level `entities:` map keyed by id, and an optional `delete:` list.", err)}
	}

	a := &applier{session: s, failed: map[string]bool{}}
	a.run(diff)
	return a.events, a.notes
}

// applyEvents runs the mechanical transitions the prose marked, in the order it
// marked them. Order matters here in a way it does not in the block: two wounds
// in one turn accumulate, and accumulating them in the wrong order is a
// different wound.
//
// These are the changes a declared end state could not express. Everything the
// engine needs is the transition itself — a severity, a number of levels — and
// the resulting condition is the rules library's to work out, which is exactly
// why health has no field in the block.
func (s *session) applyEvents(events []stateEvent) ([]archive.Event, []string) {
	a := &applier{session: s, failed: map[string]bool{}}
	for _, e := range events {
		ch, err := s.world.Character(e.Target)
		if err != nil {
			a.note("%s: %v", e.Kind, err)
			continue
		}
		switch e.Kind {
		case "damage":
			points, err := rules.DamagePointsForSeverity(e.Severity)
			if err != nil {
				a.note("damage on %s: %v", e.Target, err)
				continue
			}
			result := ch.Wounds.ApplyDamage(points)
			a.record("apply_damage", []string{e.Target}, rules.Flow("character", ch.Name, "took", e.Severity, "now", result, "scratches", ch.Wounds.Scratches, "penalty", ch.Wounds.Penalty()))
		case "heal":
			levels := e.Levels
			if levels <= 0 {
				levels = 1
			}
			result := ch.Wounds.Heal(levels)
			a.record("heal", []string{e.Target}, rules.Flow("character", ch.Name, "healed", levels, "now", result, "level", ch.Wounds.Level, "penalty", ch.Wounds.Penalty()))
		case "dead":
			ch.Wounds.DeclareDead()
			a.record("declare_dead", []string{e.Target}, rules.Flow("character", ch.Name, "status", "DEAD"))
		}
	}
	return a.events, a.notes
}

// applier carries the one thing a two-pass application needs that a single
// function signature cannot: which ids have already failed, so that everything
// depending on them is skipped rather than half-applied.
type applier struct {
	*session
	notes  []string
	events []archive.Event
	// failed holds ids the block asked for and did not get. An entity that
	// refers to one of them is skipped with a note naming the cause, so a
	// refused merchant does not leave the sword he was carrying lying nowhere.
	failed map[string]bool
}

func (a *applier) note(format string, args ...any) {
	a.notes = append(a.notes, fmt.Sprintf(format, args...))
}

// record keeps the archive indexed exactly as the tool calls kept it. The
// applier knows which internal operation each declared change turned out to be,
// so it logs that operation under the same vocabulary recall already searches.
func (a *applier) record(op string, entities []string, headline string) {
	a.events = append(a.events, archive.Event{Tool: op, Entities: entities, Headline: headline})
}

func (a *applier) run(diff worldDiff) {
	ids := slices.Sorted(maps.Keys(diff.Entities))

	// New first, and among the new, in dependency order: a room before the man
	// standing in it, the man before the key in his pocket. Nothing in the block
	// has to be written in that order — this is the pass that makes the order
	// inside the document irrelevant.
	fresh := map[string]bool{}
	var order []string
	for _, id := range ids {
		if !a.exists(id) {
			fresh[id] = true
			order = append(order, id)
		}
	}
	a.create(order, diff.Entities)

	for _, id := range ids {
		fields := diff.Entities[id]
		if fields == nil {
			a.note("state: %s carries no fields, nothing to record", id)
			continue
		}
		if a.failed[id] {
			continue
		}
		// A minted entity has already taken everything its constructor accepts —
		// its name, its place, its ratings, its traits, its purse — so running the
		// full update over it would read every consumed field back as a change to
		// something just set to exactly that. Narrative is the exception, and the
		// one field every kind of entity shares: a narrative-only entity took its
		// properties as it was created, and anything the engine holds still needs
		// them written.
		if fresh[id] {
			if a.engineKind(id) != "" {
				a.setNarrative(id, fields)
			}
			continue
		}
		a.update(id, fields)
	}

	for _, id := range diff.Delete {
		a.remove(id)
	}
}

// exists reports whether an id is a live entity — in the engine or in the
// narrative store — which is the whole of the create-or-update decision.
//
// A retired id is deliberately not live. It is still taken, and newKind refuses
// it by name, but treating it as existing here would send a block that reused it
// down the update path and answer a spent id with a puzzle about field sets.
func (a *applier) exists(id string) bool {
	return a.engineKind(id) != "" || a.narrative.Has(id)
}

// engineKind names what the rules library holds under an id, and is empty for
// anything it does not hold. It is the distinction HasEntity deliberately does
// not draw: that one answers "is this id taken", across one shared keyspace, and
// a narrative-only entity is taken without being the engine's.
func (a *applier) engineKind(id string) string {
	if _, ok := a.world.Characters[id]; ok {
		return "character"
	}
	if _, ok := a.world.Locations[id]; ok {
		return "location"
	}
	if it, err := a.world.ItemByID(id); err == nil {
		if it.IsContainer() {
			return "container"
		}
		return "item"
	}
	return ""
}

// setNarrative merges an entry's narrative map onto the entity, which is the one
// thing every kind of entity has in common and the only field a creation and an
// update handle identically.
func (a *applier) setNarrative(id string, fields map[string]any) {
	props := a.narrativeProps(id, fields)
	if len(props) == 0 {
		return
	}
	a.applyOps(narrative.Operation{Op: narrative.OpSet, Entity: id, Properties: props})
	a.record("narrative", []string{id}, fmt.Sprintf("set %s %s", id, propSummary(props)))
}

// create mints every new entity in the block, in an order it works out rather
// than one the DM had to write in. A location whose parent is also new, a
// character in that location, an item in that character's hands: each pass
// creates whatever it can, and a pass that creates nothing means what is left is
// waiting on something that will never arrive.
func (a *applier) create(fresh []string, entities map[string]map[string]any) {
	pending := fresh
	for len(pending) > 0 {
		var deferred []string
		progress := false
		for _, id := range pending {
			fields := entities[id]
			kind := a.newKind(id, fields)
			if kind == "" {
				continue // already refused, and noted
			}
			// Waiting on something else in this same block: try again after
			// the pass that creates it.
			if dep, waiting := a.awaiting(kind, fields, entities); waiting {
				if a.failed[dep] {
					a.failed[id] = true
					a.note("state: %s was skipped because %s could not be created", id, dep)
					continue
				}
				deferred = append(deferred, id)
				continue
			}
			a.mint(id, kind, fields)
			progress = true
		}
		if !progress {
			for _, id := range deferred {
				a.failed[id] = true
				a.note("state: %s refers to something that does not exist and was not created here, so it was skipped", id)
			}
			return
		}
		pending = deferred
	}
}

// awaiting reports the id this new entity needs created first, when that id is
// itself new in this block.
func (a *applier) awaiting(kind string, fields map[string]any, entities map[string]map[string]any) (string, bool) {
	var ref string
	switch kind {
	case "location":
		ref, _ = fields["parent"].(string)
	case "character":
		ref, _ = fields["location"].(string)
	case "item", "container":
		ref, _ = fields["container"].(string)
	}
	if ref == "" || a.exists(ref) {
		return "", false
	}
	if _, inBlock := entities[ref]; inBlock {
		return ref, true
	}
	return "", false
}

// newKind settles what a new entity is and whether it may be created at all. A
// new id has to say its type — there is nothing to infer it from — and it has to
// be an id nothing has ever used, including anything retired, so that every
// reference the archive holds keeps meaning what it meant.
func (a *applier) newKind(id string, fields map[string]any) string {
	kind, _ := fields["type"].(string)
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		a.failed[id] = true
		a.note("state: %s is new, so it needs a `type` saying what it is (character, item, container, location, or a narrative kind of your own such as rumor or debt)", id)
		return ""
	}
	if !rules.ValidID(id) {
		a.failed[id] = true
		a.note("state: %q is not a usable id — lowercase words joined by single hyphens, e.g. crooked-lantern", id)
		return ""
	}
	if a.world.Retired[id] {
		a.failed[id] = true
		a.note("state: %s existed earlier in this game and is gone; ids are never reused, so give this one an id of its own", id)
		return ""
	}
	if !a.check(id, kind, fields) {
		return ""
	}
	return kind
}

// mint creates one new entity of a settled kind.
func (a *applier) mint(id, kind string, fields map[string]any) {
	switch kind {
	case "character":
		a.mintCharacter(id, fields)
	case "location":
		a.mintLocation(id, fields)
	case "item":
		a.mintItem(id, fields, false)
	case "container":
		a.mintItem(id, fields, true)
	default:
		// Anything else is narrative-only: a rumour, a debt, a faction's
		// standing. The store mints it under the same keyspace and stamps it
		// with the turn, exactly as it always did.
		props := map[string]any{"type": kind}
		maps.Copy(props, a.narrativeProps(id, fields))
		a.applyOps(narrative.Operation{Op: narrative.OpCreate, Entity: id, Properties: props})
		a.record("create_entity", []string{id}, fmt.Sprintf("created %s %s", kind, id))
	}
}

func (a *applier) mintCharacter(id string, fields map[string]any) {
	f := reader{applier: a, id: id, fields: fields}
	gifts, _ := f.list("gifts")
	faults, _ := f.list("faults")
	req := rules.CharacterRequest{
		ID:          id,
		Name:        f.text("name"),
		Species:     f.text("species"),
		Description: f.text("desc"),
		Location:    f.text("location"),
		Attributes:  f.traits("attributes"),
		Skills:      f.traits("skills"),
		Gifts:       gifts,
		Faults:      faults,
	}
	if req.Name == "" {
		req.Name = id
	}
	ch, err := a.world.CreateCharacterValidated(req)
	if err != nil {
		a.failed[id] = true
		a.note("state: %s could not be created: %v", id, err)
		return
	}
	// Everybody is created broke, so a purse stated on the same entry is the one
	// field left over: the constructor does not take it, and it is the same
	// transaction here as it would be on any later turn.
	if coins, stated := f.optionalNumber("coins"); stated {
		a.setCoins(ch, coins)
	}
	a.record("create_character", []string{id}, rules.Flow("character", ch.Name, "id", ch.ID, "at", ch.Location))
}

func (a *applier) mintLocation(id string, fields map[string]any) {
	f := reader{applier: a, id: id, fields: fields}
	name := f.text("name")
	if name == "" {
		name = id
	}
	loc, err := a.world.CreateLocationWithID(id, name, f.text("desc"), f.text("parent"))
	if err != nil {
		a.failed[id] = true
		a.note("state: %s could not be created: %v", id, err)
		return
	}
	a.record("create_location", []string{id}, rules.Flow("location", loc.Name, "id", loc.ID, "in", loc.Parent))
}

func (a *applier) mintItem(id string, fields map[string]any, container bool) {
	f := reader{applier: a, id: id, fields: fields}
	name := f.text("name")
	if name == "" {
		name = id
	}
	holderID := f.text("container")
	if holderID == "" {
		a.failed[id] = true
		a.note("state: %s is a new item and needs a `container`: the id of whoever holds it or the place it lies", id)
		return
	}
	holder, err := a.world.ResolveHolder(holderID)
	if err != nil {
		a.failed[id] = true
		a.note("state: %s names no character, location or container as its `container`: %v", id, err)
		return
	}

	proto, err := a.proto(name, f.text("desc"), fields)
	if err != nil {
		a.failed[id] = true
		a.note("state: %s could not be created: %v", id, err)
		return
	}
	proto.ID = id
	if container {
		proto.Items = map[string]*rules.Item{}
	}
	count := f.count()

	spot := f.text("spot")
	if spot == "" && holder.RequiresSpot() {
		a.failed[id] = true
		a.note("state: %s lies in %s, so it needs a `spot` saying where in it the thing rests", id, holderID)
		return
	}

	landed, err := holder.AddItem(proto, spot, count)
	if err != nil {
		a.failed[id] = true
		a.note("state: %s could not be created: %v", id, err)
		return
	}
	if a.absorbed(id, landed, holderID) {
		return
	}
	a.record("add_item", []string{id, holderID}, rules.Flow("item", name, "id", id, "in", holderID, "count", count))
}

// absorbed reports that a new item landed on a stack of the same name that was
// already there, and says so rather than letting the difference pass.
//
// One pile of rations is one entity, so Accept and AddItem grow the existing
// entry and retire the arriving id — which is right, and which means the id the
// block just minted addresses nothing. Left silent, the DM would write that id
// again next turn and be told the item does not exist. The note hands back the
// id the thing actually has, and the entry is marked failed so anything in this
// same block that referred to the minted one is skipped rather than landing
// somewhere unintended.
func (a *applier) absorbed(id string, landed *rules.Item, holder string) bool {
	if landed.ID == id {
		return false
	}
	a.failed[id] = true
	a.note("state: %s is already there as %s, so the two are one stack and it now holds %d. Use %s from here on — the id you minted addresses nothing", landed.Name, landed.ID, landed.Count, landed.ID)
	a.record("add_item", []string{landed.ID, holder}, rules.Flow("item", landed.Name, "id", landed.ID, "count", landed.Count))
	return true
}

// proto builds the item a new entry starts from, rated as a weapon or a piece of
// armor when the block says so. The ratings go through the same constructors the
// tools used, so an item created by the block fights and protects identically to
// one created by a call.
func (a *applier) proto(name, description string, fields map[string]any) (*rules.Item, error) {
	f := reader{applier: a, id: name, fields: fields}
	if w, ok := f.submap("weapon"); ok {
		wf := reader{applier: a, id: name, fields: w}
		damage, _ := wf.optionalNumber("damage")
		ranged, _ := wf.optionalFlag("ranged")
		ammo, _ := wf.optionalNumber("ammo")
		capacity, _ := wf.optionalNumber("capacity")
		ammoItem := wf.text("takes")
		if ammoItem != "" && !rules.ValidID(ammoItem) {
			return nil, fmt.Errorf("ammo item %q is not a valid id — lowercase words joined by single hyphens, e.g. revolver-rounds", ammoItem)
		}
		return rules.NewWeaponItem(name, description, rules.Weapon{
			Damage:          damage,
			Ranged:          ranged,
			DamageAttribute: wf.text("adds"),
			Ammo:            ammo,
			AmmoCapacity:    capacity,
			AmmoItem:        ammoItem,
		})
	}
	if ar, ok := f.submap("armor"); ok {
		af := reader{applier: a, id: name, fields: ar}
		protection, _ := af.optionalNumber("protection")
		worn, _ := af.optionalFlag("worn")
		return rules.NewArmorItem(name, description, rules.Armor{Protection: protection, Worn: worn})
	}
	return &rules.Item{Name: name, Description: description}, nil
}

// update folds a block entry into an entity that already exists. Absent fields
// are unchanged, which is what makes an ordinary turn's block three lines long.
func (a *applier) update(id string, fields map[string]any) {
	kind := a.kindOf(id)
	if !a.check(id, kind, fields) {
		return
	}
	if t, stated := fields["type"].(string); stated && strings.ToLower(t) != kind && kind != "narrative" {
		a.note("state: %s is a %s and stays one; `type` cannot be changed", id, kind)
	}
	switch kind {
	case "character":
		a.updateCharacter(id, fields)
	case "location":
		a.updateLocation(id, fields)
	case "item", "container":
		a.updateItem(id, fields)
	}
	a.setNarrative(id, fields)
}

func (a *applier) updateCharacter(id string, fields map[string]any) {
	ch, err := a.world.Character(id)
	if err != nil {
		a.note("state: %s could not be updated: %v", id, err)
		return
	}
	f := reader{applier: a, id: id, fields: fields}

	if name := f.text("name"); name != "" {
		ch.Name = name
	}
	if desc := f.text("desc"); desc != "" {
		ch.Description = desc
	}
	if _, stated := fields["species"]; stated {
		a.note("state: %s keeps the species they were created with — it settles their scale, and changing one without the other would leave the two disagreeing", id)
	}
	if to := f.text("location"); to != "" && to != ch.Location {
		if a.skipRef(id, to) {
			return
		}
		if _, err := a.world.MoveCharacter(id, to); err != nil {
			a.note("state: %s could not be moved: %v", id, err)
		} else {
			a.record("move_character", []string{id, to}, rules.Flow("character", ch.Name, "now_at", to))
		}
	}
	if coins, stated := f.optionalNumber("coins"); stated {
		a.setCoins(ch, coins)
	}
	mergeTraits(a, ch, "attribute", ch.Attributes, a.world.Setting.Attributes, f.traits("attributes"))
	mergeTraits(a, ch, "skill", ch.Skills, a.world.Setting.Skills, f.traits("skills"))
	if stated, ok := f.list("gifts"); ok {
		if gifts, ok := validNames(a, id, "gift", stated, a.world.Setting.Gifts); ok {
			ch.Gifts = gifts
		}
	}
	if stated, ok := f.list("faults"); ok {
		if faults, ok := validNames(a, id, "fault", stated, a.world.Setting.Faults); ok {
			ch.Faults = faults
		}
	}
}

// setCoins states a purse, and the engine works out the payment. A declared
// total below zero is refused by the same check that always refused it.
func (a *applier) setCoins(ch *rules.Character, target int) {
	delta := target - ch.Coins
	if delta == 0 {
		return
	}
	var err error
	if delta > 0 {
		_, err = ch.AddCoins(delta)
	} else {
		_, err = ch.RemoveCoins(-delta)
	}
	if err != nil {
		a.note("state: %s's coins could not be set to %d: %v", ch.ID, target, err)
		return
	}
	a.record("coins", []string{ch.ID}, rules.Flow("character", ch.Name, "coins", ch.Coins, "changed_by", delta))
}

// mergeTraits writes the traits the block stated and leaves the rest alone. A
// trait the setting does not have is refused rather than invented: the sheet the
// dice read from has to hold only traits the rules can roll.
func mergeTraits[T any](a *applier, ch *rules.Character, label string, sheet map[string]int, known map[string]*T, stated map[string]int) {
	for _, key := range slices.Sorted(maps.Keys(stated)) {
		value := stated[key]
		if _, ok := known[key]; !ok {
			a.note("state: %s has no %s %q in this setting", ch.ID, label, key)
			continue
		}
		if value < -3 || value > 3 {
			a.note("state: %s's %s must be between -3 and 3", ch.ID, key)
			continue
		}
		sheet[key] = value
		a.record("trait", []string{ch.ID}, rules.Flow("character", ch.Name, label, key, "now", value))
	}
}

// validNames checks a replacement gift or fault list against the setting. The
// whole list is refused rather than the bad entry dropped: a list replaces, so a
// partly-accepted one would silently write something the DM never stated.
func validNames[T any](a *applier, id, label string, stated []string, known map[string]*T) ([]string, bool) {
	out := make([]string, 0, len(stated))
	for _, name := range stated {
		key := strings.ToLower(strings.TrimSpace(name))
		if _, ok := known[key]; !ok {
			a.note("state: %s has no %s %q in this setting, so the list was left as it was", id, label, name)
			return nil, false
		}
		out = append(out, key)
	}
	return out, true
}

func (a *applier) updateLocation(id string, fields map[string]any) {
	loc, err := a.world.Location(id)
	if err != nil {
		a.note("state: %s could not be updated: %v", id, err)
		return
	}
	f := reader{applier: a, id: id, fields: fields}
	if name := f.text("name"); name != "" {
		loc.Name = name
	}
	if desc := f.text("desc"); desc != "" {
		loc.Description = desc
	}
	if parent := f.text("parent"); parent != "" && parent != loc.Parent {
		if a.skipRef(id, parent) {
			return
		}
		if _, err := a.world.Location(parent); err != nil {
			a.note("state: %s's parent %v", id, err)
			return
		}
		loc.Parent = parent
		a.record("move_location", []string{id, parent}, rules.Flow("location", loc.Name, "now_in", parent))
	}
}

func (a *applier) updateItem(id string, fields map[string]any) {
	item, err := a.world.ItemByID(id)
	if err != nil {
		a.note("state: %s could not be updated: %v", id, err)
		return
	}
	f := reader{applier: a, id: id, fields: fields}
	holder, _ := a.world.ItemHolder(id)

	if name := f.text("name"); name != "" {
		item.Name = name
	}
	if desc := f.text("desc"); desc != "" {
		item.Description = desc
	}
	if w, ok := f.submap("weapon"); ok {
		a.rateWeapon(item, w)
	}
	if ar, ok := f.submap("armor"); ok {
		a.rateArmor(item, ar)
	}

	// Position before quantity: a stack that moved and then ran out should end
	// where the prose left it, and a stack that ran out has nowhere to move to.
	if to := f.text("container"); to != "" && to != holder {
		if a.skipRef(id, to) {
			return
		}
		if !a.moveItem(id, item, to, f.text("spot")) {
			return
		}
		holder = to
	} else if spot := f.text("spot"); spot != "" {
		item.Where = spot
	}

	if count, stated := f.optionalNumber("count"); stated {
		a.setCount(id, holder, count)
	}
}

func (a *applier) moveItem(id string, item *rules.Item, to, spot string) bool {
	res, err := a.world.MoveItem(rules.MoveRequest{Item: id, Count: item.Count, Where: spot, To: to})
	if err != nil {
		a.note("state: %s could not be moved to %s: %v", id, to, err)
		return false
	}
	a.record("move_item", []string{id, to}, rules.Flow("moved", res.Count, "item", res.Item.Name, "from", res.From, "to", res.To))
	return true
}

// setCount states how many there are, and zero is how a thing stops existing.
// It retires rather than deletes: the id stays spent, so every reference the
// archive holds to a quiver shot empty still resolves to the quiver.
func (a *applier) setCount(id, holder string, target int) {
	item, err := a.world.ItemByID(id)
	if err != nil {
		a.note("state: %s's count could not be set: %v", id, err)
		return
	}
	if target < 0 {
		a.note("state: %s cannot have a count below zero", id)
		return
	}
	if target == item.Count {
		return
	}
	if target > 0 {
		before := item.Count
		item.Count = target
		a.record("count", []string{id}, rules.Flow("item", item.Name, "count", target, "was", before))
		return
	}

	if item.IsContainer() && len(item.Items) > 0 {
		a.note("state: %s still holds something — empty it first, or its contents would be lost with no record of where they went", id)
		return
	}
	if h, err := a.world.ResolveHolder(holder); err == nil {
		if _, _, err := h.RemoveItem(item.Name, item.Where, item.Count); err != nil {
			a.note("state: %s's count could not be set: %v", id, err)
			return
		}
	}
	a.world.Retire(id)
	a.record("remove_item", []string{id}, rules.Flow("item", item.Name, "gone", true))
}

func (a *applier) rateWeapon(item *rules.Item, stated map[string]any) {
	if item.Armor != nil {
		a.note("state: %s is armor and cannot also be a weapon", item.ID)
		return
	}
	if item.Weapon == nil {
		item.Weapon = &rules.Weapon{}
	}
	f := reader{applier: a, id: item.ID, fields: stated}
	if v, ok := f.optionalNumber("damage"); ok {
		item.Weapon.Damage = v
	}
	if v, ok := f.optionalFlag("ranged"); ok {
		item.Weapon.Ranged = v
	}
	if v := f.text("adds"); v != "" {
		item.Weapon.DamageAttribute = v
	}
	if v, ok := f.optionalNumber("capacity"); ok {
		item.Weapon.AmmoCapacity = v
	}
	if v, ok := f.optionalNumber("ammo"); ok {
		if item.Weapon.AmmoCapacity > 0 && v > item.Weapon.AmmoCapacity {
			a.note("state: %s holds %d, not %d", item.ID, item.Weapon.AmmoCapacity, v)
		} else {
			item.Weapon.Ammo = v
		}
	}
	if v := f.text("takes"); v != "" {
		if !rules.ValidID(v) {
			a.note("state: %s's ammo item %q is not a valid id — lowercase words joined by single hyphens, e.g. revolver-rounds", item.ID, v)
		} else {
			item.Weapon.AmmoItem = v
		}
	}
	a.record("weapon", []string{item.ID}, rules.Flow("item", item.Name, "damage", item.Weapon.Damage))
}

func (a *applier) rateArmor(item *rules.Item, stated map[string]any) {
	if item.Weapon != nil {
		a.note("state: %s is a weapon and cannot also be armor", item.ID)
		return
	}
	if item.Armor == nil {
		item.Armor = &rules.Armor{}
	}
	f := reader{applier: a, id: item.ID, fields: stated}
	if v, ok := f.optionalNumber("protection"); ok {
		item.Armor.Protection = v
	}
	if v, ok := f.optionalFlag("worn"); ok {
		item.Armor.Worn = v
	}
	a.record("armor", []string{item.ID}, rules.Flow("item", item.Name, "protection", item.Armor.Protection, "worn", item.Armor.Worn))
}

// remove retires a narrative-only entity. Engine entities are refused, and the
// refusal says what to do instead, because a thing the engine holds always has a
// truthful end state to declare — a count of zero, or a death.
func (a *applier) remove(id string) {
	if a.engineKind(id) != "" {
		a.note("state: %s is the engine's and cannot be deleted — an item ends at `count: 0`, and a dead character stays in the world where they can still be looted and spoken of", id)
		return
	}
	a.applyOps(narrative.Operation{Op: narrative.OpDelete, Entity: id})
	a.world.Retire(id)
	a.record("delete_entity", []string{id}, "deleted "+id)
}

// narrativeProps pulls the one open namespace out of an entry. Everything else
// the DM may write is a known field of a known type; this is where a mood, a
// debt, a rumour's content and a sword's cosmetic ruin all live.
func (a *applier) narrativeProps(id string, fields map[string]any) map[string]any {
	raw, stated := fields["narrative"]
	if !stated {
		return nil
	}
	props, ok := raw.(map[string]any)
	if !ok {
		a.note("state: %s's `narrative` must be a map of facts, e.g. {mood: frightened, wants: the debt paid}", id)
		return nil
	}
	return props
}

func (a *applier) applyOps(ops ...narrative.Operation) {
	a.notes = append(a.notes, a.narrative.Apply(ops, a.turn, a.world)...)
}

// check refuses a field the type does not have, and says why in the terms the
// mistake was made in: a derived field is answered with the fact to state
// instead, and anything else with the fields that do exist.
func (a *applier) check(id, kind string, fields map[string]any) bool {
	allowed, known := writableFields[kind]
	if !known {
		allowed = writableFields["narrative"]
	}
	ok := true
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		if allowed[key] {
			continue
		}
		if why, derived := derivedFields[key]; derived {
			a.note("state: %s: %q is not written — %s", id, key, why)
			ok = false
			continue
		}
		a.note("state: %s: %q is not a field of a %s. Its fields are: %s", id, key, kind, strings.Join(slices.Sorted(maps.Keys(allowed)), ", "))
		ok = false
	}
	return ok
}

// skipRef stops an entity being written against a reference this block asked for
// and did not get.
func (a *applier) skipRef(id, ref string) bool {
	if !a.failed[ref] {
		return false
	}
	a.note("state: %s was left alone because %s could not be created", id, ref)
	return true
}

// kindOf names what an existing entity is, so its field set can be checked.
// Anything the engine does not hold is narrative-only.
func (a *applier) kindOf(id string) string {
	if kind := a.engineKind(id); kind != "" {
		return kind
	}
	return "narrative"
}

func propSummary(props map[string]any) string {
	parts := make([]string, 0, len(props))
	for _, name := range slices.Sorted(maps.Keys(props)) {
		parts = append(parts, fmt.Sprintf("%s: %v", name, props[name]))
	}
	return strings.Join(parts, ", ")
}

// reader pulls typed values out of one entity's fields, noting a value of the
// wrong shape rather than guessing at it. YAML gives back `any`, and a `count`
// that arrived as the word "three" has to be reported to be corrected.
type reader struct {
	*applier
	id     string
	fields map[string]any
}

func (r reader) text(key string) string {
	raw, stated := r.fields[key]
	if !stated || raw == nil {
		return ""
	}
	s, ok := raw.(string)
	if !ok {
		r.note("state: %s's %q must be text", r.id, key)
		return ""
	}
	return strings.TrimSpace(s)
}

func (r reader) optionalNumber(key string) (int, bool) {
	raw, stated := r.fields[key]
	if !stated || raw == nil {
		return 0, false
	}
	n, ok := raw.(int)
	if !ok {
		r.note("state: %s's %q must be a whole number", r.id, key)
		return 0, false
	}
	return n, true
}

func (r reader) optionalFlag(key string) (bool, bool) {
	raw, stated := r.fields[key]
	if !stated || raw == nil {
		return false, false
	}
	b, ok := raw.(bool)
	if !ok {
		r.note("state: %s's %q must be true or false", r.id, key)
		return false, false
	}
	return b, true
}

// count defaults to one, which is what a thing created without a number is.
func (r reader) count() int {
	if n, stated := r.optionalNumber("count"); stated && n > 0 {
		return n
	}
	return 1
}

// list reads a replacement list, and reports whether it read one at all. The
// second return is what keeps a refusal from landing as an empty list: a
// caller that cannot tell "no list" from "the empty list" writes `[]` onto the
// sheet every time it refuses something.
func (r reader) list(key string) ([]string, bool) {
	raw, stated := r.fields[key]
	if !stated {
		return nil, false
	}
	// null removes a key from a map and means nothing here: on a list it could
	// equally read as emptied, unset or left alone. It is refused, and the one
	// unambiguous spelling is named instead.
	if raw == nil {
		r.note("state: %s's %q cannot be null — write [] to empty a list, or leave the field out to change nothing", r.id, key)
		return nil, false
	}
	items, ok := raw.([]any)
	if !ok {
		r.note("state: %s's %q must be a list", r.id, key)
		return nil, false
	}
	out := make([]string, 0, len(items))
	for _, v := range items {
		s, ok := v.(string)
		if !ok {
			r.note("state: %s's %q must be a list of names", r.id, key)
			return nil, false
		}
		out = append(out, strings.TrimSpace(s))
	}
	return out, true
}

func (r reader) traits(key string) map[string]int {
	raw, stated := r.fields[key]
	if !stated || raw == nil {
		return nil
	}
	entries, ok := raw.(map[string]any)
	if !ok {
		r.note("state: %s's %q must be a map of trait names to numbers, e.g. {agility: 1}", r.id, key)
		return nil
	}
	out := make(map[string]int, len(entries))
	for name, value := range entries {
		n, ok := value.(int)
		if !ok {
			r.note("state: %s's %s %q must be a whole number between -3 and 3", r.id, key, name)
			continue
		}
		out[strings.ToLower(strings.TrimSpace(name))] = n
	}
	return out
}

func (r reader) submap(key string) (map[string]any, bool) {
	raw, stated := r.fields[key]
	if !stated || raw == nil {
		return nil, false
	}
	m, ok := raw.(map[string]any)
	if !ok {
		r.note("state: %s's %q must be a map of ratings", r.id, key)
		return nil, false
	}
	return m, true
}
