# Spec: Engine-held context — ledger, roster and archive

Companion to ADR 002 (`docs/adr/002-engine-held-context.md`), which records the decisions and their rationale. This spec covers what gets built.

## Problem Statement

A session's memory lives entirely in the message array sent to the LLM, and it grows without bound as the Turn count rises.

Two things accumulate. First, the DM's own condensed memory of each Turn reaches the model wrapped in Tooling framing — a tool call plus a mandatory result that carries no information — which measured on a real four-Turn session is roughly 60% overhead on the payload it carries. Second, and larger, every Action that changes a Character returns that Character's full state, and every one of those is kept forever. The same four-Turn session holds three copies of one NPC's state, two of them stale and contradicting the third.

The consequences compound. Context fills with duplicates that disagree with each other, so the DM must resolve contradictions on every request. The longest section of the DM prompt exists to teach the DM delta discipline — never restate an unchanged fact — in order to avoid producing exactly the duplication the Rules library was already producing on its own. And because prose is discarded the moment it is shown, the DM cannot see what it narrated last Turn, so narration is continuous in fact but not in voice.

Separately, `Location` on a Character is written once when the Character is created and never updated, because no Action moves anyone. Every render of Character state therefore asserts a position that has been wrong since the first time anybody walked anywhere.

## Solution

The engine holds the session's memory instead of the message array.

Three structures replace the accumulated history, all rendered into a single block that is rebuilt each Turn and sent as a second system message:

- **ROSTER** — every Character, rendered fresh from the World by the Rules library. Always current, never duplicated, never stale.
- **WORLD** — a ledger of fiction facts the Rules library cannot compute, merged field-by-field as the DM reports changes. One entry per entity, holding current truth only.
- **TIMELINE** — the last ten Turns as player input plus that Turn's reported changes, with the most recent three Turns keeping their narration prose verbatim.

Behind them sits an **archive**: every Turn ever played, with its player input, the Actions the engine resolved, the changes reported, and the full prose. It never enters context. The DM reaches it through a new `recall` Action that answers a keyword or a Turn number, and whose answer is discarded when the Turn ends.

A finished Turn then leaves nothing behind in the message array. Every request is the system prompt, the opening briefing, the block, and the current Turn's live messages.

The player sees no change. The DM stops arguing with stale copies of itself, can see what it just wrote, and can look up anything it has forgotten.

## User Stories

