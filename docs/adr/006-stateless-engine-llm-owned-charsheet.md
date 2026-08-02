ADR 006: Stateless engine, LLM-owned charsheet
Status: Accepted
Supersedes: parts of 001, and 002, 003, 004, 005 in full

Context
- ADRs 001-005 progressively moved authority into Go: a world ledger, a flat
  entity model with a validated keyspace, item conservation, a declarative
  `<state>` diff, engine-held context. Each was correct given the one before
  it.
- The result is roughly 8,900 lines of state machinery — `worldstate.go`,
  `engine_state.go`, `rules/{state,location,combat,equipment,holder,tables,yaml}.go`,
  `archive/`, `narrative/`, `response.go`, most of `tools.go`, and their tests
  — whose entire job is to hold facts the model also holds, and to refuse the
  model's writes when the two disagree.
- Judged subjectively in play, that machinery does not earn its weight. It is
  not fixing an observed failure; it is enforcing a boundary we are no longer
  sure is in the right place.
- This ADR is an experiment in moving the boundary, not a bug fix. Its success
  criterion is subjective play quality, explicitly not a metric.

Decision
1. **The engine becomes stateless.** It holds no world, no characters, no
   locations. What survives of `rules/` is the Fudge ladder, 4dF, and one call:
   `check(level, vs, modifier)`, where `vs` is a difficulty or an opponent's
   level — in Fudge both are ladder positions, so opposed and unopposed
   collapse into one tool. Seeded RNG stays; it makes the roll fair and the Go
   test cheap.
2. **All state lives in `charsheet.md`**, written and maintained by the LLM.
   One file: identity, attributes, skills, gifts, faults, fudge points, wounds,
   equipment, current status, world state, known facts and history. Nothing
   parses it. The harness only stores it and puts it in the context.
3. **Writes happen through file tools** — `Read`, `Edit` (exact string
   replacement), `Write` (full overwrite). The `<state>` block, event tags, and
   every bookkeeping tool are deleted.
4. **The scratchpad is the trust boundary.** `create_session_directory(character,
   adventure)` creates `saves/NNNN-<character>-<adventure>/` and is the first
   tool call of a game, made after character creation, which is prose. File
   tools take a bare filename, never a path, resolved inside that directory.
   Writes are limited to a whitelist the harness controls: `charsheet.md`
   always, plus a milestone file named by the harness when one is due. Reads
   are open within the directory, plus `charsheet_template.md` at the repo
   root, which is read once per game to seed `charsheet.md` and never copied
   in.
5. **Full conversation history returns.** Sessions are played whole; the
   message array is not collapsed between turns. Context is three blocks:
   ground rules, chat, charsheet. The charsheet is injected as a single pinned
   block at the tail, re-arranged each turn so exactly one copy is present.
6. **`/milestone` is the compaction, and the player triggers it.** The LLM
   writes a full summary of events, states and changes to `milestone-NN.md` and
   appends a one-line outcome to the charsheet's history section; the harness
   dumps the raw chat beside it as `milestone-NN-raw.md` and clears the chat
   block. The new chat block opens with the adventure briefing and the previous
   block's last narration — the briefing because the adventure text exists
   nowhere the DM can look it up, the narration so voice carries across the
   seam. Milestone files sit flat in the session directory rather than under a
   `milestones/` subdirectory: decision 4 admits no paths, so a subdirectory
   would be unreachable by the tool that has to write into it.
7. **Turn order is check → Edit → narrate**, stated in the ground rules, with
   updating `charsheet.md` as rule #1. Enforcement is a player-triggered nag,
   `/inv`, which sends a reminder as a user message. A harness-enforced variant
   was specified and dropped: it costs a wasted round on every turn where
   nothing changed.
8. **Settings collapse to prose.** `description.md` and `adventures/*.md`
   only. `rules/setting.go` and the `recfile/` package go with the `.rec` game
   data: structured data with no validator is a worse markdown. `llm_model.rec`
   is config and stays.
9. **Prompt assembly survives** (`prompts/loader.go`, setting injection,
   `-lang`). Its content inverts: `always.md` grows, because deleting combat
   and wound resolution from Go means the rules text must be in the prompt
   where the code used to be. `dm.md` is rewritten around charsheet discipline;
   `character-*.md` collapses into it.

Consequences
- Replay dies. Same seed and same inputs no longer reproduce a world. ADR 001's
  determinism rationale and the README's "stays consistent no matter how long
  it runs" are both false now and are rewritten, not quietly left standing.
- Every engine-enforced invariant dies with the code: no dangling-reference
  check, no unique-id guarantee, no item conservation, no species validation.
  The smoke suite's `.expect` patterns that assert them go too.
- Drift between the prose and `charsheet.md` becomes a playtest observation
  rather than a designed-away risk. If it shows up, the fix is prompt wording.
- Context grows with the session again, unbounded between milestones. This is
  the thing ADR 002 was written to stop; it is now the player's problem to
  solve with `/milestone`, deliberately.
- Persistence arrives for free, having never existed: the session directory is
  the save. Loading is reading `charsheet.md`.
- `Write`'s overwrite semantics plus a hallucinated filename can destroy the
  session's only state. The whitelist and the directory confinement are the
  whole mitigation.

Rationale
- The boundary between model and harness is the open question of this project,
  and 001-005 answered it by expanding the harness at every step. This is the
  other end of the range, built far enough to actually play, so the two can be
  compared as experiences rather than as arguments.
- What remains in Go is what the model genuinely cannot do: sample fairly. Not
  what it can do badly, or inconsistently, or only most of the time — those are
  now its own problem, which is the point of the experiment.
