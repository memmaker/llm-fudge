ADR 001: Deterministic LLM-driven RPG Engine
Status: Superseded by ADR 006

Note (ADR 006): the determinism this record is named for is gone. World state is
now prose written by the model, so replaying a seed and an action sequence does
not reproduce a session. Only the dice are still seeded. The rest of this
document describes the design as it stood.

Context
- We want a roleplaying engine where the DM/Narrator is an LLM. The core game state, rules, and transitions must be deterministic and implemented in Go.
- The system should expose the fudge rules as a library (not as CLI) so the LLM can drive gameplay through structured messages rather than shell commands.
- The existing Fudge rules are exposed directly as the `rules` Go package; there is no separate engine layer.
- Access to the LLM will be via the gollum-based interface (as per the gollum project), not via shell invocations.

Decision
- Implement the Go-based core in package `rules`, which models World, Character, and deterministic checks.
- Seeded RNG per World guarantees repeatable outcomes given the same seed and action sequence.
- Expose well-defined `rules.World` methods for operations such as unopposed and opposed skill checks.
- The LLM invokes those methods through structured tool calls without shelling out.
- Document decisions and terminology through ADRs and a glossary (e.g., docs/glossary.md).

Consequences
- The Go binary does not rely on CLI semantics for core game logic; it invokes the `rules` package through a gollum wrapper.
- The LLM emits structured tool calls that drive `rules.World` methods, ensuring determinism.
- We need to ensure all future fudge rules are wired as library calls rather than CLI invocations.

Rationale
- Determinism is essential for replayability, debugging, and fair GM narration.
- A library-centric approach reduces fragility and increases testability of game rules.

Status notes
- This ADR outlines the architectural direction and will be refined as the rules library grows.
