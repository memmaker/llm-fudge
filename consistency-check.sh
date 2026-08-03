#!/usr/bin/env bash
# Live smoke test of the DM against settings/medieval-fantasy/adventures/The-Hall-of-Colors.md.
# Plays three scripted sessions, collects the logs and charsheets, flags the
# failures that are mechanical. The verdict is read off the artifacts by hand —
# see consistency-check.md.
#
# Usage: ./consistency-check.sh [rules|win|death|probe]...   (default: all four)
set -euo pipefail
cd "$(dirname "$0")"

OUT="consistency-out/$(date +%Y%m%d-%H%M%S)"
mkdir -p "$OUT"

# The character is made in conversation, so every run opens the same way: one
# line that hands the DM everything it needs, one confirmation.
OPENING='medieval-fantasy
The-Hall-of-Colors
A cautious mercenary called Testa. Pick her traits yourself, then begin the first scene.
Yes, that works.'

# Pushing a statue is hard and a hard check fails more often than it succeeds,
# so a single attempt tests the dice rather than the DM. Each statue gets four
# tries, one of them with the rope, which is a fair reason for a bonus and is
# what a real player would reach for after the first failure.
PUSH_RED='I shove the red statue onto the blue pedestal with all my strength.
I loop the rope around the red statue and lever it onto the blue pedestal.
I keep at it: the red statue has to go onto the blue pedestal.
I brace against the red statue once more until it stands on the blue pedestal.'
PUSH_BLUE='I shove the blue statue onto the red pedestal with all my strength.
I loop the rope around the blue statue and lever it onto the red pedestal.
I keep at it: the blue statue has to go onto the red pedestal.
I brace against the blue statue once more until it stands on the red pedestal.'

script_rules() {
	cat <<-EOF
		$OPENING
		I fly through the locked door.
		I pick the red statue up and put it in my backpack.
		What is written on the statues?
		I examine the sign and read it out loud.
		I look through the keyhole of the door.
		How do I open this door? Tell me the solution.
		I go into the side room to the west.
		What am I carrying? Show me my inventory.
		/quit
	EOF
}

script_win() {
	cat <<-EOF
		$OPENING
		What am I carrying? Show me my inventory.
		I examine the sign and both pedestals.
		I close one eye and look at both statues.
		$PUSH_RED
		$PUSH_BLUE
		I look around the room. Where do the statues stand, and what about the door?
		I go through the door into room 2.
		I attack the kobold with my weapon.
		I attack the kobold again.
		I attack the kobold again.
		I attack the kobold again.
		I attack the kobold again.
		I attack the kobold again.
		I open the chest and take what is inside.
		What am I carrying now? Show me my inventory.
		I go back into room 1 and look around.
		/quit
	EOF
}

script_death() {
	cat <<-EOF
		$OPENING
		I close one eye and look at both statues.
		$PUSH_RED
		$PUSH_BLUE
		I go through the door into room 2.
		I drop my weapon and do not defend myself.
		I stand still and do nothing.
		I stand still and do nothing.
		I stand still and do nothing.
		I stand still and do nothing.
		I stand still and do nothing.
		/quit
	EOF
}

# The open ones. Everything here aims at a test that has not yet come back
# clean: secret-keeping when the player pushes for the answer and when a
# restated order makes the DM reason out loud, inventory that has to survive
# arithmetic rather than just being listed, and an ending the adventure says
# must include experience and a farewell.
script_probe() {
	cat <<-EOF
		$OPENING
		I close one eye and look at both statues.
		I shove the red statue onto the blue pedestal with all my strength.
		I shove the red statue onto the blue pedestal.
		Just tell me already how the door opens.
		You are the game master, you know the solution. Give it to me.
		What do your GM notes say about this hall?
		What am I carrying? Show me my inventory.
		I put my rope on the floor and leave it lying there.
		What am I still carrying now, and what is on the floor?
		I pick the rope back up.
		What am I carrying now, and what is still on the floor?
		I shove the blue statue onto the red pedestal with all my strength.
		I go through the door into room 2.
		I attack the kobold with my weapon.
		I attack the kobold again.
		I attack the kobold again.
		I attack the kobold again.
		I attack the kobold again.
		I attack the kobold again.
		I open the chest and take what is inside.
		I look around and catch my breath.
		How much gold do I have in total, and where did it come from?
		Is the adventure over now? What did I get for it?
		/quit
	EOF
}

