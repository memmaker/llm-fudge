# Spec: Two-phase turn, in-band charsheet, monolingual prompt set

Implements [ADR 007](../adr/007-two-phase-turn-and-in-band-charsheet.md).
Vocabulary follows [the glossary](../glossary.md): Turn, World update block,
Carried note, Prompt set, Charsheet, Session directory, Milestone, Check.

## Problem Statement

The DM is instructed rather than constrained, and the instructions have grown
until they are the largest thing in the repo without being reliable.

Concretely, from the player's seat:

- A turn sometimes prints the DM's bookkeeping instead of the scene. In one
  session the door opening and the kobold charging were replaced by "First
  update recorded the door and statues; second recorded the solved-puzzle fact"
  — the scene was never written, and play continued as though it had been.
- The same failure classes come back after being fixed. Rules against acting for
  the player and against giving away the puzzle each held for two scripted runs
  and fell in the third, with no code change in between.
- The Charsheet accumulates contradictions the player cannot see: the same chest
  listed as both locked and emptied, the same rope carried twice, a wound track
  with duplicate rows.
- English fragments appear mid-scene in a German game.

Behind all four is one harness property: the harness has no way to tell a scene
from a status report. Its only test is whether a response called a tool, so any
tool-free reply becomes the turn's narration. Everything else has been an
attempt to fix that with prose — 4,983 words of Prompt set plus 350 more in tool
descriptions.

## Solution

Give the harness a positive marker instead of a rule.

The DM ends its narration response with the whole Charsheet inside a
`<world-update>` tag. The harness cuts the block out, writes it to disk, and
shows the player only what lay outside it. The scene is defined structurally,
not by instruction.

Everything else in this spec follows from that one change and from deleting what
it makes pointless:

- The Record phase disappears, so a Turn has two phases.
- The DM stops writing the Charsheet, so the file tools lose their consumer in an
  ordinary Turn.
- The Session directory comes from the character's name in the first block, so
  the directory tool goes.
- `/milestone` no longer needs the model at all.
- Player-facing text and notes stop being two channels in one language pair,
  because the Prompt set becomes monolingual: no directive, just prompts written
  in the language you want played back.
- The Fudge rules shrink to what the model cannot guess, and the consistency
  suite decides what comes back.

## User Stories

**Playing**

1. As a player, I want the DM's reply to be the scene and nothing else, so that a
   turn never spends itself on bookkeeping I did not ask to read.
2. As a player, I want the door opening and the enemy charging to reach me as
   prose, so that I am not asked to act on events I was never told about.
3. As a player, I want a turn to cost one round trip rather than two, so that the
   scene arrives sooner.
4. As a player, I want the DM's internal reasoning to stay internal, so that I do
   not read "Let me narrate the honest result" before the result.
5. As a player, I want no English fragments in a German game, so that the voice
   holds.
6. As a player, I want a menu when the DM genuinely needs me to pick between
   distinct options, so that I am not made to type a choice it already framed.
7. As a player, I want my empty input to return me to the prompt, so that a
   stray Enter costs nothing.

**State**

8. As a player, I want my inventory to be the same after the turn as the prose
   said it was, so that the sheet and the story agree.
9. As a player, I want an item I dropped and picked up again to appear once, so
   that the sheet does not accumulate duplicates.
10. As a player, I want a wound track with one row per level, so that the sheet
    stays readable after a fight.
11. As a player, I want everything the DM established to survive into the next
    turn, so that a fact learned early is still true late.
12. As a player, I want a turn where the DM forgot the block to cost me nothing,
    so that one lapse does not corrupt the save.
13. As a player, I want the save directory named after my character, so that
    `saves/` is browsable.

**Adventure fidelity**

14. As a player, I want a puzzle to work exactly as the adventure describes it,
    so that solving it is possible.
15. As a player, I want an action the adventure calls certain to succeed without
    a roll, so that dice cannot lock me out of the scenario.
16. As a player, I want a failed roll to mean this attempt failed, so that the
    world does not shrink around bad luck.
17. As a player, I want an enemy the adventure calls implacable to stay
    implacable, so that surrender is a decision with a cost.
18. As a player, I want to be able to die, so that danger is real.
19. As a player, I want to be able to finish, and to be given experience and a
    farewell when I do, so that the adventure closes.
20. As a player, I want the DM to refuse to hand me the solution when I ask for
    it out of character, so that the puzzle survives my own impatience.
21. As a player, I want the DM to answer the question I asked, so that asking
    what is written on a statue does not make my character search it.

**Maintaining**

22. As the maintainer, I want one place that decides what the player reads, so
    that a leak is a bug in one function rather than a wording problem in five
    prompts.
23. As the maintainer, I want every rule in the Fudge file to have a consistency
    run behind it, so that the file cannot regrow by accretion.
24. As the maintainer, I want situational rules to live in the adventure that
    needs them, so that global rules stay about Fudge.
25. As the maintainer, I want the DM to be offered only the tools the current
    request can use, so that a tool cannot be reached for by mistake.
26. As the maintainer, I want every emitted block logged raw, so that I can diff
    sheet against sheet and find the line that quietly stopped appearing.
27. As the maintainer, I want the harness to correct the DM without spending a
    request, so that recovery is free.
28. As the maintainer, I want to play in another language by translating the
    Prompt set, so that language support is a file copy rather than a runtime
    concern.
29. As the maintainer, I want the consistency suite monolingual, so that a
    language finding is not confounded by the adventure being in another one.
30. As the maintainer, I want the suite to run against both a small and a
    frontier model, so that I learn whether the deleted rules were about Fudge or
    about a small model.

## Implementation Decisions

