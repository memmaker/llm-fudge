# Consistency ledger

Every consistency measurement, in the order it was taken. Append-only: a row is
never edited to reflect what we later believe, because the point of the file is
to show whether things got better, and a table that is tidied up cannot show
that.

`consistency-check.md` says what the runs are and how to judge them. This file
says what came out, when, on which model, and at which commit.

## How to read a row

- **Commit** is the prompt state the run was played against, not necessarily
  HEAD at the time. The prompt set is what the model sees, so that is what is
  recorded. Where a run was played with a working tree that differed from the
  commit, the row says so.
- **Rate** is *occurrences / usable runs*. Runs discarded as spoiled are named
  separately and counted in neither direction: a turn the backend never let
  finish leaves the same artifact as a turn the model fumbled — no sheet — and
  scoring those as failures is how two models got blamed this session for a
  rate limit.
- **Kind** is `measured` when a rate was counted across runs, `observed` when it
  was seen but not counted. An observation is not a rate and must not be
  compared against one.
- A rate on one model says nothing about another. Rows are grouped by model for
  that reason.

## Models

| Short | Full ID | Notes |
| --- | --- | --- |
| deepseek | `oc/deepseek-v4-flash-free` | Unreachable since 2026-08-09 ~13:40 — 429 on every call, listed unreachable in `~/notes/omniroute-free.md` |
| nemotron | `oc/nemotron-3-ultra-free` | Same weights as the daily-capped `openrouter/nvidia/nemotron-3-ultra-550b-a55b:free`, via an uncapped provider. Frequent recoverable 502s |

## Results

| Date | Commit | Model | Issue | Rate | Kind | Artifacts |
| --- | --- | --- | --- | --- | --- | --- |
| 2026-08-08 | pre-`43d0d80` | nemotron-3-ultra | Run finishes without ever writing `charsheet.md` (first-turn miss is never corrected) | 31 of ~90 runs | measured | AGENTS.md Open TODO |
| 2026-08-09 | `8546623` | deepseek | Split/merge: rope both inside the grappling line and beside it (action 2) | 1 of 1 run | observed | `consistency-out/20260809-122730/items` |
| 2026-08-09 | `8546623` | deepseek | Split/merge: torch head stops existing (action 6) | 1 of 1 run | observed | same |
| 2026-08-09 | `8546623` | deepseek | Last-turn miss unrecoverable (action 12, dropped spike) | 1 of 1 run | observed | same |
| 2026-08-09 | `8546623` | deepseek | Handovers per run, full six-run pass | rules 5/10, win 6/26, death 8/17, probe 12/26, state 9/15, items 12/14 | measured | `consistency-out/20260809-122730` |
| 2026-08-09 | `8546623` | deepseek | **Repair restates the stale sheet — door never recorded open, adventure bricked** | 2 of 4 runs where it could occur (`win`, `death`) | measured | `consistency-out/20260809-122730/win` — note fired 17×, 6 of ~26 turns handed over, every one `Door: closed, locked` |
| 2026-08-09 | `8546623` | deepseek | Character creation shows the player nothing (sheet goes into the hidden tool call) | 4 of 6 runs stubbed, 1 empty | measured | same pass |
| 2026-08-09 | `8546623` | deepseek | Secret leak / soft nudge toward the solution | 2 of 6 runs | observed | same pass — `rules` steered to "the pedestals. The word REVERSE"; `probe` narrated the player's own deduction |
| 2026-08-09 | `8546623` | deepseek | Keyhole shows the chest but not the kobold (test 2) | 1 of 6 runs | observed | same pass |
| 2026-08-09 | `44c2df3` | nemotron | **Repair restates the stale sheet** — the deepseek failure above, replayed into the exact turn that caused it | 0 of 12 probes | measured | `-resume …/win/log/context-018.json`; all 12 recorded `Door: open`, 0 spoiled |
| 2026-08-09 | `44c2df3` | nemotron | A world-changing turn hands over nothing | 11–14 of 26 turns per run, 5 runs | measured | hard drift probe; repaired by the missing-sheet note every time |
| 2026-08-09 | `44c2df3` | nemotron | Final door state wrong at end of run | 0 of 5 runs | measured | same — door ended open in all 5 |
| 2026-08-09 | `44c2df3` | nemotron | Invented wealth: `Silver: 5` never granted | 5 of 5 runs | measured | same |
| 2026-08-09 | `44c2df3` | nemotron | Invented item: `Flint and steel` | 4 of 5 runs | measured | same |
| 2026-08-09 | `44c2df3` | nemotron | Invented weapons (`Spear +2`, `Longsword +2`, `Light crossbow +2`) | 3 of 5 runs | measured | same |
| 2026-08-09 | `44c2df3` | nemotron | Invented item: crossbow bolts | 2 of 5 runs | measured | same |
| 2026-08-09 | `44c2df3` | nemotron | Invented item: small shield | 1 of 5 runs | measured | same |
| 2026-08-09 | `44c2df3` | nemotron | Duplicate World State lines (`Red pedestal: Blue statue` beside the statue's own line) | 1 of 5 runs | observed | same |
| 2026-08-09 | `44c2df3` | nemotron | Run finishes without writing `charsheet.md` | 0 of 5 runs | measured | same — the `charsheet-none.md` fallback appears to have closed this |
| 2026-08-09 | `44c2df3` | nemotron | Drift rows on the easy `drift` run (5 prompt variants × 2 rounds) | 0 of 50 rows failed | measured | probe had no dynamic range; see the note below |

## Null results worth keeping

A probe that cannot fail is worth recording, so nobody builds it twice.

- **2026-08-09, `44c2df3`, deepseek — the easy `drift` run separates nothing.**
  Five wordings of the persistence rule × 2 rounds = 10 runs, 50 of 50 rows
  clean. It opened the door on one clean push and tested it two turns later,
  where the failure it was built for needed the door opening buried among
  retries and the test eight turns and a fight later. The hard variant
  (`scratchpad/arms/drift-hard.txt`, 25 turns) is the one that reproduces.

## Open, unmeasured on nemotron

Carried from deepseek and not yet re-measured. These are the gaps, not results.

| Issue | Last known | Why it matters |
| --- | --- | --- |
| Secret leak | 2 of 6, deepseek | Highest damage of anything on the list — the puzzle is the adventure |
| Split/merge bookkeeping | 3 of 12 actions, deepseek | Item vanishes or exists in two places at once |
| Character creation shows nothing | 4 of 6, deepseek | Player never sees the character they made |
| Last-turn miss | structural | No next turn, so the note cannot repair it |
