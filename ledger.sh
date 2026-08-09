#!/usr/bin/env bash
# Builds consistency-ledger.md from the verdicts recorded beside the logs.
#
# The sessions are the source of truth. This accumulates them and does nothing
# else: it counts findings and lays them out. Whether the narration and the state
# agree is the one thing no script can decide — that is a judgement, made by an
# LLM reading the run, and it is written down in the run's own directory.
#
# Three files per run, each carrying what it is entitled to carry:
#
#   metadata.json  written by the binary — commit, branch, model, date. Facts
#                  about the run nobody should be retyping.
#   findings.tsv   the judgement — what was seen, where, and the quote proving
#                  it. The proof lives here, beside the logs it points into.
#   transcript.md  and the rest of the session, which the proofs cite.
#
#   # run: items
#   # spoiled: no
#   # judged: invented-content item-bookkeeping state-desync
#   type<TAB>proof<TAB>quote
#   invented-content<TAB>charsheet.md:73<TAB>Silver: 5, never granted
#
# Only what was found gets a row. What was *looked for* is the `judged` line, and
# it is not optional: it is the denominator. Without it a type nobody examined
# would be indistinguishable from a type that did not occur, and every run in the
# group would silently count as evidence for it. `judged: all` means every type
# in consistency-types.txt was read for.
#
# The ledger cites the session directory, not the individual proof: the finding
# is the session's own record, and following the link is how it gets checked.
#
# A run with no findings.tsv is unjudged, not clean. It is left out and named at
# the end, because a ledger that quietly drops what nobody has read yet is worse
# than no ledger.
#
# Usage: ./ledger.sh [consistency-out] > consistency-ledger.md
set -euo pipefail
cd "$(dirname "$0")"
ROOT=${1:-consistency-out}
TYPES=consistency-types.txt

field() { sed -n "s/.*\"$2\": *\"\([^\"]*\)\".*/\1/p" "$1" | head -1; }

# Unknown types are refused rather than counted. The damage is silent otherwise:
# two spellings of one finding become two rows that never add up.
bad=0
while IFS= read -r f; do
	# The rows, and the judged list in the header, which is just as capable of
	# holding a typo and would silently inflate a denominator instead of a count.
	while IFS=$'\t' read -r type _; do
		case "$type" in '#'* | 'type' | '') continue ;; esac
		grep -qxF "$type" "$TYPES" || { echo "$f: unknown type '$type' — add it to $TYPES or use an existing one" >&2; bad=1; }
	done <"$f"
	for type in $(sed -n 's/^# *judged: *//p' "$f" | head -1); do
		[ "$type" = "all" ] && continue
		grep -qxF "$type" "$TYPES" || { echo "$f: unknown type '$type' in the judged list" >&2; bad=1; }
	done
done < <(find "$ROOT" -name findings.tsv | sort)
[ "$bad" = 1 ] && { echo "refusing to build a ledger with unknown types" >&2; exit 1; }

collect() {
	while IFS= read -r f; do
		dir=$(dirname "$f")
		meta="$dir/metadata.json"
		commit=$(field "$meta" commit); [ -z "$commit" ] && commit="unrecorded"
		model=$(field "$meta" model); [ -z "$model" ] && model="unrecorded"
		# The tier rides in the model's own heading. Two runs of one model on
		# different processing tiers are two measurements, and pooling them under
		# one heading is how a difference disappears into an average.
		tier=$(field "$meta" service_tier); [ -n "$tier" ] && model="$model ($tier)"
		branch=$(field "$meta" branch); [ -z "$branch" ] && branch="unrecorded"
		prompts=$(field "$meta" prompts); [ -z "$prompts" ] && prompts="unrecorded"
		date=$(field "$meta" started)
		run=$(sed -n 's/^# *run: *//p' "$f" | head -1)
		spoiled=$(sed -n 's/^# *spoiled: *//p' "$f" | head -1)
		[ "$spoiled" = "yes" ] && { printf '%s\t%s\t%s\t%s\t%s\t%s\tSPOILED\tSPOILED\n' "$date" "$prompts" "$commit" "$branch" "$model" "$dir"; continue; }

		# `judged` is what the run was read for; the rows are what was found in it.
		# Both are needed and neither implies the other: without `judged` a type
		# nobody looked for would read as a type that did not occur, and the
		# denominator would quietly count runs that never examined it.
		judged=$(sed -n 's/^# *judged: *//p' "$f" | head -1)
		[ "$judged" = "all" ] && judged=$(grep -v '^#' "$TYPES" | grep -v '^[[:space:]]*$' | tr '\n' ' ')
		for t in $judged; do
			printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\tLOOKED\n' "$date" "$prompts" "$commit" "$branch" "$model" "$dir" "$t"
		done

		awk -F'\t' -v d="$date" -v p="$prompts" -v c="$commit" -v b="$branch" -v m="$model" -v dir="$dir" '
			/^#/ || /^type\t/ || /^[[:space:]]*$/ { next }
			{ print d "\t" p "\t" c "\t" b "\t" m "\t" dir "\t" $1 "\tFOUND" }
		' "$f"
	done
}