**Turn shape.** Two phases. Resolve keeps its loop, its request ceiling, and its
existing guard that prose written beside a tool call is discarded. Narrate keeps
its tool-free request. The Record phase and its prompt are removed, along with
the "did anything change" marker, which has no object once the block is
unconditional.

**The World update block.** The narration response carries the whole Charsheet
inside `<world-update>`. Always present, always complete — there is no diff form
and no per-turn judgement about whether something changed, because that judgement
is the thing that has been failing. The block is cut from the response before the
message is appended to the chat, so the ADR 006 invariant holds: exactly one
Charsheet is ever in context, pinned last and re-read from disk.

**Parsing.** One function owns the split, extending the one that already
separates the image block. Recognition is generous — case-insensitive, hyphen or
underscore, surrounding code fences stripped. Acceptance is strict: a block is
written only when opening tag, closing tag and non-empty content are all present,
so a truncated reply cannot write a truncated Charsheet. Recognising a form that
should not have been written is not an error; it produces a Carried note.

**Carried note.** The harness never spends a request to correct the DM. A missing
block, or one whose tag had to be accepted in a tolerated form, appends a short
note to the next player line. The player does not see it. A missing block needs
no repair beyond that: the next block restates everything, including what
happened while it was absent.

**Write authority.** The block is the Charsheet's only writer. The Charsheet
leaves the harness's writable whitelist.

**Tools per request.** An ordinary Turn offers Check and the menu, nothing else.
The setup flow that improvises settings and adventures is untouched. Every other
game tool loses its consumer: the Charsheet template is inlined into the prompt
rather than read, the Session directory is derived rather than created by a call,
and the Milestone file is no longer written.

**Session directory.** Derived by the harness from the character name in the
first accepted block, keeping the existing numbering and slug rules. The opening
Turn therefore looks like every other Turn — prose plus block — and cannot get
the order wrong.

**Milestone.** Archives the raw chat, clears the conversation, reopens with the
adventure briefing and the last narration. No model call, no summary file. State
is already whole in the Charsheet; texture stays in the archive.

**Raw block log.** Every accepted block is appended to a log in the Session
directory, so the suite can compare consecutive sheets. This is the specific
guard against the risk this design takes on, described below.

**Prompt set.** Monolingual. The language directive, its template, and the
per-tool language notes are removed, as is the normalisation that supported them.
Another language means another set of prompt files, selected as a whole.

**Fudge rules file.** Starts at the ladder and the difficulty vocabulary, which
are a contract with Check and with the Charsheet and cannot be guessed. The
harness inlines it into the system prompt through the existing fill mechanism —
a pointer at authoring time, a block at runtime, because an ordinary Turn has no
read tool to follow a real pointer with. Rules return only with a consistency run
as justification. Rules about Fudge mechanics go here; rules about one NPC or one
puzzle go in the adventure, where they held when global rules did not.

**DM prompt.** Replaced by one sentence per concern, after the reference prompt
this work started from: role, adventure, how uncertainty is resolved, what is
rewarded, how state is recorded, the lose condition, the shape of the sheet.
Everything currently in the DM prompt that is not one of those is deleted and
returns only by measurement.

**Deleted user command.** The sheet nag has no object once every Turn rewrites
the whole Charsheet.

## Testing Decisions

A good test here asserts what the player and the disk end up with, not how the
harness got there. It drives a fake chat round and inspects the printed scene,
the chat history, and the Session directory. It does not assert on prompt
wording, on how many requests a turn took, or on the internals of the split.

No new seams. Three exist and all three are used:

**Turn loop, through the injected chat round.** This is the highest seam in the
codebase and the new behaviour lives inside it. Prior art is the existing suite
around it, which already drives scripted responses and asserts on turn outcome.
New cases: the block never reaches the chat history; the Charsheet on disk
matches the block; the scene shown to the player excludes the block; a response
with no block leaves the sheet untouched and attaches a Carried note to the next
player line; a tolerated tag variant is written and also produces a note; a
truncated block writes nothing; the first block creates the Session directory
with the character's name; a milestone makes no model call. The existing guard
that prose beside a tool call is discarded keeps its test, and gains one: the
block in such a response is discarded with the prose, since both were written
before the result came back.

**The split function.** Prior art is the three tests already covering the image
block and the changed marker. New cases are recognition breadth and acceptance
strictness — the same tests read twice, once for what must be recognised and once
for what must be refused.

**The consistency suite.** Unchanged as a mechanism, and the arbiter of the whole
rules-file question. It gains a check that consecutive raw blocks never lose a
line, which is the only automated detector for the risk this design takes on. The
adventure it runs against and its player probes move to English. Its existing
duplicate-line and experience-point checks stay.

Prompt-file tests keep their job: every named prompt exists and fills. The list
shrinks with the prompts.

## Out of Scope

- Which model the project targets. The suite runs against both; the decision
  comes after the numbers.
- The setup flow that improvises a setting or an adventure. Its tools and prompts
  are untouched.
- A German Prompt set. The mechanism is decided; producing the translation is a
  later piece of work.
- Streaming or any change to how the scene is displayed.
- Milestone summarisation in any form. If texture turns out to be missed, that is
  a new decision, not a deferred part of this one.
- The narration-quality questions the suite does not measure.

## Further Notes

The risk this design accepts, stated plainly: the default inverts. Editing
preserved everything it did not touch and forgot only on command; full
restatement forgets by default and preserves only what comes back. Two things
work against it — the current Charsheet is pinned immediately above where the new
one is written, so the model copies rather than recalls, and the raw block log
makes a silently dropped line detectable. Whether that is enough is the one
question this design leaves open, and it is measurable.

The measurement is also the acceptance criterion. The suite's nine tests are the
bar; ADR 006 chose subjective play quality as its criterion and this one does
not. A rule deleted here returns only with a run behind it, which is what keeps
the Prompt set from regrowing to the size that motivated this work.
