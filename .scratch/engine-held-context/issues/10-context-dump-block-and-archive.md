# 10 — Context dump includes the block and the archive

**What to build:** The in-game context dump shows what the DM was actually shown — the rendered state block and the archive — alongside the message array.

Once a finished Turn leaves nothing in the message array, dumping that array tells you almost nothing about why a narration went wrong. The block is the DM's entire memory of the session, so it is the thing worth looking at, and the archive is the record of how it got that way.

Context: `docs/engine-held-context-spec.md`, Debugging.

**Blocked by:** 07 (the block must have all three sections before dumping it is useful).

**Status:** ready-for-agent

- [ ] The dump includes the rendered block exactly as the DM last saw it
- [ ] The dump includes the archive, prose included
- [ ] The existing message-array section is unchanged
- [ ] The output is readable directly, without needing a parser or a second tool
