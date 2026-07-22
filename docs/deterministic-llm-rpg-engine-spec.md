# Deterministic LLM-Driven Fudge RPG Engine

## Problem Statement

The current Fudge binary has a minimal DM loop and only exposes seeded unopposed and opposed skill checks. It does not yet provide a complete campaign loop, a validated configurable ruleset, or deterministic state mutations for characters, combat, equipment, advancement, and session persistence.

The LLM must remain responsible for narrative interpretation and prose, but it must not invent dice results, silently mutate game state, or apply unvalidated mechanics. The Go engine needs to enforce the deterministic part of the Fudge rules while exposing that capability through LLM tools.

## Solution

Build a generic, data-defined Fudge campaign engine in Go. A selected ruleset folder supplies configurable attributes, skills, gifts, faults, powers, and species. The ruleset is created or amended during setup, validated, and frozen when the first character is created.

The rules package owns the deterministic world model, rules operations, LLM tool definitions, tool dispatch, rec-file serialization, and validation. The LLM supplies narrative decisions such as whether an action needs a check, which trait applies, the difficulty, situational modifiers, item proposals, and narration. Go validates every request, consumes the seeded RNG only for valid operations, applies state transitions atomically, and returns rec-file text inside the JSON content envelope required by the gollum LLM protocol.

## User Stories

1. As a game host, I want to select a ruleset by slug when creating a World, so that different campaigns can use different configurable content.
2. As a game host, I want rulesets organized in folders, so that western, science-fiction, and other campaign profiles remain independent.
3. As a player, I want the LLM and me to create missing ruleset definitions during setup, so that a campaign can define its own vocabulary without changing Go code.
4. As a game host, I want Go to validate the complete ruleset before character creation, so that invalid definitions cannot enter play.
5. As a game host, I want the ruleset frozen when the first character is created, so that mechanics and references cannot change mid-session.
6. As a player, I want attributes and skills described with slugs, display names, and prose, so that the engine remains generic while narration remains readable.
7. As a player, I want species to provide a validated scale, so that creature size affects combat without requiring species-specific code.
8. As a player, I want to create a character subjectively with the LLM and discuss it before committing, so that character concepts are flexible rather than constrained by a point-buy workflow.
9. As a game host, I want Go to validate character trait slugs and level bounds, so that subjective creation cannot bypass the Fudge scale.
10. As a player, I want omitted attributes to default to Fair and omitted skills to default to Poor, so that incomplete subjective character proposals remain playable.
11. As a game host, I want to designate the player character separately, so that character creation order does not determine player identity.
12. As a player, I want to resolve unopposed actions using a chosen attribute or skill, difficulty, and situational modifier, so that the LLM can apply narrative judgment while Go resolves the roll.
13. As a player, I want to resolve opposed actions with independently chosen traits and modifiers, so that conflicts can use different abilities on each side.
14. As a game host, I want all action rolls to use seeded 4dF, so that a session is deterministic for a given seed and valid action sequence.
15. As a player, I want critical success and critical failure reported from natural die results, so that the LLM can narrate exceptional outcomes without inventing them.
16. As a player, I want to spend a Fudge Point before a roll to add one to the die result, so that I can influence an uncertain outcome through a validated resource spend.
17. As a player, I want to repeat a check with a reroll flag, so that the old result is discarded and the new result is deterministic and costs one Fudge Point.
18. As a game host, I want to add or remove Fudge Points through tools, so that narrative rewards and player expenditures update the character balance explicitly.
19. As a player, I want Fudge Points to reduce attack damage by one wound step, so that the resource has a meaningful defensive use.
20. As a player, I want to declare a coincidence through conversation, so that the LLM can give it narrative meaning while the actual cost is enforced by the Fudge Point removal tool.
21. As a player, I want story-element combat rather than initiative rounds, so that combat remains dramatic and prose-driven.
22. As a player, I want an atomic attack operation, so that one validated operation resolves the contest, hit threshold, damage, wounds, and relevant resource spend.
23. As a player, I want melee attacks to use Strength and ranged attacks not to use Strength, so that weapon range has deterministic mechanical meaning.
24. As a player, I want weapon skill, damage rating, range, and tags to be validated, so that LLM-created weapons remain within Fudge-compatible bounds.
25. As a player, I want armor to be explicitly equipped and automatically used in attacks, so that defensive equipment is deterministic without requiring the LLM to repeat it every time.
26. As a player, I want attacks against incapacitated or near-death defenders to become unopposed against Poor defense, so that story-element combat handles helpless targets.
27. As a game host, I want attacks against dead characters rejected, so that dead is a terminal state.
28. As a player, I want Fudge wound boxes to accumulate and escalate, so that repeated scratches and wounds have the intended Fudge consequences.
29. As a player, I want manual healing to reduce wound levels, so that recovery can be narrated without introducing time-based healing rules.
30. As a player, I want to earn experience points and raise skills, attributes, or gifts, so that campaign progression is deterministic and useful.
31. As a game host, I want advancement costs hardcoded, so that the LLM cannot invent cheaper upgrades.
32. As a player, I want to create generic items, weapons, and armor at runtime, so that the LLM can introduce objects discovered during play.
33. As a game host, I want generated item slugs globally unique and immutable, so that item references remain stable.
34. As a player, I want all items to have counts and be transferable between characters, containers, and locations, so that inventory state can model ordinary and unique objects.
35. As a player, I want locations and containers created implicitly when first referenced, so that the LLM can establish the world organically.
36. As a player, I want to move characters explicitly, so that location state is not changed by narration alone.
37. As a player, I want to add and remove coins through validated tools, so that purchases and rewards are stateful.
38. As a game host, I want read-only state tools and a compact state summary, so that the LLM can make informed decisions while the player sees only narration.
39. As a game host, I want tool failures to be atomic and machine-readable, so that the LLM can retry invalid proposals without corrupting state.
40. As a game host, I want a world snapshot in rec-file format, so that a session can be saved and resumed using the same external ruleset.

