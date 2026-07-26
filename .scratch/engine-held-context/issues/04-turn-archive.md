# 04 — Turn archive with keyword and Turn lookup

**What to build:** A store the engine writes every Turn into and the DM can query, holding the Turn number, the player's input verbatim, headlines of the Actions the engine resolved, the changes the DM reported, and the full narration prose.

The archive never enters context. It is read by the block's timeline (ticket 07) and by the `recall` Action (ticket 09), and it is the only place a session's prose survives at all — today prose is discarded the moment it is shown, so an NPC's actual words are unrecoverable.

Queries answer either a keyword, matched case-insensitively across player input, reported changes and prose, or a Turn number or range. Prose is returned only when asked for, since it dominates the size of any result. Results are bounded in count and size so one lookup cannot flood the DM's context.

Capturing Action headlines is the engine's job, not the DM's, so events survive whether or not the DM thought to write them down.

Context: `docs/engine-held-context-spec.md`, Archive and the `recall` Action.

**Blocked by:** None — can start immediately.

**Status:** ready-for-agent

- [ ] A Turn is recorded with number, player input, Action headlines, reported changes and prose
- [ ] Keyword query matches across player input, changes and prose, most recent first
- [ ] Lookup by Turn number, and by a range of Turns, returns those Turns
- [ ] Prose is omitted from results unless explicitly requested
- [ ] Results are bounded in count and total size
- [ ] A query with no matches renders as unmistakably empty, so the DM cannot read it as licence to invent
- [ ] Rendering is deterministic across runs
- [ ] Tests assert on rendered query results, never on internal representation
