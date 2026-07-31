# Fudge

A terminal RPG where an LLM is the Dungeon Master and a Go engine holds the
rules. The model never rolls dice, never does arithmetic, and never remembers
anything on its own: it narrates, and calls into the engine for everything a
result depends on. Between turns, the whole of what it knows is rebuilt from
the engine's own record — not carried forward as conversation history — so the
game stays consistent no matter how long it runs.

The ruleset is [Fudge](https://fudgerpg.com/): a light, generic system built
on a seven-level trait scale (Terrible to Superb) rather than numeric stats.

## Quick start

```bash
go build -o fudge .
./fudge
```

You'll be walked through picking or describing a setting, an adventure, and a
character, then play begins. Type actions in plain English; the DM narrates.

An API key is required — see [Configuration](#configuration) below.

Each turn also overwrites a few plain files in the working directory, for
whatever wants to read the current state without parsing terminal output:
`charsheet.txt` (the player character's sheet), `narration.txt` (the last
scene's prose), and — with `-img-gen`, or on demand via `/image` — `img-gen.md`
(an image prompt for the current scene).

## CLI usage

```
./fudge [flags]
```

| Flag | Default | Effect |
| --- | --- | --- |
| `-debug` | off | Show tool calls, their arguments and results, and the raw reply — `<state>` block and event tags included — instead of the cleaned-up prose a player sees. |
| `-img-gen` | off | Ask the DM for an image prompt each turn (written to `img-gen.md`) alongside the narration. |
| `-lang` | English | Language the player reads: narration, menus, setup. A name or ISO code (`German`, `Deutsch`, `de`). Engine data (ids, setting files) stays English regardless. |
| `-script` | — | Play non-interactively from a file of answers, one per line, instead of a live terminal. See [Scripted play](#scripted-play). |

### In-game commands

Typed at the `>` prompt during play, anything starting with `/`:

| Command | Effect |
| --- | --- |
| `/quit`, `/exit` | End the session. |
| `/context` | Dump the DM's current context — world state, timeline, archive, tool schemas, and the raw message list — to `context.txt`. The tool for inspecting what a turn actually costs and what the model actually sees. |
| `/style <text>` | Append a style directive to the DM's system prompt for the rest of the session (e.g. `/style Write shorter scenes.`). Bare `/style` clears it. |
| `/image` | Write an image prompt for the current scene to `img-gen.md` — a one-off request, using the same guidance as `-img-gen`, without turning it on for the rest of the session. Costs one extra LLM call, only when asked for. |

### Scripted play

For testing without a live terminal, `-script` feeds a file of answers into
every prompt the game would otherwise ask interactively:

```bash
./fudge -script scripts/dryfall.txt
```

One answer per line; `#` comments and blank lines are skipped. A menu (choosing
a setting, an adventure, a creation method) is answered by naming the option
text exactly; anything else is typed as-is. `-script -` reads the script from
stdin. Output goes to stdout exactly as a live session would print it, with
terminal control codes (cursor moves, screen clears, the spinner) suppressed
since there's no terminal to move a cursor on. The session ends cleanly when
the script runs out of lines. See [scripts/dryfall.txt](scripts/dryfall.txt)
for a worked example.

## Configuration

The LLM backend is a [gollum](https://github.com/memmaker/gollum) client,
configured independently of this project:

- An API key, via `OPENAI_API_KEY` in the environment or a `.apikey` file in
  the working directory.
- Which model and provider to use, via `llm_model.rec` in the working
  directory (falls back to `~/.config/gollum/config.rec`). See that file in
  this repo for the format.

## How a turn works

Each player line becomes a turn. The DM gets up to five requests to finish it:

1. **Resolve.** If anything is uncertain — a check, an attack, a table roll —
   the DM calls into the engine and writes no prose yet. Results come back on
   the next request.
2. **Narrate.** Ordinary prose, exactly what the player reads, with wounds and
   deaths marked inline as tags (`<damage target="..." severity="hurt"/>`).
3. **Close with `<state>`**: a YAML block declaring what the prose left true —
   an item that changed hands, a mood that shifted, a door now barred. The
   engine reads it as a diff against what it already knows, not as commands.

The fifth request has every tool withdrawn and exists purely to force a
narration if the first four didn't produce one — a turn always ends with the
player having read something.

Between turns, nothing is carried in message history. What the DM knows next
turn is rebuilt from three things the engine actually holds:

- **World state** — every character, item and location, current, under a
  permanent id.
- **Timeline** — the last several turns: what was typed, what the engine
  resolved (dice, tables, attacks). Not what the state block changed — that's
  already current above.
- **Recent prose** — the last scene, verbatim, so the DM's own voice carries
  forward.

Anything older lives in the archive and is one `recall` call away, indexed by
which entities took part in each turn.

The full design rationale — why state is declarative rather than a command
grammar, why health is the one exception (an event tag, not a state field),
why ids are flat rather than nested — is in
[docs/adr/](docs/adr/) as a sequence of decision records, each superseding
parts of the ones before it as the design evolved. Start with
[001](docs/adr/001-deterministic-llm-rpg-engine.md) for the shape of the whole
thing, or [docs/glossary.md](docs/glossary.md) for terminology.

## Project layout

```
main.go                    CLI entry point, setup flow, the turn loop
engine_state.go            Renders the WORLD STATE / TIMELINE / RECENT PROSE
                            block the DM reads every turn
worldstate.go               Applies the DM's <state> block back into the world
response.go                 Splits a reply into prose, state block, event tags
tools.go                    Every tool the DM can call, and their handlers
tools_combat_test.go, ...   Tests, one file per area (combat, inventory,
                            locations, phases, state) alongside the code

rules/                      The deterministic engine: no LLM, no I/O.
  rules.go, state.go          Character, Wounds, checks and rolls
  combat.go                   Opposed/unopposed attacks, damage, armor
  location.go                 Locations, containment, movement
  equipment.go                 Weapons, armor, ammunition
  setting.go                   Loading a setting's species/attributes/etc.
  tables.go, tables_data.go    Random tables (npc-reaction, weather, ...)
  yaml.go                      The block's YAML dialect

archive/                    The turn-by-turn record: what was played,
                            searchable by entity or keyword, source for recall
narrative/                 The `narrative` property store — the DM's own
                            facts (mood, goals, rumors) keyed by entity id

prompts/                   Assembles a system prompt from always.md + the
                            setting + the language directive
system-prompts/            The actual prompt text: always.md (rules
                            reference, output constraints), dm.md (the DM's
                            job), character-*.md (creation flows), etc.

settings/<name>/            One folder per playable setting: description.md,
  adventures/                 species/attributes/skills/gifts/faults as
                               .rec files, and pre-written adventures under
                               adventures/. Ship with deadlands,
                               medieval-fantasy, neon-grid, starlight-empires,
                               everyday-modern.

ui/                         Terminal I/O: readline input, choice menus,
                            spinner, text wrapping — and where scripted
                            (-script) input is threaded in instead

docs/                       ADRs (docs/adr/), a glossary, and longer specs
scripts/                    Example -script files for non-interactive runs
recfile/                    The .rec (GNU recutils-style) file reader used
                            for settings and species/attribute data
```

## Testing

```bash
go test ./...
```

Tests sit next to the code they cover, one file per concern rather than one
per source file — `tools_combat_test.go`, `tools_inventory_test.go`, and so on
for `tools.go`. For an end-to-end check of an actual session, run a scripted
game and inspect `/context`'s output rather than reading assertions:

```bash
./fudge -script scripts/dryfall.txt
```
