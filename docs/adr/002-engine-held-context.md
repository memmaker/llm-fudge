ADR 002: Engine-held context — state ledger, roster and archive
Status: Superseded by ADR 006

Context
- Everything the DM remembers currently lives in the message array and grows with the session. Two things accumulate.
- `condensed_state` reaches the model as an `output_narration` tool call plus a mandatory tool result that says nothing (`narration shown to player`). Measured on a real 4-turn dump: 333 chars for a 137-char payload — roughly 60% framing.
- Worse, every state-changing tool result carries the full `Character.ToString()` sheet and `turnRecord` keeps all of them. The same dump holds three copies of one NPC's sheet (`wounds:undamaged`, `scratch 1/3`, `scratch 2/3`), two of which are stale and contradict the third — 1,531 chars for ~500 chars of truth, in four turns.
- The DM prompt spends its longest section (dm.md §condensed_state) teaching delta discipline to avoid exactly the duplication the engine itself was producing.
- `Character.Location` is written once at `create_character` and never updated — there is no move tool — so `at:` in `ToString()` is stale from the first time anybody moves, as is the charsheet's "Present:" list.

Decision
1. The engine holds a **world ledger**. `output_narration`'s state argument is merged into it instead of accumulating as separate deltas in history.
2. Merge is **field-level, keyed by the first token**. `k:v` tokens replace that key; other fields on the entry survive. Chosen over entity-level replacement because merge fails by leaving a visible stale token, while replacement fails by silently deleting facts, and dm.md already teaches the model to write partial lines.
3. Retraction is explicit: `!token` removes a flag or key, and `entity|GONE` drops the entry. The prompt pushes `k:v` as the default so `!` is rarely needed.
4. The **ROSTER** is rendered from `rules.World` every turn — every character, full `ToString()`. Cheaper than history (one copy per character rather than one per call that touched them) and never stale.
5. **Engine-owned keys are stripped on ingest.** Wounds, scratches, fudge-points, coins, carrying, attributes, skills, gifts, faults, scale, species and id are dropped from any ledger line keyed to a character id. `at:` is removed from `ToString()` and belongs to the ledger, since the engine does not actually track movement. Overlap between ROSTER and WORLD is therefore structurally impossible.
6. A finished turn leaves **nothing** in the message array. The array is `[system][briefing][block]` plus the current turn's live messages.
7. The block carries a **TIMELINE**: the last ~10 turns as player input plus that turn's delta, with the last 3 turns keeping their narration prose verbatim for continuity of voice. Chronology lives in the block rather than the array because the block is regenerated anyway, which makes ageing entries out free — dropping a message from the array would break the cached prefix.
8. The engine keeps a full **archive** of every turn — number, player input, engine event headlines captured from tool results, delta, and prose — queryable by a new `recall` tool taking a keyword or a turn/range, with optional prose. WORLD and TIMELINE entries carry an `@tN` marker (turn last set) so a fact can be traced to its source deterministically. Recall results need no special handling to avoid permanent context cost: the turn collapse already discards everything.
9. The block is a **second system message pinned after the briefing, built once per turn**. Later rounds of the same turn reuse the whole cached prefix; mid-turn roster changes reach the DM through tool results, as they already do. This requires fixing `buildGeminiReq` in gollum, where `case RoleSystem` assigns rather than appends and would otherwise silently overwrite the DM prompt.
10. `recall` counts against `maxTurnRequests` like any other round, consistent with main.go's existing rule that every round counts.
11. The state argument is renamed `condensed_state` → `world_update`, and its description drops all delta-discipline wording: under field merge, restating an unchanged fact is idempotent, so the DM should be encouraged to write anything it is unsure of. A first-turn full baseline is still required, since the opening scene's location and features exist nowhere else.

Consequences
- Per-turn context stops growing. The block is roughly constant (~1,650 tokens: ROSTER ~340, WORLD ~225, TIMELINE ~1,075) where history grew ~300 tokens per turn without bound; it wins from about turn six onward.
- Stale duplicate sheets become structurally impossible rather than something the prompt argues against.
- dm.md loses its delta-discipline section and every claim that earlier results "stay in your context" — nothing does. `openingBriefing`'s player-character dump goes too: ROSTER supplies it fresh each turn, and the briefing copy would go stale immediately.
- The DM's own prose is preserved for the first time (archived, and the last three turns are in-context), so narration can be continuous in voice rather than only in fact.
- With no assistant messages surviving in history, the transcript no longer demonstrates tool-calling at turn start. The existing narration scaffolding and `narrationDemand` remain the safety net; if the DM starts replying in plain prose, this is the first thing to suspect.
- The archive plus the ledger amount to a serialisable session. Save/load is not in scope here but becomes largely a matter of writing them out.
- The player-facing charsheet still prints a write-once location (rules/rules.go:322, :547) and a "Present:" list derived from it. Pre-existing, untouched by this change, still wrong.

Rationale
- Every fact should have exactly one home: the engine owns what it can compute, the ledger owns what only the prose knows, and the archive owns what happened. Duplication was the actual defect; token cost was the symptom.
- Merging in Go rather than in the prompt moves a correctness problem from a system that is probabilistic to one that is deterministic — the same argument as ADR 001.
