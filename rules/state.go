package rules

import (
	"fmt"
	"maps"
	"math/rand"
	"regexp"
	"slices"
	"sort"
	"strings"
)

type WoundLevel int

const (
	Undamaged WoundLevel = iota
	Scratch
	Hurt
	VeryHurt
	Incapacitated
	NearDeath
	Dead
)

func (l WoundLevel) String() string {
	switch l {
	case Undamaged:
		return "Undamaged"
	case Scratch:
		return "Scratch"
	case Hurt:
		return "Hurt"
	case VeryHurt:
		return "Very Hurt"
	case Incapacitated:
		return "Incapacitated"
	case NearDeath:
		return "Near Death"
	case Dead:
		return "Dead"
	default:
		return "Unknown"
	}
}

type WoundState struct {
	Scratches int
	Level     WoundLevel
}

// Weapon is the combat rating carried by an item that can be attacked with. Damage
// is the weapon's own share of the offensive damage factor (§4.5.4). A
// muscle-powered weapon also names the attribute added to it, so the same sword
// swings harder in stronger hands while a blaster does not care who holds it.
type Weapon struct {
	Damage          int
	Ranged          bool
	DamageAttribute string
	Ammo            int
	AmmoCapacity    int
	AmmoItem        string
}

// TracksAmmo reports whether this weapon counts its shots. A capacity of zero
// means it does not — a sword, or a bow whose arrows the DM would rather not
// tally — and such a weapon never runs dry.
func (wp *Weapon) TracksAmmo() bool {
	return wp.Ranged && wp.AmmoCapacity > 0
}

// Loaded reports whether there is a shot left to fire.
func (wp *Weapon) Loaded() bool {
	return !wp.TracksAmmo() || wp.Ammo > 0
}

// Armor is the protection rating carried by a wearable item: its share of the
// defensive damage factor (§4.5.4). Only armor actually worn protects anybody —
// a spare mail shirt in a pack is cargo.
type Armor struct {
	Protection int
	Worn       bool
}

// Item is one stack of a thing, either in a character's inventory or lying at a
// location. Name and Description are the DM's words, shown to the player on the
// character sheet.
//
// ID is the item's identity, minted once by the engine and never written again.
// It used not to exist: an inventory keyed on the slugified name and a location's
// floor keyed on name-plus-spot, which meant an item's address *was* its name and
// where it happened to be. Moving it therefore changed everything about how it
// was found, and nothing linked what vanished from the old key to what appeared
// under the new one — inventory tracking broke in play for exactly this reason.
// Name and spot are still how the DM refers to a thing in prose, so they remain
// a lookup onto the id; they are no longer the identity itself.
//
// Weapon and Armor are the specialisations: an item carries at most one of them,
// and plain gear carries neither. They hold the ratings combat needs, so a hit
// is resolved from what the character actually carries rather than from a
// severity the DM guessed at.
//
// Where is set only while the item lies at a location: it is the short prose
// saying where in that place it rests. A carried thing lies nowhere, so picking
// one up clears it.
type Item struct {
	ID          string
	Name        string
	Description string
	Count       int
	Weapon      *Weapon
	Armor       *Armor
	Where       string
}

// clone copies an item and the ratings hanging off it, so the copy shares
// nothing with the original. Count and Where are left to whoever places it:
// how many moved and where they came to rest are the move's business, not the
// original's.
//
// The id is left behind too, and deliberately. A clone is what MoveItem sends to
// the destination, and whether that stack keeps the original's identity or needs
// a freshly minted one depends on whether the whole stack moved — a question only
// the caller can answer. Copying the id here would quietly put two live stacks
// under one identity the first time half a pile was handed over.
func (it *Item) clone() *Item {
	copied := &Item{Name: it.Name, Description: it.Description}
	if it.Weapon != nil {
		weapon := *it.Weapon
		copied.Weapon = &weapon
	}
	if it.Armor != nil {
		armor := *it.Armor
		copied.Armor = &armor
	}
	return copied
}

