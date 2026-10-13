package rules

import (
	"math/rand"
	"strings"
	"testing"
)

func TestLadderIsSevenLevelsInOrder(t *testing.T) {
	// The names in order, and the values they have to carry: a check reads the
	// difficulty off the same scale, so a level worth the wrong number silently
	// moves every roll made against it.
	for want, name := range []string{"Terrible", "Poor", "Mediocre", "Fair", "Good", "Great", "Superb"} {
		v, ok := LevelValue(name)
		if !ok {
			t.Fatalf("LevelValue(%q) not recognised", name)
		}
		if v != want-3 {
			t.Fatalf("LevelValue(%q) = %d, want %d", name, v, want-3)
		}
	}
	if len(ladder) != 7 {
		t.Fatalf("the ladder holds %d levels, not 7", len(ladder))
	}
	if _, ok := LevelValue("Splendid"); ok {
		t.Fatal("invented level accepted")
	}
}

func TestDifficultyVocabularyIsSeparate(t *testing.T) {
	// A difficulty word means unopposed; a ladder word means opposed.
	// The two vocabularies must not overlap, or the tool cannot tell them apart.
	for name := range difficulties {
		if _, ok := LevelValue(name); ok {
			t.Fatalf("%q is both a difficulty and a ladder level", name)
		}
	}
}

func TestUnopposedUsesFixedDifficulty(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	r, err := Check("Good", "Average", 0, rng)
	if err != nil {
		t.Fatal(err)
	}
	if r.Opposed {
		t.Fatal("difficulty word produced an opposed check")
	}
	if r.OpposingRoll != 0 {
		t.Fatal("unopposed check rolled for the opposition")
	}
	// Good (+1) + roll + 0 modifier, against Average (0).
	if want := 1 + r.Roll - 0; r.Degree != want {
		t.Fatalf("Degree = %d, want %d", r.Degree, want)
	}
}

func TestOpposedRollsBothSides(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	r, err := Check("Good", "Great", 0, rng)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Opposed {
		t.Fatal("ladder word did not produce an opposed check")
	}
	// Good (+1) + own roll, against Great (+2) + opposing roll.
	if want := (1 + r.Roll) - (2 + r.OpposingRoll); r.Degree != want {
		t.Fatalf("Degree = %d, want %d", r.Degree, want)
	}
}

func TestModifierShiftsTheActorOnly(t *testing.T) {
	a, err := Check("Fair", "Average", 0, rand.New(rand.NewSource(7)))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Check("Fair", "Average", 2, rand.New(rand.NewSource(7)))
	if err != nil {
		t.Fatal(err)
	}
	if b.Degree-a.Degree != 2 {
		t.Fatalf("modifier +2 shifted degree by %d", b.Degree-a.Degree)
	}
}

func TestMeetingTheDifficultySucceeds(t *testing.T) {
	// Fudge: matching the difficulty is a success, not a failure.
	r := Result{Degree: 0}
	if !r.Success() {
		t.Fatal("degree 0 treated as failure")
	}
	if (Result{Degree: -1}).Success() {
		t.Fatal("degree -1 treated as success")
	}
}

func TestCriticalsFollowTheNaturalRoll(t *testing.T) {
	if !(Result{Roll: 4, Degree: 3}).Critical() {
		t.Fatal("natural +4 on a success is not critical")
	}
	if !(Result{Roll: -4, Degree: -3}).Critical() {
		t.Fatal("natural -4 on a failure is not critical")
	}
	if (Result{Roll: 4, Degree: -1}).Critical() {
		t.Fatal("natural +4 on a failure counted as critical")
	}
}

func TestRollStaysInRange(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	for i := 0; i < 500; i++ {
		r, err := Check("Fair", "Average", 0, rng)
		if err != nil {
			t.Fatal(err)
		}
		if r.Roll < -4 || r.Roll > 4 {
			t.Fatalf("4dF rolled %d", r.Roll)
		}
	}
}

func TestSeedRepeats(t *testing.T) {
	a, _ := Check("Fair", "Hard", 0, rand.New(rand.NewSource(42)))
	b, _ := Check("Fair", "Hard", 0, rand.New(rand.NewSource(42)))
	if a.Roll != b.Roll {
		t.Fatal("same seed gave different rolls")
	}
}

func TestUnknownVsIsRejectedWithGuidance(t *testing.T) {
	_, err := Check("Fair", "Quite Tricky", 0, rand.New(rand.NewSource(1)))
	if err == nil {
		t.Fatal("unknown vs accepted")
	}
	if !strings.Contains(err.Error(), "Average") {
		t.Fatalf("error does not name the accepted words: %v", err)
	}
}

func TestUnknownLevelIsRejected(t *testing.T) {
	if _, err := Check("Splendid", "Average", 0, rand.New(rand.NewSource(1))); err == nil {
		t.Fatal("unknown actor level accepted")
	}
}

func TestDescribeIsReadable(t *testing.T) {
	r, err := Check("Great", "Good", 0, rand.New(rand.NewSource(3)))
	if err != nil {
		t.Fatal(err)
	}
	s := r.Describe()
	for _, want := range []string{"Great", "Good", "degree"} {
		if !strings.Contains(s, want) {
			t.Fatalf("Describe() = %q, missing %q", s, want)
		}
	}
}