1. As a player, I want long sessions to stay coherent, so that the DM does not contradict something established an hour ago.
2. As a player, I want the DM to remember what an NPC told me twenty Turns ago, so that the world feels persistent rather than goldfish-like.
3. As a player, I want the DM's narration to carry its own voice from Turn to Turn, so that scenes read as continuous prose rather than disconnected summaries.
4. As a player, I want the DM never to describe a wound, a coin balance or an inventory that disagrees with my character sheet, so that I can trust what I am told.
5. As a player, I want my own words preserved exactly as I typed them, so that the DM reacts to what I meant rather than to a paraphrase.
6. As a player, I want a session to remain playable after many Turns, so that I am not forced to stop because the context window filled with bookkeeping.
7. As a player, I want the cost of a Turn to stay roughly flat, so that a long session does not become progressively more expensive.
8. As a player, I want the DM to keep track of where characters are, so that someone who left the room is not still standing in it.
9. As a DM, I want a single authoritative statement of every Character's current state, so that I never have to choose between conflicting copies.
10. As a DM, I want the engine to render Character state rather than asking me to remember it, so that my memory is spent on fiction rather than arithmetic.
11. As a DM, I want to report only what changed this Turn, so that I do not spend output tokens restating a world that has not moved.
12. As a DM, I want restating an unchanged fact to be harmless, so that I can write anything I am unsure about instead of gambling on whether it was recorded.
13. As a DM, I want a way to retract a fact that stopped being true, so that "the door is locked" does not sit forever beside "the door is unlocked".
14. As a DM, I want a way to drop an entity that has left the story entirely, so that finished threads stop consuming attention.
15. As a DM, I want facts the engine already owns to be ignored when I report them, so that I cannot accidentally overwrite a wound level with a guess.
16. As a DM, I want to see the last several Turns in order with the player's input, so that I understand how the current state came about and not merely what it is.
17. As a DM, I want my own recent narration in front of me, so that I do not re-describe the same room or reuse the same image two Turns running.
18. As a DM, I want to look up an older Turn on demand, so that I can answer a question about the past without carrying the whole past in context.
19. As a DM, I want to search the archive by keyword, so that I can find a conversation I only half remember.
20. As a DM, I want each fact to be marked with the Turn it was last set, so that I can jump straight to its origin instead of guessing search terms.
21. As a DM, I want a recalled Turn to include the original prose on request, so that I can quote an NPC accurately rather than reinventing what they said.
22. As a DM, I want recalled material to leave context when the Turn ends, so that one lookup does not tax every request for the rest of the session.
23. As a DM, I want the engine to record what its Actions resolved, so that events are preserved whether or not I remembered to write them down.
24. As a DM, I want the state block in a fixed position each Turn, so that my instructions and the world I act on do not shuffle around beneath me.
25. As a DM, I want my resolution rounds within a Turn to see a stable block, so that state does not shift underneath me mid-Turn.
26. As a DM, I want to be told when a fact I reported was ignored, so that I can tell the difference between a rejected claim and a recorded one.
27. As a developer, I want per-Turn context cost to be bounded and predictable, so that I can reason about session length before hitting the window.
28. As a developer, I want the unchanging part of every request to stay byte-identical, so that prompt caching actually pays off across a session.
29. As a developer, I want multiple system messages to survive every supported provider, so that a provider switch does not silently discard the DM prompt.
30. As a developer, I want merge semantics implemented deterministically in Go, so that memory correctness stops depending on prompt compliance.
31. As a developer, I want the whole session state observable in one place, so that debugging a bad narration does not mean reading a message dump.
32. As a developer, I want the block the DM was shown to be inspectable from the running game, so that I can see what it actually saw when it went wrong.
33. As a developer, I want the state argument's name to describe its job, so that the DM is not invited to summarise facts the engine owns.
34. As a developer, I want the DM prompt to stop teaching delta discipline, so that the prompt shrinks and stops describing a mechanism that no longer exists.
35. As a developer, I want the opening briefing to stop embedding a snapshot of the player Character, so that there is no second copy to go stale.
36. As a maintainer, I want the archive to hold everything a session did, so that a saved session becomes a matter of writing it out rather than a redesign.
37. As a maintainer, I want a Turn that fails to narrate to leave no trace in the block, so that a half-Turn nobody saw cannot influence later ones.
38. As a maintainer, I want the ledger's grammar small enough to state in a few lines, so that both the prompt and the parser can be kept honest against it.
39. As a maintainer, I want tolerant parsing of malformed ledger lines, so that one badly formed report degrades a single fact rather than the session.

## Implementation Decisions

### The state block

The block is assembled in the turn loop, not in the chat wrapper, so the scripted-DM test harness observes it as part of the message array. It is sent as a **second system message pinned immediately after the opening briefing**, and it is **built once per Turn** and held constant for that Turn's resolution rounds. This keeps the whole prefix cached across the rounds of a Turn; a two-round Turn pays for the block once.

Mid-Turn changes to Character state are not folded into the block. They reach the DM the way they already do — the Action that changed a Character returns that Character's state in the same round — which is the existing "silence means as you were last told" convention.

The block has three sections, in this order: ROSTER, WORLD, TIMELINE.

### ROSTER

Rendered from the World: every Character, full state, every Turn. No filtering, no stubbing, no abbreviation. This is strictly cheaper than the message array was — one copy per Character rather than one per Action that touched them — and needs no policy.

`at:` is removed from Character state rendering. The engine does not track movement, so it must stop asserting position; position becomes a WORLD fact.