# What each branch was trying, taken from the EXPERIMENT.md committed on that
# branch, so the description travels with the experiment instead of being
# remembered. Newlines are escaped to keep one branch on one line.
branches=$(mktemp)
trap 'rm -f "$branches"' EXIT
find "$ROOT" -name metadata.json -exec sed -n 's/.*"branch": *"\([^"]*\)".*/\1/p' {} + | sort -u |
	while IFS= read -r b; do
		[ -z "$b" ] && continue
		text=$(git show "$b:EXPERIMENT.md" 2>/dev/null | awk '{printf "%s\\n", $0}') || true
		# A branch with no EXPERIMENT.md is not an error — it simply has nothing to
		# say yet. `if` rather than `&&`, so the loop does not end on a false test
		# and take `set -e` with it.
		if [ -n "$text" ]; then printf '%s\t%s\n' "$b" "$text"; fi
	done >"$branches"

find "$ROOT" -name findings.tsv | sort | collect | awk -F'\t' -v blurbfile="$branches" '
	BEGIN {
		while ((getline line < blurbfile) > 0) {
			split(line, b, "\t")
			gsub(/\\n/, "\n", b[2])
			blurb[b[1]] = b[2]
		}
	}
	{
		date=$1; prompts=$2; commit=$3; branch=$4; model=$5; dir=$6; type=$7; occurred=$8
		# Grouped by the prompt set, not the commit. Prompts are read from disk at
		# run time, so a commit describes the binary and not what the model was
		# actually told — three commits today shipped byte-identical prompts, and
		# grouping by commit would have split one measurement into three.
		# Grouped by what was measured, not by when: every run has its own
		# timestamp, so a key carrying one puts each run in a section of its own.
		key = prompts "\t" model
		if (!(key in seen) || date < first[key]) first[key] = date
		seen[key] = 1
		commits[key] = commits[key] (index(commits[key], commit) ? "" : (commits[key] == "" ? "" : ", ") "`" commit "`")
		branches[key] = branch
		if (type == "SPOILED") { spoiled[key]++; next }
		kt = key "\t" type
		types[kt] = type
		sessions[key] = sessions[key] (index(sessions[key], dir) ? "" : "\n- `" dir "`")
		# LOOKED is the denominator, FOUND the numerator. A found type that was
		# never declared as looked-for still counts in both: finding it is proof
		# enough that somebody was reading for it.
		if (occurred == "LOOKED") runs[kt]++
		else { hits[kt]++; if (!((dir "\t" type) in counted)) runs[kt]++ }
		if (occurred == "LOOKED") counted[dir "\t" type] = 1
	}
	END {
		n = 0
		for (k in seen) order[++n] = first[k] "\t" k
		for (i = 1; i < n; i++)
			for (j = i + 1; j <= n; j++)
				if (order[i] > order[j]) { t = order[i]; order[i] = order[j]; order[j] = t }

		print "# Consistency ledger"
		print ""
		print "Generated by `./ledger.sh`. Do not edit — change the verdicts in the"
		print "sessions and regenerate. The sessions are the source of truth: each holds"
		print "the run\x27s own `metadata.json`, the judgement in `findings.tsv`, and the"
		print "logs its proofs cite."
		print ""
		print "Occurrences and Runs are run-level: in how many sessions did this happen at"
		print "all. A `-dirty` commit means the tree had uncommitted changes, so the"
		print "prompts were not the ones that commit names."

		lastprompts = ""
		for (i = 1; i <= n; i++) {
			split(order[i], f, "\t")
			# order carries the date only to sort by; the group is the rest of it.
			key = f[2] "\t" f[3]
			if (f[2] != lastprompts) {
				printf "\n---\n\n# prompt set `%s` — from %s — branch `%s`\n", f[2], substr(f[1], 1, 10), branches[key]
				printf "\nBuilt at: %s\n", commits[key]
				lastprompts = f[2]
				if (branches[key] in blurb) printf "\n%s", blurb[branches[key]]
			}
			printf "\n## %s\n\n| Type | Occurrences | Runs |\n| --- | --- | --- |\n", f[3]
			for (kt in types) {
				split(kt, g, "\t")
				if (g[1] "\t" g[2] != key) continue
				printf "| %s | %d | %d |\n", g[3], (kt in hits) ? hits[kt] : 0, runs[kt]
			}
			printf "\nSessions:%s\n", sessions[key]
			if (key in spoiled)
				printf "\nSpoiled and excluded: %d run(s).\n", spoiled[key]
		}
	}
'

# Unjudged runs are named on stderr so the ledger stays clean and the reminder
# still lands. A session directory is one that has a metadata.json.
find "$ROOT" -name metadata.json | sort | while IFS= read -r m; do
	d=$(dirname "$m")
	[ -f "$d/findings.tsv" ] || echo "unjudged: $d" >&2
done
