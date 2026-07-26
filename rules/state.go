package rules

import (
	"fmt"
	"math/rand"
	"regexp"
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

// Item is one stack in a character's inventory. Name and Description are the
// DM's words, shown to the player on the character sheet; the inventory keys
// on the slugified name, so "Rope" and "rope" land on the same stack.
//
// Weapon and Armor are the specialisations: an item carries at most one of them,
// and plain gear carries neither. They hold the ratings combat needs, so a hit
// is resolved from what the character actually carries rather than from a
// severity the DM guessed at.
type Item struct {
	Name        string
	Description string
	Count       int
	Weapon      *Weapon
	Armor       *Armor
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
	Setting          *SettingInfos
	AdventureContent string
	Player           string
	Characters       map[string]*Character
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
	if _, exists := w.Characters[id]; exists {
		return nil, fmt.Errorf("character slug already exists: %s", id)
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
	// location default
	loc := req.Location
	if loc == "" {
		loc = "default"
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

// AddItem puts count copies of plain gear into the inventory. An item the
// character already carries stacks onto the existing entry instead of opening a
// second one; a description given for it replaces the old wording, an empty one
// leaves it as it was.
func (c *Character) AddItem(name, description string, count int) (*Item, error) {
	return c.addStack(name, description, count, "gear", nil)
}

// AddWeapon puts a rated weapon into the inventory. Restocking one the character
// already carries stacks onto it and leaves the ratings — and a partly emptied
// magazine — as they stand, so a second knife does not silently reload the first.
func (c *Character) AddWeapon(name, description string, count int, weapon Weapon) (*Item, error) {
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
	return c.addStack(name, description, count, "weapon", func(it *Item) { it.Weapon = &weapon })
}

// AddArmor puts a rated piece of armor into the inventory, worn unless the
// caller says otherwise.
func (c *Character) AddArmor(name, description string, count int, armor Armor) (*Item, error) {
	if armor.Protection < 0 {
		return nil, fmt.Errorf("armor protection cannot be negative")
	}
	return c.addStack(name, description, count, "armor", func(it *Item) { it.Armor = &armor })
}

// addStack is the one place an inventory entry is opened or grown. rate fills in
// the specialisation of a newly opened entry and is not run again on a restock:
// the ratings an item was recorded with are the ones it keeps.
//
// An item already carried as something else is refused rather than converted.
// Naming loot after a weapon on the sheet would otherwise wipe the ratings that
// weapon fights with, and the DM would only find out mid-swing.
func (c *Character) addStack(name, description string, count int, kind string, rate func(*Item)) (*Item, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("item name is required")
	}
	if count < 1 {
		return nil, fmt.Errorf("item count must be at least 1")
	}
	key := slug(name)
	if key == "" {
		return nil, fmt.Errorf("invalid item name: %s", name)
	}
	if c.Inventory == nil {
		c.Inventory = make(map[string]*Item)
	}
	item, held := c.Inventory[key]
	if held && item.Kind() != kind {
		return nil, fmt.Errorf("%s already carries %s as %s, not as %s", c.Name, item.Name, item.Kind(), kind)
	}
	if !held {
		item = &Item{Name: name}
		if rate != nil {
			rate(item)
		}
		c.Inventory[key] = item
	}
	if description != "" {
		item.Description = description
	}
	item.Count += count
	return item, nil
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
	item, held := c.Inventory[slug(name)]
	if !held {
		return nil, 0, fmt.Errorf("%s does not carry any %s", c.Name, name)
	}
	if count > item.Count {
		return item, item.Count, fmt.Errorf("%s carries only %d %s", c.Name, item.Count, item.Name)
	}
	item.Count -= count
	if item.Count == 0 {
		delete(c.Inventory, slug(name))
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