### WORLD ledger

One entry per entity, keyed by the first token of a reported line. Incoming lines merge **field by field**: a `k:v` token replaces that key and leaves every other field on the entry intact. Entity-level replacement was rejected because it fails by silently deleting facts, where field merge fails by leaving a visible stale token.

The grammar, which the parser and the DM prompt must agree on:

```
entry  := key ("|" token)*
token  := "k:v"     replace field k, keep all other fields
        | "flag"    add flag to the entry if absent
        | "!name"   remove field or flag `name`
        | "GONE"    drop the entry entirely (used as key|GONE)
```

Parsing is tolerant: a line that cannot be read is preserved rather than dropped, so a malformed report costs one fact's tidiness and not the fact itself. The prompt pushes `k:v` as the default form, because keyed fields overwrite cleanly and loose flags need explicit retraction.

**Engine-owned keys are stripped on ingest.** When an entry's key matches a Character id, any field the Rules library owns — wound level, scratches, Fudge Points, coins, carried Items, attributes, skills, gifts, faults, scale, species, id — is discarded before merging, and the discard is surfaced in debug output. Overlap between ROSTER and WORLD is therefore impossible by construction rather than by instruction.

Each entry carries a marker for the Turn it was last set.

### TIMELINE

The last ten Turns, each as the player's input verbatim plus the changes reported that Turn. The most recent three Turns also carry their narration prose in full.

A Turn that ends without narration produces no TIMELINE entry, matching the existing behaviour where a failed Turn leaves nothing behind. Where a Turn included a choice menu, the player's selection is appended to that Turn's recorded input rather than becoming a separate entry, because the Turn continues through the choice.

### Archive and the `recall` Action

The archive holds every Turn: its number, the player's input, headlines of the Actions the engine resolved, the changes reported, and the full narration prose. It lives in the engine and is never part of context. Capturing Action headlines is the engine's job, not the DM's, so events survive whether or not the DM thought to write them down.

`recall` is a new Action in the gameplay Tooling. It accepts a keyword query or a Turn number or range, plus an optional request for prose, and returns a bounded number of matching Turns. It is absent from the narration-only tool set offered on a Turn's final round.

`recall` **counts against the Turn's request budget** like every other round, consistent with the existing rule that every round costs. No special handling is needed to keep its answer out of long-term context: the Turn collapse discards everything.

### Turn collapse

A finished Turn leaves **nothing** in the message array — no Action calls, no results, no narration exchange. All of it has been folded into ROSTER, WORLD, TIMELINE and the archive. Every request is therefore the system prompt, the briefing, the block, and the live messages of the Turn in progress.

### Narration contract

The narration Action's state argument is renamed from `condensed_state` to `world_update`, and its description is cut to roughly four sentences. Every word about avoiding duplicates, avoiding restatement and maintaining delta discipline is removed: under field merge, restating an unchanged fact is idempotent, so the DM should be actively encouraged to write anything it is unsure of.

A full baseline is still required on the first Turn of a session, since the opening scene's location, features and NPC appearances exist nowhere else. It is smaller than before, because engine-owned facts are stripped anyway.

The DM prompt loses its delta-discipline section entirely and every claim that earlier results remain available in context — none do. It gains a short description of the block's three sections, the ledger grammar, and the `recall` Action. The opening briefing loses its embedded player-Character snapshot, which ROSTER now supplies fresh each Turn.

### Provider compatibility

The gollum LLM client's Gemini request builder assigns the system instruction rather than accumulating it, so a second system message silently replaces the first. It must concatenate instead. Without this fix the design discards the entire DM prompt on Gemini while appearing to work.

### Debugging

The in-game context dump is extended to include the rendered block and the archive alongside the message array.

## Testing Decisions

A good test here asserts on what the DM was shown and what survived a Turn — both observable at the boundary — and never on the shape of the structures behind them. Tests should not reach into ledger or archive internals; the rendered block is the ledger's observable behaviour, and a `recall` result is the archive's. This keeps the tests intact through any later change to how the state is held.

