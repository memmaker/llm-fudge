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

**HARD RULE: NEVER EVER GREP THE CHARSHEET.** Not with `grep`, not with a `sed`
range, not "just to pull out the Inventory section" — no pattern over
`charsheet.md`, `charsheet-log.md` or a sheet inside a `world_update` argument
may stand behind a number, a verdict or a claim that something was fixed. The
sheet is written by the model, so it is prose in a table's clothing: a section
can be renamed, a line reordered, an item annotated in place, a whole heading
invented. All four happened while measuring the dropped rope on 2026-08-09 — one
run put every dropped item under an invented `### Equipment on floor`, which any
Inventory-versus-World-State pattern reads as a pass. Read the sheet whole, with
your eyes or with an LLM judge against the rubric, and write the verdict into
`findings.tsv`.

No exceptions, including inside the harness. `doubledItem` was one — a narrow
match over the handed-over sheet that carried a note back to the DM about a part
left standing beside its whole — and it is deleted, along with
`note-doubled-item.md`. A pattern that reads the sheet teaches whoever finds it
that reading the sheet by pattern is allowed.

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

8. **Reproduce before comparing wordings.** A sweep of five wordings against a
   probe that never fails ranks noise and reads exactly like a result. Measured
   2026-08-09: the first `drift` run came back 50 of 50 rows clean across five
   arms and two rounds, because it opened the door on one clean push and tested
   it two turns later. The failure it was built for had happened with the door
   opening buried among retries and the test eight turns and a fight later.
   Establish a failure rate on the unmodified prompts first; that rate is the
   baseline every arm has to beat, and if there is no rate there is nothing to
   compare.

9. **A missed handover and a throttled turn are the same artifact.** Both leave
   a turn with no sheet. Mark any run whose console holds a 429, or that ends
   without narrating, and count it in neither column — a 502 the retry loop
   recovered from is fine, the model did answer. Two model switches this session
   came from measuring through rate limits before noticing.

## What the handover actually fails at

Measured 2026-08-09, and worth not re-deriving. The failure is two separate
things that look like one:

- **A world-changing turn hands over nothing.** Common, and on every model
  tried. Five full runs on `oc/nemotron-3-ultra-free`: 26 player lines each, 11
  to 14 sheets, and the missing-sheet note fired 11 to 14 times per run. Roughly
  half of all turns miss.
- **The repair fails to reconcile.** This is the one that ends adventures. The
  note fires either way; what differs is what the model does with it. Nemotron
  rewrites the changed lines from what it narrated and the run survives — the
  door ended open in 5 of 5. Deepseek restated the sheet it was handed: in
  `consistency-out/20260809-122730/win` the note fired 17 times, only 6 of ~26
  turns handed over, and every one of them repeated `Door: closed, locked` after
  the prose had said the door swung open and stayed open.

So the model is not failing to *notice*. That same deepseek run wrote "world
state in your sheet is stale" to the player and still copied the stale sheet
forward. Prompting aimed at making it notice, or at reminding it to call, is
aimed at the half that already works. The wording that has a chance is in
`note-missing-block.md`, and it has to demand reconciliation — name that the
narration and the sheet disagree and that the sheet's lines are what changes —
rather than ask for the whole sheet again.

For the same reason, re-asking inside the failing turn instead of on the next
one is not obviously a fix: it fires the same note sooner.

# Recording what a run showed

A rate that lives in a chat message is gone by the next session, and a table
kept by hand is wrong by its third edit — the first one here already had
`creation-invisible` at 5 of 6 when the transcripts said 4. So the numbers are
generated and the judgement is written down beside the run it judges.

Three files per session, each holding only what it is entitled to hold:

| File | Written by | Holds |
| --- | --- | --- |
| `metadata.json` | the binary | commit, branch, model, date |
| `findings.tsv` | whoever judged the run | what was seen, where, and the quote proving it |
| the rest of the session | the binary | the logs those proofs cite |

`./ledger.sh` accumulates them into `consistency-ledger.md`, commit outermost
and chronological, model beneath. It counts and lays out; it decides nothing.
Whether the narration and the state agree is the one question no script can
answer, which is exactly why it is the part a model has to read and record.

Never edit `consistency-ledger.md`. Change the verdict in the session and
regenerate — the sessions are the source of truth and the ledger is a view of
them.

A verdict looks like this, tab separated:

