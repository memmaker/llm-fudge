#!/usr/bin/env bash
# Live smoke test of the DM against settings/en/medieval-fantasy/adventures/The-Hall-of-Colors.md.
# Plays six scripted sessions and records what each one did. It renders no
# verdict and matches no text: whether the DM stayed consistent is a question
# about meaning, and reading is a job for an LLM, not for a pattern — see the
# rule in AGENTS.md. Hand the artifacts to a model with consistency-check.md as
# the rubric.
#
# Usage: ./consistency-check.sh [rules|win|death|probe|state|items|drift]...  (default: all seven)
set -euo pipefail
cd "$(dirname "$0")"

OUT="consistency-out/$(date +%Y%m%d-%H%M%S)"
mkdir -p "$OUT"

# Setting and adventure are flags, not script lines: the menus test nothing and
# a renamed option would break every script here at once. The character is made
# in conversation and stays in the script — it is the DM's first real job.
WORLD=(-setting medieval-fantasy -adventure The-Hall-of-Colors)
OPENING='A cautious mercenary called Testa. Pick her traits yourself, then begin the first scene.
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

# The state run needs a known inventory to manipulate, so this opening names the
# gear instead of leaving it to the DM. Everything else about it is the same
# conversation: the character is still made in play.
# One line, however long: the script feeds a line per turn, so a wrapped opening
# would be played as three separate player actions.
STATE_OPENING='A cautious mercenary called Testa. Give her a 15 m rope, three torches, six rations, a waterskin, a dagger and twelve silver coins, and pick the rest of her traits yourself. Then begin the first scene.
Yes, that works.'

# Ten actions, every one of which moves something the sheet records, and five of
# them inventory: an item leaves, comes back, is split in two, is put back
# together, and a stack is spent down. The other five move the world instead — a
# fact learned, two statues, a room entered, a fight joined. Nothing here asks a
# question or looks around: a turn that changes nothing is a turn with nothing to
# measure, and the whole point of this run is that every turn owes an update.
#
# Nothing here depends on winning the fight. Looting the chest was the obvious
# tenth action and is the wrong one: the kobold has 20 HP, a turn of attacks
# rarely finishes it, and the run then ends on a turn that correctly changes
# nothing because the chest is still guarded. Striking the kobold moves the sheet
# whichever way the dice fall.
script_state() {
	cat <<-EOF
		$STATE_OPENING
		I drop my rope on the floor and leave it lying there.
		I pick the rope back up.
		I cut the rope into two equal halves with my dagger.
		I tie the two halves back together into one rope.
		I light one of my torches.
		I close one eye and look at both statues.
		I shove the red statue onto the blue pedestal with all my strength.
		I shove the blue statue onto the red pedestal with all my strength.
		I go through the door into room 2.
		I attack the kobold with my weapon.
		/quit
	EOF
}

# The items run needs a kit with parts to it, so its opening names one. One line,
# for the reason above.
ITEMS_OPENING='A cautious mercenary called Testa. Give her a 15 m rope, a crossbow with twelve bolts, three torches, a dagger, an iron spike and twelve silver coins, and pick the rest of her traits yourself. Then begin the first scene.
Yes, that works.'

# Splitting and recombining is the one operation no wording has fixed: a rope cut
# in two survives the turn that ties it back together, or comes back the wrong
# length. This run does nothing else. Twelve actions, all of them in room 1, none
# of them touching the puzzle, the door or the kobold — a bookkeeping failure
# here cannot be blamed on the adventure.
#
# It works up in difficulty. The rope halves are one object of one kind. The
# grappling line joins two kinds into one and takes them apart again. The
# crossbow comes apart into three named parts, and then a part of it and a part
# of a torch become a third object that was in neither. The coins are the same
# question with numbers instead of lengths, which a model can get right by
# arithmetic while getting the rope wrong by imagination.
script_items() {
	cat <<-EOF
		$ITEMS_OPENING
		I cut the rope into two equal halves with my dagger.
		I tie one half to the iron spike to make a grappling line.
		I untie the spike from that rope half again.
		I tie the two rope halves back together into one rope.
		I take my crossbow apart into stock, prod and string.
		I pull the head off one torch and keep the bare wooden shaft.
		I string the crossbow's string onto the bare shaft to make a short bow.
		I take the string off the shaft and put the crossbow back together.
		I lash the torch head back onto its shaft with a short length cut from my rope.
		I split my silver coins into two purses of six.
		I tip both purses back into one.
		I drop the iron spike and leave it lying there.
		/quit
	EOF
}

