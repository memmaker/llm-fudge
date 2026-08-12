#!/usr/bin/env bash
# Plays the three-trigger reproduction script for one error type.
#
# A full pass is ~120 model calls and most of them exist to walk the character
# back to where things go wrong. These resume into a recorded turn instead and
# spend their calls on the error itself: three player lines, each a different way
# of provoking the same type, so one phrasing that happens not to fire cannot
# read as a fix (AGENTS.md, step 2).
#
# Three lines is three triggers. It is not always three API calls: a line the DM
# answers in prose costs one request, a line that moves the world costs two or
# three, because the tool round and the narration are their own calls. The
# question-shaped types — secret-leaked, meta-leak — do come in at three.
#
# Two types are once per session and say so in their own header: a run either
# writes a first sheet or it does not, and character creation happens once. Run
# those files three times and count runs.
#
# Usage: ./repro.sh <type> [runs]     e.g. ./repro.sh secret-leaked 5
set -euo pipefail
cd "$(dirname "$0")"

TYPE=${1:?type — one of: $(ls repro/*.txt | xargs -n1 basename | sed 's/\.txt//' | tr '\n' ' ')}
RUNS=${2:-3}
# The third argument is the language, for a probe written in one: it reaches the
# binary as -lang and picks the prompt and settings tree with it.
LANG_ARG=${3:-}
# A path instead of a type runs that file as it stands: the probes under probes/
# ask one question each and are not a finding vocabulary, so they are not gated
# on consistency-types.txt and their findings.tsv is left for the judge to name.
if [ -f "$TYPE" ]; then
	SCRIPT="$TYPE"
	TYPE=$(basename "$SCRIPT" .txt)
	JUDGED=""
else
	SCRIPT="repro/$TYPE.txt"
	JUDGED="$TYPE"
	[ -f "$SCRIPT" ] || { echo "no script for '$TYPE'"; exit 1; }
	grep -qxF "$TYPE" consistency-types.txt || { echo "'$TYPE' is not in consistency-types.txt"; exit 1; }
fi

# A script may name more than one recording to resume into, and they differ in
# ways that turn out to matter (probes/join.txt). Runs take them in turn, so a
# fix that only works from one history shows up as half the runs.
RESUMES=($(sed -n 's|^# resume: ||p' "$SCRIPT"))
OUT="consistency-out/$(date +%Y%m%d-%H%M%S)-repro-$TYPE"
mkdir -p "$OUT"

# The comments are for the reader; the binary gets the player lines and a quit.
{ grep -v '^#' "$SCRIPT" | grep -v '^[[:space:]]*$'; echo /quit; } >"$OUT/$TYPE.txt"

go build -o fudge .
for i in $(seq 1 "$RUNS"); do
	args=(-debug -script "$OUT/$TYPE.txt")
	if [ -n "$LANG_ARG" ]; then args+=(-lang "$LANG_ARG"); fi
	if [ ${#RESUMES[@]} -gt 0 ]; then
		RESUME=${RESUMES[$(( (i - 1) % ${#RESUMES[@]} ))]}
		[ -f "$RESUME" ] || { echo "resume point missing: $RESUME"; exit 1; }
		args+=(-resume "$RESUME")
		echo "run $i: resuming from $RESUME"
	else
		args+=(-setting medieval-fantasy -adventure The-Hall-of-Colors)
	fi

	./fudge "${args[@]}" 2>&1 | LC_ALL=C sed -E $'s/\x1b\\[[0-9;?]*[a-zA-Z]//g' >"$OUT/run$i-terminal.log"
	# Read the directory the binary announced. Diffing `ls saves` instead breaks
	# the moment two of these run at once — see the note in consistency-check.sh.
	session=$(sed -n 's/^\[saving to saves\/\(.*\)\]$/\1/p' "$OUT/run$i-terminal.log" | head -1)
	[ -z "$session" ] && { echo "run $i wrote no session"; continue; }
	cp -R "saves/$session" "$OUT/run$i"

	if grep -q 'API 429' "$OUT/run$i-terminal.log" || grep -q 'did not narrate this turn' "$OUT/run$i-terminal.log"; then
		printf '# run: run%s\n# spoiled: yes\n# judged: %s\n\ntype\tproof\tquote\n' "$i" "$JUDGED" >"$OUT/run$i/findings.tsv"
		echo "run $i: SPOILED (backend), verdict pre-filled"
	else
		# The header is pre-filled because the denominator is not the judge's to
		# choose: this script exists to read one type, and a run of it that found
		# nothing still counts as a run that looked.
		printf '# run: run%s\n# spoiled: no\n# judged: %s\n\ntype\tproof\tquote\n' "$i" "$JUDGED" >"$OUT/run$i/findings.tsv"
		echo "run $i: done — read it and add a row to $OUT/run$i/findings.tsv if it occurred"
	fi
done

cat <<EOF

$OUT
Read each run against the three triggers in $SCRIPT and record the verdict in
its findings.tsv. Then ./ledger.sh. A run left unjudged is named by the ledger,
not counted as clean.
EOF