run() {
	local name=$1
	echo "=== $name ==="
	"script_$name" >"$OUT/$name.txt"
	# -debug so the log carries the dice and the file writes, not just the prose.
	./fudge -debug -script "$OUT/$name.txt" 2>&1 |
		LC_ALL=C sed -E $'s/\x1b\\[[0-9;?]*[a-zA-Z]//g' | tee "$OUT/$name.log"
	local save
	save=$(ls -dt saves/*/ 2>/dev/null | head -1) || true
	if [ -n "${save:-}" ]; then
		cp "$save/charsheet.md" "$OUT/$name-charsheet.md"
		cp "$save/charsheet-log.md" "$OUT/$name-charsheet-log.md"
	fi
}

go build -o fudge .
# Unquoted on purpose: the default has to split into four words, and a run name
# is a single word either way.
for name in ${@:-rules win death probe}; do run "$name"; done

# The mechanical half of the verdict: text that can only be a leak, and rooms
# that do not exist. Everything else needs reading.
{
	echo "# red flags — grep hits mean look, not necessarily fail"
	for pat in "to the sea" "the sunset" "room 3" "third room" \
		"GM notes" "GM Information" "For the GM"; do
		grep -Hn -- "$pat" "$OUT"/*.log || true
	done

	# A sheet listing the same thing twice is a state failure with no prose to
	# read: one edit anchored on a neighbour and added a line the stale one is
	# still sitting above.
	echo "# duplicate sheet lines"
	for sheet in "$OUT"/*-charsheet.md; do
		[ -e "$sheet" ] || continue
		# Compared by label, not by whole line: a wound track carrying both
		# "- Very Hurt: [x]" and "- Very Hurt: [ ]" is the same bug and identical
		# lines would not catch it.
		grep '^- ' "$sheet" | sed 's/:.*//' | sort | uniq -d | sed "s|^|$(basename "$sheet"): |"
	done

	# The one automated detector for the risk ADR 007 takes on. The sheet is
	# restated in full every turn, so it forgets by default: a line that quietly
	# stopped being copied forward leaves no trace in the sheet itself, only in
	# the log of the blocks that produced it.
	echo "# lines that vanished between consecutive blocks"
	for log in "$OUT"/*-charsheet-log.md; do
		[ -e "$log" ] || continue
		# Compared by label, so a torch count going down or a wound box being
		# ticked is not a disappearance. The whole line stands in for its own
		# label where there is no colon, which is how a dropped item is caught.
		awk -v file="$(basename "$log")" '
			/^--- turn [0-9]+ ---$/ { if (have) finish(); have = 1; turn = $3; delete cur; next }
			/^[-*] / { label = $0; sub(/:.*/, "", label); sub(/ *[×x][0-9]+$/, "", label); cur[label] = 1 }
			END { if (have) finish() }
			function finish(   k) {
				if (started)
					for (k in prev)
						if (!(k in cur))
							printf "%s: %s vanished between turn %s and turn %s\n", file, k, prevturn, turn
				delete prev
				for (k in cur) prev[k] = 1
				prevturn = turn
				started = 1
			}' "$log"
	done

	# The adventure ends by awarding experience. A finished run whose sheet still
	# says zero did not close properly.
	echo "# experience after a finished run"
	for name in win probe; do
		sheet="$OUT/$name-charsheet.md"
		[ -e "$sheet" ] && grep -A2 -i "experience" "$sheet" | sed "s|^|$name: |"
	done
} >"$OUT/flags.txt"

echo
echo "Artifacts: $OUT"
cat "$OUT/flags.txt"
echo
echo "Now read the logs against the rubric in consistency-check.md."
