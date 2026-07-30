# Fudge RPG Engine

A Go-based role-playing game engine for the Fudge system, designed to facilitate narrative-driven gameplay with AI-assisted narration and rule resolution.

---

## Table of Contents

- [Overview](#overview)
- [Features](#features)
- [Architecture](#architecture)
- [Installation](#installation)
- [Usage](#usage)
- [Configuration](#configuration)
- [API Reference](#api-reference)
- [Data Models](#data-models)
- [File Structure](#file-structure)
- [Testing](#testing)
- [Contributing](#contributing)
- [License](#license)

---

## Overview

The Fudge RPG Engine is a comprehensive role-playing game system implementation in Go that brings the classic Fudge rules to life with AI-assisted narration. It manages character creation, combat resolution, inventory tracking, and world state while providing a terminal interface for gameplay.

### Key Design Principles

1. **Flat Entity Model** - All entities (characters, locations, items, narrative-only facts) share a unified ID keyspace
2. **Separation of Concerns** - Rules computation, narrative storage, and archive recording are distinct packages
3. **Deterministic Rendering** - World state is rendered consistently every turn for reliable debugging
4. **AI Integration** - Built on top of the Gollum LLM framework for natural language interaction

---

## Features

### Core Gameplay

- **Character Creation** - Full character creation with attributes, skills, gifts, and faults
- **Combat System** - Fudge-style combat with opposed/unopposed attacks, weapons, and armor
- **Inventory Management** - Track items across characters and locations with weapon/armor ratings
- **World State** - Maintain a persistent world state including locations, NPCs, and objects
- **Narrative Properties** - Store DM-only facts alongside engine-owned data

### AI Integration

- **Natural Language Commands** - Interact with the game using natural language
- **Tool Calls** - Structured function calls for precise control over game state
- **LLM Model Selection** - Support for multiple LLM providers with configurable context windows

### Persistence & Recording

- **Turn Archive** - Record every turn of gameplay for recall and replay
- **Recfile Support** - Import/export character sheets in recfile format
- **Session Management** - Track game state across sessions

---

## Architecture

### Package Structure

```
├── main.go                    # Application entry point and game loop
├── rules/                     # Core Fudge rules engine
│   ├── combat.go              # Combat resolution (attacks, wounds)
│   ├── equipment.go           # Weapon and armor ratings
│   ├── location.go            # Location management
│   ├── rules.go              # Trait resolution and checks
│   ├── state.go               # World state and character data structures
│   └── yaml.go                # YAML rendering helpers
├── archive/                   # Turn recording and history
│   └── archive.go            # Archive structure and query methods
├── narrative/                 # DM-only narrative properties
│   └── narrative.go          # Narrative store and operations
├── ui/                        # Terminal user interface
│   ├── terminal.go           # Main terminal handler
│   ├── input.go              # Input parsing and validation
│   └── spinner.go            # Loading indicators
├── recfile/                  # Recfile format parsing/writing
│   ├── rec.go                # Recfile reader and writer
│   └── string.go             # String formatting helpers
├── prompts/                   # LLM prompt templates
│   └── loader.go             # Prompt loading and management
├── tools.go                  # LLM tool definitions
└── engine_state.go           # World state rendering
```

### Core Data Structures

#### Entity Model

All entities share a flat ID keyspace:

```go
type World struct {
    Characters map[string]*Character
    Locations  map[string]*Location
    Items      map[string]*Item
}

type Character struct {
    ID          string
    Name        string
    Species     string
    Attributes  map[string]int
    Skills      map[string]int
    Gifts       []string
    Faults      []string
    FudgePoints int
    Wounds      WoundState
    Inventory   map[string]*Item
    Coins       int
    Scale       int
}

type Item struct {
    ID          string
    Name        string
    Description string
    Count       int
    Weapon      *Weapon  // For weapons
    Armor       *Armor   // For armor
    Where       string   // Location when not carried
}
```

#### Narrative Store

Narrative-only entities (rumours, debts, objectives) share the same ID keyspace as engine-owned entities but are separated by a reserved word list:

```go
type Store struct {
    ids      []string
    entities map[string]*Entity
}

type Entity struct {
    ID       string
    Type     string  // "rumour", "debt", "job", etc.
    names    []string  // Order of first mention
    props    map[string]any
    turn     int       // Turn when last written
}
```

#### Archive Structure

Every turn is recorded with its events and prose:

```go
type Turn struct {
    Number      int
    PlayerInput string
    Events      []Event
    Prose       string
    Unnarrated  bool
}

type Event struct {
    Tool      string
    Entities  []string
    Headline  string
}
```

### Game Loop

The engine follows a strict turn-based structure:

1. **Bookkeeping** - Update state from previous turn
2. **Narration** - Generate prose and tool calls for LLM
3. **Record** - Write turn to archive
4. **Bookkeeping (again)** - Process tool call results

```
┌───────────── Pinned Messages ────────────┐
│ DM Prompt | Opening Briefing             │
├──────────────────────────────────────────┤
│ WORLD STATE                              │
│ - Characters (player first)              │
│ - Locations (tree order)                 │
│ - Items                                  │
│ - Narrative-only entities                │
├──────────────────────────────────────────┤
│ TIMELINE (last 10 turns)                 │
│ - Recent prose (3 turns)                 │
│ - Archive of all turns                   │
└──────────────────────────────────────────┘
```

---

## Installation

### Prerequisites

- Go 1.25 or later
- A compatible LLM provider (Claude, GPT, etc.)

### Setup

```bash
# Clone or download the repository
cd fudge

# Download dependencies
go mod download

# Run tests (optional but recommended)
go test ./...

# Build the application
go build -o fudge_engine .
```

### Environment Variables

Set these before running the engine:

| Variable | Description | Default |
|----------|-------------|---------|
| `FUDGE_LLM_MODEL` | LLM model name | See `contextWindowSizes` in main.go |
| `FUDGE_LLM_API_KEY` | LLM API key (if required) | - |
| `FUDGE_DEBUG` | Enable debug mode | false |

---

## Usage

### Running the Engine

```bash
# Interactive mode with readline support
./fudge_engine

# With specific LLM model
FUDGE_LLM_MODEL=claude-3-5-sonnet ./fudge_engine

# With debug output
FUDGE_DEBUG=true ./fudge_engine
```

### Terminal Interface

The terminal interface supports:

- **Natural Language Commands** - Describe actions in prose
- **World Update** - Direct state changes via JSON
- **Recall** - Query the archive for past events
- **Character Sheets** - View and edit character data

### Example Commands

```bash
# Start a new game with setting creation
./fudge_engine --create-setting "My Campaign"

# Load an existing character sheet (recfile format)
./fudge_engine --load-characters characters/*.rec

# Query the archive for past events
./fudge_engine --recall "ghosts"

# View world state
./fudge_engine --state
```

---

## Configuration

### Command-Line Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--model` | LLM model to use | auto-detect |
| `--debug` | Enable debug mode | false |
| `--create-setting` | Create a new setting | - |
| `--load-characters` | Load character sheets from recfiles | - |
| `--recall` | Query archive for keyword | - |
| `--state` | Show current world state | - |

### Configuration File

Create a `.fudge.yaml` file in the project root:

```yaml
llm:
  model: "claude-3-5-sonnet"
  api_key: "${FUDGE_LLM_API_KEY}"

game:
  debug: false
  max_turn_requests: 5
  max_record_requests: 2

ui:
  prompt_prefix: "Fudge RPG Engine v1.0"
```

---

## API Reference

### LLM Tools

The engine exposes a set of tools for AI interaction:

#### Character Creation

| Tool | Description |
|------|-------------|
| `create_setting` | Initialize a new game setting |
| `write_setting_file` | Write setting files (description, species, attributes, etc.) |
| `create_character` | Create a new character with traits |

#### Combat

| Tool | Description |
|------|-------------|
| `opposed_attack` | Strike against a resisting target |
| `unopposed_attack` | Strike against no resistance (difficulty-based) |
| `reload_weapon` | Reload a ranged weapon's magazine |

#### Inventory & Items

| Tool | Description |
|------|-------------|
| `add_weapon` | Give a character a weapon with rating |
| `add_armor` | Give a character armor with protection |
| `add_item` | Add an item to inventory or location |
| `remove_item` | Destroy an item (eaten, burned, etc.) |
| `equip_armor` | Put armor on or take it off |

#### World Management

| Tool | Description |
|------|-------------|
| `create_location` | Create a new location in the world |
| `write_adventure` | Persist an improvised adventure to disk |

### World Update Operations

Direct state changes via JSON:

```json
{
  "op": "set",
  "entity": "kyle-katarn",
  "properties": {
    "attributes": {"strength": 2, "dexterity": 1},
    "skills": {"shootin": 1}
  }
}
```

---

## Data Models

### Trait Resolution

The engine uses a 7-level trait scale:

| Level | Value | Adjective |
|-------|-------|-----------|
| +3    | Superb |
| +2    | Great  |
| +1    | Good   |
| 0     | Fair   |
| -1    | Mediocre |
| -2    | Poor   |
| -3    | Terrible |

### Combat Resolution

```go
type CheckResult struct {
    Success     bool
    Total       int      // Trait + Roll + Modifier
    Roll        int      // Raw die roll
    CritSuccess bool     // Natural 4 (4dF) or -4 (1d6)
    CritFailure bool     // Natural failure
    Rerolled    bool     // Fudge Point spent for reroll
    DiscardedRoll int    // Lost roll on reroll

    Hit        bool      // Blow landed
    Graze      bool      // Touch but no damage
    Miss       string    // Why it missed (standoff, outrolled, etc.)

    WeaponName      string
    DamageFactor    int  // Offensive damage factor
    StrengthBonus   int
    ArmorProtection int  // Defensive protection

    Wound     string      // New wound level
    Scratches int         // Scratch track increment
    Penalty   int         // Wound penalty applied
}
```

### Wound System

```go
type WoundLevel int

const (
    Undamaged WoundLevel = iota
    Scratch       // +1 scratch track
    Hurt          // -1 penalty, can act
    VeryHurt      // -2 penalty, can act
    Incapacitated // -3 penalty, cannot act
    NearDeath     // -4 penalty, cannot act
    Dead          // Game over
)

type WoundState struct {
    Scratches int  // Scratch track (0-3)
    Level     WoundLevel
}

func (w *WoundState) Penalty() int {
    // Returns -0 to -4 based on level
}

func (w *WoundState) CanAct() bool {
    // Returns false when incapacitated or near death
}
```

---

## File Structure

### Source Files Summary

| Package | Files | Purpose |
|---------|-------|---------|
| `main` | 1 file | Application entry point, game loop, constants |
| `rules` | 9 files | Core Fudge rules: combat, traits, locations, items |
| `archive` | 1 file | Turn recording and history management |
| `narrative` | 1 file | DM-only narrative properties store |
| `ui` | 3 files | Terminal user interface and input handling |
| `recfile` | 2 files | Recfile format parsing/writing |
| `prompts` | 1 file | LLM prompt templates and loading |

### Test Coverage

The codebase includes comprehensive test coverage:

- **Engine State Tests** - World state rendering and entity ordering
- **Combat Tests** - Attack resolution, wound calculation
- **Archive Tests** - Turn recording and query operations
- **Narrative Tests** - Property storage and retrieval
- **Rule Tests** - Trait resolution, checks, and validation

Run tests with:

```bash
go test ./... -v
```

---

## Testing

### Running Tests

```bash
# Run all tests
go test ./... -v

# Run specific package tests
go test ./rules/... -v
go test ./archive/... -v

# Run with coverage
go test ./... -coverprofile=coverage.out
```

### Test Structure

Tests follow the pattern of testing:

1. **Entity uniqueness** - Each entity appears exactly once in world state
2. **Current state rendering** - Wounds and changes reflect latest turn
3. **Player precedence** - Player character listed first
4. **Dead characters persist** - Characters remain in world when dead
5. **Narrative separation** - Engine and narrative namespaces don't conflict

---

## Contributing

### Development Guidelines

1. **Write tests** - All changes should be accompanied by tests
2. **Follow naming conventions** - IDs are lowercase with hyphens
3. **Maintain flat entity model** - Don't create separate namespaces
4. **Document changes** - Update README if API or behavior changes

### Adding New Features

1. Define the data structure in `rules/state.go`
2. Add validation and resolution logic to appropriate rule file
3. Create LLM tools in `tools.go`
4. Write tests for the new functionality

### Adding New LLM Models

Edit `contextWindowSizes` in `main.go`:

```go
var contextWindowSizes = map[string]int{
    "your-model": 200000,
}
```

---

## License

This project is open source and available under the terms specified in the LICENSE file.

---

## Related Documentation

- [Fudge System Reference Document](docs/fudge.md) - Complete Fudge rules
- [Fudge in a Nutshell](docs/fudge_condensed.md) - Quick reference
- [Deterministic LLM RPG Engine Spec](docs/deterministic-llm-rpg-engine-spec.md) - Architecture specification
- [Engine-Held Context Spec](docs/engine-held-context-spec.md) - Context management

---

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-07-30 | Initial release with core Fudge rules implementation |

---

*Generated for the Fudge RPG Engine project*