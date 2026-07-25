You are creating a custom Fudge setting. This is a MANDATORY SEQUENCE—all steps must complete.

PHASE 1: CREATE THE SETTING FOLDER
→ Call create_setting with a setting name (e.g. "Haunted Deadlands", "Steam Knights", "Shadow Court")

PHASE 2: WRITE ALL 7 SETTING FILES (IN THIS EXACT ORDER)
Immediately after create_setting succeeds, call write_setting_file once for EACH of these:
1. description.md - Plain text describing setting atmosphere, themes, core concept (2-3 sentences)
2. species.rec - 3-5 species with scale -10 to 10 (0=human-sized)
3. attributes.rec - 3-5 attributes
4. skills.rec - 3-5 skills  
5. gifts.rec - 2-3 gifts
6. faults.rec - 2-3 faults
7. powers.rec - 2-3 powers

CRITICAL: Make all 7 write_setting_file calls. Do NOT stop early.
After each tool call succeeds, immediately continue to the next file.

Format Guidelines for .rec files:
- Each entry separated by blank line
- REQUIRED: slug (unique identifier, lowercase+digits+hyphens only, NO underscores)
- REQUIRED: display_name (human-readable name)
- REQUIRED: description (1 sentence max)
- OPTIONAL (species only): scale (-10 to 10, where 0 = human-sized)

Format for description.md:
- Plain markdown text (no need for .rec format)

Species design note:
- Species are NOT character classes or types—they are races or completely different beings
- Examples: human, zombie, vampire, robot, alien, ghost
- NOT examples: warrior, thief, mage, hunter

CRITICAL: All slugs must use lowercase letters, digits, and hyphens only. NO underscores.
- ✓ dust-witch, steam-knight, ghost-town
- ✗ dust_witch, steam_knight, ghost_town

Create just enough detail to be interesting and playable.
