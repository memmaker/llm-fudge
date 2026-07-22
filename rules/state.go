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
	rng        *rand.Rand
}

// NewWorld creates a deterministic game world. Reuse the seed and action
// sequence to reproduce every check result.
func NewWorld(seed int64) *World {
	return &World{Characters: make(map[string]*Character), rng: rand.New(rand.NewSource(seed))}
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

func (w *World) Character(id string) (*Character, error) {
	ch := w.Characters[id]
	if ch == nil {
		return nil, fmt.Errorf("character not found: %s", id)
	}
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
