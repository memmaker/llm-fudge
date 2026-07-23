package rules

import (
	"fmt"
	"math/rand"
	"regexp"
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
	Inventory   map[string]int
	Coins       int
	Scale       int
	Description string
	Location    string
}

type World struct {
	Player     string
	Characters map[string]*Character
	Ruleset    *Ruleset
	Seed       int64
	rng        *rand.Rand
}

// NewWorld creates a deterministic game world with a selected setting.
// Reuse the seed and action sequence to reproduce every check result.
func NewWorld(seed int64, path string) (*World, error) {
	setting, err := LoadSetting(path)
	if err != nil {
		return nil, err
	}
	w := &World{Characters: make(map[string]*Character), Seed: seed, rng: rand.New(rand.NewSource(seed))}
	w.Ruleset = setting
	return w, nil
}

func (w *World) CreateCharacter(id, name string, attributes, skills map[string]int) *Character {
	if id == "" {
		id = slug(name)
	}
	if w.Characters == nil {
		w.Characters = make(map[string]*Character)
	}
	ch := &Character{ID: id, Name: name, Attributes: attributes, Skills: skills, Inventory: make(map[string]int)}
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

// Character retrieves a character by id and handles error if character does not exist
func (w *World) Character(id string) (*Character, error) {
	ch, exists := w.Characters[id]
	if !exists {
		return nil, fmt.Errorf("character not found: %s", id)
	}
	return ch, nil
}

// CharacterRequest holds arguments for create_character tool
// slug optional; name, species required
// attributes and skills maps may be nil
// location optional
// description optional
// gifts, faults, powers optional
// creates and freezes the setting on first use

type CharacterRequest struct {
	Slug        string         `json:"slug"`
	Name        string         `json:"name"`
	Species     string         `json:"species"`
	Description string         `json:"description"`
	Location    string         `json:"location"`
	Attributes  map[string]int `json:"attributes"`
	Skills      map[string]int `json:"skills"`
	Gifts       []string       `json:"gifts"`
	Faults      []string       `json:"faults"`
}

// CreateCharacterValidated validates and creates a character according to the setting
func (w *World) CreateCharacterValidated(req CharacterRequest) (*Character, error) {
	if w.Ruleset == nil {
		return nil, fmt.Errorf("world has no selected setting")
	}
	if w.Ruleset.Frozen {
		return nil, fmt.Errorf("setting is frozen")
	}
	if req.Name == "" {
		return nil, fmt.Errorf("character name is required")
	}
	id := req.Slug
	if id == "" {
		id = slug(req.Name)
	}
	if !slugPattern.MatchString(id) {
		return nil, fmt.Errorf("invalid character slug: %s", id)
	}
	if _, exists := w.Characters[id]; exists {
		return nil, fmt.Errorf("character slug already exists: %s", id)
	}
	specKey := strings.ToLower(req.Species)
	spec, ok := w.Ruleset.Species[specKey]
	if !ok {
		return nil, fmt.Errorf("species not found: %s", req.Species)
	}
	// attributes default fair (0)
	attrs := make(map[string]int, len(w.Ruleset.Attributes))
	for key := range w.Ruleset.Attributes {
		attrs[key] = 0
	}
	for key, val := range req.Attributes {
		keyLower := strings.ToLower(key)
		if _, ok := w.Ruleset.Attributes[keyLower]; !ok {
			return nil, fmt.Errorf("unknown attribute: %s", key)
		}
		if val < -3 || val > 3 {
			return nil, fmt.Errorf("attribute %s must be between -3 and 3", key)
		}
		attrs[keyLower] = val
	}
	// skills default poor (-2)
	skills := make(map[string]int, len(w.Ruleset.Skills))
	for key := range w.Ruleset.Skills {
		skills[key] = -2
	}
	for key, val := range req.Skills {
		keyLower := strings.ToLower(key)
		if _, ok := w.Ruleset.Skills[keyLower]; !ok {
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
		if _, ok := w.Ruleset.Gifts[giftKey]; !ok {
			return nil, fmt.Errorf("unknown gift: %s", giftSlug)
		}
		gifts = append(gifts, giftKey)
	}
	// validate and populate faults
	faults := make([]string, 0, len(req.Faults))
	for _, faultSlug := range req.Faults {
		faultKey := strings.ToLower(faultSlug)
		if _, ok := w.Ruleset.Faults[faultKey]; !ok {
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
		Inventory:   make(map[string]int),
		Coins:       0,
		Scale:       spec.Scale,
		Description: req.Description,
		Location:    loc,
	}
	w.Characters[id] = ch
	if w.Player == "" {
		w.Player = id
	}
	w.Ruleset.Frozen = true
	return ch, nil
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
	w.Scratches = 0
	w.Level = Undamaged

	if dp <= 0 {
		return "Undamaged"
	}
	if dp <= 2 {
		w.Scratches = 1
		w.Level = Scratch
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
