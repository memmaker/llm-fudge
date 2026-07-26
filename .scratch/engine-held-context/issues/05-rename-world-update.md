# 05 — Rename the narration state argument to `world_update`

**What to build:** The DM reports fiction changes through an argument named `world_update` instead of `condensed_state`. A pure rename, no semantic change and no behaviour change.

The old name asks the DM to condense everything it knows, and measured sessions show it doing exactly that — reporting wounds, coins and inventory that the rules library owns and renders itself. The new name describes the actual job, which is updating the world ledger with what only the prose knows.

This is a prefactor: it makes ticket 06 (which feeds the ledger from this argument) and ticket 11 (which rewrites the argument's description) straightforward changes rather than tangled ones. Make the change easy, then make the easy change.

Context: `docs/engine-held-context-spec.md`, Narration contract.

**Blocked by:** None — can start immediately.

**Status:** ready-for-agent

- [ ] The narration Action's schema exposes `world_update`
- [ ] Payload parsing and validation use the new name, still required and still rejected when empty
- [ ] No reference to the old name remains anywhere in code or prompts
- [ ] Existing tests updated
- [ ] Gameplay behaviour is otherwise identical — this ticket changes a name and nothing else
