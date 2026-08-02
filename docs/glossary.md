Glossary

Reflects ADR 006. Terms belonging to the engine-held world model — Entity,
Kernel, State block, Event tag, Under-determination test, Derived field,
Narrative properties, Conservation, Retirement, Query tool — are retired along
with the code that gave them meaning. They are still defined in ADRs 002-005,
which is where to look when reading old commits.

- Session: One played game, whole. Lives in a Session directory, is played as a
  single conversation, and is not resumed by replaying anything — the Charsheet
  is what carries over.
- Session directory: The scratchpad the game writes into,
  `saves/NNNN-<character>-<adventure>/`. Created by the LLM's first tool call,
  made after character creation. Every file tool resolves a bare filename inside
  it and can reach nowhere else; the directory is a trust boundary, not a
  convention.
- Charsheet: `charsheet.md`, the single file holding everything the game knows —
  the character, the world around them, and what has been established as true.
  Written and maintained by the LLM through Edit. Nothing parses it. It is at
  once the character sheet, the world state, and the save file.
- Charsheet template: `charsheet_template.md` at the repo root, read once per
  game to shape the initial Charsheet and never again — thereafter the Charsheet
  is its own template.
- Context: What the DM sees, in three blocks and no more: Ground Rules, Chat,
  Charsheet. The Charsheet is pinned last and re-arranged each turn so exactly
  one copy of it is ever present.
- Ground Rules: The assembled system prompt — the Fudge reference, the DM's job,
  the setting prose, the language directive. Carries the rules text that used to
  be Go code.
- Chat: The live conversation, kept whole for the session rather than collapsed
  between turns. Grows until a Milestone clears it.
- Milestone: Player-triggered compaction, `/milestone`. The LLM writes a full
  summary of events, states and changes to `milestone-NN.md` and appends a
  one-line outcome to the Charsheet's history; the harness archives the raw chat
  as `milestone-NN-raw.md` and clears the Chat block. The new Chat opens with the
  adventure briefing and the previous one's last narration — the first because
  the adventure exists nowhere else the DM can reach, the second so its voice
  carries across the seam.
- Nag: `/inv`, a player-sent reminder that the Charsheet was not updated. The
  only enforcement of Charsheet discipline — a harness-enforced variant was
  specified and rejected, because it costs a round on every turn where nothing
  changed.
- Turn: One player line and the DM's answer to it. Ordered check, then Edit,
  then narration; a turn ends when the player has read prose.
- Check: The one thing the engine still does. `check(level, vs, modifier)`, where
  `vs` is a difficulty or an opponent's level, since in Fudge both are Ladder
  positions. Returns the relative degree of success. Opposed and unopposed are
  the same call.
- Ladder: The seven-level Fudge trait scale — Terrible, Poor, Mediocre, Fair,
  Good, Great, Superb. What a character is, what a task demands, and what a roll
  produces all sit on it, which is why the engine needs no arithmetic beyond a
  shift along it.
- Wound track: The Fudge damage record — three Scratch boxes, then Hurt (-1),
  Very Hurt (-2), Incapacitated, Near Death. Kept in the Charsheet by the LLM;
  there is no wound code any more.
- Damage factor: A weapon's contribution to how hard a hit lands, in Ladder steps
  rather than dice.
- Fudge point: The player's spendable luck. A number in the Charsheet.
- Setting: A playable world, prose only — `description.md` plus
  `adventures/*.md`. The species and skill lists that used to be `.rec` data are
  paragraphs the LLM reads, not a vocabulary it is checked against.
- Adventure: A pre-written scenario within a Setting, and half of a Session
  directory's name.
- DM: The LLM. Narrates, decides consequences, keeps the Charsheet, and rolls
  nothing.
- Engine: What is left of `rules/` — the Ladder, 4dF, and Check. Holds no state.
  Seeded, so a roll is fair and testable in Go, but a Session as a whole no
  longer replays.
- File tools: `Read`, `Edit` (exact string replacement; errors on no match and on
  ambiguous match), `Write` (full overwrite). The entire write surface. Writes go
  only to a whitelist the harness controls — the Charsheet always, plus a
  Milestone file when one is due.
- NPC: A character the DM runs. A few lines in the Charsheet's world state, with
  no sheet of its own.
