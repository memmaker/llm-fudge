package rules

import (
	"fmt"
	"sort"
	"strings"
)

// Location is a place the world holds: a room, a clearing, a ship's hold. It
// exists so that an item can be somewhere other than in somebody's hands, which
// is what lets an item be conserved rather than created and destroyed as the
// fiction moves it about.
//
// Description is the place's identity — what it permanently is — and is written
// once, when the DM invents it. What has since *changed* about a place (the door
// barred, the fire lit, the body in the corner) is not here: that is the world
// ledger's, and splitting them this way is what keeps one fact from having two
// homes that age out of step.
//
// Parent nests locations for the DM's orientation and so the block can render a
// map without spelling out every room every turn. It carries no reach: an item
// is taken from the exact location a character stands in, never from the room
// next door or the building around it, because "at the tavern" does not say
// whether that is the taproom or the landing and the engine must not pretend it
// knows.
type Location struct {
	ID          string
	Name        string
	Description string
	Parent      string
	// Items is what lies here, keyed by each item's id. Two daggers in two
	// corners are two entries; one pile of arrows is one — the spot is still what
	// decides whether an arriving stack merges, it is simply no longer half of the
	// key. It used to be, and that was the bug: a key built out of where a thing
	// currently is changes the moment the thing moves, so nothing connected what
	// left one key to what appeared under another.
	Items map[string]*Item
}

// CreateLocation mints a location from its name, the same way a character is
// minted, so the DM reads an id off the result rather than deriving one.
//
// A parent must already exist. That requirement is also the whole of the cycle
// check: a location can only point at one created before it, so the parent
// chain is acyclic by construction and nothing has to walk it.
func (w *World) CreateLocation(name, description, parent string) (*Location, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("location name is required")
	}
	id := slug(name)
	if !slugPattern.MatchString(id) {
		return nil, fmt.Errorf("invalid location slug: %s", id)
	}
	if w.Locations == nil {
		w.Locations = make(map[string]*Location)
	}
	if _, exists := w.Locations[id]; exists {
		return nil, fmt.Errorf("location already exists: %s — use that id rather than creating a second one", id)
	}
	// And not taken by anything else either. One id means one entity: a location
	// sharing an id with a character or an item would render as a single key in the
	// flat entity map, and two different things would reach the DM as one record.
	if w.HasEntity(id) {
		return nil, fmt.Errorf("id already taken: %s — every character, location and item shares one id namespace. Name the place more fully so the id differs", id)
	}
	if parent != "" {
		if _, err := w.Location(parent); err != nil {
			return nil, fmt.Errorf("unknown parent: %w", err)
		}
	}
	loc := &Location{
		ID:          id,
		Name:        name,
		Description: description,
		Parent:      parent,
		Items:       make(map[string]*Item),
	}
	w.Locations[id] = loc
	return loc, nil
}

// Location retrieves a location by id. Like Character, a miss is answered with
// the list of ids that do exist: the DM is one call away from a correct retry
// and should not have to guess a second time.
func (w *World) Location(id string) (*Location, error) {
	loc, exists := w.Locations[id]
	if exists {
		return loc, nil
	}
	if len(w.Locations) == 0 {
		return nil, fmt.Errorf("location not found: %s — no locations exist yet, create it first with create_location", id)
	}
	return nil, fmt.Errorf("location not found: %s — the locations that exist are: %s. Use one of those ids, or create this one first with create_location", id, strings.Join(w.LocationIDs(), ", "))
}

// Scene is where the story currently is: the player's location, because every
// narration states the place it happened in and that is what moves them. It is
// empty before play opens, when no location exists yet.
//
// There is no separate scene field. One would be a second answer to a question
// the player's position already answers, and the two would drift the first time
// only one of them was updated.
func (w *World) Scene() string {
	player, err := w.Character(w.Player)
	if err != nil {
		return ""
	}
	return player.Location
}

// MoveCharacter puts somebody somewhere the engine knows about. The player is
// moved by their narration's location instead; this is for everyone else, who
// arrive, leave, and go about their business off the back of the scene.
func (w *World) MoveCharacter(id, location string) (*Character, error) {
	ch, err := w.Character(id)
	if err != nil {
		return nil, err
	}
	loc, err := w.Location(location)
	if err != nil {
		return nil, err
	}
	ch.Location = loc.ID
	return ch, nil
}

// LocationName is the place's name as a player would hear it, for the sheet
// they read. It falls back to the id, which is all there is to say before the
// first scene has placed anybody.
func (w *World) LocationName(id string) string {
	if loc, exists := w.Locations[id]; exists {
		return loc.Name
	}
	return id
}

