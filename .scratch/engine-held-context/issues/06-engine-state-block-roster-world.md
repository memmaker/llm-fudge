# 06 — ENGINE STATE block with ROSTER and WORLD

**What to build:** The DM receives a state block, pinned immediately after the opening briefing as a second system message, built once per Turn and held constant across that Turn's resolution rounds.

Two sections for now:

- **ROSTER** — every Character rendered fresh from the World, full state, no filtering and no abbreviation. Dead and long-departed Characters included; pruning was considered and deliberately rejected for a first version. This is strictly cheaper than what history was doing, since it holds one copy per Character rather than one per Action that touched them.
- **WORLD** — the ledger from ticket 03, fed from each narration's `world_update`.

Building it once per Turn rather than once per request keeps the whole prefix cached across the rounds of a Turn, so a resolve-then-narrate Turn pays for the block once. Character changes made mid-Turn are not folded in; they reach the DM the way they already do, through the Action result that changed them, which is the existing "silence means as you were last told" convention.

**Hard constraint:** the block must be assembled in the turn loop, not in the chat wrapper. The scripted-DM test harness records the full message array shown on every round, so a block built in the loop is observable at the existing seam with no new plumbing — and one built lower down is not testable there at all.

Turn collapse is unchanged by this ticket: history still accumulates as it does today. The resulting duplication between block and history is deliberate and temporary, so that the riskier collapse (ticket 08) lands on its own with the block already proven in play.

Context: `docs/engine-held-context-spec.md`, The state block and ROSTER.

**Blocked by:** 01 (system messages must survive), 02 (roster must not assert position), 03 (the ledger), 05 (the argument it is fed from).

**Status:** ready-for-agent

- [ ] The block reaches the DM as a system message pinned immediately after the opening briefing
- [ ] It is built once per Turn and is byte-identical across every round of that Turn
- [ ] It is rebuilt between Turns and reflects everything that changed
- [ ] ROSTER holds every Character in the World, current, including the dead
- [ ] WORLD reflects `world_update` reports from earlier Turns, merged
- [ ] Facts the rules library owns appear in ROSTER and never in WORLD
- [ ] The block is observable through the existing scripted-DM harness
- [ ] A full session still plays end to end, opening scene through combat
