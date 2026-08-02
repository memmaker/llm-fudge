# Fudge

A terminal RPG where an LLM is the Dungeon Master. The model narrates, plays
every NPC, applies the rules, and keeps the books — all of them, in one markdown
file it writes itself. The Go harness rolls the dice, holds the file, and builds
the context. It holds no world of its own.

The ruleset is [Fudge](https://fudgerpg.com/): a light, generic system built on
a seven-level trait scale (Terrible to Superb) rather than numeric stats.

This is the far end of an experiment about where the line between model and
harness belongs. Earlier versions of this repo put the world in Go — an entity
model, item conservation, a validated state diff — and
[ADR 006](docs/adr/006-stateless-engine-llm-owned-charsheet.md) deletes all of
it on purpose. If you are looking for that design, it is in ADRs 001-005 and in
the git history.

## Quick start

```bash
go build -o fudge .
./fudge
```

You'll pick a setting and an adventure, then the DM makes your character with
you in conversation and opens the first scene. Type actions in plain English.

An API key is required — see [Configuration](#configuration) below.

## CLI usage

```
./fudge [flags]
```

| Flag | Default | Effect |
| --- | --- | --- |
| `-debug` | off | Show tool calls, their arguments and results, and what each reply cost. |
| `-img-gen` | off | Ask the DM for an image prompt each turn, written to `img-gen.md`. |
| `-lang` | English | Language the player reads: narration, menus, setup. A name or ISO code (`German`, `Deutsch`, `de`). Filenames and the character sheet stay English. |
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
| `/inv` | Tell the DM its character sheet has fallen behind and to bring it current. |
| `/milestone` | Archive the session so far and continue from a summary. See below. |

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

## How a turn works

Each player line becomes a turn. The DM gets up to six requests to finish it:

1. **Roll**, if anything is uncertain — one `check` call, and no prose in that
   response. The result comes back on the next one.
2. **Write it down.** Whatever the outcome made true goes into `charsheet.md`
   through `edit_file`, one edit per thing that changed.
3. **Narrate.** Ordinary prose, exactly what the player reads.

The last request has every tool withdrawn and exists purely to force a narration
if the earlier ones didn't produce one — a turn always ends with the player
having read something.

## What the DM sees

Three blocks, and nothing else:

- **Ground rules** — the system prompt: the Fudge reference, the DM's job, the
  setting's prose, the language directive.
- **The chat** — the whole conversation, kept for the session. Unlike earlier
  versions of this project, nothing is folded away between turns.
- **The character sheet** — `charsheet.md`, pinned last, re-read from disk for
  every request so exactly one copy is present and it is never stale.

`charsheet.md` is the entire world: the character, where they are, who is with
them, and every fact the game has established. Nothing parses it. It is also the
save file — the session directory is the save.

### Milestones

The chat grows. `/milestone` is how it stops: the DM writes a full account of
the stretch just played to `milestone-NN.md`, appends a line to the sheet's
history, and the harness archives the raw chat beside it and clears the
conversation. Play resumes from the last narration, so the DM's voice carries
across the seam.

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
  charsheet.md              the world, written by the DM
  milestone-01.md           the DM's account of a stretch of play
  milestone-01-raw.md       the raw chat behind it, written by the harness
```

The DM reaches these by bare filename and cannot address anything outside the
directory. Writes are limited to a list the harness controls; reads are open
within it, plus `charsheet_template.md` at the repo root, which is read once at
the start to shape the first sheet.

Alongside those, the working directory gets `narration.txt` (the last scene) and
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
session.go                  The session directory, the file tools and their
                             confinement, and the pinned charsheet block
tools.go                    Every tool the DM can call, and their handlers

rules/check.go              The whole engine: the ladder, 4dF, one check

prompts/                    Assembles a system prompt from always.md + the
                             setting + the language directive, and fills
                             {{PLACEHOLDER}}s in the rest
system-prompts/             Every word sent to the model that is not a tool
                             description: always.md (the Fudge reference),
                             dm.md (the DM's job), the setup prompts, and the
                             turn loop's own text — the narration demand, the
                             milestone instructions, the /inv nag, the header
                             above the character sheet

settings/<name>/            One folder per playable setting: description.md
  adventures/                and pre-written adventures. Ships with deadlands,
                             medieval-fantasy, neon-grid, starlight-empires,
                             everyday-modern.

ui/                         Terminal I/O: readline input, choice menus,
                             spinner, text wrapping, scripted input

docs/                       ADRs (docs/adr/), the glossary, the Fudge rules
scripts/                    Example -script files for non-interactive runs
```

## Testing

```bash
go test ./...
```

The tests cover the two things with logic in them: the check, and the file
tools' confinement. Everything else is a conversation with a model, and is
checked by playing it.
