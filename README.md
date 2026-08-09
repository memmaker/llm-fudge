# Fudge

A terminal RPG where an LLM is the Dungeon Master. The model narrates, plays
every NPC, applies the rules, and keeps the books — all of them, in one markdown
file it restates at the end of every scene. The Go harness rolls the dice, cuts
that file out of the reply, writes it, and builds the context. It holds no world
of its own.

The ruleset is [Fudge](https://fudgerpg.com/): a light, generic system built on
a seven-level trait scale (Terrible to Superb) rather than numeric stats.

This is the far end of an experiment about where the line between model and
harness belongs. Earlier versions of this repo put the world in Go — an entity
model, item conservation, a validated state diff — and
[ADR 006](docs/adr/006-stateless-engine-llm-owned-charsheet.md) deletes all of
it on purpose. If you are looking for that design, it is in ADRs 001-005 and in
the git history.
[ADR 007](docs/adr/007-two-phase-turn-and-in-band-charsheet.md) is the current
shape: the sheet arrives in band, and prompts are selected as a language-specific
set.

## Quick start

```bash
go build -o fudge .
./fudge
```

You'll pick a setting and an adventure, then the DM makes your character with
you in conversation and opens the first scene. Type actions in plain English.

The game uses English prompts by default. Pass `--lang de` to use the German
prompt set; future languages can be added as another subdirectory under
`prompts/`.

An API key is required — see [Configuration](#configuration) below.

## CLI usage

```
./fudge [flags]
```

| Flag | Default | Effect |
| --- | --- | --- |
| `-debug` | off | Show tool calls, their arguments and results, and what each reply cost — and write every request and response in full to `log/` in the session directory. Also records every answer typed to `input-log.txt`, in `-script` format, so a session that hit a bug can be replayed. |
| `-img-gen` | off | Ask the DM for an image prompt each turn, written to `img-gen.md`. |
| `-lang` | `en` | Select the prompt language, such as `en` or `de`. |
| `-script` | — | Play non-interactively from a file of answers, one per line. See [Scripted play](#scripted-play). |

### In-game commands

Typed at the `>` prompt during play, anything starting with `/`:

| Command | Effect |
| --- | --- |
| `/help` | List these commands. |
| `/quit`, `/exit` | End the session. |
| `/context` | Dump what a request is actually made of — messages, the pinned character sheet, and the tool schemas — to `context.txt`. |
| `/style <text>` | Append a style directive to the DM's system prompt for the rest of the session. Bare `/style` clears it. |
| `/image` | Write an image prompt for the current scene to `img-gen.md`. One extra LLM call, only when asked for. |
| `/milestone` | Archive the chat so far and clear it. Costs no model call. See below. |

### Scripted play

For testing without a live terminal, `-script` feeds a file of answers into
every prompt the game would otherwise ask interactively:

```bash
./fudge -script scripts/dryfall.txt
```

One answer per line; `#` comments and blank lines are skipped. A menu is
answered by naming the option text exactly; anything else is typed as-is.
`-script -` reads from stdin. The session ends cleanly when the script runs out.
See [scripts/dryfall.txt](scripts/dryfall.txt) for a worked example.

A script does not have to be written by hand: `-debug` writes every answer of a
live session to `input-log.txt` in exactly this format, so a bug found by
playing is reproduced with `./fudge -script input-log.txt`.

## How a turn works

Each player line becomes a turn, and a turn has two phases:

1. **Resolve.** If anything is uncertain, one `check` call. The result comes
   back on the next response. The DM is asked to roll without prose beside it,
   because prose written beside a roll was written before the dice were known —
   but if it writes some anyway, the player reads it.
2. **Narrate.** Ordinary prose, and under it the whole character sheet inside a
   `<world-update>` tag. The harness cuts the block out, writes it to
   `charsheet.md`, and shows the player only what lay outside it.

The scene is everything the DM said across the turn, in the order it said it —
beside a roll, above a menu, and again once the results are in. Nothing written
for the player is dropped for having arrived in a response the harness was not
expecting it in. The world update block is the one exception, and only because
it is addressed to the harness rather than to them.

That block is the design. The harness used to have no way of telling a scene
from a status report — its only test was whether a reply called a tool — so any
tool-free reply became the turn's narration, bookkeeping included. Now the scene
is defined structurally: it is what lies outside the tag.

The sheet is restated in full every turn rather than edited, so there is no
per-turn judgement about whether anything changed. A block that never arrives
costs nothing: the sheet stands, and a note rides in under the next player line
asking for it, because the next block restates everything anyway.

## What the DM sees

Three blocks, and nothing else:

- **Ground rules** — the system prompt: the Fudge reference, the DM's job, the
  shape of the sheet, the setting's prose.
- **The chat** — the whole conversation, kept for the session. Unlike earlier
  versions of this project, nothing is folded away between turns.
- **The character sheet** — `charsheet.md`, pinned last, re-read from disk for
  every request so exactly one copy is present and it is never stale.

`charsheet.md` is the entire world: the character, where they are, who is with
them, and every fact the game has established. Nothing parses it except for the
character's name, which is what the save directory is named after. It is also
the save file — the session directory is the save.

### Milestones

The chat grows. `/milestone` is how it stops: the harness archives the raw chat
and clears the conversation, then reopens it with the adventure briefing and the
last scene, so the DM's voice carries across the seam. It costs no model call —
the state is already whole in the sheet, and what the archive keeps is texture.

Compaction is the player's call, not a threshold the harness trips.

## The engine

What remains of the Go rules is one call:

```go
rules.Check(level, vs, modifier, rng)
```

`level` is a ladder position from the sheet. `vs` is either a difficulty
(`Average`, `Very hard`) for an unopposed action, or an opponent's level
(`Good`) for an opposed one — in Fudge both sit on the same scale, so opposed
and unopposed are the same call, and which one was meant is told by which
vocabulary was used. It returns a relative degree.

The RNG is seeded, so a roll is fair and testable. A *session* does not replay:
the world is prose written by a model, and the same inputs will not reproduce
it. That was true of ADR 001's design and is not true of this one.

## Files a game writes

```
saves/0007-garret-dryfall/
  charsheet.md              the world, as the last world update block left it
  charsheet-log.md          every block in order, so one sheet can be diffed
                             against the next
  milestone-01-raw.md       the chat up to a milestone, archived whole
  log/0001.md               one file per request, under -debug only
```

The DM has no file tools at all. Every one of them lost its consumer when the
block took over the sheet: the template is inlined into the prompt, the session
directory is derived from the first block's character name, and a milestone
writes no summary. A turn offers `check` and `user_choice`, and nothing else.

`log/NNNN.md` is one complete exchange per file: the model, every message sent,
every tool schema sent with them, and everything that came back — prose, every
call with its arguments, and the token usage. The terminal's debug lines say
what a reply cost and what it called; every question that outlives them is about
the text nobody printed.

Alongside those, the working directory gets `narration.txt` (the whole of the
last turn's prose) and
— with `-img-gen` or `/image` — `img-gen.md`, for whatever wants to read the
current scene without parsing terminal output.

## Configuration

The LLM backend is a [gollum](https://github.com/memmaker/gollum) client,
configured independently of this project:

- An API key, via `OPENAI_API_KEY` in the environment or a `.apikey` file in
  the working directory.
- Which model and provider to use, via `llm_model.rec` in the working
  directory (falls back to `~/.config/gollum/config.rec`).

## Project layout

```
main.go                     CLI entry point, setup flow, the turn loop,
                             /milestone and the other meta commands
session.go                  The session directory, the charsheet writer and
                             its raw log, and the pinned charsheet block
tools.go                    Every tool the DM can call, and their handlers

rules/check.go              The whole engine: the ladder, 4dF, one check

prompts/                    Prompt loader plus language-specific model-facing
                             text in prompts/en/ and prompts/de/. The loader
                             fills {{PLACEHOLDER}}s and appends the setting.

settings/<lang>/<name>/     One folder per playable setting and language:
  description.md              description plus pre-written adventures. Ships
  adventures/                 with English and German catalogs.

ui/                         Terminal I/O: readline input, choice menus,
                             spinner, text wrapping, scripted input

docs/                       ADRs (docs/adr/), the glossary, the Fudge rules
scripts/                    Example -script files for non-interactive runs
```

## Testing

```bash
go test ./...
```

The tests cover the three things with logic in them: the check, the split
between the scene and the world update block, and the turn loop around it —
driven with scripted responses, asserting on what the player read and what
landed on disk. Everything else is a conversation with a model, and is checked
by playing it: see [consistency-check.md](consistency-check.md).