# The drift run needs gear it can spend and change, so its opening names some.
# One line, for the reason above.
DRIFT_OPENING='A cautious mercenary called Testa. Give her a full waterskin, three torches, six rations, a dagger and a 15 m rope, and pick the rest of her traits yourself. Then begin the first scene.
Yes, that works.'

# Divergence between what the DM says and what the sheet holds. Every other run
# asks whether a turn handed over a sheet at all; this one asks whether the sheet
# it handed over is the world the DM just narrated, and whether the DM still
# believes it two turns later.
#
# That is a different failure and it is the expensive one. A missed handover is
# recorded as "nothing changed" when what happened is "unknown", the stale line
# is pinned to every later request, and the header tells the DM the sheet
# outranks anything said earlier — so one omission hardens into a world where a
# door the player opened is locked again. It cannot be seen in a single turn:
# the turn that narrates the change looks perfect. It shows up only when
# something later depends on the change.
#
# So every probe here is a triple: change it, let a turn pass, then act on it.
# The acting turn is the measurement — the change turn never fails visibly.
# Five of them, and the first three are deliberately free of the puzzle, the
# door and the kobold, so a push that goes wrong voids rows 8-13 and nothing
# else.
script_drift() {
	cat <<-EOF
		$DRIFT_OPENING
		I pour my waterskin out on the floor until it is completely empty.
		I light one of my torches.
		I eat two of my rations.
		I drink from my waterskin.
		I put the burning torch out and stow it again.
		I eat one more ration.
		I close one eye and look at both statues.
		I shove the red statue onto the blue pedestal with all my strength.
		I shove the blue statue onto the red pedestal with all my strength.
		I go through the door into room 2.
		I go back into room 1.
		I go through the door into room 2 again.
		What did the statues say when I closed one eye?
		/quit
	EOF
}

run() {
	local name=$1
	echo "=== $name ==="
	"script_$name" >"$OUT/$name.txt"

	# The run's own session directory, taken from the line the binary prints when
	# it opens one. Diffing `ls saves` before and after looks equivalent and is
	# not: two of these running at once will each pick up the other's session, and
	# the artifacts then get read under the wrong script's name. That happened on
	# 2026-08-09 — two concurrent measurements produced nine directories holding
	# seven sessions, two of them filed as a run they were not.
	local session
	# -debug so the log carries the dice and the file writes, not just the prose.
	./fudge -debug "${WORLD[@]}" -script "$OUT/$name.txt" 2>&1 |
		LC_ALL=C sed -E $'s/\x1b\\[[0-9;?]*[a-zA-Z]//g' | tee "$OUT/$name-terminal.log"
	session=$(sed -n 's/^\[saving to saves\/\(.*\)\]$/\1/p' "$OUT/$name-terminal.log" | head -1)

	if [ -z "$session" ] || [ ! -d "saves/$session" ]; then
		echo "$name wrote no session directory"
		return
	fi
	cp -R "saves/$session" "$OUT/$name"
}

go build -o fudge .
# Unquoted on purpose: the default has to split into seven words, and a run name
# is a single word either way.
for name in ${@:-rules win death probe state items drift}; do run "$name"; done

cat <<EOF

Artifacts: $OUT
  <run>/transcript.md          what the player read and what they typed
  <run>/log/context-*.json     every request, and every tool call in it
  <run>/charsheet-log.md       the sheet as it stood each turn
  <run>-terminal.log           the screen, dice and file writes included

Nothing here is a verdict. Read the transcripts against consistency-check.md.
EOF