// LocationIDs lists every location id, sorted, for the errors and the block.
func (w *World) LocationIDs() []string {
	ids := make([]string, 0, len(w.Locations))
	for id := range w.Locations {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// AddItem puts a stack down at this location. proto supplies the name, the
// wording, any ratings and the id the caller has already minted; an existing stack
// of the same name in the same spot grows instead of a second one opening, and
// keeps its own id — one pile of arrows is one entity, so the arriving id is
// retired rather than carried alongside.
//
// Ratings are the proto's only when the stack is new, matching how a character's
// inventory treats a restock: the entry keeps what it was recorded with, so
// merging a half-empty pistol into a pile of full ones does not silently reload
// anything.
func (l *Location) AddItem(proto *Item, where string, count int) (*Item, error) {
	if proto == nil || strings.TrimSpace(proto.Name) == "" {
		return nil, fmt.Errorf("item name is required")
	}
	if count < 1 {
		return nil, fmt.Errorf("item count must be at least 1")
	}
	where = strings.TrimSpace(where)
	if where == "" {
		return nil, fmt.Errorf("say where in %s the %s comes to rest", l.Name, proto.Name)
	}
	if l.Items == nil {
		l.Items = make(map[string]*Item)
	}
	item := l.stackAt(proto.Name, where)
	if item != nil && item.Kind() != proto.Kind() {
		return nil, fmt.Errorf("%s already holds %s as %s, not as %s", l.Name, item.Name, item.Kind(), proto.Kind())
	}
	if item == nil {
		if proto.ID == "" {
			return nil, fmt.Errorf("no id minted for %s: mint one with MintItemID before putting it down, or an item lands where nothing can address it", proto.Name)
		}
		item = &Item{
			ID:          proto.ID,
			Name:        proto.Name,
			Description: proto.Description,
			Weapon:      proto.Weapon,
			Armor:       proto.Armor,
			Where:       where,
		}
		l.Items[item.ID] = item
	}
	if proto.Description != "" {
		item.Description = proto.Description
	}
	item.Count += count
	return item, nil
}

// stackAt finds the stack of a name lying in a given spot, which is the pair that
// decides whether an arriving stack merges. It is a scan because the id, not the
// pair, is now the key; a room's floor holds few enough entries that looking at
// all of them is cheaper than any index would be.
func (l *Location) stackAt(ref, where string) *Item {
	wantedWhere := slug(where)
	for _, it := range l.SortedItems() {
		if it.refers(ref) && slug(it.Where) == wantedWhere {
			return it
		}
	}
	return nil
}

// RemoveItem takes a stack, or part of one, off the floor. It is the location's
// half of destruction: what leaves this way leaves the world.
func (l *Location) RemoveItem(name, where string, count int) (*Item, int, error) {
	if count < 1 {
		return nil, 0, fmt.Errorf("item count must be at least 1")
	}
	item, err := l.find(name, where)
	if err != nil {
		return nil, 0, err
	}
	if count > item.Count {
		return item, item.Count, fmt.Errorf("%s holds only %d %s", l.Name, item.Count, item.Name)
	}
	item.Count -= count
	if item.Count == 0 {
		delete(l.Items, item.ID)
	}
	return item, item.Count, nil
}

// find resolves a name, and optionally a spot, to one stack lying here. Without
// a spot it will not guess between two stacks of the same name: it says where
// each of them is and lets the caller name the one it meant.
//
// The nameless scan walks SortedItems rather than the map, because map order is
// randomised and this engine is deterministic by design: which of two like-named
// stacks a caller gets, and the order the refusal lists them in, must not depend
// on the run.
func (l *Location) find(name, where string) (*Item, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("item name is required")
	}
	if strings.TrimSpace(where) != "" {
		item := l.stackAt(name, where)
		if item == nil {
			return nil, fmt.Errorf("no %s lies %s in %s", name, where, l.Name)
		}
		return item, nil
	}

	var matches []string
	var found *Item
	for _, item := range l.SortedItems() {
		if !item.refers(name) {
			continue
		}
		matches = append(matches, item.Where)
		found = item
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no %s lies in %s", name, l.Name)
	case 1:
		return found, nil
	}
	sort.Strings(matches)
	return nil, fmt.Errorf("%s holds %d lots of %s: %s. Name the one you mean", l.Name, len(matches), name, strings.Join(matches, "; "))
}

// SortedItems renders the floor in a fixed order — by name, then by spot — so
// the block reads the same way every turn and its cached prefix is not rewritten
// for nothing.
func (l *Location) SortedItems() []*Item {
	items := make([]*Item, 0, len(l.Items))
	for _, it := range l.Items {
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Name != items[j].Name {
			return items[i].Name < items[j].Name
		}
		if items[i].Where != items[j].Where {
			return items[i].Where < items[j].Where
		}
		// Name and spot together are unique per location, so this never decides
		// anything today. It is here because the id is the one key that cannot
		// tie, and a sort that can fall through to "equal" on a randomised map
		// walk is a determinism bug waiting for the invariant above to relax.
		return items[i].ID < items[j].ID
	})
	return items
}