## Implementation Decisions

- The existing `rules` package remains the deterministic domain model. No separate CLI-driven rules engine is introduced.
- The `World` is created with a seed and a ruleset path/slug. The selected ruleset is loaded and validated before play.
- The ruleset folder contains six recognized rec files: `attributes.rec`, `skills.rec`, `gifts.rec`, `faults.rec`, `powers.rec`, and `species.rec`.
- `attributes.rec` is the only mandatory ruleset file. The other files may be absent until setup creates them. Character creation still requires a species definition and a species slug.
- Attribute, skill, gift, fault, and power records require `slug`, `display_name`, and `description`. Species records additionally require `scale`.
- Slugs are lowercase ASCII words separated by single hyphens and must match `[a-z0-9]+(?:-[a-z0-9]+)*`. They are unique within their definition type and are referenced exactly.
- Species scale is constrained to `-10..+10` and is read from the character’s species during combat. Character scale is not copied into independent mutable state.
- Gifts, faults, and powers are descriptive only. Go does not infer mechanical effects from their prose. Species is the explicit exception because its scale field is mechanical.
- Setup uses a `write_ruleset_file` tool restricted to the selected ruleset folder and the six recognized filenames. It supports `replace` and `append` modes. Each write validates path safety and rec syntax; complete schema validation occurs when the first `create_character` call freezes the ruleset.
- The local `recfile` package is authoritative. Multi-record files use `%rec` categories and `ReadMulti`/`WriteMulti`; typed values are interpreted by the Go schema. Ruleset and world persistence use rec-file data.
- LLM tool calls use the JSON envelope required by `github.com/memmaker/gollum/llm`. Tool arguments are JSON, while tool result content is rec-file text inside the JSON message content.
- `rules` owns the LLM tool definitions and dispatch. Tool schemas describe operations, while runtime validation checks the selected ruleset and current World.
- Multiple sequential tool calls are allowed for one player input, ending when the LLM returns ordinary narration. A maximum of eight tool calls prevents loops; there is no `action_complete` tool.
- Every tool call is atomic. Go validates all arguments and preconditions before mutating state, consuming RNG, or charging a resource. Errors leave the World unchanged.
- General unopposed checks accept `trait_type`, `trait_slug`, `difficulty`, `situational_modifier`, `add_one`, and `reroll` as appropriate. Traits must be configured attributes or skills. Difficulty is any integer; general modifiers are `-4..+4`.
- Opposed checks accept independent actor and opponent slugs, trait types, trait slugs, and modifiers. Only the actor can spend a Fudge Point. Ties are not wins. Configured but unassigned skills default to Poor; configured but unassigned attributes default to Fair.
- All checks use 4dF. Natural `+4` on a successful check reports critical success; natural `-4` on an unsuccessful check reports critical failure. Critical flags do not independently mutate state.
- A reroll is a repeated check call with `reroll: true`; it charges one Fudge Point and returns only the new roll. `add_one: true` charges one Fudge Point before rolling and adds one to the die result.
- Fudge Point add/remove operations mutate the character balance. Removal cannot reduce the balance below zero. The LLM handles coincidence narration separately and uses the removal tool; there is no coincidence domain concept.
- Character creation is subjective. `create_character` accepts a generated character slug from the display name, description, required species, attributes, skills, gifts, faults, and initial location details. Slug collisions reject creation rather than adding suffixes.
- Character levels are constrained to `-3..+3`; Legendary levels are unavailable during creation and advancement. Initial resources are zero Fudge Points, zero EP, zero coins, undamaged, empty inventory, and no equipped armor.
- `make_player` sets one character as the sole player; any previous player becomes an NPC.
- Story-element combat uses `resolve_attack` with `attacker_slug`, `defender_slug`, optional `weapon_slug`, `attack_skill_slug` for unarmed attacks, `defense_skill`, `distance`, `attacker_modifier`, `defender_modifier`, and `reduce_damage`.
- A weapon’s `skill_used` must reference an available skill slug. Weapons have `slug`, `display_name`, `skill_used`, `damage_rating`, `range_in_meters`, `tags`, and `count`. Melee is represented by range zero; ranged attacks require integer `distance` within the weapon range. Weapon damage rating is constrained to `0..20`.
- Armor has `slug`, `display_name`, `armor_rating`, `tags`, and `count`. Armor rating is constrained to `0..10`. Tags are short, non-mechanical prose.
- Unarmed attacks use the supplied attack skill and damage rating zero. Melee looks up the fixed `strength` attribute slug and contributes zero if it is absent. Ranged attacks do not use Strength.
- Attack damage is based on relative degree plus offensive factors minus defensive factors: melee includes attacker Strength, ranged does not; attacker and defender scale and weapon/armor ratings are applied as agreed. An attacker must win and achieve at least Poor (`-2`) to deal damage. There is no relative-degree-one grazing cap.
- Attacks against incapacitated or near-death defenders become unopposed against fixed Poor defense. Attacks against dead defenders are rejected. Attacks against dead actors and all actions by dead characters are rejected.
- `reduce_damage` charges one Fudge Point only when the attack would otherwise apply a wound. It reduces the computed wound severity by one step before wound-box application: Near Death to Incapacitated, Incapacitated to Very Hurt, Very Hurt to Hurt, Hurt to Scratch, and Scratch to Undamaged.
- Wounds use SRD wound boxes: three Scratches, one Hurt, one Very Hurt, Incapacitated, Near Death, and Dead. Additional wounds escalate to the next available severity, and only the highest wound penalty applies. Additional damage to Near Death becomes Dead.
- `heal_character` accepts a positive number of levels and applies healing up to the current wound state. Dead characters cannot be healed. No time-based or skill-based healing is included initially.
- `declare_death` accepts any character and a required reason. Dead is terminal: no healing, actions, or resurrection.
- Advancement tracks EP in Go. `add_experience_points` accepts positive integers. `raise_skill` raises one skill level and uses the accepted Normal cost table; `raise_attribute` raises one attribute level for a fixed 10 EP; `buy_gift` buys one configured gift for 10 EP. Advancement cannot exceed Superb and spends EP internally and atomically.
- Runtime item creation uses `create_item`, `create_weapon`, and `create_armor`. Creation requires an owner type and owner slug; there are no unassigned items. Item slugs are globally unique and cannot be redefined. Counts are positive integers and the definition is shared by all units of that slug.
- Item owners and destinations are typed as `character`, `container`, or `location`. `transfer_item` requires a valid source containing the requested quantity and a destination. Missing locations and containers are created with slug, display name, and description when first referenced. Containers hold item counts only and have no capacity or nesting mechanics.
- `equip_armor` and `unequip_armor` are explicit mutations. A character has at most one equipped armor item; equipping replaces previous armor. Resolve attack automatically reads equipped armor. Weapons are never explicitly equipped and remain inventory items.
- `move_character` requires a character slug and location slug, display name, and description; missing locations are created. Characters have one current location.
- Currency uses separate `add_coins` and `remove_coins` tools with positive integer amounts. Removal rejects insufficient funds.
- NPC reaction and NPC morale are read-only deterministic tools. Both use 4dF and validated situational modifiers. Reaction uses the SRD `-4..+4` scale; morale maps `-3 or less` through `+3 or more` to surrender/flee through fanatic.
- No deterministic model is added for doors, switches, quests, clues, or other narrative facts. The LLM tracks these in narration and context.
- Read-only tools include `get_character`, `get_inventory`, `get_location`, `get_world_entities`, and `get_ruleset_definition`. The LLM sees complete deterministic state, including hidden NPC state; the player sees narration only.
- World snapshots use one rec file with multiple record categories. Snapshot metadata includes `ruleset_slug`, `seed`, and `player_slug`. Snapshot-only persistence is required; RNG position and action-log replay after reload are explicitly out of scope.
- No turn, phase, or time state is introduced initially.

