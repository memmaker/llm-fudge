ADR 007: Two-phase turn, in-band charsheet, monolingual prompt set
Status: Accepted
Supersedes: parts of 006 — the Record phase, and the LLM's write access to the
charsheet

Context
- ADR 006 moved the world into `charsheet.md` and gave the LLM the file tools to
  keep it. The turn grew a third phase to make that work: resolve, narrate,
  record, the last one a separate request asking "what changed?" once the prose
  existed.
- A day of scripted consistency runs against `Die-Halle-der-Farben` measured
  what that costs. Four failure classes recurred, each answered with more prompt
  text, and `system-prompts/` reached 4,983 words plus 350 more in tool
  descriptions. Several rules held for two runs and fell in the third.
- The harness cannot tell a scene from a status report. Its only test is whether
  a response called a tool: `main.go:896` accepts any tool-free reply as the
  turn's narration. A DM that recorded inline and then wrote a summary of its own
  recording had that summary printed to the player as the scene, and the scene
  itself — a door opening, a kobold charging — was never written.
- The reference point for this ADR is a nine-sentence prompt the author has run
  successful sessions from, one sentence per concern, no rule set at all.

Decision
1. **The turn has two phases.** Resolve, where checks are called, and narrate.
   The record phase is deleted.
2. **The charsheet arrives in band.** The narration response carries the whole
   sheet inside a `<world-update>` tag. The harness cuts it out, writes it, and
   appends only the prose to the chat — so the one-copy invariant from ADR 006
   survives untouched, and the block also becomes the harness's first positive
   test for what the player may read: the scene is what lies outside the tag.
3. **Always present, always complete.** No per-turn judgement about whether
   anything changed — that judgement is what failed all day. Restating the whole
   sheet costs roughly 500 output tokens against a 110-token median response, and
   buys back a request whose input was the entire conversation.
4. **The LLM no longer writes the charsheet.** The block is its only writer.
   Every consumer of the file tools fell away in turn: the template is inlined,
   the session directory is derived from the first block's character name, and
   the milestone summary is gone. What remains in play is `check` and
   `user_choice`.
5. **`/milestone` stops calling the model.** It archives the raw chat and clears
   the conversation. State is already safe in the sheet; the raw archive keeps
   the texture.
6. **The prompt set is monolingual.** No language directive. Playing in another
   language means translating the set, because a model prompted in a language
   answers in it, and an instruction to switch can be disobeyed — every observed
   language slip was meta text escaping into the player's channel.
7. **The Fudge rules file starts nearly empty** — the ladder and the difficulty
   vocabulary, which are a contract with `check` and with the sheet — and the
   consistency suite fills it back. Nothing goes in without a run behind it.
   Mechanics rules go there; rules about one NPC or one puzzle go in the
   adventure, where they measurably held when global rules did not.

Consequences
- Roughly 5,300 words of prompt and 119 lines of file-confinement machinery lose
  their reason to exist, along with `sheet-nag.md`, `/inv`, `turn-bookkeeping.md`,
  `turn-narration-demand.md`, `milestone-demand.md`, `language.md` and the
  `<unchanged>` marker.
- The known risk is the inverse of the old one. `Edit` preserved by default and
  forgot only what it touched; full restatement forgets by default and preserves
  only what comes back. The mitigations are that the current sheet is pinned
  immediately above where the new one is written, so the model copies rather than
  recalls, and that every block is logged raw, so the suite can diff sheet
  against sheet and catch a line that quietly stopped appearing.
- A missing or malformed block is never punished with a request. The harness
  accepts what it can recognise and appends a carried note to the next player
  line. A missing block is self-repairing: the next one restates everything,
  including whatever happened while it was absent.
- `Die-Halle-der-Farben.md` and the consistency probes move to English, because
  under a monolingual set the measuring instrument must not be the one place
  where languages mix.
- Success criterion is the consistency suite, not taste. The suite runs against
  the small free model and a frontier model, and the difference between them is
  the answer to whether these 5,300 words were ever about Fudge or only ever
  about a 4B-class model.
