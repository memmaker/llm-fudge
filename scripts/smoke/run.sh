#!/usr/bin/env bash
# Smoke tests: real LLM, real binary, loose assertions.
#
# Each <name>.txt is a -script fixture (see ../dryfall.txt for the format).
# It should end with "/dump-state" then "/quit" so there is a ground-truth
# state-dump.yaml to check alongside the debug log.
#
# Each <name>.expect is a list of grep -E patterns, one per line. A pattern
# prefixed with "!" must NOT appear; everything else must. Patterns are
# checked against the debug log and the state dump combined, so either can
# satisfy them. Blank lines and "#" comments are skipped.
#
# Usage:
#   scripts/smoke/run.sh              # every *.txt in this directory
#   scripts/smoke/run.sh combat foo   # just combat.txt and foo.txt
set -uo pipefail

cd "$(dirname "$0")/../.."
out="scripts/smoke/out"
mkdir -p "$out"

go build -o fudge . || exit 1

names=("$@")
if [ ${#names[@]} -eq 0 ]; then
	for f in scripts/smoke/*.txt; do
		names+=("$(basename "${f%.txt}")")
	done
fi

fail=0
for name in "${names[@]}"; do
	script="scripts/smoke/$name.txt"
	expect="scripts/smoke/$name.expect"
	log="$out/$name.log"
	state="$out/$name.state.yaml"

	if [ ! -f "$script" ]; then
		echo "SKIP $name (no $script)"
		continue
	fi

	rm -f state-dump.yaml
	./fudge -debug -script "$script" >"$log" 2>&1
	run_status=$?
	[ -f state-dump.yaml ] && mv state-dump.yaml "$state" || : >"$state"

	if [ $run_status -ne 0 ]; then
		echo "FAIL $name: exited $run_status"
		fail=1
		continue
	fi

	haystack="$(cat "$log")"$'\n'"$(cat "$state" 2>/dev/null)"
	broke=""
	if [ -f "$expect" ]; then
		while IFS= read -r pattern; do
			[ -z "$pattern" ] && continue
			case "$pattern" in
			\#*) continue ;;
			'!'*)
				needle="${pattern#!}"
				if grep -Eq "$needle" <<<"$haystack"; then
					broke+="  unexpected: $needle"$'\n'
				fi
				;;
			*)
				if ! grep -Eq "$pattern" <<<"$haystack"; then
					broke+="  missing: $pattern"$'\n'
				fi
				;;
			esac
		done <"$expect"
	fi

	if [ -n "$broke" ]; then
		echo "FAIL $name"
		printf '%s' "$broke"
		fail=1
	else
		echo "PASS $name"
	fi
done

exit $fail