// refers reports whether ref names this item, by its id or by its name. Both are
// accepted because both are what the DM has: the world state gives it ids and
// tells it they are what every call names, while its own prose gives it names.
//
// The two are deliberately NOT ranked, and an id does not short-circuit. Ranking
// it first looked obviously right and was wrong: with two stacks both named
// "Dagger", the first holds the id `dagger`, so the plain name "Dagger" would
// match that id exactly and resolve silently to one of the two — losing the
// ambiguity refusal that is the only thing standing between the DM and a guess.
// Every candidate is gathered, and one candidate is what settles a lookup.
func (it *Item) refers(ref string) bool {
	if ref = strings.TrimSpace(ref); ref == "" {
		return false
	}
	return it.ID == itemID(ref) || slug(it.Name) == slug(ref)
}

// itemID normalises a reference for comparison against an id. Ids are minted
// lowercase and matched literally, so a reference that differs only in case or
// surrounding space is the same reference.
func itemID(ref string) string {
	return strings.ToLower(strings.TrimSpace(ref))
}

// Kind names what an item is, for messages that have to tell a sword from a
// mail shirt from a coil of rope.
func (it *Item) Kind() string {
	switch {
	case it.Weapon != nil:
		return "weapon"
	case it.Armor != nil:
		return "armor"
	default:
		return "gear"
	}
}

type Character struct {
	ID          string
	Name        string
	Species     string
	Attributes  map[string]int
	Skills      map[string]int
	Gifts       []string
	Faults      []string
	FudgePoints int
	Wounds      WoundState
	Inventory   map[string]*Item
	Coins       int
	Scale       int
	Description string
	Location    string
}

type World struct {
	// AlsoTaken reports ids claimed outside the rules library. Narrative-only
	// entities — a rumour, a job, an objective — live in the same single id
	// keyspace as characters, locations and items, but they are not the rules
	// library's to hold, so it cannot see them without being told how.
	//
	// It is a hook rather than a dependency because the ownership boundary runs
	// both ways: this package must not be able to reach into narrative state, and
	// a package this one imported could not have been the one importing it. Nil
	// means nothing outside is claimed, which is true of a bare world in a test.
	//
	// Without it, minting is only half guarded. `narrative` refuses to create an
	// entity whose id the engine already holds; nothing stopped the engine
	// minting an item onto an id `narrative` already held, and an item named
	// "Iron Key" landing on the objective `iron-key` merged two different things
	// into one record.
	AlsoTaken        func(id string) bool
	Setting          *SettingInfos
	AdventureContent string
	Player           string
	Characters       map[string]*Character
	Locations        map[string]*Location
	Seed             int64
	Rng              *rand.Rand
}

func NewWorld(seed int64, settingPath string) (*World, error) {
	setting, err := LoadSetting(settingPath)
	if err != nil {
		return nil, err
	}
	return &World{
		Setting:    setting,
		Seed:       seed,
		Rng:        rand.New(rand.NewSource(seed)),
		Characters: make(map[string]*Character),
		Locations:  make(map[string]*Location),
	}, nil
}

func (w *World) CreateCharacter(id, name string, attributes, skills map[string]int) *Character {
	if id == "" {
		id = slug(name)
	}
	if w.Characters == nil {
		w.Characters = make(map[string]*Character)
	}
	ch := &Character{ID: id, Name: name, Attributes: attributes, Skills: skills, Inventory: make(map[string]*Item)}
	if ch.Attributes == nil {
		ch.Attributes = make(map[string]int)
	}
	if ch.Skills == nil {
		ch.Skills = make(map[string]int)
	}
	w.Characters[id] = ch
	if w.Player == "" {
		w.Player = id
	}
	return ch
}