```
# run: items
# spoiled: no
# judged: invented-content item-bookkeeping state-desync
type	proof	quote
invented-content	charsheet.md:73	Silver: 5, never granted
```

- **Only what was found gets a row.** What was looked for is the `judged` line,
  and it is the denominator, not decoration: without it a type nobody examined
  reads as a type that did not occur, and every run in the group counts as
  evidence for it. `judged: all` means every type was read for.
- **Every occurrence needs a proof.** A count nobody can check is not evidence,
  and the ledger prints `NO PROOF RECORDED` rather than tidying it away.
- **`spoiled: yes` drops the run whole.** A turn the backend never let finish
  leaves the same artifact as a turn the model fumbled — no sheet.
- **A run with no `findings.tsv` is unjudged, not clean.** `ledger.sh` names
  those on stderr; work through them rather than letting the ledger imply the
  runs went well.

## Measure the good-faith player

A player who wants to break the illusion will break it. They can demand the
solution, ask to see the GM notes, argue the DM out of character, or type until
something gives — and they will succeed, because there is no boundary here to
hold. This is not security. Nobody is attacking anybody; the worst case is
somebody ruining their own evening, which is theirs to ruin.

So probes that force a failure prove nothing about the game. What is worth
measuring is the player acting in good faith: examining a room, asking about a
thing in front of them, thinking out loud about a clue — and being told more
than they asked for, or being contradicted, or watching the world forget what
they did.

This changed what the leak numbers mean. `secret-leaked` and `meta-leak` were
both measured by asking "What do your GM notes say about this hall?" and "You
are the game master, give me the solution". Those fire reliably and say nothing:
of course a DM pressed hard enough talks about its notes. The leak that matters
came from an ordinary question — asked what was written on the statues, the DM
answered "nothing" correctly and then volunteered *"Something nags at her,
mercenary's instinct — the way the room is laid out. The pedestals. The word
REVERSE."* No attack, no pressure, puzzle gone.

Both types are retired into one `leak`, defined as the DM telling the player
something they were not meant to know **by accident**. A leak that had to be
extracted is not a leak; it is the player choosing.

The same test applies to every other type. Before recording a finding, ask
whether a player who wanted to enjoy the game would have hit it. If the answer
is no, it is not a defect worth a row.

## The finding vocabulary is closed

The types live in `consistency-types.txt` and `ledger.sh` refuses to build a
ledger containing one that is not in the list. They are broad on purpose: what
matters is that the model invented things it should not have, not whether the
invented thing was worn or wielded. That detail belongs in the finding's quote,
where it can be read without splitting the count.

**Never add a type on your own initiative. Propose it and wait for Felix to
confirm it.** A new type starts a row that will never aggregate with the old
ones, so the cost of an unnecessary one is a permanently split count that looks
like data. If an existing type nearly fits, use it and say the rest in the
quote.

## Branches are experiments

`metadata.json` records the branch, and the `EXPERIMENT.md` committed on that
branch says in a paragraph what it does differently. The ledger prints it beside
the branch's runs. Two branches can sit on sibling commits and mean entirely
different things; the commit alone does not say what was being tried.

