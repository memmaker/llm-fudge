# 02 — Stop the engine asserting Character position

**What to build:** Character state, as rendered for the DM, no longer claims a location.

A Character's location is set once when they are created and never updated, because no Action moves anyone. Every render since has asserted a position that goes stale the moment the fiction moves someone, and the DM reads that render as authoritative engine truth. The engine should stop stating something it does not track; position becomes a fiction fact owned by the world ledger (ticket 03).

The location field itself stays on the Character. The player-facing character sheet still reads it, and that sheet's staleness — along with the "present characters" list derived from it — is explicitly out of scope.

Context: `docs/engine-held-context-spec.md`, Problem Statement and the ROSTER decision.

**Blocked by:** None — can start immediately.

**Status:** ready-for-agent

- [ ] Rendered Character state contains no location field
- [ ] Every other field renders unchanged, in the same order and format
- [ ] Action results that carry Character state no longer mention position
- [ ] Existing Character-rendering tests updated to match
- [ ] The player-facing character sheet is untouched