// Character retrieves a character by id. The id is exact: every character's id
// reaches the DM in the result of the call that created or last moved them, and
// those results stay in its context, so a miss is a mistake to correct and not
// something to resolve by guessing.
//
// The correction is what the error is for. It names the roster, so the retry can
// pick an id that exists instead of guessing a second time.
func (w *World) Character(id string) (*Character, error) {
	ch, exists := w.Characters[id]
	if exists {
		return ch, nil
	}
	if len(w.Characters) == 0 {
		return nil, fmt.Errorf("character not found: %s — nobody exists yet, create them first", id)
	}
	return nil, fmt.Errorf("character not found: %s — the characters that exist are: %s. Use one of those ids, or create this one first if they are new", id, strings.Join(w.CharacterIDs(), ", "))
}

// EntityIDs lists every id the world holds, characters first, then locations,
// then items, each group sorted. It exists for the same reason CharacterIDs
// does: a refusal that names what exists can be corrected on the next turn,
// where one that does not can only be guessed at again.
func (w *World) EntityIDs() []string {
	ids := w.CharacterIDs()
	ids = append(ids, w.LocationIDs()...)
	return append(ids, w.ItemIDs()...)
}

// HasEntity reports whether an id is taken anywhere in the world: by a
// character, a location, or an item in anybody's pockets or on any floor.
//
// There is one global id keyspace, not one per type, because the DM addresses
// entities by bare id in tool calls — `iron-key` has to mean exactly one thing or
// the call is a coin toss. It is exported because the narrative-entity store
// mints ids into this same keyspace and must check it before doing so; a rumor
// and a character colliding would be the same ambiguity one layer out.
//
// It walks every inventory and every floor. That is a scan per check and MintItemID
// loops on it, so minting is quadratic in the world's item count on a collision —
// fine at the scale of a session's pockets and rooms, and worth revisiting only if
// an id map is ever wanted for something else too.
func (w *World) HasEntity(id string) bool {
	if id == "" {
		return false
	}
	if _, taken := w.Characters[id]; taken {
		return true
	}
	if _, taken := w.Locations[id]; taken {
		return true
	}
	for _, ch := range w.Characters {
		if _, taken := ch.Inventory[id]; taken {
			return true
		}
	}
	for _, loc := range w.Locations {
		if _, taken := loc.Items[id]; taken {
			return true
		}
	}
	return w.AlsoTaken != nil && w.AlsoTaken(id)
}

// MintItemID picks the id an item will carry for the rest of its existence. The
// name is the obvious starting point — an id the DM can read and reuse without
// looking it up — and a numeric suffix settles the second iron key, since two
// things sharing an id is the one thing the keyspace cannot survive.
func (w *World) MintItemID(name string) string {
	base := slug(name)
	if base == "" {
		// A name of nothing but punctuation still needs an addressable id;
		// returning the empty string would mint a thing nobody can name.
		base = "item"
	}
	id := base
	for n := 2; w.HasEntity(id); n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	return id
}

// ItemByID finds an item anywhere in the world. Like Character, a miss names what
// does exist: the DM reads an item's id off the result of the call that created or
// last moved it, so a miss is a mistake to correct rather than a licence to guess.
func (w *World) ItemByID(id string) (*Item, error) {
	for _, cid := range w.CharacterIDs() {
		if it, held := w.Characters[cid].Inventory[id]; held {
			return it, nil
		}
	}
	for _, lid := range w.LocationIDs() {
		if it, lying := w.Locations[lid].Items[id]; lying {
			return it, nil
		}
	}
	existing := w.ItemIDs()
	if len(existing) == 0 {
		return nil, fmt.Errorf("item not found: %s — no items exist anywhere yet, create it first with add_item", id)
	}
	return nil, fmt.Errorf("item not found: %s — the items that exist are: %s. Use one of those ids, or create this one first with add_item if it is new", id, strings.Join(existing, ", "))
}

