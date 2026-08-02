package rules

import (
	"fmt"
	"sort"
	"strings"
)

// Holder is anything an item can be held by: a Character carries it, a
// Location or a container Item holds it at a spot within itself. The three
// kinds used to be a hardcoded two-way (and now three-way) branch at every
// place an item changes hands; this is the one shape all three satisfy, so
// resolving "what does this id hold items for" is a single lookup rather than
// a branch repeated at every call site.
type Holder interface {
	HolderID() string
	HolderName() string
	// RequiresSpot reports whether a fresh item needs a spot to be readable at
	// all — true for a Location or a container Item ("where in the room does
	// this lie"), false for a Character (carrying needs no spot, though one
	// may still be given — "in hand", "in the left pocket" — to tell two
	// same-named things apart).
	RequiresSpot() bool
	AddItem(proto *Item, spot string, count int) (*Item, error)
	RemoveItem(name, spot string, count int) (*Item, int, error)
	FindItem(name, spot string) (*Item, error)
	SortedItems() []*Item
}

// ResolveHolder finds what an id refers to as a place to hold items: a
// character, a location, or a container item. The three are one shared
// keyspace, so at most one of these ever resolves.
func (w *World) ResolveHolder(id string) (Holder, error) {
	if ch, err := w.Character(id); err == nil {
		return ch, nil
	}
	if loc, err := w.Location(id); err == nil {
		return loc, nil
	}
	if it, err := w.ItemByID(id); err == nil {
		if !it.IsContainer() {
			return nil, fmt.Errorf("%s cannot hold anything — it isn't a container", it.Name)
		}
		return it, nil
	}
	return nil, fmt.Errorf("nothing exists with id %s that could hold an item — it must be a character, a location, or an item created as a container", id)
}

// addItem is the one implementation every Holder's AddItem delegates to,
// against its own map. A proto that is itself a container never stacks —
// count is meaningless on one, and two containers sharing a name and spot are
// still two different things, unlike two piles of the same coin.
func addItem(items map[string]*Item, holderName string, proto *Item, spot string, count int) (*Item, error) {
	if proto == nil || strings.TrimSpace(proto.Name) == "" {
		return nil, fmt.Errorf("item name is required")
	}
	if count < 1 {
		return nil, fmt.Errorf("item count must be at least 1")
	}
	spot = strings.TrimSpace(spot)
	var item *Item
	if !proto.IsContainer() {
		item = stackAt(items, proto.Name, spot)
	}
	if item != nil && item.Kind() != proto.Kind() {
		return nil, fmt.Errorf("%s already holds %s as %s, not as %s", holderName, item.Name, item.Kind(), proto.Kind())
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
			Where:       spot,
			Items:       proto.Items,
		}
		items[item.ID] = item
	}
	if proto.Description != "" {
		item.Description = proto.Description
	}
	item.Count += count
	return item, nil
}

// removeItem is the one implementation every Holder's RemoveItem delegates
// to. It takes a stack, or part of one, off whichever map it is given.
func removeItem(items map[string]*Item, holderName, name, spot string, count int) (*Item, int, error) {
	if count < 1 {
		return nil, 0, fmt.Errorf("item count must be at least 1")
	}
	item, err := findInMap(items, holderName, name, spot)
	if err != nil {
		return nil, 0, err
	}
	if count > item.Count {
		return item, item.Count, fmt.Errorf("%s holds only %d %s", holderName, item.Count, item.Name)
	}
	item.Count -= count
	if item.Count == 0 {
		delete(items, item.ID)
	}
	return item, item.Count, nil
}

// stackAt finds the stack of a name at a given spot, which is the pair that
// decides whether an arriving stack merges. It is a scan because the id, not
// the pair, is the key; a holder's contents run few enough that looking at
// all of them is cheaper than any index would be.
func stackAt(items map[string]*Item, ref, spot string) *Item {
	wantedSpot := slug(spot)
	for _, it := range sortedItems(items) {
		if it.refers(ref) && slug(it.Where) == wantedSpot {
			return it
		}
	}
	return nil
}

// findInMap resolves a name, and optionally a spot, to one stack. Without a
// spot it will not guess between two stacks of the same name: it says where
// each of them is and lets the caller name the one it meant.
func findInMap(items map[string]*Item, holderName, name, spot string) (*Item, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("item name is required")
	}
	if strings.TrimSpace(spot) != "" {
		item := stackAt(items, name, spot)
		if item == nil {
			return nil, fmt.Errorf("no %s lies %s in %s", name, spot, holderName)
		}
		return item, nil
	}

	var matches []string
	var found *Item
	for _, item := range sortedItems(items) {
		if !item.refers(name) {
			continue
		}
		where := item.Where
		if where == "" {
			where = "unspecified"
		}
		matches = append(matches, where)
		found = item
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("%s holds no %s", holderName, name)
	case 1:
		return found, nil
	}
	sort.Strings(matches)
	return nil, fmt.Errorf("%s holds %d lots of %s: %s. Name the spot you mean", holderName, len(matches), name, strings.Join(matches, "; "))
}

// sortedItems renders a holder's contents in a fixed order — by name, then by
// spot, then by id — so the block reads the same way every turn and its
// cached prefix is not rewritten for nothing.
func sortedItems(items map[string]*Item) []*Item {
	out := make([]*Item, 0, len(items))
	for _, it := range items {
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		if out[i].Where != out[j].Where {
			return out[i].Where < out[j].Where
		}
		// Name and spot together are unique per holder today, so this never
		// decides anything. It is here because the id is the one key that
		// cannot tie, and a sort that can fall through to "equal" on a
		// randomised map walk is a determinism bug waiting for that invariant
		// to relax.
		return out[i].ID < out[j].ID
	})
	return out
}
