package rules

type TableEntry struct {
	Range RollRange
	Value string
}

// RollRange is an inclusive range of roll results. An absent bound represents
// an open-ended range.
type RollRange struct {
	Min    int
	Max    int
	HasMin bool
	HasMax bool
}

func ExactRoll(value int) RollRange {
	return RollRange{Min: value, Max: value, HasMin: true, HasMax: true}
}

func RollRangeBetween(min, max int) RollRange {
	return RollRange{Min: min, Max: max, HasMin: true, HasMax: true}
}

func RollAtMost(max int) RollRange {
	return RollRange{Max: max, HasMax: true}
}

func RollAtLeast(min int) RollRange {
	return RollRange{Min: min, HasMin: true}
}

func (r RollRange) Contains(roll int) bool {
	return (!r.HasMin || roll >= r.Min) && (!r.HasMax || roll <= r.Max)
}

type RandomTable struct {
	Entries []TableEntry
}

// Every table is rolled with 4dF and so is indexed over -4..+4, with the ends
// left open so a modifier cannot push a roll off the table.
var tables = map[string]RandomTable{
	"npc-reaction": {
		Entries: []TableEntry{
			{RollAtMost(-4), "Violently hostile"},
			{RollRangeBetween(-3, -2), "Hostile"},
			{ExactRoll(-1), "Unreceptive"},
			{ExactRoll(0), "Neutral/uncertain"},
			{ExactRoll(1), "Receptive"},
			{RollRangeBetween(2, 3), "Friendly"},
			{RollAtLeast(4), "Extremely friendly/helpful"},
		},
	},
	"situational-roll": {
		Entries: []TableEntry{
			{RollAtMost(-4), "Extremely bad luck"},
			{ExactRoll(-3), "Very bad luck"},
			{ExactRoll(-2), "Bad luck"},
			{ExactRoll(-1), "Slight misfortune"},
			{ExactRoll(0), "Status quo"},
			{ExactRoll(1), "Slight fortune"},
			{ExactRoll(2), "Good luck"},
			{ExactRoll(3), "Very good luck"},
			{RollAtLeast(4), "Extremely good luck"},
		},
	},
	"weather": {
		Entries: []TableEntry{
			{RollAtMost(-4), "Catastrophic storm"},
			{ExactRoll(-3), "Storm"},
			{ExactRoll(-2), "Heavy rain/snow"},
			{ExactRoll(-1), "Light rain/overcast"},
			{ExactRoll(0), "Cloudy/moderate"},
			{ExactRoll(1), "Pleasant"},
			{ExactRoll(2), "Clear and fine"},
			{ExactRoll(3), "Beautiful weather"},
			{RollAtLeast(4), "Perfect weather"},
		},
	},
	"npc-morale": {
		Entries: []TableEntry{
			{RollAtMost(-3), "Surrender/Flee"},
			{ExactRoll(-2), "Wavering"},
			{ExactRoll(-1), "Unsure"},
			{ExactRoll(0), "Steady"},
			{ExactRoll(1), "Confident"},
			{ExactRoll(2), "Determined"},
			{RollAtLeast(3), "Fanatic"},
		},
	},
}
