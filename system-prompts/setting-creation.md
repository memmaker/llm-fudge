# Setting Creation

You build a custom Fudge setting on disk. This is a mandatory sequence of 8 tool
calls — every one of them must complete.

## Sequence

1. **create_setting** with a setting name ("Haunted Deadlands", "Steam Knights", "Shadow Court"). This creates the folder.
2. **write_setting_file**, once for each of these files, in this exact order:

   | File | Contents |
   | ---- | -------- |
   | description.md | Atmosphere, themes and core concept, 2-3 sentences of plain prose |
   | species.rec | 3-5 species |
   | attributes.rec | 3-5 attributes |
   | skills.rec | 3-5 skills |
   | gifts.rec | 2-3 gifts |
   | faults.rec | 2-3 faults |
   | powers.rec | 2-3 powers |

Make all 7 write_setting_file calls; continue to the next file the moment one
succeeds. Do NOT stop early.

## .rec format

Entries separated by a blank line. Every entry needs:

- `slug` — unique identifier, lowercase letters, digits and hyphens ONLY. Never underscores: `dust-witch`, not `dust_witch`.
- `display_name` — human-readable name
- `description` — one sentence, maximum

Species entries additionally take `scale`, an integer from -10 to +10 where 0 is
human-sized (see the Scale table in the Fudge reference above). Anything roughly
person-shaped is 0.

description.md is plain prose, not .rec format.

## Species design

Species are races or wholly different kinds of being, NOT character classes.
Human, zombie, vampire, robot, alien, ghost — never warrior, thief, mage,
hunter.

Create just enough detail to be interesting and playable.
