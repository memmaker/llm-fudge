# 11 — Prompt rewrites

**What to build:** The DM prompt describes the world that now exists rather than the one it replaced.

Removals, all of which are now false or counterproductive:

- The delta-discipline section in full. Under field merge, restating an unchanged fact is idempotent, so the DM should be actively encouraged to write anything it is unsure of. The current text tells it the opposite, at length, and warns of stale duplicates that the merge now makes impossible.
- Every claim that earlier Action results, Character ids or reported state remain available in its context. None do. The block supplies them, current, every Turn.
- The player-Character snapshot embedded in the opening briefing. ROSTER supplies it fresh each Turn, and the briefing's copy would be stale from the second Turn onward. Its test asserts the snapshot is present and needs updating.

Additions:

- The block's three sections and what each is authoritative for.
- The ledger grammar, including retraction and dropping an entry.
- The `recall` Action and when reaching for it is worth a round.

And the narration argument's description drops to roughly four sentences, keeping the first-Turn baseline requirement — the opening scene's location, features and NPC appearances exist nowhere else — but shedding every word about duplicates, restatement and delta discipline.

Last in the sequence deliberately, so the prompt describes something that exists and can be verified against it.

Context: `docs/engine-held-context-spec.md`, Narration contract.

**Blocked by:** 08 (the collapse must be real before the prompt describes it), 09 (`recall` must exist before the prompt teaches it).

**Status:** ready-for-agent

- [ ] No instruction anywhere claims earlier results or state stay in the DM's context
- [ ] The delta-discipline section is gone, not softened
- [ ] The block's three sections are described, with what each is authoritative for
- [ ] The ledger grammar is documented, including retraction and dropping an entry
- [ ] `recall` is documented, with guidance on when it is worth a round
- [ ] The narration argument's description is around four sentences and keeps the first-Turn baseline requirement
- [ ] The opening briefing no longer embeds player-Character state, and its test is updated
- [ ] The prompt-assembly test still passes
- [ ] A fresh session plays correctly from the opening scene through a fight and a purchase