## Testing Decisions

- The primary seam is the rules-owned LLM tool dispatcher operating against a `World`. Tests should submit tool calls and inspect rec-file results plus observable World state, rather than test private helpers or implementation details.
- Existing deterministic check tests remain prior art for same-seed behavior. Extend them through the tool dispatcher for unopposed, opposed, critical, modifier, default-trait, reroll, and Fudge Point behavior.
- Existing wound tests are prior art for wound behavior. Add external-behavior tests for scratch accumulation, escalation, penalties, healing, Near Death to Dead, terminal death, and damage reduction.
- Ruleset tests should load complete and incomplete ruleset folders, validate required fields and slugs, accept missing optional files during setup, reject malformed records at freeze, enforce species scale and trait bounds, and reject writes after freeze.
- Combat tests should cover melee and ranged damage, Strength inclusion/exclusion, scale, weapon and armor ratings, range validation, Poor hit threshold, unopposed helpless defenders, dead-target rejection, armor equipment, and atomic Fudge Point spending.
- Character and progression tests should cover slug generation/collisions, subjective defaults, species requirements, player designation, EP costs, one-level advancement, gift purchase, and maximum levels.
- Inventory tests should cover globally unique item slugs, counts, typed owners, transfers, insufficient quantities, implicit locations/containers, equipment replacement, and atomic failures.
- Snapshot tests should round-trip the single multi-category rec file and preserve observable state, ruleset slug, seed, and player slug. They should not claim replay equivalence after reload because RNG position persistence is out of scope.
- LLM adapter tests should verify JSON tool-call envelopes, rec-file tool result content, sequential tool calls, the eight-call safety cap, normal narration termination, and no mutation on rejected calls.
- Tests should assert externally visible values and state transitions, not the order of internal helper calls or concrete data structures.