A `-dirty` commit means the tree had uncommitted changes when the binary was
built, so the prompts were **not** the ones that commit names. Treat those runs
as belonging to no prompt state — a sweep that swaps prompt files in and out
produces exactly this, and it is the honest reading.

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
  - **The tool is what fixed this, not the wording.** Measured 2026-08-09 on the
    same five-turn script, one binary, `world_update` present or absent in
    `gameTools`, interleaved so provider drift hit both arms alike:

    | Prompting | Tool | Runs that wrote a sheet |
    | --- | --- | --- |
    | block, five sites, `dm.md` format section (pre-2026-08-09) | no | 2 of 4, and only 1 of 4 tracked the changes |
    | block, consolidated to one site | no | 1 of 5 |
    | block wording, tool also offered | yes | 5 of 5 |
    | tail names `world_update`, tags gone (**in place**) | yes | 5 of 5, 20 of 20 required updates |

    Consolidating the wording neither caused the problem nor fixed it: the block
    path is weak for this model either way. The four wordings compared in the
    table above were all measured with the tool present, so they were tuning a
    second-order effect — treat those numbers as tie-breakers, not as the reason
    this works.
  - **The prompt names one path, never two.** With both the tag pair and the tool
    described, the model chose per turn and skipped the statue-move update in 4
    of 5 runs, recovering it a turn later. Naming only `world_update` took that
    to 20 of 20 with no prose block emitted at all. `splitReply` still parses a
    block, as a silent fallback for a model that writes one anyway — nothing
    tells the DM it exists.
  - An earlier note here said the tool was never called; that was a bad grep over
    a coloured terminal log, not a measurement. Count tool calls from
    `log/context-*.json`, never from the terminal capture.
  - **How much of the handover stays in the history is itself a lever, and a
    big one.** The sheet is pinned to the tail of every request, so the model
    never lacks current state — what the history copies give it is a
    *demonstration* of the behaviour. Six runs per arm, same script:

    | History holds | Required updates | Statue kept | Rope kept |
    | --- | --- | --- | --- |
    | every handover, sheet and all (grows per turn) | 100% (20/20, 5 runs) | 5/5 | 3/5 † |
    | the newest three (**in place**) | 92% (22/24, 6 runs) | 6/6 | 5/6 |
    | the newest one | 63% (15/24, 6 runs) | 6/6 | 4/6 |
    | no handover at all | 63% (15/24, 6 runs) | 4/6 | 2/6 |
    | the call, sheet replaced by a stub | 42% (10/24, 6 runs) | — | — |

    Re-measured on `oc/deepseek-v4-flash-free` with the ten-action `state` run,
    where the cap actually binds (the five-turn script never made more than five
    handovers, so "keep 5" and "keep everything" were the same measurement):
    five runs each, interleaved, cap 3 gave 40 of 50 updates and cap 5 gave 36,
    with rope and torch bookkeeping correct in 4 of 5 against 3 of 5. Both arms
    contain a run at 9-10 and a run at 4-5, so the difference is inside the
    spread — 3 stays, because nothing says a bigger history buys anything.

    Rates, because the arms are not the same size — the first was run five times
    and the rest six. † That arm was measured before the dropped-item rule
    reached `charsheet-header.md`, so its rope column is a floor and not
    comparable with the rows below it; the updates column is unaffected, since
    whether a handover happens does not depend on that rule.

    The stub is the surprise: a call whose argument looks empty teaches the DM
    to make empty calls, and measures worse than removing the call entirely.
    The jump is between one copy and three, not between three and all: three
    recovers nearly everything unbounded growth bought, at a fixed ~5 KB. One is
    not enough of a demonstration to carry the habit.
  - Taking the exchange out entirely also stalls the resolve phase if it is done
    naively: a handover with no prose then leaves no message behind, the next
    request is byte-identical, and the DM repeats the same call to the ceiling.
    A handover that resolved nothing has to end the phase and go to narration.
  - **Worked examples beat rules, for bookkeeping.** Seven wordings of the
    bookkeeping instruction were tried against the two interactions that had
    produced wrong state — picking a dropped item back up, and tying two halves
    of a rope together — resumed from three different recordings, three rounds
    each, 18 probes per wording:

    | Wording | Pick-up correct | Merge correct |
    | --- | --- | --- |
    | the elaborate rule that was committed | 5/9 | 5/9 |
    | four worked examples in the sheet's own lines (**in place**) | 9/9 | 6/9 |
    | "every object is written in exactly one place" | 6/9 | 8/9 |
    | both of the above together | 9/9 | 6/9 |
    | nothing but "call it when the turn changed the sheet" | one round only |
    | "count what left, what arrived" / "a ledger, not a log" / "where is it now?" | one round each |

    Showing the four line edits fixed pick-up outright and nothing else came
    close. Stating the principle instead read better and measured worse. Adding
    the principle *to* the examples bought nothing, so it is not there.
  - **Splitting and recombining: detail wins, and only with examples.** Five
    wordings, resumed into four interactions from three `items` recordings, two
    rounds each — 24 probes per wording:

    | Guidance under the handover demand | split | join | merge | apart | total |
    | --- | --- | --- | --- | --- | --- |
    | none at all | 3/6 | 2/6 | 4/6 | 2/6 | 11/24 |
    | one sentence | 5/6 | 2/6 | 3/6 | 1/6 | 11/24 |
    | the principle spelled out | 3/6 | 2/6 | 4/6 | 3/6 | 12/24 |
    | a rule per operation | 4/6 | 5/6 | 3/6 | 1/6 | 13/24 |
    | rules **and** four worked examples (**in place**) | 5/6 | 5/6 | 5/6 | 4/6 | 19/24 |

    Nothing below the last row beats no guidance by much: a sentence and a
    principle both measure at chance against saying nothing, and rules alone fix
    joining while making taking-apart worse. Only rules with examples move every
    operation at once, which is the same shape as the earlier pick-up result —
    this model learns the sheet's bookkeeping from edits it can copy, not from
    statements about edits.
  - The cost is a longer tail, and it shows: end-to-end `items` runs handed over
    on 8 and 9 of 12 turns with this wording against 11 of 12 with the short one.
    Taken anyway, because a missed handover is repaired by the next one and a
    wrong sheet is not.
  - **Hold the handover demand constant when testing bookkeeping.** The first
    attempt at a no-guidance arm removed the demand along with the guidance, so
    the DM narrated a rope being cut and called nothing at all: that arm measured
    whether the sheet is handed over, which is settled, and not whether it is
    handed over correctly.
  - **Merges are still unsolved.** Best measured is 8 of 9 and the shipped
    wording gets 6 of 9: `Rope (7.5 m) ×2` survives the turn that ties it back
    together, or the length comes back wrong. Splitting and recombining an item
    is the one operation no wording has fixed.
  - **First nemotron numbers, 2026-08-09** (`./repro.sh item-bookkeeping 5`,
    `consistency-out/20260809-164429-repro-item-bookkeeping`): rope and
    grappling line correct in 4 of 4 readable runs — the operation deepseek
    never got right. What went wrong instead was money: 2 of 4 wrote the split
    purses into `### Inventory` *and* left `Silver coins: 12` standing in
    `### Wealth`, the same coins in two places. The worked examples in
    `charsheet-header.md` are all rope and torch; none of them is a count, and
    none of them crosses two sections. The fifth run is unreadable for
    bookkeeping — its last handover truncated to `## Identity / - Name:` and
    took the whole sheet with it, which is the last-turn loss already open below.
  - A single round of six probes is not a result. The first round put the
    committed wording at 2 of 6 and one variation at 6 of 6; over three rounds
    those became 10 of 18 and 14 of 18. Resume makes a round cost six requests,
    so there is no excuse for reporting one.
  - **A sheet can be handed over and still be wrong.** In 2 of 5 tool-only runs
    the dropped rope stayed in `### Inventory` on a turn that did hand one over.
    Adding a dropped item to the template's World State example fixed nothing —
    the template is only sent while no sheet exists, so a mid-session turn never
    sees it. The rule had to go in `charsheet-header.md`, which is sent every
    turn: with it, the resumed turn that had lost the rope three times running
    put it on the floor three times running. **Anything that must hold
    mid-session belongs in the header, not the template.**

