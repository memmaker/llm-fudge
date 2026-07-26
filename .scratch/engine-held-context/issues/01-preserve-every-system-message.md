# 01 — Preserve every system message when building provider requests

**What to build:** A request carrying more than one system message reaches the model with all of them intact, in order.

Today the Gemini request builder assigns the system instruction each time it encounters a system message, so the last one silently replaces every earlier one. The engine-held-context design sends the state block as a second system message; on Gemini that would discard the entire DM prompt and the setting reference, and the game would carry on looking healthy while the DM had no rules at all. Providers that pass messages straight through already behave correctly.

This lives in the gollum LLM client repository, which this project consumes through a local `replace` directive, so the fix is a change in a repo you own rather than a packaging exercise.

Context: `docs/adr/002-engine-held-context.md`, decision 9.

**Blocked by:** None — can start immediately.

**Status:** ready-for-agent

- [ ] Two system messages in one request both reach the Gemini model, in the order they were given
- [ ] A request with a single system message behaves exactly as it does today
- [ ] Providers that pass messages through unchanged are unaffected
- [ ] Covered by a test in the client repository
- [ ] This project builds and plays against the updated client
