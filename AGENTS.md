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

3. **Resume into the failing turn instead of playing up to it.** A full pass is
   ~25 requests per run and most of them only exist to walk the character back
   to where it goes wrong. Every recorded turn is a resume point:

   ```bash
   ./fudge -debug -resume consistency-out/<pass>/<run>/log/context-NNN.json -script one-line.txt
   ```

   Setting, adventure, character and the whole conversation up to that turn come
   from the recording; the script is the one player line you want to try, then
   `/quit`. One or two requests instead of twenty-five, so a variation can be
   tried, read and thrown away in a minute. Pick the sidecar by reading
   `transcript.md` and counting turns — `context-NNN.json` is the request that
   *opened* turn NNN.

   Two limits, both from ADR 009. A resumed run continues from a conversation
   today's prompts did not produce, so it measures the prompt's effect on *that
   turn*, not on a session; and a recording made before a prompt change carries
   the old behaviour in its history, which is the point when the question is
   "does the new wording recover a turn the old one lost" and a confound when it
   is not. Confirm a fix that passes here with one full run before believing it.

4. **Run it and record everything.** The runner plays the script and copies the
   session directory out; it renders no verdict. Each run leaves
   `transcript.md` (what the player read and typed), `log/context-NNN.json`
   (every request and every tool call in it) and `charsheet-log.md` (the sheet
   each turn). Run it several times — three or more — because one run of a model
   is an anecdote.

5. **Judge by reading, and give a rate.** An LLM reads the transcripts against
   the rubric in `consistency-check.md` and says what it found:
   "reproduced in 2 of 3 runs, both on the sign question". Never a grep — see
   the rule above. Where the question is structural rather than about meaning
   ("was the `secret` tool called at all?"), answer it from the context
   sidecars, which are JSON.

6. **Fix, then go back to 4.** Change one thing, re-run the same script, count
   again. Compare rates, not single runs, and keep the numbers honest: a fix
   that moves 3-of-3 to 2-of-3 has barely moved.

7. **Check the fix did not break the game.** A prompt that stops the DM saying
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
  without ever writing a `charsheet.md`. **Left open on purpose** (decision
  2026-08-09): nagging during character creation risks a half-made sheet being
  copied forward as truth, which is the worse failure. ADR 009 makes such a run
  readable — its logs now survive — but not correct.
  - Half of that is now closed: since 2026-08-09 an empty-handed request sends
    the template where the sheet would go (`charsheet-none.md`), so the demand
    and the tail checklist reach every turn including the first. The demand
    itself is no longer stated anywhere else — `dm.md`, `scene-demand.md` and
    `opening-briefing.md` each used to repeat it in their own wording.
  - **Count blockless turns only where the turn changed something.** A turn that
    answers a question or restates the room changes nothing the sheet records,
    and no block is the right answer to it; counting those as failures made the
    problem look about twice its size. The measurement that matters is narrower
    still: did the change reach the *final* sheet? A miss mid-session is
    recovered, because the note fires on the next player line and the next block
    restates everything. A miss on the **last** turn of a session is not — there
    is no next turn — and that is where the two losses below happened.
  - Measured 2026-08-09 against `nvidia/nemotron-3-ultra-550b-a55b`, five-turn
    sessions in which four turns must update the sheet (character made, fact
    learned, statue moved, item dropped):

    | Wording of the demand at the tail | Updates | Runs that lost state |
    | --- | --- | --- |
    | "every reply that narrates anything" | 10/12 | 0 of 3 |
    | keyed to what changed (**in place**) | 19/20 | 0 of 8 |
    | names `world_update` as the only way | 11/12 | 1 of 3 |
    | consequence framing ("it did not happen") | 10/12 | 1 of 3 |

    Keying the demand to what changed won, and it is also the wording that
    matches how the result is judged. Consequence framing — telling the model
    the scene is not remembered — measured worst of the four despite reading as
    the most convincing.
  - The `world_update` tool went in at the same time and is now the path this
    model actually uses: 4 to 6 calls per five-turn session, with the prose
    block near zero. An earlier note here said the tool was never called; that
    was a bad grep over a coloured terminal log, not a measurement. Count tool
    calls from `log/context-*.json`, never from the terminal capture.
- **A sheet missed on the last turn of a session is lost for good.** The
  missing-block note fires on the next player line, so every other miss is
  repaired by the next full restatement; the final turn has no next line. Two of
  the fourteen runs measured on 2026-08-09 lost a change this way. A cheap fix
  would be one request at `/quit` when the last turn handed over nothing.
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
- ~~`consistency-check.sh` copies the newest `saves/*/charsheet.md` by mtime~~ —
  done: it now captures the session directory that appeared while the run was
  playing, so a run that wrote no sheet shows up as a missing `charsheet.md`
  rather than as the previous run's.