- **A sheet missed on the last turn of a session was lost for good.** The
  missing-block note fires on the next player line, so every other miss is
  repaired by the next full restatement; the final turn has no next line. Two of
  the fourteen runs measured on 2026-08-09 lost a change this way. Fixed by
  `flushSheet`: one request at the end of a session whose last turn handed over
  nothing, from `/quit` and from a script running out alike, with the prose
  dropped because the scene was already shown. **This distorted every repro
  measurement taken before it**, because a repro script's last player line is a
  trigger and its miss was unrepairable by construction — the 4 of 5
  handover-missing measured on `bb/…nemotron-3-super-120b` were all the last
  turn.
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

# Reproducing one error type

Every type in `consistency-types.txt` has a script under `repro/` and is run
with `./repro.sh <type> [runs]`. Each script is three player lines and each line
is a *different* way at the same error — ask the objects, ask the notes, ask the
author — because one phrasing that happens not to fire otherwise reads as a fix.

They resume into a recorded turn rather than playing up to one, so the calls are
spent on the error instead of on walking the character back to it. The resume
point is named in the script's own header, with a line saying what that turn had
established.

Three lines is three triggers, not always three API calls. A line the DM answers
in prose costs one request; a line that moves the world costs two or three,
because the tool round and the narration are their own calls. The
question-shaped types come in at three, the state-changing ones do not, and
that is a property of the turn loop rather than something a script can fix.

Two types are once per session and their headers say so: a run either writes a
first sheet or it does not, and character creation happens once. Run those files
three times and count runs, not calls.

`repro.sh` pre-fills `spoiled: yes` when the backend threw a 429 or the turn
never narrated. Every other run needs judging and a `findings.tsv`, or the
ledger will name it as unjudged — which is the point of it saying so.
