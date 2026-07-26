# 08 — Collapse a finished Turn to nothing

**What to build:** A finished Turn leaves nothing behind in the message array. Every request becomes the system prompt, the opening briefing, the state block, and the live messages of the Turn in progress. Per-Turn context growth goes to zero.

This is the payoff ticket. Everything a Turn used to leave behind now has a home: Character state in ROSTER, fiction facts in WORLD, chronology in TIMELINE, and the full record in the archive. The Action exchanges and the narration exchange are duplicates of all three and stop surviving the Turn.

The existing turn-record tests assert that Action exchanges and the narration call survive a Turn — precisely the behaviour being removed. They need rewriting to assert the new contract rather than adapting.

**Known risk, worth carrying into the work.** With no assistant message surviving, the transcript no longer demonstrates that a Turn ends by calling the narration Action. The existing retry scaffolding and the final-round narration demand remain the safety net. If the DM starts replying in plain prose after this lands, this is the cause, and the cheap fix is to keep the narration call per Turn with its arguments emptied — roughly 85 characters — purely as a shape anchor.

Context: `docs/engine-held-context-spec.md`, Turn collapse and Further Notes.

**Blocked by:** 07 (nothing may be dropped until the archive and timeline preserve it).

**Status:** ready-for-agent

- [ ] After a Turn completes, the message array holds none of its Action calls, Action results, or narration exchange
- [ ] No assistant message survives a completed Turn
- [ ] A request mid-Turn is the system prompt, the briefing, the block, and that Turn's live messages only
- [ ] Context size is flat across a ten-Turn session rather than growing per Turn
- [ ] A Turn that fails to narrate still leaves nothing behind, as it does today
- [ ] Multi-round Turns still resolve and then narrate correctly
- [ ] Turn-record tests rewritten against the new contract
- [ ] A twenty-Turn session plays through without the DM losing track of established facts
