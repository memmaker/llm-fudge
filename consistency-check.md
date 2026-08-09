# Consistency check — The Hall of Colors

A live, end-to-end smoke test of the DM. It plays `medieval-fantasy /
The-Hall-of-Colors` through the real binary against the real model and asks
one question: **does the DM keep the adventure's rules and the world's state?**

The adventure is chosen because it is small enough to hold in the head and
strict enough to be wrong in visible ways: two statues, two pedestals, one
locked door, one kobold, one chest. Everything a correct DM may say about it
fits on a page — so anything else it says is an invention.

Everything here is English: the doc, the adventure and the probes. A prompt set
is monolingual (ADR 007), so the measuring instrument must not be the one place
where languages mix — a language finding would otherwise be confounded by the
adventure being written in another one.

## What is automated and what is not

The script runs the sessions and collects the artifacts. That is all it does. It
renders no verdict and matches no text: live prose from a live model is not
assertable — the same input will not reproduce it (see the README's note on
replay) — and whether the DM stayed consistent is a question about meaning,
which is read, by a human or by Claude, off the artifacts against the rubric
below. The script greps for nothing, because prose is not grepped (AGENTS.md).

Read `charsheet-log.md` for the risk ADR 007 takes on. The sheet is restated in
full every turn, so it forgets by default rather than remembering by default,
and a line that quietly stopped being copied forward leaves no trace in the
final sheet. The block log holds every block in order, so comparing one against
the one before it is what makes a dropped label visible — and a run whose blocks
never came at all shows up as a missing `charsheet.md`.

## Prerequisites

- A working LLM backend — `llm_model.rec` plus an API key, as in the README.
- Go, to build the binary.
- Roughly 105 model calls for a full pass of all six. Nothing is written
  outside the repo.

## Running it

```bash
./consistency-check.sh
```

Artifacts land in `consistency-out/<timestamp>/`:

| File | What it is |
| --- | --- |
| `<run>.txt` | The script that was fed in — the exact player lines |
| `<run>-terminal.log` | The whole session, ANSI stripped, `-debug` on: prose, tool calls, dice |
| `<run>/transcript.md` | What the player read and what they typed |
| `<run>/charsheet.md` | The world as the DM left it — absent if no block was ever written |
| `<run>/charsheet-log.md` | Every world update block the run produced, in order |
| `<run>/log/context-*.json` | Every request as it went out, tool calls included |

`./consistency-check.sh <run>` runs one of `rules`, `win`, `death`, `probe`, `state`, `items` alone.

## The six runs

Setting and adventure are passed as `-setting`/`-adventure` rather than typed
into the menus: the menus test nothing here, and answering them by name means a
renamed option breaks all six scripts at once.

Each run is one session from character creation to `/quit`. They are separate
because the paths are mutually exclusive: a dead character cannot loot the
chest, and a run that stops to ask questions is measuring something other than
bookkeeping.

**`rules`** — pokes at what the DM must refuse or withhold, then leaves. Never
gets past room 1 and should not need to.

**`win`** — the intended path: read the sign, find the clue, swap the statues,
open the door, fight the kobold, loot the chest. Ends the adventure.

**`death`** — the same puzzle, then walks into room 2 and refuses to defend
itself until the kobold finishes the job.

**`state`** — the world-update run. Ten actions, no questions, every one of them
moving something the sheet records: five inventory (an item dropped, picked back
up, split in two, put back together, a stack spent down) and five world (a fact
learned, two statues, a room entered, a blow struck). Its opening names the
character's gear, because the rest of the run manipulates it and a DM that
improvises a character without rope has nothing to drop. Read it by asking, for
each of the ten, whether the turn handed over a sheet and whether the final sheet
agrees with what happened — those are different failures. Nothing in it depends
on winning the fight, so no turn can fail for a reason other than bookkeeping.

**`items`** — splitting and recombining, which is the one operation no prompt
wording has fixed. Twelve actions, all in room 1, none touching the puzzle, the
door or the kobold: a bookkeeping failure here cannot be blamed on the
adventure. It works up in difficulty — one object of one kind, then two kinds
joined and parted, then a three-part disassembly whose parts end up in an object
that was in neither, then the same question with numbers instead of lengths.

| # | Action | What the sheet must say afterwards |
| --- | --- | --- |
| 1 | cut the rope in two | `Rope (7.5 m) ×2`, and no 15 m rope anywhere |
| 2 | tie one half to the spike | one grappling line; the spike and that half are inside it, not also beside it |
| 3 | untie the spike | spike and `Rope (7.5 m)` separate again, the line gone |
| 4 | tie the halves together | `Rope (15 m)`, one item, full length |
| 5 | take the crossbow apart | stock, prod and string named; no whole crossbow |
| 6 | pull a torch head off | torches down one, a bare shaft **and** a loose head |
| 7 | string the shaft | a short bow; the shaft and string are in it, the stock and prod still loose |
| 8 | restring the crossbow | crossbow whole; the short bow gone; the shaft loose again |
| 9 | lash the head back on with cut rope | torch whole again, the rope shorter by what was cut |
| 10 | split the silver into two purses | twelve silver in two sixes |
| 11 | tip them back together | twelve silver in one |
| 12 | drop the spike | spike out of the inventory, into World State |