## Out of Scope

- Implementing every optional Fudge SRD variant.
- Alternative dice methods, percentile resolution, 1d6 Fudge, or LLM-selected dice.
- Initiative, alternating turns, simultaneous rounds, tactical combat postures, hit locations, damage die options, ranged weapon special cases, and detailed armor coverage.
- Magic, psi, miracles, superpowers, or species abilities as deterministic mechanics. Powers and most traits remain descriptive only.
- Objective character creation, point-buy budgets, templates, and character creation trading rules.
- Natural healing, healing checks, downtime, elapsed time, turns, phases, and time-based death.
- Resurrection, non-terminal death, or recovery from Dead.
- Deterministic quests, doors, switches, clues, world facts, or arbitrary narrative state.
- Item crafting, item redefinition, nested containers, capacities, access restrictions, or equipment systems beyond one explicit armor slot.
- Replay after snapshot reload, RNG-state serialization, and action-log reconstruction.
- Player-visible state APIs; the player receives LLM narration rather than direct engine state.
- GitHub issue publication until the repository is associated with an issue tracker and the target repository/triage vocabulary is provided.

## Further Notes

- The current code has only the minimal check and wound foundations, so this is a substantial engine expansion rather than a small tool addition.
- The local rec-file implementation is currently an untracked addition in the working tree and should be reviewed and tested before it becomes the persistence boundary.
- The proposed combat damage calculation should be implemented as one authoritative operation; do not allow the LLM to separately invent hit, damage, or wound transitions.
- The issue tracker could not be identified: the repository has no Git remote or tracker configuration, although GitHub CLI authentication is available. The spec was therefore written locally and was not published or labeled.