// Children lists the locations nested directly inside this one, sorted, so the
// map renders the same way every turn.
func (w *World) Children(parent string) []*Location {
	var children []*Location
	for _, id := range w.LocationIDs() {
		if w.Locations[id].Parent == parent {
			children = append(children, w.Locations[id])
		}
	}
	return children
}

// CharactersAt lists who is standing in a location, by name, sorted. The dead
// are included: a body in the room is very much part of the scene.
func (w *World) CharactersAt(id string) []string {
	var present []string
	for _, cid := range w.CharacterIDs() {
		if w.Characters[cid].Location == id {
			present = append(present, w.Characters[cid].Name)
		}
	}
	return present
}

// MoveRequest names one move: what, to whom or where, and — only when the name
// alone is ambiguous — from whom or where.
//
// Source and destination are separate fields for characters and locations
// rather than one slug apiece, because both namespaces are slugs and an NPC
// called Mira and a room called Mira would otherwise be indistinguishable at
// exactly the moment it mattered.
type MoveRequest struct {
	Item          string
	FromCharacter string
	FromLocation  string
	FromWhere     string
	ToCharacter   string
	ToLocation    string
	Where         string
	Count         int
}

// MoveResult reports a completed move in the terms the DM needs to narrate it:
// what moved, how many, and the two holders it moved between.
type MoveResult struct {
	Item  *Item
	Count int
	From  string
	To    string
}

// MoveItem is the one way an item changes hands. Picking up, dropping, giving,
// looting, stashing and retrieving a thrown knife are all this call, because
// they are all the same fact: an item is in exactly one place, and that place
// changed.
//
// It moves the thing itself, ratings and wording and all, which is why it beats
// the remove-then-add pair it replaces. A pair can half-succeed and destroy what
// it was supposed to move, and re-adding a weapon means restating a damage
// rating from memory — the DM's memory being the thing the engine exists to
// stop relying on.
func (w *World) MoveItem(req MoveRequest) (*MoveResult, error) {
	if req.ToCharacter != "" && req.ToLocation != "" {
		return nil, fmt.Errorf("name one destination: a character or a location, not both")
	}
	if req.ToCharacter == "" && req.ToLocation == "" {
		return nil, fmt.Errorf("name a destination: to_character or to_location")
	}
	if req.FromCharacter != "" && req.FromLocation != "" {
		return nil, fmt.Errorf("name one source: a character or a location, not both")
	}
	count := req.Count
	if count < 1 {
		count = 1
	}

	source, err := w.resolveSource(req)
	if err != nil {
		return nil, err
	}

	// The destination is checked before anything is taken off the source, so a
	// refusal leaves the world exactly as it was. A move that failed halfway
	// would destroy the item it was meant to relocate, which is the failure this
	// whole call exists to make impossible.
	var toCharacter *Character
	var toLocation *Location
	if req.ToCharacter != "" {
		if toCharacter, err = w.Character(req.ToCharacter); err != nil {
			return nil, err
		}
		if source.character == toCharacter {
			return nil, fmt.Errorf("%s already carries the %s", toCharacter.Name, source.item.Name)
		}
	} else {
		if toLocation, err = w.Location(req.ToLocation); err != nil {
			return nil, err
		}
		if strings.TrimSpace(req.Where) == "" {
			return nil, fmt.Errorf("say where in %s the %s comes to rest", toLocation.Name, source.item.Name)
		}
		if source.location == toLocation && slug(source.item.Where) == slug(req.Where) {
			return nil, fmt.Errorf("the %s already lies %s in %s", source.item.Name, source.item.Where, toLocation.Name)
		}
	}
	if count > source.item.Count {
		return nil, fmt.Errorf("%s has only %d %s", source.name, source.item.Count, source.item.Name)
	}

	// The stack is copied, not handed over: the copy is what lands, and the
	// source keeps whatever is left of the original entry.
	//
	// The ratings are copied by value, not by pointer. A partial move leaves two
	// stacks where there was one, and two stacks sharing a Weapon share a
	// magazine: a shot fired from the pistol on the floor would empty the one
	// still in the holster, and taking off one jerkin would strip whoever was
	// handed the other.
	moved := source.item.clone()

	// Identity follows the whole stack. A thing that moves in one piece is the
	// same thing at the far end and keeps its id — that is what the id is for, and
	// what the old name-and-spot key could not express. A partial move genuinely
	// creates a second thing: two stacks now exist where one did, they will be
	// counted, moved and destroyed separately from here on, and so the portion that
	// left needs its own id while whatever stayed behind keeps the original's.
	//
	// Either id may still be retired a moment from now, if the destination already
	// holds a matching stack; Accept and AddItem decide that, and their answer wins.
	if count == source.item.Count {
		moved.ID = source.item.ID
	} else {
		moved.ID = w.MintItemID(moved.Name)
	}

	var landed *Item
	var destination string
	if toCharacter != nil {
		// A carried thing lies nowhere, so the spot it had on the floor is not
		// carried across.
		if landed, err = toCharacter.Accept(moved, count); err != nil {
			return nil, err
		}
		destination = toCharacter.Name
	} else {
		if landed, err = toLocation.AddItem(moved, req.Where, count); err != nil {
			return nil, err
		}
		destination = toLocation.Name
	}

	if err := source.take(count); err != nil {
		return nil, err
	}
	return &MoveResult{Item: landed, Count: count, From: source.name, To: destination}, nil
}

