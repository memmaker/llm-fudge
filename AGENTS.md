This is the go (golang) source code for the fudge RPG system project.
Any code written must be written in go.
Read the existing code before making any changes.
Never write python or javascript code.

# Never grep prose

**We never grep prose to achieve anything. It is unreliable, full stop.**

No test, check script, or verdict may be built on pattern-matching narration,
transcripts, logs or any other text the model wrote. A phrase can be reworded,
wrapped, split across a line, or said in words nobody anticipated — a pattern
that misses reports success, which is worse than no test at all.

What to use instead, in order of preference:

1. **Structured artefacts the harness itself wrote**: tool calls and their
   arguments in `log/context-NNN.json`, `metadata.json`, `charsheet-log.md`
   entries, exit codes, file existence. The harness knows what happened; it does
   not have to guess from prose.
2. **An LLM judge** reading `transcript.md` against a written rubric, when the
   question is genuinely about meaning ("did the DM give away the solution?").
   A model reads prose; a regex does not.

This applies to `consistency-check.sh` and `secrets-check.sh` as they stand —
both currently grep, and both are wrong for it. See the TODO below.

# Working on a model-behaviour issue

The DM is a model, so its failures are not bugs that reproduce on demand — they
recur at some rate. The loop is therefore: reproduce, count, fix, count again.

1. **Name the issue in one sentence.** "The DM gives away the puzzle solution."
   "The world state stops being updated after a fight." One issue at a time; a
   script that chases two proves neither.

2. **Write a session script that provokes it three different ways.** Three
   distinct player actions, not the same action three times — one phrasing that
   happens not to trigger it would otherwise read as a fix. Keep the script as
   short as the issue allows: every turn before the interesting one is latency
   and money. Model it on `secrets-check.sh`, which asks after the statues, the
   sign, the solution and the GM's notes — four ways into one secret.

3. **Run it and record everything.** The runner plays the script and copies the
   session directory out; it renders no verdict. Each run leaves
   `transcript.md` (what the player read and typed), `log/context-NNN.json`
   (every request and every tool call in it) and `charsheet-log.md` (the sheet
   each turn). Run it several times — three or more — because one run of a model
   is an anecdote.

4. **Judge by reading, and give a rate.** An LLM reads the transcripts against
   the rubric in `consistency-check.md` and says what it found:
   "reproduced in 2 of 3 runs, both on the sign question". Never a grep — see
   the rule above. Where the question is structural rather than about meaning
   ("was the `secret` tool called at all?"), answer it from the context
   sidecars, which are JSON.

5. **Fix, then go back to 3.** Change one thing, re-run the same script, count
   again. Compare rates, not single runs, and keep the numbers honest: a fix
   that moves 3-of-3 to 2-of-3 has barely moved.

6. **Check the fix did not break the game.** A prompt that stops the DM saying
   too much can also stop it doing anything — a rule reading like a ban on tools
   once silenced the dice as well. Before believing a fix, play a scenario
   through to its end and confirm it still finishes.

Stop when the issue stops reproducing, or when the rate stops moving and the
next thing to try is a different kind of change. Record what was measured and
what was only observed; ADRs 008 and 009 are what that looks like written down.

# Open TODO

- **The DM is never told it forgot the *first* world update block.** A session
  whose opening turn emits no block is never corrected — the missing-block note
  is gated on a sheet having been written at least once, so no block means no
  note means no block, for the whole run. Measured on 2026-08-08: about a third
  of otherwise clean runs (31 of ~90, rate-limited runs excluded) finished
  without ever writing a `charsheet.md`, which also means `withCharsheet` sent
  no sheet and no tail rules for the entire session. **Left open on purpose**
  (decision 2026-08-09): nagging during character creation risks a half-made
  sheet being copied forward as truth, which is the worse failure. ADR 009 makes
  such a run readable — its logs now survive — but not correct.
- Because of the above, the checklist at the tail of `prompts/*/charsheet-header.md`
  was absent from about a third of the runs it was credited for. Re-measure
  whether it still earns its tokens: `./secrets-check.sh 6` with and without it.
- **Both check scripts grade by grepping prose, which the rule above forbids.**
  `secrets-check.sh` matches leak phrases against the terminal capture;
  `consistency-check.sh` has a red-flags grep over the same. Every number either
  has produced — including the 0/8 → 20/22 secret-keeping result recorded in ADR
  008 — is only as good as a pattern list, and must be treated as indicative
  rather than measured until they are rebuilt.
  - For secret-keeping specifically there is a structural answer that needs no
    prose at all: since ADR 008 the real secret text can only reach the DM
    through a `secret` tool call, and every tool call is recorded in
    `log/context-NNN.json`. "Did the DM read a secret whose trigger the player
    never fired?" is then a question about recorded calls, not about words. What
    that does not catch is the DM *guessing* a solution it never read — that one
    is about meaning, so it wants an LLM judge over `transcript.md`.
- `consistency-check.sh` copies the newest `saves/*/charsheet.md` by mtime, so a
  run that wrote no sheet is silently graded against the previous run's. Since
  ADR 009 the session directory exists from startup, so the run's own directory
  can be captured directly instead of guessed at by mtime.
