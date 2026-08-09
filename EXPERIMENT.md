Branch `world-update-block`.

Replaces the `<world-update>` prose block with a `world_update` tool call, and
chases what happens after the handover rather than the handover itself. The
sheet is pinned to the tail of every request, only the newest few calls stay in
the history, and the missing-sheet note is the repair path when a turn changes
the world and hands over nothing.

What it is trying to settle: a turn that narrates a change and records nothing
is common on every model tried and is usually repaired a turn later. The failure
that ends adventures is the repair — a model that restates the pinned sheet
instead of reconciling it against what it just narrated will keep a door locked
that the player already opened.