// itemSource is a resolved holder of one stack: where the move takes from, and
// how to take it.
type itemSource struct {
	item      *Item
	name      string
	character *Character
	location  *Location
}

// take empties the source of what moved. The entry is dropped only if it is still
// the one this source resolved to: a whole stack moved from one spot in a room to
// another spot in the same room keeps its id, so the arriving stack is written into
// the very map slot the departing one occupied — which is the right outcome, and
// exactly why the emptied original must not then delete its own replacement.
func (s *itemSource) take(count int) error {
	if s.character != nil {
		_, _, err := s.character.RemoveItem(s.item.Name, count)
		return err
	}
	s.item.Count -= count
	if s.item.Count == 0 && s.location.Items[s.item.ID] == s.item {
		delete(s.location.Items, s.item.ID)
	}
	return nil
}

// resolveSource finds the stack a move is about. A stated source settles it;
// without one the world is searched, and a name in more than one place is
// refused with every holder listed rather than guessed at.
func (w *World) resolveSource(req MoveRequest) (*itemSource, error) {
	if strings.TrimSpace(req.Item) == "" {
		return nil, fmt.Errorf("name the item to move")
	}
	switch {
	case req.FromCharacter != "":
		ch, err := w.Character(req.FromCharacter)
		if err != nil {
			return nil, err
		}
		item := ch.carried(req.Item)
		if item == nil {
			return nil, fmt.Errorf("%s does not carry any %s", ch.Name, req.Item)
		}
		return &itemSource{item: item, name: ch.Name, character: ch}, nil
	case req.FromLocation != "":
		loc, err := w.Location(req.FromLocation)
		if err != nil {
			return nil, err
		}
		item, err := loc.find(req.Item, req.FromWhere)
		if err != nil {
			return nil, err
		}
		return &itemSource{item: item, name: loc.Name, location: loc}, nil
	}

	sources := w.findItem(req.Item)
	switch len(sources) {
	case 0:
		return nil, fmt.Errorf("nothing called %s is anywhere in the world: nobody carries one and none lies about. Create it with add_item if it is new", req.Item)
	case 1:
		return sources[0], nil
	}
	held := make([]string, 0, len(sources))
	for _, s := range sources {
		if s.character != nil {
			held = append(held, fmt.Sprintf("%s carries %d", s.character.ID, s.item.Count))
			continue
		}
		held = append(held, fmt.Sprintf("%d lies %s in %s", s.item.Count, s.item.Where, s.location.ID))
	}
	sort.Strings(held)
	return nil, fmt.Errorf("more than one %s exists: %s. Name from_character or from_location to say which", req.Item, strings.Join(held, "; "))
}

// findItem lists every stack of a name anywhere in the world, characters first
// and then locations, each in a fixed order so an ambiguity error reads the same
// way twice.
func (w *World) findItem(ref string) []*itemSource {
	// An id is unique across the whole world, so a reference that is one resolves
	// to exactly one holder and never reaches the ambiguity refusal.
	var sources []*itemSource
	for _, id := range w.CharacterIDs() {
		ch := w.Characters[id]
		if item := ch.carried(ref); item != nil {
			sources = append(sources, &itemSource{item: item, name: ch.Name, character: ch})
		}
	}
	for _, id := range w.LocationIDs() {
		loc := w.Locations[id]
		for _, item := range loc.SortedItems() {
			if !item.refers(ref) {
				continue
			}
			sources = append(sources, &itemSource{item: item, name: loc.Name, location: loc})
		}
	}
	return sources
}
