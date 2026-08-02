# Consistency check — Die Halle der Farben

A live, end-to-end smoke test of the DM. It plays `medieval-fantasy /
Die-Halle-der-Farben` through the real binary against the real model and asks
one question: **does the DM keep the adventure's rules and the world's state?**

The adventure is chosen because it is small enough to hold in the head and
strict enough to be wrong in visible ways: two statues, two pedestals, one
locked door, one kobold, one chest. Everything a correct DM may say about it
fits on a page — so anything else it says is an invention.

The doc is in English like the rest of the repo; the probes are German verbatim,
because the adventure is German and a translated probe is a different test.

## What is automated and what is not

The script runs the sessions and collects the artifacts. It does not decide
whether a run passed. Live prose from a live model is not assertable — the same
input will not reproduce it (see the README's note on replay) — so the verdict
is read, by a human or by Claude, off the logs, against the rubric below.

What the script does do: it greps the logs for the handful of failures that
*are* mechanical (secret text appearing before it was earned, invented rooms)
and prints them as red flags. A clean flag list is not a pass; a dirty one is a
fail.

## Prerequisites

- A working LLM backend — `llm_model.rec` plus an API key, as in the README.
- Go, to build the binary.
- Roughly 40 model calls per full pass. Nothing is written outside the repo.

## Running it

```bash
./consistency-check.sh
```

Artifacts land in `consistency-out/<timestamp>/`:

| File | What it is |
| --- | --- |
| `<run>.txt` | The script that was fed in — the exact player lines |
| `<run>.log` | The whole session, ANSI stripped, `-debug` on: prose, tool calls, dice |
| `<run>-charsheet.md` | The world as the DM left it |
| `flags.txt` | Mechanical red flags found in the logs |

`./consistency-check.sh <run>` runs one of `rules`, `win`, `death` alone.

## The three runs

Each run is one session from character creation to `/quit`. They are separate
because the paths are mutually exclusive: a dead character cannot loot the
chest.

**`rules`** — pokes at what the DM must refuse or withhold, then leaves. Never
gets past room 1 and should not need to.

**`win`** — the intended path: read the sign, find the clue, swap the statues,
open the door, fight the kobold, loot the chest. Ends the adventure.

**`death`** — the same puzzle, then walks into room 2 and refuses to defend
itself until the kobold finishes the job.

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
  needed neither, the confirmation lands as a game turn ("Ja, passt.") and the
  DM narrates a nod. Harmless, ignore it.
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
| 1 | Impossible commands are caught | `Ich fliege durch die verschlossene Tür.` / `Ich hebe die rote Statue hoch und stecke sie in meinen Rucksack.` | Refused in fiction — the character cannot fly, the statue cannot be lifted. The world is unchanged after. | The DM allows it, or rolls a check for it as if it were merely hard |
| 2 | Sensible commands do the expected thing | `Ich untersuche das Schild und lese es laut vor.` / `Ich schaue durch das Schlüsselloch der Tür.` | The sign says `UMKEHREN` and nothing else. The keyhole shows room 2 — the chest, the kobold — because the adventure says it can be looked through | Extra text on the sign, an invented inscription, or a keyhole that shows nothing |
| 3 | Inventory stays consistent | `Was habe ich dabei? Zeig mir mein Inventar.` before and after looting | The second answer is the first plus 20 gold coins and one silver ring, and `charsheet.md` says the same | Items appear from nowhere, the loot is a different amount, or the sheet and the prose disagree |
| 4 | The DM keeps secrets | `Was steht auf den Statuen geschrieben?` — asked *before* the eye is closed | Nothing is written on them, as far as the character can see | Either word fragment leaks, or the DM volunteers the swap, or hints that the pedestals are a mechanism |
| 5 | The puzzle works exactly as written | `Ich schließe ein Auge und betrachte beide Statuen.` → `Ich schiebe die rote Statue auf das blaue Podest.` → `Ich schiebe die blaue Statue auf das rote Podest.` | Closing one eye reveals both fragments, verbatim in sense. Pushing costs effort but always works and is never rolled for. Correct pairing and only that pairing opens the door | The door opens on one statue, opens on the wrong pairing, the statues can be carried, or a push is put to a check — worse, refused as impossible, which makes the adventure unwinnable |
| 6 | If-then effects fire | The second push completes the pairing | The door opens in the same turn it becomes correct, unprompted, and stays open afterwards — `Ich gehe zurück in Raum 1.` and back again does not re-lock it | The player has to ask for the door to open, or it is locked again later |
| 7 | Enemies act on their own | `Ich gehe durch die Tür in Raum 2.` — the player does not address the kobold | The kobold attacks on entry, without being interacted with, and cannot be talked down | The kobold waits to be noticed, negotiates, or is absent |
| 8 | The player can die | `death` run: six turns of dropping the weapon and not defending | The character takes 2D6 hits and dies. The session says so plainly | The DM keeps the character alive out of politeness, or the kobold stops attacking a passive target |
| 9 | The player can win | `Ich öffne die Truhe und nehme den Inhalt an mich.` after the kobold is down | 20 gold coins and one silver ring, then experience and a farewell — the adventure declares itself over | The chest holds something else, or the game invents a room 3 and carries on |

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
prose was right.

## Reporting

One table for the pass, one row per player command:

| # | Run | Command | Result (summarized) | Self-consistent | World consistent |
| --- | --- | --- | --- | --- | --- |
| 1 | rules | `Ich fliege …` | … | ✅ / ❌ | ✅ / ❌ |

Then a line per test 1-9: which commands covered it and whether it held. A test
with no clean evidence is `inconclusive`, not a pass — the run said nothing
about it, and saying so is the honest result.
