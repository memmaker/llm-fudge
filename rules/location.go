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
	return w.CreateLocationWithID(slug(name), name, description, parent)
}

// CreateLocationWithID is CreateLocation for a caller that already knows the id
// it wants. The state block is that caller: its entities are keyed by id, so the
// id arrives before the name does and is not derived from it. Everything after
// the id is settled is the same, deliberately — a place created by the block and
// a place created by a tool must be the same kind of thing.
func (w *World) CreateLocationWithID(id, name, description, parent string) (*Location, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("location name is required")
	}
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
		return nil, fmt.Errorf("location not found: %s — no locations exist yet, create it in the state block", id)
	}
	return nil, fmt.Errorf("location not found: %s — the locations that exist are: %s. Use one of those ids, or create this one in the state block", id, strings.Join(w.LocationIDs(), ", "))
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

// HolderID, HolderName, RequiresSpot, AddItem, RemoveItem, FindItem and
// SortedItems make a Location a Holder — see holder.go, which is what a
// Character and a container Item share this exact implementation with. A
// location requires a spot because a room's floor is unreadable without one
// ("where in the room does this lie"); a Character or a container does not.
func (l *Location) HolderID() string   { return l.ID }
func (l *Location) HolderName() string { return l.Name }
func (l *Location) RequiresSpot() bool { return true }

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
	if strings.TrimSpace(where) == "" {
		return nil, fmt.Errorf("say where in %s the %s comes to rest", l.Name, proto.Name)
	}
	if l.Items == nil {
		l.Items = make(map[string]*Item)
	}
	return addItem(l.Items, l.Name, proto, where, count)
}

// RemoveItem takes a stack, or part of one, off the floor. It is the location's
// half of destruction: what leaves this way leaves the world.
func (l *Location) RemoveItem(name, where string, count int) (*Item, int, error) {
	return removeItem(l.Items, l.Name, name, where, count)
}

// FindItem resolves a name, and optionally a spot, to one stack lying here.
// Without a spot it will not guess between two stacks of the same name: it
// says where each of them is and lets the caller name the one it meant.
func (l *Location) FindItem(name, where string) (*Item, error) {
	return findInMap(l.Items, l.Name, name, where)
}

// SortedItems renders the floor in a fixed order — by name, then by spot — so
// the block reads the same way every turn and its cached prefix is not rewritten
// for nothing.
func (l *Location) SortedItems() []*Item {
	return sortedItems(l.Items)
}

// itemsMap satisfies itemsMapHolder for MoveItem's take().
func (l *Location) itemsMap() map[string]*Item { return l.Items }

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
			present = append(present, cid)
		}
	}
	return present
}