// ItemIDs lists every item id in the world, sorted, for the roster an error hands
// back.
func (w *World) ItemIDs() []string {
	var ids []string
	for _, cid := range w.CharacterIDs() {
		for id := range w.Characters[cid].Inventory {
			ids = append(ids, id)
		}
	}
	for _, lid := range w.LocationIDs() {
		for id := range w.Locations[lid].Items {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// CharacterIDs lists every character's id, sorted, for the roster an error hands
// back.
func (w *World) CharacterIDs() []string {
	ids := make([]string, 0, len(w.Characters))
	for id := range w.Characters {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// CharacterRequest holds arguments for create_character tool
// slug optional; name, species required
// attributes and skills maps may be nil
// location optional
// description optional
// gifts, faults, powers optional

type CharacterRequest struct {
	Name        string         `json:"name"`
	Species     string         `json:"species"`
	Description string         `json:"description"`
	Location    string         `json:"location"`
	Attributes  map[string]int `json:"attributes"`
	Skills      map[string]int `json:"skills"`
	Gifts       []string       `json:"gifts"`
	Faults      []string       `json:"faults"`
}

// CreateCharacterValidated validates and creates a character according to the setting.
// Assumes the very first character created is the player character and will set their slug in the world.
func (w *World) CreateCharacterValidated(req CharacterRequest) (*Character, error) {
	if w.Setting == nil {
		return nil, fmt.Errorf("world has no selected setting")
	}

	if req.Name == "" {
		return nil, fmt.Errorf("character name is required")
	}

	id := slug(req.Name)
	if !slugPattern.MatchString(id) {
		return nil, fmt.Errorf("invalid character slug: %s", id)
	}
	// Checked against the whole keyspace, not just the characters. One id means
	// one entity: a character sharing an id with a location or an item would
	// render as one key in the flat entity map, and two different things would
	// arrive at the DM as one record.
	if w.HasEntity(id) {
		return nil, fmt.Errorf("id already taken: %s — every character, location and item shares one id namespace. Use a fuller name so the id differs", id)
	}
	specKey := strings.ToLower(req.Species)
	spec, ok := w.Setting.Species[specKey]
	if !ok {
		return nil, fmt.Errorf("species not found: %s", req.Species)
	}
	// attributes default fair (0)
	attrs := make(map[string]int, len(w.Setting.Attributes))
	for key := range w.Setting.Attributes {
		attrs[key] = 0
	}
	for key, val := range req.Attributes {
		keyLower := strings.ToLower(key)
		if _, ok := w.Setting.Attributes[keyLower]; !ok {
			return nil, fmt.Errorf("unknown attribute: %s", key)
		}
		if val < -3 || val > 3 {
			return nil, fmt.Errorf("attribute %s must be between -3 and 3", key)
		}
		attrs[keyLower] = val
	}
	// skills default poor (-2)
	skills := make(map[string]int, len(w.Setting.Skills))
	for key := range w.Setting.Skills {
		skills[key] = -2
	}
	for key, val := range req.Skills {
		keyLower := strings.ToLower(key)
		if _, ok := w.Setting.Skills[keyLower]; !ok {
			return nil, fmt.Errorf("unknown skill: %s", key)
		}
		if val < -3 || val > 3 {
			return nil, fmt.Errorf("skill %s must be between -3 and 3", key)
		}
		skills[keyLower] = val
	}
	// An NPC is created because they have just walked into the story, so the
	// scene is where they are unless the DM says otherwise — and the scene is
	// wherever the player stands, since every narration puts them somewhere.
	// The empty string is the honest answer before play opens: during character
	// creation no location exists yet, and the first narration is what places
	// everybody.
	loc := req.Location
	if loc == "" {
		loc = w.Scene()
	} else if _, err := w.Location(loc); err != nil {
		return nil, err
	}
	// validate and populate gifts
	gifts := make([]string, 0, len(req.Gifts))
	for _, giftSlug := range req.Gifts {
		giftKey := strings.ToLower(giftSlug)
		if _, ok := w.Setting.Gifts[giftKey]; !ok {
			return nil, fmt.Errorf("unknown gift: %s", giftSlug)
		}
		gifts = append(gifts, giftKey)
	}
	// validate and populate faults
	faults := make([]string, 0, len(req.Faults))
	for _, faultSlug := range req.Faults {
		faultKey := strings.ToLower(faultSlug)
		if _, ok := w.Setting.Faults[faultKey]; !ok {
			return nil, fmt.Errorf("unknown fault: %s", faultSlug)
		}
		faults = append(faults, faultKey)
	}
	// description can be empty
	ch := &Character{
		ID:          id,
		Name:        req.Name,
		Species:     specKey,
		Attributes:  attrs,
		Skills:      skills,
		Gifts:       gifts,
		Faults:      faults,
		FudgePoints: 0,
		Wounds:      WoundState{},
		Inventory:   make(map[string]*Item),
		Coins:       0,
		Scale:       spec.Scale,
		Description: req.Description,
		Location:    loc,
	}
	w.Characters[id] = ch
	if w.Player == "" {
		w.Player = id
	}

	return ch, nil
}

// GiveItem puts count copies of plain gear into a character's hands. An item the
// character already carries stacks onto the existing entry instead of opening a
// second one; a description given for it replaces the old wording, an empty one
// leaves it as it was.
//
// It hangs off World rather than Character, along with GiveWeapon and GiveArmor,
// because minting an id needs to see the whole keyspace, and a Character cannot.
// It is not the only route in — tools.go mints for the calls that build their own
// proto, and MoveItem mints for the half of a split stack that leaves — so the
// invariant is not "everything goes through here" but the one Accept and AddItem
// enforce between them: a new entry refuses a proto whose id is empty. That
// refusal is what catches a route that forgot, rather than a convention that a
// new caller has no way of knowing about.
func (w *World) GiveItem(c *Character, name, description string, count int) (*Item, error) {
	return c.Accept(&Item{ID: w.MintItemID(name), Name: name, Description: description}, count)
}

// NewWeaponItem builds a rated weapon, settling its magazine: an unstated load
// means full, and no weapon holds more than its capacity. It exists apart from
// the inventory because a weapon can now be created lying on the ground as
// readily as in somebody's hands, and both routes must rate it the same way.
func NewWeaponItem(name, description string, weapon Weapon) (*Item, error) {
	if weapon.AmmoCapacity < 0 || weapon.Ammo < 0 {
		return nil, fmt.Errorf("ammunition cannot be negative")
	}
	if !weapon.Ranged && (weapon.AmmoCapacity > 0 || weapon.AmmoItem != "") {
		return nil, fmt.Errorf("%s is a melee weapon and holds no ammunition", name)
	}
	if weapon.AmmoCapacity > 0 && weapon.Ammo == 0 {
		weapon.Ammo = weapon.AmmoCapacity
	}
	if weapon.Ammo > weapon.AmmoCapacity {
		weapon.Ammo = weapon.AmmoCapacity
	}
	return &Item{Name: name, Description: description, Weapon: &weapon}, nil
}

// NewArmorItem builds a rated piece of armor.
func NewArmorItem(name, description string, armor Armor) (*Item, error) {
	if armor.Protection < 0 {
		return nil, fmt.Errorf("armor protection cannot be negative")
	}
	return &Item{Name: name, Description: description, Armor: &armor}, nil
}

// GiveWeapon puts a rated weapon into a character's hands. Restocking one they
// already carry stacks onto it and leaves the ratings — and a partly emptied
// magazine — as they stand, so a second knife does not silently reload the first.
func (w *World) GiveWeapon(c *Character, name, description string, count int, weapon Weapon) (*Item, error) {
	proto, err := NewWeaponItem(name, description, weapon)
	if err != nil {
		return nil, err
	}
	proto.ID = w.MintItemID(name)
	return c.Accept(proto, count)
}

// GiveArmor puts a rated piece of armor into a character's hands, worn unless the
// caller says otherwise.
func (w *World) GiveArmor(c *Character, name, description string, count int, armor Armor) (*Item, error) {
	proto, err := NewArmorItem(name, description, armor)
	if err != nil {
		return nil, err
	}
	proto.ID = w.MintItemID(name)
	return c.Accept(proto, count)
}

// Accept takes a stack into the inventory, keeping the ratings and the wording
// it arrives with. It is the inventory's counterpart to Location.AddItem, and
// the reason a retrieved weapon comes back able to fight: nothing about it is
// restated along the way, so nothing about it can be restated wrongly.
//
// It is the one place an inventory entry is opened or grown, and it takes a proto
// whose id the caller has already minted. Landing on a stack the character
// already carries grows that stack and keeps *its* id, retiring the proto's: one
// pile of rations is one entity, and carrying two ids for it would be exactly the
// ambiguity the id exists to remove.
//
// An item already carried as something else is refused rather than converted.
// Naming loot after a weapon on the sheet would otherwise wipe the ratings that
// weapon fights with, and the DM would only find out mid-swing.
func (c *Character) Accept(proto *Item, count int) (*Item, error) {
	if proto == nil || strings.TrimSpace(proto.Name) == "" {
		return nil, fmt.Errorf("item name is required")
	}
	if count < 1 {
		return nil, fmt.Errorf("item count must be at least 1")
	}
	if slug(proto.Name) == "" {
		return nil, fmt.Errorf("invalid item name: %s", proto.Name)
	}
	if c.Inventory == nil {
		c.Inventory = make(map[string]*Item)
	}
	item := c.carried(proto.Name)
	if item != nil && item.Kind() != proto.Kind() {
		return nil, fmt.Errorf("%s already carries %s as %s, not as %s", c.Name, item.Name, item.Kind(), proto.Kind())
	}
	if item == nil {
		if proto.ID == "" {
			return nil, fmt.Errorf("no id minted for %s: mint one with MintItemID before handing it over, or an item lands where nothing can address it", proto.Name)
		}
		item = &Item{ID: proto.ID, Name: proto.Name, Weapon: proto.Weapon, Armor: proto.Armor}
		c.Inventory[item.ID] = item
	}
	if proto.Description != "" {
		item.Description = proto.Description
	}
	item.Count += count
	return item, nil
}

// carried finds the one stack of a name on this sheet. The inventory keys on the
// item's id now, so a name is a lookup rather than an address, and the lookup is a
// scan. That costs nothing worth measuring — this is one character's pockets — and
// Accept never opens a second entry under a name already on the sheet, so the scan
// can never have two answers to choose between.
func (c *Character) carried(ref string) *Item {
	// One inventory holds at most one stack per name, so a scan cannot be
	// ambiguous here the way a location's floor or the whole world can be. The
	// order is fixed anyway: Go randomises map iteration, and this engine is
	// deterministic by design.
	for _, id := range sortedItemIDs(c.Inventory) {
		if c.Inventory[id].refers(ref) {
			return c.Inventory[id]
		}
	}
	return nil
}

// sortedItemIDs orders the keys of an item map, so a scan that can match more
// than one stack picks the same one every run. This engine is deterministic by
// design and Go randomises map iteration.
func sortedItemIDs(items map[string]*Item) []string {
	return slices.Sorted(maps.Keys(items))
}

// RemoveItem takes count copies off a stack and drops the entry once nothing is
// left. It returns the item as it stood before the removal, plus how many
// remain, so callers can report both the name and the new total.
func (c *Character) RemoveItem(name string, count int) (*Item, int, error) {
	if strings.TrimSpace(name) == "" {
		return nil, 0, fmt.Errorf("item name is required")
	}
	if count < 1 {
		return nil, 0, fmt.Errorf("item count must be at least 1")
	}
	item := c.carried(name)
	if item == nil {
		return nil, 0, fmt.Errorf("%s does not carry any %s", c.Name, name)
	}
	if count > item.Count {
		return item, item.Count, fmt.Errorf("%s carries only %d %s", c.Name, item.Count, item.Name)
	}
	item.Count -= count
	if item.Count == 0 {
		delete(c.Inventory, item.ID)
	}
	return item, item.Count, nil
}

// AddCoins credits the character's purse and returns the new balance.
func (c *Character) AddCoins(amount int) (int, error) {
	if amount < 1 {
		return c.Coins, fmt.Errorf("coin amount must be at least 1")
	}
	c.Coins += amount
	return c.Coins, nil
}

// RemoveCoins debits the character's purse and returns the new balance. A purse
// never goes negative: spending more than is carried fails outright.
func (c *Character) RemoveCoins(amount int) (int, error) {
	if amount < 1 {
		return c.Coins, fmt.Errorf("coin amount must be at least 1")
	}
	if amount > c.Coins {
		return c.Coins, fmt.Errorf("%s has only %d coins", c.Name, c.Coins)
	}
	c.Coins -= amount
	return c.Coins, nil
}

func slug(s string) string {
	re := regexp.MustCompile(`[^a-z0-9-]`)
	s = strings.ToLower(s)
	s = re.ReplaceAllString(s, "-")
	s = regexp.MustCompile(`-+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

func levelString(v int) string {
	switch v {
	case 3:
		return "Superb"
	case 2:
		return "Great"
	case 1:
		return "Good"
	case 0:
		return "Fair"
	case -1:
		return "Mediocre"
	case -2:
		return "Poor"
	case -3:
		return "Terrible"
	default:
		if v > 3 {
			return "Legendary"
		}
		return "Abysmal"
	}
}

func levelValue(s string) (int, bool) {
	m := map[string]int{
		"superb":   3,
		"great":    2,
		"good":     1,
		"fair":     0,
		"mediocre": -1,
		"poor":     -2,
		"terrible": -3,
	}
	v, ok := m[strings.ToLower(s)]
	return v, ok
}

func (w *WoundState) Penalty() int {
	switch w.Level {
	case Dead, NearDeath, Incapacitated:
		return 0 // can't act at all
	case VeryHurt:
		return -2
	case Hurt:
		return -1
	default:
		return 0
	}
}

func (w *WoundState) CanAct() bool {
	return w.Level < Incapacitated
}

func (w *WoundState) ApplyDamage(dp int) string {
	if w.Level == Dead {
		return "Dead (cannot be damaged)"
	}

	if dp <= 0 {
		return w.Level.String()
	}

	// Handle scratch accumulation for Undamaged/Scratch levels
	if w.Level == Undamaged || w.Level == Scratch {
		if dp <= 2 {
			// Accumulate scratches (up to 3)
			w.Scratches++
			if w.Scratches >= 3 {
				w.Scratches = 0
				w.Level = Hurt
				return "Hurt"
			}
			w.Level = Scratch
			return "Scratch"
		}
	}

	// Direct damage mapping for higher levels
	w.Scratches = 0
	if dp <= 2 {
		w.Level = Scratch
		w.Scratches = 1
		return "Scratch"
	}
	if dp <= 4 {
		w.Level = Hurt
		return "Hurt"
	}
	if dp <= 6 {
		w.Level = VeryHurt
		return "Very Hurt"
	}
	if dp <= 8 {
		w.Level = Incapacitated
		return "Incapacitated"
	}
	w.Level = NearDeath
	return "Near Death"
}

func (w *WoundState) Heal(levels int) string {
	if w.Level == Dead {
		return "Cannot heal the dead"
	}

	for i := 0; i < levels; i++ {
		switch w.Level {
		case NearDeath:
			w.Level = Incapacitated
		case Incapacitated:
			w.Level = VeryHurt
		case VeryHurt:
			w.Level = Hurt
		case Hurt:
			w.Level = Undamaged
		case Scratch:
			w.Scratches = 0
			w.Level = Undamaged
		default:
			if i == 0 {
				return "Already fully healed"
			}
			break
		}
	}
	return "Healed"
}

func (w *WoundState) DeclareDead() {
	w.Level = Dead
	w.Scratches = 0
}
