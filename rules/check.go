// Package rules is what is left of the engine: the Fudge ladder, 4dF, and one
// check. It holds no world state — see docs/adr/006.
package rules

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
)

// ladder is the seven-level Fudge trait scale.
var ladder = map[string]int{
	"terrible": -3,
	"poor":     -2,
	"mediocre": -1,
	"fair":     0,
	"good":     1,
	"great":    2,
	"superb":   3,
}

// difficulties is a deliberately separate vocabulary from the ladder. Which one
// the `vs` argument comes from is how a check knows whether anything is rolling
// back: a difficulty is fixed, an opponent rolls.
var difficulties = map[string]int{
	"trivially easy": -3,
	"very easy":      -2,
	"easy":           -1,
	"average":        0,
	"hard":           1,
	"very hard":      2,
	"extremely hard": 3,
}

// LevelValue maps a ladder name to its value. Names above Superb are not
// accepted here; Legendary is a narrative label, not a check input.
func LevelValue(name string) (int, bool) {
	v, ok := ladder[strings.ToLower(strings.TrimSpace(name))]
	return v, ok
}

// LevelName maps a value back onto the ladder.
func LevelName(v int) string {
	switch {
	case v > 3:
		return "Legendary"
	case v < -3:
		return "Abysmal"
	}
	for name, val := range ladder {
		if val == v {
			return strings.ToUpper(name[:1]) + name[1:]
		}
	}
	return "Fair"
}

// Result is one resolved check.
type Result struct {
	Actor        string // the actor's level, as named
	ActorValue   int
	Vs           string // difficulty or opposing level, as named
	VsValue      int
	Modifier     int
	Roll         int  // the actor's 4dF, -4..+4
	OpposingRoll int  // the opponent's 4dF; zero on an unopposed check
	Opposed      bool // true when `vs` named a ladder level rather than a difficulty
	Degree       int  // relative degree: how far the actor came out ahead
}

// Success reports whether the attempt worked. Meeting the difficulty exactly is
// a success in Fudge, not a failure.
func (r Result) Success() bool { return r.Degree >= 0 }

// Critical reports a natural best- or worst-possible roll that went the same way
// as the outcome — a +4 that failed anyway is not a critical success.
func (r Result) Critical() bool {
	return (r.Roll == 4 && r.Success()) || (r.Roll == -4 && !r.Success())
}

// Describe renders the result for the DM to read.
func (r Result) Describe() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%+d) rolled %+d", r.Actor, r.ActorValue, r.Roll)
	if r.Modifier != 0 {
		fmt.Fprintf(&b, ", modifier %+d", r.Modifier)
	}
	if r.Opposed {
		fmt.Fprintf(&b, " vs %s (%+d) rolled %+d", r.Vs, r.VsValue, r.OpposingRoll)
	} else {
		fmt.Fprintf(&b, " vs %s (%+d)", r.Vs, r.VsValue)
	}
	outcome := "failure"
	if r.Success() {
		outcome = "success"
	}
	if r.Critical() {
		outcome = "critical " + outcome
	}
	fmt.Fprintf(&b, " — %s, relative degree %+d", outcome, r.Degree)
	return b.String()
}

// Check resolves one action. `level` is the acting trait's ladder level. `vs` is
// either a difficulty ("Average", "Hard") for an unopposed action, or an
// opposing trait's ladder level ("Good") for an opposed one — in Fudge both sit
// on the same scale, so this is one call rather than two.
func Check(level, vs string, modifier int, rng *rand.Rand) (Result, error) {
	actorValue, ok := LevelValue(level)
	if !ok {
		return Result{}, fmt.Errorf("unknown trait level %q — use one of: %s", level, names(ladder))
	}

	r := Result{
		Actor:      strings.TrimSpace(level),
		ActorValue: actorValue,
		Vs:         strings.TrimSpace(vs),
		Modifier:   modifier,
		Roll:       roll4dF(rng),
	}

	key := strings.ToLower(strings.TrimSpace(vs))
	switch v, isDifficulty := difficulties[key]; {
	case isDifficulty:
		r.VsValue = v
		r.Degree = actorValue + r.Roll + modifier - v
	default:
		v, isLevel := ladder[key]
		if !isLevel {
			return Result{}, fmt.Errorf("unknown difficulty or opposing level %q — use a difficulty (%s) or a trait level (%s)",
				vs, names(difficulties), names(ladder))
		}
		r.Opposed = true
		r.VsValue = v
		r.OpposingRoll = roll4dF(rng)
		r.Degree = (actorValue + r.Roll + modifier) - (v + r.OpposingRoll)
	}
	return r, nil
}

func roll4dF(rng *rand.Rand) int {
	total := 0
	for i := 0; i < 4; i++ {
		total += rng.Intn(3) - 1
	}
	return total
}

func names(m map[string]int) string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return m[out[i]] < m[out[j]] })
	for i, s := range out {
		out[i] = strings.ToUpper(s[:1]) + s[1:]
	}
	return strings.Join(out, ", ")
}