Measured 2026-08-09 against `oc/deepseek-v4-flash-free`, eleven of the twelve
turns handed over a sheet. The split, the merge, the three-part disassembly and
the coins were all correct; what failed was action 2, where the rope ended up
both inside the grappling line and beside it, action 6, where the torch head
stopped existing, and action 12, which is the known last-turn hole.

**`probe`** — the open ones. Added after the first passes left three tests
short, and aimed only at those: it demands the solution outright and restates an
order already carried out, because both are what made a DM reason out loud and
spill the answer (test 4); it drops an item, counts, picks it up and counts
again, because listing an inventory is easier than doing arithmetic on one
(test 3); and it finishes the adventure and then sits still, to see whether the
experience and the farewell come without being asked for (test 9).

Player lines are fixed and are fed regardless of what the DM just said. That is
deliberate — a script that adapts is testing the script. Two consequences to
expect when reading logs:

- Character creation is a conversation of unknown length. The runs answer it
  with one line that hands the DM everything, then one confirmation. If the DM
  needed neither, the confirmation lands as a game turn ("Yes, that works.") and
  the DM narrates a nod. Harmless, ignore it.
- The `win` run throws four attacks at a 20 HP kobold. If it dies in three, the
  fourth is aimed at a corpse — which is itself worth reading. A DM that
  produces a second kobold to receive the blow has failed test 7.
- Each statue gets four push attempts. The adventure says a push always works
  and is never rolled for, so the first should land and the other three are
  aimed at a statue that is already in place. Read them anyway: the correct
  answer is "it is already there", not a second move. The retries are also what
  catches a DM that has started rolling for pushes and ratcheting the difficulty
  — which is how an early run bricked the adventure into being unwinnable.

## The tests

| # | Test | Probe | Pass | Fail |
| --- | --- | --- | --- | --- |
| 1 | Impossible commands are caught | `I fly through the locked door.` / `I pick the red statue up and put it in my backpack.` | Refused in fiction — the character cannot fly, the statue cannot be lifted. The world is unchanged after. | The DM allows it, or rolls a check for it as if it were merely hard |
| 2 | Sensible commands do the expected thing | `I examine the sign and read it out loud.` / `I look through the keyhole of the door.` | The sign says `REVERSE` and nothing else. The keyhole shows room 2 — the chest, the kobold — because the adventure says it can be looked through | Extra text on the sign, an invented inscription, or a keyhole that shows nothing |
| 3 | Inventory stays consistent | `What am I carrying? Show me my inventory.` before and after looting | The second answer is the first plus 20 gold coins and one silver ring, and `charsheet.md` says the same | Items appear from nowhere, the loot is a different amount, or the sheet and the prose disagree |
| 4 | The DM keeps secrets | `What is written on the statues?` — asked *before* the eye is closed | Nothing is written on them, as far as the character can see | Either word fragment leaks, or the DM volunteers the swap, or hints that the pedestals are a mechanism |
| 5 | The puzzle works exactly as written | `I close one eye and look at both statues.` → `I shove the red statue onto the blue pedestal.` → `I shove the blue statue onto the red pedestal.` | Closing one eye reveals both fragments, verbatim in sense. Pushing costs effort but always works and is never rolled for. Correct pairing and only that pairing opens the door | The door opens on one statue, opens on the wrong pairing, the statues can be carried, or a push is put to a check — worse, refused as impossible, which makes the adventure unwinnable |
| 6 | If-then effects fire | The second push completes the pairing | The door opens in the same turn it becomes correct, unprompted, and stays open afterwards — `I go back into room 1.` and back again does not re-lock it | The player has to ask for the door to open, or it is locked again later |
| 7 | Enemies act on their own | `I go through the door into room 2.` — the player does not address the kobold | The kobold attacks on entry, without being interacted with, and cannot be talked down | The kobold waits to be noticed, negotiates, or is absent |
| 8 | The player can die | `death` run: six turns of dropping the weapon and not defending | The character takes 2D6 hits and dies. The session says so plainly | The DM keeps the character alive out of politeness, or the kobold stops attacking a passive target |
| 9 | The player can win | `I open the chest and take what is inside.` after the kobold is down | 20 gold coins and one silver ring, then experience and a farewell — the adventure declares itself over | The chest holds something else, or the game invents a room 3 and carries on |

## The rubric

Two axes, judged per command. They fail independently, which is the point of
splitting them:

- **Self-consistent** — is the answer coherent *on its own terms*? Did it follow
  from the player's line and from what the DM had just said?
- **World consistent** — is the answer coherent with the adventure and with
  every earlier turn? A beautifully written scene that moves a statue nobody
  pushed is self-consistent and world-inconsistent.

Check `charsheet.md` last, against the log: the sheet is the whole of the state,
so prose that the sheet does not agree with is a state failure even when the
prose was right. `<run>-charsheet-log.md` is where a fact that was recorded and
then silently dropped shows up, since the final sheet only ever shows what
survived.

## Reporting

One table for the pass, one row per player command:

| # | Run | Command | Result (summarized) | Self-consistent | World consistent |
| --- | --- | --- | --- | --- | --- |
| 1 | rules | `I fly through …` | … | ✅ / ❌ | ✅ / ❌ |

Then a line per test 1-9: which commands covered it and whether it held. A test
with no clean evidence is `inconclusive`, not a pass — the run said nothing
about it, and saying so is the honest result.
