Branch `claude/caveman-ultra-e87a7e`, off `main`. Two rounds of work: the two
failures the luna survey left standing, and then the bookkeeping block itself,
rebuilt from nothing.

Arm 1, in place: the put-down and pick-up worked examples are back in
`charsheet-header.md`. They were measured at 9 of 9 in `263641f` and dropped by
`8546623`, which replaced the whole worked list with the split/join/apart one.

Arm 2, in place: `charsheet-none.md` says the call is not the reply — the player
never sees the tool call, so the same reply names every attribute, skill, gift
and fault before the first scene.

Arm 3, in place: the bookkeeping block is now one sentence and seven worked
examples, and the five rules that stood above them are gone. Measured against
the twelve-action `items` run, which is the fixture that still fails — the
three-trigger `repro.sh item-bookkeeping` came back 15 of 15 clean on the
shipped wording and had nothing left to say.

| Guidance in the header | Runs | Actions correct |
| --- | --- | --- |
| nothing at all | 2 | 9/24 |
| one sentence | 2 | 14/24 |
| that sentence and three worked examples | 2 | 18/24 |
| and the crossbow and torch crossings | 2 | 20/24 |
| and put-down and pick-up (**in place**) | 2 | 21/24 |
| five rules and eight examples (the shipped wording) | 3 | 26/36 |

Two things the numbers say. Stripping the block costs about half the operations,
so the guidance was earning its place; and the rules above the examples were
not — the wording in place is shorter than the one it replaces and measures
better. The examples that moved it were the two that cross a section boundary:
the crossbow between the `### Weapons` table and `### Inventory`, and the torch
head folding back into a count. Both had failed in nearly every run of every
other arm, and both go clean once an example shows the edit.

What is left: the loose torch head still stops existing when the head is pulled
off, in every arm including this one, and one run in twelve dropped an item on
the last turn without it reaching `# World State`.
