# 09 — `recall` Action

**What to build:** The DM can look up an older Turn on demand — by keyword, or by Turn number or range — with the original prose returned when it asks for it. This is how a session keeps decades of memory without carrying it: the archive holds everything, and context holds only what was asked for.

The natural use is tracing a fact to its source. WORLD entries carry the Turn they were last set, so the DM reading a fact it wants the history of can go straight to that Turn rather than guessing search terms. Keyword search covers the case where it only half remembers something.

`recall` counts against the Turn's request budget like every other round, consistent with the existing rule that every round costs — the budget exists to bound what a single line of player input can cost. It is absent from the narration-only tool set offered on a Turn's final round, so a lookup can never prevent the Turn from ending.

Its answer needs no special handling to stay out of long-term context: once ticket 08 lands, the Turn collapse discards everything, and before that it collapses with the rest of the Turn.

Can be built and tested against a seeded archive without waiting for the turn loop work.

Context: `docs/engine-held-context-spec.md`, Archive and the `recall` Action.

**Blocked by:** 04 (the archive it queries).

**Status:** ready-for-agent

- [ ] The Action is available during gameplay and absent from the narration-only tool set
- [ ] A keyword query returns matching Turns, most recent first
- [ ] A Turn number, and a range of Turns, return those Turns
- [ ] Prose is returned only when requested
- [ ] A query with no matches reads unmistakably as nothing found
- [ ] The call consumes one round of the Turn budget
- [ ] Results are bounded so a single call cannot dominate the DM's context
- [ ] Tested at the existing Action-dispatch seam against a seeded archive
