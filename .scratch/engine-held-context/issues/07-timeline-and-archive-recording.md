# 07 — TIMELINE section and archive recording

**What to build:** The engine records every Turn to the archive as it completes, and the block gains a third section giving the DM chronology rather than only current state.

**TIMELINE** shows the last ten Turns, oldest first, each as the player's input verbatim plus the changes reported that Turn. The most recent three Turns also carry their narration prose in full, so the DM can see what it just wrote rather than inferring it from its own summary — continuity of voice, imagery and dangling threads, none of which survives a delta.

Chronology lives in the block rather than as messages in the array because the block is regenerated every Turn anyway, which makes ageing an entry out free. Dropping a message from the array would break the cached prefix instead.

Two edge cases, both settled during design:

- A Turn that ends without narration is recorded to the archive and marked, but produces no timeline entry — matching the existing behaviour where a failed Turn leaves nothing behind, so a half-Turn the player never saw cannot influence later ones.
- Where a Turn included a choice menu, the player's selection joins that Turn's recorded input rather than becoming a separate entry, because the Turn continues through the choice.

Context: `docs/engine-held-context-spec.md`, TIMELINE.

**Blocked by:** 04 (the archive it renders), 06 (the block it is a section of).

**Status:** ready-for-agent

- [ ] Every completed Turn is recorded to the archive with all of its parts
- [ ] TIMELINE renders the last ten Turns in order, oldest first
- [ ] The three most recent Turns carry their prose; older ones carry only input and reported changes
- [ ] Entries age out of the section once past the window, and the archive still holds them
- [ ] A Turn that ends without narration produces no timeline entry
- [ ] A choice selection is appended to that Turn's recorded input
- [ ] Player input is preserved exactly as typed, never paraphrased
- [ ] Observable through the existing scripted-DM harness
