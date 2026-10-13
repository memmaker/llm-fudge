#!/usr/bin/env bash
# Live smoke test of the DM against settings/en/medieval-fantasy/adventures/The-Hall-of-Colors.md.
# Plays the scripted sessions under checks/ and records what each one did. It
# renders no verdict and matches no text: whether the DM stayed consistent is a
# question about meaning, and reading is a job for an LLM, not for a pattern —
# see the rule in AGENTS.md. Hand the artifacts to a model with
# consistency-check.md as the rubric.
#
# One file per run, in the same format repro/ and probes/ use: `#` lines are
# comments, everything else is a player line, and the file says in its own words
# what it is for. They were seven heredocs in this script until 2026-08-12.
#
# Usage: ./consistency-check.sh [rules|win|death|probe|state|items|drift|secrets]...
#        (default: all eight)
set -euo pipefail
cd "$(dirname "$0")"

OUT="consistency-out/$(date +%Y%m%d-%H%M%S)"
mkdir -p "$OUT"

# Setting and adventure are flags, not script lines: the menus test nothing and
# a renamed option would break every script here at once. The character is made
# in conversation and stays in the script — it is the DM's first real job.
WORLD=(-setting medieval-fantasy -adventure The-Hall-of-Colors)

run() {
	local name=$1
	local script="checks/$name.txt"
	[ -f "$script" ] || { echo "no script: $script"; return 1; }
	echo "=== $name ==="

	# The comments are for the reader; the binary gets the player lines and a
	# quit. The played script is kept beside the artifacts, so what ran is on
	# record even after the file it came from is edited.
	{ grep -v '^#' "$script" | grep -v '^[[:space:]]*$'; echo /quit; } >"$OUT/$name.txt"

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
# Unquoted on purpose: the default has to split into one word per run, and a run
# name is a single word either way.
for name in ${@:-rules win death probe state items drift secrets}; do run "$name"; done

cat <<EOF

Artifacts: $OUT
  <run>/transcript.md          what the player read and what they typed
  <run>/log/context-*.json     every request, and every tool call in it
  <run>/charsheet-log.md       the sheet as it stood each turn
  <run>-terminal.log           the screen, dice and file writes included

Nothing here is a verdict. Read the transcripts against consistency-check.md.
EOF