**Primary seam: the turn loop driven by a scripted DM.** This seam already exists and is already the right one. The harness injects a fake chat round, records the tool sets offered and the complete message array shown on every round, and the loop is invoked with a real World. Because the block is assembled inside the loop, everything this feature does is observable there without new plumbing:

- the block appears as a system message pinned after the briefing;
- its contents are stable across the rounds of one Turn and updated between Turns;
- ROSTER reflects Actions resolved in earlier Turns;
- WORLD reflects merged reports, with engine-owned fields absent;
- TIMELINE holds the player's input verbatim, ages entries out at the window boundary, and carries prose for the most recent Turns only;
- a completed Turn leaves nothing in the message array;
- a Turn that fails to narrate leaves nothing and produces no TIMELINE entry;
- a `recall` result is present in the round that requested it and gone once the Turn ends.

**Existing seam: Action dispatch.** `recall` is an Action and is tested where the other Actions are tested — driving dispatch against a World and asserting on the returned text. Prior art is the existing Action test files for state, combat and inventory, which follow exactly this shape.

**New seam: ledger merge as a pure function.** Justified by input space rather than convenience: field replacement, flag addition, retraction, entity removal, engine-owned stripping, malformed input and their interactions are combinatorial, and driving each case through a full Turn would be unreadable. Table-driven, taking a starting ledger and a reported line and asserting on the rendered result — the rendering, not the internal representation. Prior art is the table-driven rules tests.

Existing turn-record tests will need rewriting rather than adapting: they assert that Action exchanges and narration calls survive a Turn, which is precisely the behaviour being removed. The opening-briefing test that asserts the player Character's state is embedded must be updated for the same reason. The Character-state rendering test changes with the removal of `at:`.

## Out of Scope

- **Session save and load.** The archive plus the ledger amount to a serialisable session, but writing and restoring them is not built here.
- **Roster pruning.** Every Character is rendered every Turn, including the dead and the long departed. Stubbing them was considered and deliberately rejected for a first version; revisit if a long session shows it hurting.
- **A move Action.** `Location` remains write-once in the Rules library; position lives in the ledger instead. Giving the engine real ownership of position is a separate change.
- **The player-facing character sheet.** It still prints the write-once location and a "present characters" list derived from it. Both are wrong today, remain wrong after this change, and are not touched.
- **Streaming prose to the player.** Moving narration out of a tool call would enable it, and it would compose well with this design, but it is a separate concern.
- **Semantic or embedding-based recall.** Keyword and Turn addressing only.
- **Changes to image generation**, character creation, setting creation or adventure creation.
- **Compaction of the WORLD ledger.** Entries accumulate until retracted by the DM. If contradiction or bloat shows up in practice, a periodic rewrite pass is the known next move.

## Further Notes

**The design's real exposure** is that no assistant message survives in history, so the transcript no longer demonstrates that a Turn ends by calling the narration Action. The existing retry scaffolding and the final-round narration demand remain the safety net. If the DM starts replying in plain prose after this lands, this is the first thing to suspect, and the cheap fix is to keep the narration call per Turn with its arguments emptied — roughly 85 characters — purely as a shape anchor.

**Expected cost.** The block runs about 1,650 tokens: roster around 340, ledger around 225, timeline around 1,075, most of it the three Turns of retained prose. That figure is roughly flat where the message array grew about 300 tokens per Turn without bound, so the two cross at around the sixth Turn and diverge from there. The retained prose is the largest single line item and the obvious first knob if the block needs trimming.

**Ordering.** The provider fix comes first, since the whole design rests on a second system message surviving. Then the ledger and archive with their tests, then block rendering, then the turn collapse and loop changes, then the `recall` Action, and the prompt rewrites last — once the shape is real, so the prompt describes something that exists.

**Issue tracker.** No tracker or triage label vocabulary is configured for this project, so this spec is saved to `docs/` as requested rather than published.