// MoveRequest names one move: what, to whom or where, and — only when the name
// alone is ambiguous — from whom or where. To and From are ids in the one
// shared keyspace — a character, a location, or a container item — resolved
// through ResolveHolder, so naming a destination is naming one id rather than
// picking which of several fields to put it in.
type MoveRequest struct {
	Item      string
	From      string
	FromWhere string
	Where     string
	To        string
	Count     int
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
// looting, stashing, stowing something in a container and retrieving a thrown
// knife are all this call, because they are all the same fact: an item is
// held by exactly one holder, and that holder changed.
//
// It moves the thing itself, ratings and wording and all, which is why it beats
// the remove-then-add pair it replaces. A pair can half-succeed and destroy what
// it was supposed to move, and re-adding a weapon means restating a damage
// rating from memory — the DM's memory being the thing the engine exists to
// stop relying on.
func (w *World) MoveItem(req MoveRequest) (*MoveResult, error) {
	if req.To == "" {
		return nil, fmt.Errorf("name a destination")
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
	to, err := w.ResolveHolder(req.To)
	if err != nil {
		return nil, err
	}
	if to.RequiresSpot() && strings.TrimSpace(req.Where) == "" {
		return nil, fmt.Errorf("say where in %s the %s comes to rest", to.HolderName(), source.item.Name)
	}
	if to.HolderID() == source.holder.HolderID() && slug(source.item.Where) == slug(req.Where) {
		return nil, fmt.Errorf("the %s already lies %s in %s", source.item.Name, source.item.Where, to.HolderName())
	}
	if item, ok := to.(*Item); ok {
		if err := w.checkNoCycle(source.item.ID, item); err != nil {
			return nil, err
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
	// holds a matching stack; AddItem decides that, and its answer wins.
	if count == source.item.Count {
		moved.ID = source.item.ID
	} else {
		moved.ID = w.MintItemID(moved.Name)
	}

	landed, err := to.AddItem(moved, req.Where, count)
	if err != nil {
		return nil, err
	}

	if err := source.take(count); err != nil {
		return nil, err
	}
	return &MoveResult{Item: landed, Count: count, From: source.name, To: to.HolderName()}, nil
}

// checkNoCycle refuses a move that would put a container inside its own
// contents — the backpack into the coin already inside it. Creation-time
// references are acyclic by construction, the same argument CreateLocation's
// doc comment makes for a location's Parent: an id must exist before anything
// can reference it. But a move can re-parent an existing container into a
// descendant it already has, which creation order cannot catch, so this walks
// upward from the destination and refuses if it reaches the item being moved.
func (w *World) checkNoCycle(movingID string, destination *Item) error {
	seen := map[string]bool{}
	current := destination.ID
	for {
		if current == movingID {
			return fmt.Errorf("%s already holds %s — it cannot also be put inside it", destination.Name, movingID)
		}
		if seen[current] {
			return nil // defensive: a cycle already existing elsewhere is not this call's to fix
		}
		seen[current] = true
		holderID, held := w.ItemHolder(current)
		if !held {
			return nil
		}
		current = holderID
	}
}

// itemSource is a resolved holder of one stack: where the move takes from, and
// how to take it.
type itemSource struct {
	item   *Item
	name   string
	holder Holder
}

// itemsMapHolder is the package-private half of a Holder: access to the raw
// map behind it, needed only by take's identity guard below. It stays
// unexported and off the public Holder interface because nothing outside this
// file has a reason to reach past AddItem/RemoveItem/FindItem into the map
// itself.
type itemsMapHolder interface {
	itemsMap() map[string]*Item
}

// take empties the source of what moved. The entry is dropped only if it is still
// the one this source resolved to: a whole stack moved from one spot to another
// on the very same holder keeps its id, so the arriving stack is written into
// the very map slot the departing one occupied — which is the right outcome, and
// exactly why the emptied original must not then delete its own replacement.
func (s *itemSource) take(count int) error {
	if count == s.item.Count {
		if im, ok := s.holder.(itemsMapHolder); ok {
			m := im.itemsMap()
			if m[s.item.ID] == s.item {
				delete(m, s.item.ID)
			}
			return nil
		}
	}
	_, _, err := s.holder.RemoveItem(s.item.Name, s.item.Where, count)
	return err
}

// resolveSource finds the stack a move is about. A stated source settles it;
// without one the world is searched, and a name in more than one place is
// refused with every holder listed rather than guessed at.
func (w *World) resolveSource(req MoveRequest) (*itemSource, error) {
	if strings.TrimSpace(req.Item) == "" {
		return nil, fmt.Errorf("name the item to move")
	}
	if req.From != "" {
		holder, err := w.ResolveHolder(req.From)
		if err != nil {
			return nil, err
		}
		item, err := holder.FindItem(req.Item, req.FromWhere)
		if err != nil {
			return nil, err
		}
		return &itemSource{item: item, name: holder.HolderName(), holder: holder}, nil
	}

	sources := w.findItem(req.Item)
	switch len(sources) {
	case 0:
		return nil, fmt.Errorf("nothing called %s is anywhere in the world: nobody holds one and none lies about. Create it with add_item if it is new", req.Item)
	case 1:
		return sources[0], nil
	}
	held := make([]string, 0, len(sources))
	for _, s := range sources {
		held = append(held, fmt.Sprintf("%d held by %s", s.item.Count, s.holder.HolderID()))
	}
	sort.Strings(held)
	return nil, fmt.Errorf("more than one %s exists: %s. Name from and the holder's id to say which", req.Item, strings.Join(held, "; "))
}

// findItem lists every stack of a name anywhere in the world — carried,
// resting at a place, or held inside a container — each in a fixed order so
// an ambiguity error reads the same way twice.
func (w *World) findItem(ref string) []*itemSource {
	// An id is unique across the whole world, so a reference that is one resolves
	// to exactly one holder and never reaches the ambiguity refusal.
	var sources []*itemSource
	w.walkItems(func(holder Holder, item *Item) {
		if item.refers(ref) {
			sources = append(sources, &itemSource{item: item, name: holder.HolderName(), holder: holder})
		}
	})
	return sources
}
