Glossary

Reflects ADR 007. Terms belonging to the engine-held world model — Entity,
Kernel, State block, Event tag, Under-determination test, Derived field,
Narrative properties, Conservation, Retirement, Query tool — are retired along
with the code that gave them meaning. They are still defined in ADRs 002-005,
which is where to look when reading old commits.

- Session: One played game, whole. Lives in a Session directory, is played as a
  single conversation, and is not resumed by replaying anything — the Charsheet
  is what carries over.
- Session directory: The scratchpad the game writes into,
  `saves/NNNN-<character>-<adventure>/`. Created by the harness from the first
  World update block, whose sheet carries the character's name — so the opening
  turn looks like every other turn and cannot get the order wrong. Nothing but
  the harness writes into it.
- Charsheet: `charsheet.md`, the single file holding everything the game knows —
  the character, the world around them, and what has been established as true.
  Emitted whole by the LLM in every World update block and written by the
  harness. Nothing parses it beyond the character's name on the first one. It is
  at once the character sheet, the world state, and the save file.
- Request log: `log/NNNN.md` in the Session directory, one file per call to the
  model, written only under `-debug`: everything sent, every tool schema sent
  with it, and everything that came back. Calls made before the Session
  directory exists are held and written the moment it does.
- Charsheet log: `charsheet-log.md`, every accepted World update block in order.
  The Charsheet only ever shows the state that survived, so a line that quietly
  stopped being restated would leave no trace anywhere else. It is the one
  automated detector for the risk full restatement takes on.
- Charsheet template: `charsheet_template.md` at the repo root, inlined into the
  Ground Rules to shape the sheet. Inlined rather than pointed at because an
  ordinary Turn has no read tool to follow a pointer with.
- Context: What the DM sees, in three blocks and no more: Ground Rules, Chat,
  Charsheet. The Charsheet is pinned last and re-arranged each turn so exactly
  one copy of it is ever present.
- Ground Rules: The assembled system prompt — `dm.md`, which carries the DM's
  job, the Fudge reference and the shape of the sheet, plus the setting prose.
  One file rather than a wrapper and a body: the wrapper was prepended to the
  setup prompts too, which have no use for a ladder.
- Prompt set: Every model-facing word, in one language. A game is monolingual:
  the language the player reads is the language the prompt set is written in,
  and there is no directive asking for it. Playing in another language means
  translating the set, not instructing the model — an instruction can be
  disobeyed, and today's drift was always meta text escaping into the player's
  channel. There is no engine channel any more, so there are no two channels.
- Chat: The live conversation, kept whole for the session rather than collapsed
  between turns. Grows until a Milestone clears it.
- Milestone: Player-triggered compaction, `/milestone`. The harness archives the
  raw chat as `milestone-NN-raw.md` and clears the Chat block. No model call and
  no summary: the state is already whole in the Charsheet, and what the archive
  keeps is texture. The new Chat opens with the adventure briefing and the
  previous one's last narration — the first because the adventure exists nowhere
  else the DM can reach, the second so its voice carries across the seam.
- Carried note: The harness's only way of correcting the DM. It never spends a
  request of its own: whatever it has to say — a World update block that was
  missing, or one whose tag it had to accept in a form it should not have been
  written in — is appended to the next player line and rides in with the next
  ordinary turn. The player never sees it.
- Turn: One player line and the DM's answer to it. Two phases: resolve, where
  checks are called, and narrate, which carries both the scene and the World
  update block. A turn ends when the player has read prose.
- Scene: Everything the DM wrote for the player in one Turn, in the order it was
  written — beside a roll, above a menu, and again once the results are in. The
  harness discards none of it: prose arriving in a response it was not expected
  in is still prose written for the player. Only the World update block is cut,
  because that one is addressed to the harness.
- World update block: The whole Charsheet, emitted inside a tag in the same
  response as the narration, and written to disk by the harness. It replaces the
  Record phase: the sheet is restated in full every turn rather than edited, so
  there is no per-turn judgement about whether anything changed. It is also the
  harness's only positive test for what the player may read — the scene is what
  lies outside the tag.
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
- DM: The LLM. Narrates, decides consequences, restates the Charsheet at the end
  of every scene, and rolls nothing.
- Engine: What is left of `rules/` — the Ladder, 4dF, and Check. Holds no state.
  Seeded, so a roll is fair and testable in Go, but a Session as a whole no
  longer replays.
- Tools: What a Turn offers the DM, which is Check and the menu. Every other
  game tool lost its consumer when the World update block became the Charsheet's
  only writer — the template is inlined, the Session directory is derived, and a
  Milestone writes nothing. A tool the DM is never shown is one it cannot reach
  for by mistake. The file tools of ADR 006, and the confinement around them,
  are gone with them.
- NPC: A character the DM runs. A few lines in the Charsheet's world state, with
  no sheet of its own.
