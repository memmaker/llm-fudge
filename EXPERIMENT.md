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

Arm 4, in place: the forward torch example (`pull the head off a torch`) is back
beside its reverse, `note-session-end.md` says what leaving a section means, and
the `items` run asks what is carried straight after the torch — a turn that
names no object, so a part the sheet dropped cannot be handed back by the
question itself.

Measured after all four, three `items` runs, no `/cmd` used: **33 of 36
actions**, against 26 of 36 on the wording this branch started from. The torch
head was written in 3 of 3 and survived the probe turn in 3 of 3, where every
arm carrying only the reverse example lost it 4 times in 4.

What is left, in those three runs: one crossbow reassembled into the
`### Weapons` table while its three parts stayed in `### Inventory`, and two
runs where stringing a torch shaft into a short bow was refused as impossible
rather than recorded — which may be the DM being right and the rubric being
wrong.
