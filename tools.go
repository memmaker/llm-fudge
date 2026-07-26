package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fudge/rules"
	"fudge/ui"

	"github.com/memmaker/gollum/llm"
)

func settingCreationTools() []llm.Tool {
	return []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "create_setting", Description: "Initialize a new setting with a chosen name. Call this FIRST before writing any setting files. The name becomes the folder name and setting identifier (converted to lowercase, spaces to hyphens). Example: 'Haunted Deadlands' becomes 'haunted-deadlands'.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string", "description": "The setting name (will be slugified for folder)"}}, "required": []string{"name"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "write_setting_file", Description: "Write one setting file. CRITICAL SEQUENCE: After create_setting, call this tool once for EACH of these files in this order: (1) description.md, (2) species.rec, (3) attributes.rec, (4) skills.rec, (5) gifts.rec, (6) faults.rec, (7) powers.rec.\n\nFor .rec files: Format each entry separated by blank line. REQUIRED FIELDS per entry: slug (lowercase+digits+hyphens only, NO underscores), display_name (human-readable name), description (1 sentence). OPTIONAL: scale (species only; integer -10 to +10 where 0 is human-sized, so anything roughly person-shaped is 0). Slug format: use hyphens not underscores, e.g. \"dust-witch\" not \"dust_witch\". Example entry:\nslug: dust-witch\ndisplay_name: Dust Witch\ndescription: Occult wanderers channeling desert spirits.\nscale: 0\n\nFor description.md: Plain markdown text describing the setting's atmosphere, themes, and core concept (2-3 sentences).", Parameters: map[string]any{"type": "object", "properties": map[string]any{"filename": map[string]any{"type": "string"}, "mode": map[string]any{"type": "string", "enum": []string{"replace", "append"}}, "content": map[string]any{"type": "string"}}, "required": []string{"filename", "mode", "content"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "user_choice", Description: "Present the user with 2-4 mutually exclusive options. ONLY use when: (1) the player's action has exactly 2-4 meaningful, dramatically distinct outcomes; (2) you cannot narrate forward without knowing which they choose; (3) the moment is critical enough to warrant explicit choice. REQUIREMENTS: Write a clear prompt/question before calling this tool. The menu appears after your text. Never include a 'Type your answer' option—it's added automatically.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"options": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "List of 2-4 mutually exclusive options. Do NOT include a 'Type your answer' or freeform option—it is added automatically."}}, "required": []string{"options"}}}},
	}
}

func adventureCreationTools() []llm.Tool {
	return []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "write_adventure", Description: "Persist the improvised adventure to disk so it can be replayed later, and hand it to the DM as the briefing for this session. REQUIRED: call this exactly once, as your only output—do not write the adventure as plain text. It is saved under the current setting's adventures folder, named after the title.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"title": map[string]any{"type": "string", "description": "Short evocative adventure title (3-6 words), used as the filename"}, "content": map[string]any{"type": "string", "description": "The full adventure text: hook, situation, immediate stakes"}}, "required": []string{"title", "content"}}}},
	}
}

func characterCreationTools() []llm.Tool {
	tools := []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "create_character", Description: "Create a new character. The character ID is automatically generated from the name. Only use traits (attributes, skills, gifts, faults) that exist in the setting. Species is required.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}, "species": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}, "location": map[string]any{"type": "string"}, "attributes": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, "skills": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, "gifts": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "faults": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, "required": []string{"name", "species"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "user_choice", Description: "Present the user with 2-4 mutually exclusive options. ONLY use when: (1) the player's action has exactly 2-4 meaningful, dramatically distinct outcomes; (2) you cannot narrate forward without knowing which they choose; (3) the moment is critical enough to warrant explicit choice. REQUIREMENTS: Write a clear prompt/question before calling this tool. The menu appears after your text. Never include a 'Type your answer' option—it's added automatically.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"options": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "List of 2-4 mutually exclusive options. Do NOT include a 'Type your answer' or freeform option—it is added automatically."}}, "required": []string{"options"}}}},
	}
	return append(tools, inventoryTools()...)
}

// inventoryTools are shared by character creation and play: the same calls outfit
// a starting character, hand out loot and settle a bar tab. Weapons and armor go
// on through their own calls because they carry ratings combat reads — a sword
// added as plain gear cannot be swung.
func inventoryTools() []llm.Tool {
	characterID := map[string]any{"type": "string", "description": "Whose possessions change. Omit it to mean the player character—during character creation their id does not exist yet."}
	return []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "add_weapon", Description: "Give a character a weapon. Use this instead of add_item for anything meant to be attacked with—the damage rating you set here is what the attack tools do their arithmetic with, and a blade recorded as plain gear cannot be swung at all. Rate the weapon by what it is (Fudge damage factors): -1 bare fist, +0 a knife or small club, +1 a sword or hand axe, +2 a large one-handed or light two-handed weapon, +3 a heavy two-handed weapon, +1 more for a genuinely sharp edge. Firearms: +0 to +1 derringer, +2 to +3 handgun, +5 or more for a powerful two-hander, +10 for a bazooka. Set damage_attribute for anything driven by muscle (sword, axe, bow, thrown spear) so the wielder's own strength counts; leave it out for anything with its own power source, where the weapon hits the same however weak the hand holding it.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": characterID, "name": map[string]any{"type": "string", "description": "The weapon's name as the player will read it, e.g. \"Blaster Pistol\"."}, "description": map[string]any{"type": "string", "description": "One short concrete sentence describing this particular weapon."}, "damage": map[string]any{"type": "integer", "description": "Damage rating: the weapon's own contribution to a hit. See the guidance above."}, "ranged": map[string]any{"type": "boolean", "description": "True for anything fired or shot from a distance (gun, bow, crossbow). False for hand-to-hand weapons. Defaults to false."}, "damage_attribute": map[string]any{"type": "string", "description": "Slug of the attribute added to damage for muscle-powered weapons—the setting's strength-like attribute. Omit for firearms, energy weapons and anything else that does not depend on the wielder's build."}, "ammo_capacity": map[string]any{"type": "integer", "description": "How many shots it holds when full. Ranged only. Omit or 0 to not count shots at all, which is fine for a bow whose arrows you would rather not tally."}, "ammo": map[string]any{"type": "integer", "description": "How many shots are in it right now. Defaults to full."}, "ammo_item": map[string]any{"type": "string", "description": "The name of the item a reload spends, e.g. \"Power Cell\". Add that item separately with add_item—reloading fails when they carry none, and a character with a full magazine and no spares is one firefight from empty."}, "count": map[string]any{"type": "integer", "description": "How many are gained. Defaults to 1."}}, "required": []string{"name", "description", "damage"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "add_armor", Description: "Give a character armor. Use this instead of add_item for anything worn as protection—the rating you set here is subtracted from every blow that lands on them. Rate it by what it is made of: +1 light non-metal (padded, leather), +2 heavy non-metal or light metal (mail shirt, flak vest), +3 medium metal, +4 heavy metal (full plate), +5 or more for powered or science-fiction armor. Armor goes on worn: it protects from the moment you record it. Pass worn false for a piece being hauled rather than used—salvage in a pack, a breastplate looted off a body and not yet put on.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": characterID, "name": map[string]any{"type": "string", "description": "The armor's name as the player will read it, e.g. \"Flak Vest\"."}, "description": map[string]any{"type": "string", "description": "One short concrete sentence describing this particular piece."}, "protection": map[string]any{"type": "integer", "description": "Protection rating: how much it takes off a blow. See the guidance above."}, "worn": map[string]any{"type": "boolean", "description": "Whether it is being worn. Defaults to true. Only worn armor protects, and only the single best worn piece counts—layering does not add up."}, "count": map[string]any{"type": "integer", "description": "How many are gained. Defaults to 1."}}, "required": []string{"name", "description", "protection"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "equip_armor", Description: "Put armor on or take it off. Armor is already worn when you add it, so this is for the moments the fiction turns on it: stripping a corpse and shrugging into what it wore, peeling off a soaked coat, being stripped of a shield by captors. Armor that is not worn protects nobody.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": characterID, "name": map[string]any{"type": "string", "description": "The armor's name as it was added."}, "worn": map[string]any{"type": "boolean", "description": "True to put it on, false to take it off."}}, "required": []string{"name", "worn"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "add_item", Description: "Give a character an item. Call it whenever something enters their hands: starting equipment at creation, loot, a purchase, a gift, salvage. An item they already carry stacks onto the entry they have rather than opening a second one, so use the same name you used before. The description is what the player reads on their sheet, so write one concrete line about this particular thing—what it is, what shape it is in, where it came from—not a catalogue entry. Items are recorded here and nowhere else: an object you only narrate is not carried.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": characterID, "name": map[string]any{"type": "string", "description": "The item's name as the player will read it, e.g. \"Blaster Pistol\". Name the thing itself, without the count."}, "description": map[string]any{"type": "string", "description": "One short concrete sentence describing this item."}, "count": map[string]any{"type": "integer", "description": "How many are gained. Defaults to 1."}}, "required": []string{"name", "description"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "remove_item", Description: "Take an item away from a character: spent, sold, given away, stolen, broken, left behind. Use the name it is carried under. Removing the last of a stack drops it from the sheet entirely. The call fails if they do not carry that many—read the error and narrate what they actually have rather than insisting.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": characterID, "name": map[string]any{"type": "string", "description": "The item's name as it was added."}, "count": map[string]any{"type": "integer", "description": "How many are lost. Defaults to 1."}}, "required": []string{"name"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "add_coins", Description: "Credit a character's purse: starting money at creation, a reward, a sale, found treasure. Coins are the setting's ordinary currency, whatever it is called there.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": characterID, "amount": map[string]any{"type": "integer", "description": "How many coins are gained. Must be positive."}}, "required": []string{"amount"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "remove_coins", Description: "Debit a character's purse: a purchase, a bribe, a toll, a theft. A purse never goes negative, so the call fails when they cannot afford it—when it does, they cannot pay, and the scene goes differently.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": characterID, "amount": map[string]any{"type": "integer", "description": "How many coins are spent. Must be positive."}}, "required": []string{"amount"}}}},
	}
}

// combatTools resolve a blow from the roll through to the wound in one call.
// Everything they need is already on the sheets — the weapon's rating, the arm
// behind it, the armor in the way, both Scales — so the DM names the traits and
// the circumstances and the engine does the rest, rather than rolling a check and
// then guessing at a severity.
func combatTools() []llm.Tool {
	attackerID := map[string]any{"type": "string", "description": "Who is attacking, by character id."}
	traitType := map[string]any{"type": "string", "enum": []string{"attribute", "skill"}}
	weapon := map[string]any{"type": "string", "description": "The weapon being used, by the name it is carried under. It must be on their sheet as a weapon (add_weapon); the call fails if they do not carry it or it is out of ammunition, and that failure is the scene—narrate the empty chamber, do not retry without it. Omit for a bare-handed attack."}
	thrown := map[string]any{"type": "boolean", "description": "True when the weapon is thrown rather than swung or fired: it leaves their hand and comes off the sheet. Pick a throwing skill as the trait. They can pick it up again afterwards with add_weapon."}
	addOne := map[string]any{"type": "boolean", "description": "Spend one of the attacker's Fudge Points for +1 on the roll."}
	reroll := map[string]any{"type": "boolean", "description": "Spend one of the attacker's Fudge Points to roll twice and keep the better roll. Asking for this and add_one together spends two."}
	modifier := map[string]any{"type": "integer", "description": "Circumstances, roughly -4 to +4, 0 being the common case: position, footing, light, surprise, reach, being outnumbered (about -1 per extra foe), a called shot, all-out attack or defence. Do NOT include wound penalties—the engine already applies those."}

	return []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "opposed_attack", Description: "Strike at someone who is fighting back—the ordinary shape of Fudge combat. This is one call, not three: it rolls the attacker's offence against the defence you name, decides whether the blow landed, works the weapon's damage and the defender's armor and both Scales into a wound, and applies it. Never follow it with apply_damage; never work out damage yourself. CRITICAL SEQUENCE: if the opponent does not exist yet, call create_character first in the same response, then attack. Choose the defender's trait for how they are actually defending—a parry or weapon skill, a dodge, a shield. The result tells you the whole story: a standoff where neither got through, a graze that only nicked them, a hit stopped dead by armor, or a wound. Narrate exactly what comes back and nothing more.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"attacker_id": attackerID, "attacker_trait_type": traitType, "attacker_trait_slug": map[string]any{"type": "string", "description": "The skill or attribute they attack with."}, "attacker_modifier": modifier, "defender_id": map[string]any{"type": "string", "description": "Who is being attacked, by character id."}, "defender_trait_type": traitType, "defender_trait_slug": map[string]any{"type": "string", "description": "The skill or attribute they defend with—parry, dodge, shield, or the weapon skill itself."}, "defender_modifier": modifier, "weapon": weapon, "thrown": thrown, "add_one": addOne}, "required": []string{"attacker_id", "attacker_trait_type", "attacker_trait_slug", "defender_id", "defender_trait_type", "defender_trait_slug"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "unopposed_attack", Description: "Strike at someone who is not resisting: unaware of the attacker, asleep, bound, cornered, already down, or shot at from cover they have not spotted. Same one-call resolution as opposed_attack—roll, hit, damage, wound—but against a difficulty instead of a defence. Leave difficulty out and it is Poor, which is what an undefended target usually is; raise it for a hard shot at distance, in bad light, at a moving target. There is no defence roll, so the margin does the work of the relative degree and these attacks are correspondingly brutal: that is what an ambush is worth. Use opposed_attack the moment the target can do anything at all about it.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"attacker_id": attackerID, "attacker_trait_type": traitType, "attacker_trait_slug": map[string]any{"type": "string", "description": "The skill or attribute they attack with."}, "attacker_modifier": modifier, "target_id": map[string]any{"type": "string", "description": "Who is being attacked, by character id."}, "difficulty": map[string]any{"type": "string", "enum": rules.DifficultyNames(), "description": "How hard the shot or blow is. Defaults to very easy (Poor), the standard for a target that cannot respond."}, "weapon": weapon, "thrown": thrown, "add_one": addOne, "reroll": reroll}, "required": []string{"attacker_id", "attacker_trait_type", "attacker_trait_slug", "target_id"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "reload_weapon", Description: "Reload a ranged weapon, spending one of the ammunition items it feeds on. Every shot fired counts down the magazine—hits and misses alike—and an empty weapon simply will not fire until this is called. It costs the character their action in the fiction, so narrate the moment: a fumbled magazine mid-firefight is a scene, not bookkeeping. It fails when they carry no ammunition left, and then they are out for good and must find more, take a weapon off someone, or fight with something else.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": map[string]any{"type": "string", "description": "Whose weapon is reloaded. Omit to mean the player character."}, "weapon": map[string]any{"type": "string", "description": "The weapon's name as it is carried."}}, "required": []string{"weapon"}}}},
	}
}

// narrationTool is the one call a turn cannot end without, so it is built on its
// own: a turn that has spent every resolution round it is allowed still has to be
// narrated, and it is offered this and nothing else.
func narrationTool() llm.Tool {
	// Under --img-gen the scene's image prompt rides along as a third argument of
	// output_narration rather than a tool of its own: a separate tool is a
	// separate round-trip, and the DM ended its turn with the narration instead of
	// ever getting there. Required, so every narrated scene produces one.
	narrationDescription := "Show the turn's narration to the player. This is the ONLY channel to them: they read the prose argument and nothing else—no dice, no tool names, no tool results, and no text you write outside this call. Text written as a plain reply reaches nobody. Every turn ends with exactly one output_narration, and the turn is not over until you call it. Call it only once you know what happened: if a check, a new NPC, damage or healing decides the outcome, make those calls first with no narration, read the results when they come back, and narrate in your next response. It must be your LAST tool call, after every state-changing call, so it can reflect their outcome. Argument prose: the narration the player reads. Argument world_update (REQUIRED): the facts your prose just established, merged into the WORLD ledger you can see in ENGINE STATE."
	narrationProperties := map[string]any{
		"prose":        map[string]any{"type": "string", "description": "The turn's narration exactly as the player will read it: normal, descriptive, engaging prose in full sentences. It must carry everything the tool calls established, since the player sees none of them. No titles or speaker prefixes (no \"DM:\"), no meta-commentary, no mention of dice, difficulty or mechanics, and no \"What do you do?\"—narrate and stop."},
		"world_update": map[string]any{"type": "string", "description": "Required, never empty. The facts your prose just established, keyed by entity: `catnip|at:cellar|holding:dagger; door|state:locked`. Merged into the WORLD ledger field by field, so restating something unchanged is harmless—write anything you are unsure of rather than gambling that it was recorded. Use `!fact` to retract one (`combat|!engaged|over`) and `entity|GONE` to drop an entry. Wounds, Fudge Points, coins, possessions, traits and ids belong to the engine, which renders them in ROSTER and ignores them here; where a character is standing is yours. Prefer `key:value` over bare words, since a keyed fact overwrites cleanly next time it moves. First call of a game: full baseline of the opening scene—NPCs, location, features, hooks—since nothing is recorded yet."},
	}
	narrationRequired := []string{"prose", "world_update"}

	if imgGen {
		narrationDescription += " Argument img_gen (REQUIRED): the prompt for an image generation AI depicting the scene this prose just narrated. Write it for every narration—it is not optional and not something to save for dramatic moments."
		narrationProperties["img_gen"] = map[string]any{"type": "string", "description": "Prompt for an image generation AI depicting the scene the prose narrates. Self-contained: the image AI sees only this text, none of your context. Describe the location and its lighting and atmosphere in detail; then EVERY character present, each with their clothing, appearance, where they stand in the scene, their pose, and what they are doing; then the state and items that show—wounds, drawn weapons, carried or worn gear, doors open or shut, fire, debris. Plain visual description, present tense, no story, no names the picture cannot show."}
		narrationRequired = append(narrationRequired, "img_gen")
	}

	return llm.Tool{Type: "function", Function: llm.ToolFunction{Name: "output_narration", Description: narrationDescription, Parameters: map[string]any{"type": "object", "properties": narrationProperties, "required": narrationRequired}}}
}

// narrationOnlyTools is the tool set a turn ends on. Withholding everything else
// is what makes the ending certain: there is no call left to make except the one
// that hands the player their turn.
func narrationOnlyTools() []llm.Tool {
	return []llm.Tool{narrationTool()}
}

func gameplayTools() []llm.Tool {
	tools := []llm.Tool{
		narrationTool(),
		{Type: "function", Function: llm.ToolFunction{Name: "unopposed_check", Description: "Roll an action that nothing is actively resisting: climbing a wall, picking a lock, recalling lore, crafting. Use it whenever the outcome is uncertain and no opponent opposes it—if one does, use opposed_check instead. Skip it entirely for trivial actions with no real chance of failure and no stakes; just narrate those. You name the difficulty and the engine rolls; narrate only what comes back, and never invent a roll or adjust the result. situational_modifier is roughly -4 to +4 for circumstances (terrain, lighting, preparation, exhaustion, poor tools); 0 should be the common case and ±3/±4 is for genuinely extreme conditions. add_one and reroll each spend one of the character's Fudge Points, and asking for both spends two.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"actor_id": map[string]any{"type": "string"}, "trait_type": map[string]any{"type": "string", "enum": []string{"attribute", "skill"}}, "trait_slug": map[string]any{"type": "string"}, "difficulty": map[string]any{"type": "string", "enum": rules.DifficultyNames(), "description": "How hard the task is. The engine converts this to a target number."}, "situational_modifier": map[string]any{"type": "integer"}, "add_one": map[string]any{"type": "boolean", "description": "Spend one Fudge Point for +1 on the roll."}, "reroll": map[string]any{"type": "boolean", "description": "Spend one Fudge Point to roll twice and keep the better roll. The result names the roll that was thrown away, which is worth narrating—the slip that almost happened, the grip that almost went."}}, "required": []string{"actor_id", "trait_type", "trait_slug", "difficulty"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "opposed_check", Description: "Resolve a direct conflict between two characters that is not a blow being struck: a grapple, a chase, a deception, a stare-down, resisting a spell, wrestling for a dropped gun. Use when two characters have conflicting intentions. For an actual attack use opposed_attack instead—it settles the roll, the weapon, the armor and the wound in one call, and this tool never wounds anybody. CRITICAL SEQUENCE: (1) If an NPC doesn't exist yet, call create_character FIRST in the same response; (2) THEN immediately call opposed_check with both actor_id and opponent_id (use the character's actual ID). actor_id is whoever initiated the action; opponent_id is whoever reacts. Do NOT narrate the defense separately—the check outcome tells the story. Narrate using only the tool result.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"actor_id": map[string]any{"type": "string"}, "actor_trait_type": map[string]any{"type": "string", "enum": []string{"attribute", "skill"}}, "actor_trait_slug": map[string]any{"type": "string"}, "actor_modifier": map[string]any{"type": "integer"}, "opponent_id": map[string]any{"type": "string"}, "opponent_trait_type": map[string]any{"type": "string", "enum": []string{"attribute", "skill"}}, "opponent_trait_slug": map[string]any{"type": "string"}, "opponent_modifier": map[string]any{"type": "integer"}, "add_one": map[string]any{"type": "boolean"}}, "required": []string{"actor_id", "actor_trait_type", "actor_trait_slug", "opponent_id", "opponent_trait_type", "opponent_trait_slug"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "create_character", Description: "Create a new NPC. CRITICAL: call this BEFORE any check involving that NPC—the character must exist before you can make a check against them. If a player targets an NPC for the first time, create them immediately, THEN call the appropriate check (opposed_check or unopposed_check) in the same response. The character ID is generated from the name and comes back in the reply as its id: field—that is the handle every later call to this character needs, so read the id off it instead of deriving one from the name. It is in the ROSTER from the next turn onward, for as long as they exist. Only use traits (attributes, skills, gifts, faults) listed in the Setting Information section of your system prompt. Species is required.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}, "species": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}, "location": map[string]any{"type": "string"}, "attributes": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, "skills": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, "gifts": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "faults": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, "required": []string{"name", "species"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "apply_damage", Description: "Wound a character by something no attack roll settles: a fall, a fire, an explosion, poison, drowning, a collapsing floor, an execution. NOT for blows struck in a fight—opposed_attack and unopposed_attack wound the target themselves, and calling this after one of them wounds the same character twice over. You name the wound the fiction calls for and the engine does the arithmetic: it accumulates scratches, escalates wound levels and applies the resulting penalty. Death is never a side effect: characters stop at near death and are killed only by an explicit declare_dead.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": map[string]any{"type": "string"}, "severity": map[string]any{"type": "string", "enum": rules.SeverityNames(), "description": "The wound this hit inflicts on an undamaged target. Existing wounds escalate from there."}}, "required": []string{"character_id", "severity"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "roll_on_table", Description: "Roll on a random table when the fiction needs an outcome that no character is choosing. Use it instead of deciding these yourself—that is the only way they are genuinely random. npc-reaction: how an NPC you have not characterized yet feels about the party on first meeting. npc-morale: whether an NPC presses on, wavers or breaks, checked when a fight turns against them. weather: conditions on the road or at the start of a day outdoors. situational-roll: how luck breaks when there is no actor to check at all. The modifier is roughly -4 to +4 for whatever the established fiction says should tilt it (a reputation, a bribe, the season, a rout already in progress); 0 is the common case. Narrate the result the table gives you.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"table": map[string]any{"type": "string", "enum": rules.TableNames()}, "modifier": map[string]any{"type": "integer"}}, "required": []string{"table"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "heal", Description: "Heal a character by reducing wound levels. Cannot heal the dead.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": map[string]any{"type": "string"}, "levels": map[string]any{"type": "integer"}}, "required": []string{"character_id", "levels"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "declare_dead", Description: "Permanently declare a character dead. Dead characters cannot act or be healed.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": map[string]any{"type": "string"}}, "required": []string{"character_id"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "recall", Description: "Look up a turn from earlier in this session. Everything ever played is kept—what the player typed, what the engine resolved, what you narrated word for word—but only the last few turns ride in ENGINE STATE, so this is how you reach the rest. Use it when you need something you cannot see: what an NPC actually said, the exact terms of a deal, what a room looked like the first time. Every WORLD entry carries the turn it was last set (@t12), so to learn how a fact came about, recall that turn. Otherwise search by keyword. Ask for include_prose when you need the words themselves rather than what changed. This costs you one of the turn's calls like anything else, so spend it on something you will use—and if it comes back empty, then it did not happen: say so in the fiction rather than inventing it.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string", "description": "Words to search for across player input, recorded changes and narration. Omit when asking for a turn by number."}, "turn": map[string]any{"type": "integer", "description": "A turn number to fetch, e.g. the 12 from an @t12 marker in WORLD."}, "to": map[string]any{"type": "integer", "description": "End of a turn range, when you want several turns around one moment. Requires turn."}, "include_prose": map[string]any{"type": "boolean", "description": "Return the narration itself, not just what changed. Costly—ask only when the wording matters."}, "limit": map[string]any{"type": "integer", "description": "How many turns to return. Defaults to a few."}}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "user_choice", Description: "Present the player with 2-4 mutually exclusive options. ONLY use when: (1) the player's action has exactly 2-4 meaningful, dramatically distinct outcomes; (2) you cannot narrate forward without knowing which they choose; (3) the moment is critical enough to warrant explicit choice. For normal gameplay where the player types an action, just narrate the outcome. Do NOT use this as a generic \"What do you do?\" prompt. REQUIREMENTS: call output_narration in the same response, with prose that poses the question—the menu is shown underneath it. The player's answer comes back to you as this tool's result, and the turn continues: narrate what their choice leads to. Never include a 'Type your answer' option—it's added automatically.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"options": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "List of 2-4 mutually exclusive options. Do NOT include a 'Type your answer' or freeform option—it is added automatically."}}, "required": []string{"options"}}}},
	}

	tools = append(tools, combatTools()...)
	return append(tools, inventoryTools()...)
}

func loadSetting(w *rules.World, name string) error {
	path := filepath.Join("settings", name)
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("setting not found: %s", name)
	}

	setting, err := rules.LoadSetting(path)
	if err != nil {
		return fmt.Errorf("failed to load setting: %w", err)
	}

	w.Setting = setting

	return nil
}

func loadAdventureTool(w *rules.World, name string) (string, error) {
	if w.Setting == nil {
		return "", fmt.Errorf("no setting loaded")
	}

	adventurePath := filepath.Join(w.Setting.Path, "adventures", name+".md")
	content, err := os.ReadFile(adventurePath)
	if err != nil {
		return "", fmt.Errorf("failed to load adventure: %w", err)
	}

	return string(content), nil
}

func writeCharsheet(w *rules.World) error {
	if w.Player == "" {
		return nil
	}
	content := rules.StateFileText(w)
	return os.WriteFile("charsheet.txt", []byte(content), 0o644)
}

// writeImagePrompt persists the image prompt the DM attached to its narration.
// Each turn replaces the file: it describes the scene on screen right now, and
// whatever tool picks it up wants the current one.
func writeImagePrompt(prompt string) error {
	return os.WriteFile("img-gen.md", []byte(prompt), 0o644)
}

func getCharacterFromJSON(w *rules.World, jsonArgs string) (*rules.Character, map[string]interface{}, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(jsonArgs), &raw); err != nil {
		return nil, nil, fmt.Errorf("parse arguments: %w", err)
	}
	charID, ok := raw["character_id"].(string)
	if !ok {
		return nil, nil, fmt.Errorf("character_id required")
	}
	ch, err := w.Character(charID)
	if err != nil {
		return nil, nil, err
	}
	return ch, raw, nil
}

// targetCharacter resolves the character a call names, falling back to the
// player when character_id is left out: during character creation the model has
// not seen the generated id yet, and the character being outfitted is the
// player either way.
func targetCharacter(w *rules.World, id string) (*rules.Character, error) {
	if id == "" {
		if w.Player == "" {
			return nil, fmt.Errorf("character_id required: no player character exists yet")
		}
		id = w.Player
	}
	return w.Character(id)
}

// withState appends a character's condensed state to a tool result, but only
// when the call actually moved it. The DM reads the consequences of its own
// call off the engine rather than tracking them in its head — the fudge point a
// check just spent, the wound a hit escalated to, the item that came off the
// sheet — but a call that changed nothing has nothing to report: restating
// unchanged state burns context and leaves two copies to pick between. Silence
// means "as you were last told", the same convention world_update uses.
//
// before is the character's state as it stood ahead of the call. create_character
// takes no part in this: a character who did not exist has no unchanged state to
// suppress, so it answers with ToString and nothing else.
func withState(result, before string, ch *rules.Character) string {
	return withStates(result, []charSnapshot{{ch: ch, before: before}})
}

// charSnapshot is a character as they stood before a call, paired with the
// character themselves so the two can be compared once the call is done.
type charSnapshot struct {
	ch     *rules.Character
	before string
}

// snapshot records the characters a call is about to move. Order is kept: an
// attack reports the attacker first and the target second, the same way round
// every time, so the DM never has to work out which block is whose.
func snapshot(chs ...*rules.Character) []charSnapshot {
	snaps := make([]charSnapshot, 0, len(chs))
	for _, ch := range chs {
		snaps = append(snaps, charSnapshot{ch: ch, before: ch.ToString()})
	}
	return snaps
}

// withStates is withState for a call that can move more than one character — an
// attack moves both sides at once, the attacker's magazine and the target's
// wounds. Each is reported only if it actually moved.
func withStates(result string, snaps []charSnapshot) string {
	for _, snap := range snaps {
		if after := snap.ch.ToString(); after != snap.before {
			result += "\n" + after
		}
	}
	return result
}

func executeAction(w *rules.World, call llm.ToolCall) (string, error) {
	switch call.Function.Name {
	case "create_setting":
		var args struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse create_setting: %w", err)
		}
		if args.Name == "" {
			return "", fmt.Errorf("setting name required")
		}
		settingSlug := slugify(args.Name)
		settingPath := filepath.Join("settings", settingSlug)
		if err := os.MkdirAll(settingPath, 0o755); err != nil {
			return "", fmt.Errorf("failed to create setting directory: %w", err)
		}
		w.Setting = &rules.SettingInfos{
			Slug:       settingSlug,
			Path:       settingPath,
			Attributes: make(map[string]*rules.Attribute),
			Skills:     make(map[string]*rules.Skill),
			Species:    make(map[string]*rules.Species),
			Gifts:      make(map[string]*rules.Gift),
			Faults:     make(map[string]*rules.Fault),
			Powers:     make(map[string]*rules.Power),
		}
		return fmt.Sprintf("Setting '%s' created (folder: %s)", args.Name, settingSlug), nil
	case "user_choice":
		var args struct {
			Options []string `json:"options"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse user_choice: %w", err)
		}
		if len(args.Options) == 0 {
			return "", fmt.Errorf("user_choice requires at least one option")
		}
		choice, err := ui.ShowChoiceMenu(args.Options)
		if err != nil {
			return "", err
		}
		return choice, nil
	case "unopposed_check":
		var args struct {
			ActorID             string `json:"actor_id"`
			TraitType           string `json:"trait_type"`
			TraitSlug           string `json:"trait_slug"`
			Difficulty          string `json:"difficulty"`
			SituationalModifier int    `json:"situational_modifier"`
			AddOne              bool   `json:"add_one"`
			Reroll              bool   `json:"reroll"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse unopposed check: %w", err)
		}
		if args.ActorID == "" || args.TraitType == "" || args.TraitSlug == "" {
			return "", fmt.Errorf("unopposed check requires actor_id, trait_type, and trait_slug")
		}
		difficulty, err := rules.DifficultyLevel(args.Difficulty)
		if err != nil {
			return "", err
		}
		// Snapshotted ahead of the roll: a check that spends a Fudge Point
		// leaves the actor changed, one that does not leaves them untouched.
		actor, err := w.Character(args.ActorID)
		if err != nil {
			return "", err
		}
		before := actor.ToString()
		result, err := w.UnopposedCheck(args.ActorID, args.TraitType, args.TraitSlug, difficulty, args.SituationalModifier, args.AddOne, args.Reroll)
		if err != nil {
			return "", err
		}
		var critStr string
		if result.CritSuccess {
			critStr = "|CRIT"
		} else if result.CritFailure {
			critStr = "|FAIL"
		}
		successStr := "no"
		if result.Success {
			successStr = "ok"
		}
		// A rerolled check names the roll it threw away, so the DM can narrate the
		// Fudge Point doing something: the fumble that was, and then was not.
		rollStr := fmt.Sprintf("roll%+d", result.Roll)
		if result.Rerolled {
			rollStr += fmt.Sprintf("(rerolled%+d)", result.DiscardedRoll)
		}
		return withState(fmt.Sprintf("%s|%s|tot%d|%s(%+d)|%s%s", actor.Name, rollStr, result.Total, args.Difficulty, difficulty, successStr, critStr), before, actor), nil
	case "opposed_check":
		var args struct {
			ActorID           string `json:"actor_id"`
			ActorTraitType    string `json:"actor_trait_type"`
			ActorTraitSlug    string `json:"actor_trait_slug"`
			ActorModifier     int    `json:"actor_modifier"`
			OpponentID        string `json:"opponent_id"`
			OpponentTraitType string `json:"opponent_trait_type"`
			OpponentTraitSlug string `json:"opponent_trait_slug"`
			OpponentModifier  int    `json:"opponent_modifier"`
			AddOne            bool   `json:"add_one"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse opposed check: %w", err)
		}
		if args.ActorID == "" || args.ActorTraitType == "" || args.ActorTraitSlug == "" || args.OpponentID == "" || args.OpponentTraitType == "" || args.OpponentTraitSlug == "" {
			return "", fmt.Errorf("opposed check requires actor_id, all trait types/slugs, and opponent_id")
		}
		// Snapshotted ahead of the roll: only the actor can spend a Fudge Point
		// here, and only if the DM asked for one.
		actor, err := w.Character(args.ActorID)
		if err != nil {
			return "", err
		}
		before := actor.ToString()
		result, err := w.OpposedCheck(args.ActorID, args.ActorTraitType, args.ActorTraitSlug, args.ActorModifier, args.OpponentID, args.OpponentTraitType, args.OpponentTraitSlug, args.OpponentModifier, args.AddOne)
		if err != nil {
			return "", err
		}
		opponent, _ := w.Character(args.OpponentID)
		var critStr string
		if result.ActorCrit {
			critStr += "|" + actor.Name + "-CRIT"
		}
		if result.OpponentCrit {
			critStr += "|" + opponent.Name + "-CRIT"
		}
		outcomeStr := "tie"
		if result.ActorWins {
			outcomeStr = actor.Name
		} else {
			outcomeStr = opponent.Name
		}
		// A wound on either side arrives through apply_damage, not from here.
		return withState(fmt.Sprintf("%s|%d|vs|%s|%d|deg%+d|win:%s%s", actor.Name, result.ActorTotal, opponent.Name, result.OpponentTotal, result.RelativeDegree, outcomeStr, critStr), before, actor), nil
	case "write_setting_file":
		var args struct {
			Filename string `json:"filename"`
			Mode     string `json:"mode"`
			Content  string `json:"content"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse write setting file: %w", err)
		}
		if w.Setting == nil {
			return "", fmt.Errorf("world has no setting selected")
		}
		if args.Filename == "" || args.Mode == "" {
			return "", fmt.Errorf("write setting file requires filename, mode, and content")
		}
		if err := w.Setting.WriteFile(args.Filename, args.Mode, []byte(args.Content)); err != nil {
			return "", err
		}
		return fmt.Sprintf("wrote %s (%s)", args.Filename, args.Mode), nil
	case "write_adventure":
		var args struct {
			Title   string `json:"title"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse write_adventure: %w", err)
		}
		if w.Setting == nil {
			return "", fmt.Errorf("world has no setting selected")
		}
		if args.Title == "" || args.Content == "" {
			return "", fmt.Errorf("write_adventure requires title and content")
		}
		path, err := w.Setting.WriteAdventure(args.Title, []byte(args.Content))
		if err != nil {
			return "", err
		}
		w.AdventureContent = args.Content
		return fmt.Sprintf("saved adventure %q to %s", args.Title, path), nil
	case "create_character":
		var req struct {
			Name        string         `json:"name"`
			Species     string         `json:"species"`
			Description string         `json:"description"`
			Location    string         `json:"location"`
			Attributes  map[string]int `json:"attributes"`
			Skills      map[string]int `json:"skills"`
			Gifts       []string       `json:"gifts"`
			Faults      []string       `json:"faults"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &req); err != nil {
			return "", fmt.Errorf("parse create_character: %w", err)
		}
		charReq := rules.CharacterRequest{
			Name:        req.Name,
			Species:     req.Species,
			Description: req.Description,
			Location:    req.Location,
			Attributes:  req.Attributes,
			Skills:      req.Skills,
			Gifts:       req.Gifts,
			Faults:      req.Faults,
		}
		ch, err := w.CreateCharacterValidated(charReq)
		if err != nil {
			if strings.Contains(err.Error(), "character slug already exists") {
				return "", fmt.Errorf("a character named %q already exists; please choose a different name", req.Name)
			}
			return "", err
		}
		// The character is the whole answer: nothing about them was known a
		// moment ago, and their id — the handle every later call needs — is the
		// first field of it. A "created character X" headline on top would only
		// say that again.
		return ch.ToString(), nil
	case "apply_damage":
		ch, raw, err := getCharacterFromJSON(w, call.Function.Arguments)
		if err != nil {
			return "", err
		}
		severity, ok := raw["severity"].(string)
		if !ok {
			return "", fmt.Errorf("severity required (%s)", strings.Join(rules.SeverityNames(), ", "))
		}
		damagePoints, err := rules.DamagePointsForSeverity(severity)
		if err != nil {
			return "", err
		}
		before := ch.ToString()
		result := ch.Wounds.ApplyDamage(damagePoints)
		return withState(fmt.Sprintf("%s|hit:%s|now:%s|scratch:%d|pen%+d", ch.Name, severity, result, ch.Wounds.Scratches, ch.Wounds.Penalty()), before, ch), nil
	case "heal":
		ch, raw, err := getCharacterFromJSON(w, call.Function.Arguments)
		if err != nil {
			return "", err
		}
		levels, ok := raw["levels"].(float64)
		if !ok {
			return "", fmt.Errorf("levels required")
		}
		lvl := int(levels)
		if lvl <= 0 {
			return "", fmt.Errorf("levels to heal must be positive")
		}
		before := ch.ToString()
		result := ch.Wounds.Heal(lvl)
		return withState(fmt.Sprintf("%s|heal%d|%s|level:%s|pen%+d", ch.Name, lvl, result, ch.Wounds.Level, ch.Wounds.Penalty()), before, ch), nil
	case "declare_dead":
		ch, _, err := getCharacterFromJSON(w, call.Function.Arguments)
		if err != nil {
			return "", err
		}
		before := ch.ToString()
		ch.Wounds.DeclareDead()
		return withState(fmt.Sprintf("%s|DEAD", ch.Name), before, ch), nil
	case "add_item", "remove_item":
		var args struct {
			CharacterID string `json:"character_id"`
			Name        string `json:"name"`
			Description string `json:"description"`
			Count       int    `json:"count"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse %s: %w", call.Function.Name, err)
		}
		ch, err := targetCharacter(w, args.CharacterID)
		if err != nil {
			return "", err
		}
		if args.Count == 0 {
			args.Count = 1
		}
		before := ch.ToString()
		if call.Function.Name == "add_item" {
			item, err := ch.AddItem(args.Name, args.Description, args.Count)
			if err != nil {
				return "", err
			}
			return withState(fmt.Sprintf("%s|+%d %s|has:%d", ch.Name, args.Count, item.Name, item.Count), before, ch), nil
		}
		item, left, err := ch.RemoveItem(args.Name, args.Count)
		if err != nil {
			return "", err
		}
		return withState(fmt.Sprintf("%s|-%d %s|has:%d", ch.Name, args.Count, item.Name, left), before, ch), nil
	case "add_weapon":
		var args struct {
			CharacterID     string `json:"character_id"`
			Name            string `json:"name"`
			Description     string `json:"description"`
			Damage          int    `json:"damage"`
			Ranged          bool   `json:"ranged"`
			DamageAttribute string `json:"damage_attribute"`
			Ammo            int    `json:"ammo"`
			AmmoCapacity    int    `json:"ammo_capacity"`
			AmmoItem        string `json:"ammo_item"`
			Count           int    `json:"count"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse add_weapon: %w", err)
		}
		ch, err := targetCharacter(w, args.CharacterID)
		if err != nil {
			return "", err
		}
		// The attribute a weapon draws its strength from has to be one the
		// setting actually has, or the arm behind every swing silently counts
		// for nothing.
		attr := strings.ToLower(args.DamageAttribute)
		if attr != "" && w.Setting != nil {
			if _, ok := w.Setting.Attributes[attr]; !ok {
				return "", fmt.Errorf("unknown damage_attribute %q: use one of the setting's attributes, or omit it for a weapon that does not depend on the wielder", args.DamageAttribute)
			}
		}
		if args.Count == 0 {
			args.Count = 1
		}
		before := ch.ToString()
		item, err := ch.AddWeapon(args.Name, args.Description, args.Count, rules.Weapon{
			Damage:          args.Damage,
			Ranged:          args.Ranged,
			DamageAttribute: attr,
			Ammo:            args.Ammo,
			AmmoCapacity:    args.AmmoCapacity,
			AmmoItem:        args.AmmoItem,
		})
		if err != nil {
			return "", err
		}
		return withState(fmt.Sprintf("%s|+%d %s (weapon)|has:%d", ch.Name, args.Count, item.Name, item.Count), before, ch), nil
	case "add_armor":
		var args struct {
			CharacterID string `json:"character_id"`
			Name        string `json:"name"`
			Description string `json:"description"`
			Protection  int    `json:"protection"`
			Worn        *bool  `json:"worn"`
			Count       int    `json:"count"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse add_armor: %w", err)
		}
		ch, err := targetCharacter(w, args.CharacterID)
		if err != nil {
			return "", err
		}
		if args.Count == 0 {
			args.Count = 1
		}
		// Armor is worn unless the DM says otherwise: the common case by far is a
		// character who has it on, and armor recorded as cargo protects nobody.
		worn := args.Worn == nil || *args.Worn
		before := ch.ToString()
		item, err := ch.AddArmor(args.Name, args.Description, args.Count, rules.Armor{Protection: args.Protection, Worn: worn})
		if err != nil {
			return "", err
		}
		return withState(fmt.Sprintf("%s|+%d %s (armor)|has:%d", ch.Name, args.Count, item.Name, item.Count), before, ch), nil
	case "equip_armor":
		var args struct {
			CharacterID string `json:"character_id"`
			Name        string `json:"name"`
			Worn        bool   `json:"worn"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse equip_armor: %w", err)
		}
		ch, err := targetCharacter(w, args.CharacterID)
		if err != nil {
			return "", err
		}
		before := ch.ToString()
		item, err := ch.EquipArmor(args.Name, args.Worn)
		if err != nil {
			return "", err
		}
		state := "removed"
		if args.Worn {
			state = "worn"
		}
		return withState(fmt.Sprintf("%s|%s:%s", ch.Name, item.Name, state), before, ch), nil
	case "reload_weapon":
		var args struct {
			CharacterID string `json:"character_id"`
			Weapon      string `json:"weapon"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse reload_weapon: %w", err)
		}
		ch, err := targetCharacter(w, args.CharacterID)
		if err != nil {
			return "", err
		}
		before := ch.ToString()
		item, ammoLeft, err := ch.Reload(args.Weapon)
		if err != nil {
			return "", err
		}
		result := fmt.Sprintf("%s|reloaded %s|ammo:%d/%d", ch.Name, item.Name, item.Weapon.Ammo, item.Weapon.AmmoCapacity)
		if ammoLeft >= 0 {
			result += fmt.Sprintf("|%s left:%d", item.Weapon.AmmoItem, ammoLeft)
		}
		return withState(result, before, ch), nil
	case "opposed_attack", "unopposed_attack":
		var args struct {
			AttackerID        string `json:"attacker_id"`
			AttackerTraitType string `json:"attacker_trait_type"`
			AttackerTraitSlug string `json:"attacker_trait_slug"`
			AttackerModifier  int    `json:"attacker_modifier"`
			DefenderID        string `json:"defender_id"`
			DefenderTraitType string `json:"defender_trait_type"`
			DefenderTraitSlug string `json:"defender_trait_slug"`
			DefenderModifier  int    `json:"defender_modifier"`
			TargetID          string `json:"target_id"`
			Difficulty        string `json:"difficulty"`
			Weapon            string `json:"weapon"`
			Thrown            bool   `json:"thrown"`
			AddOne            bool   `json:"add_one"`
			Reroll            bool   `json:"reroll"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse %s: %w", call.Function.Name, err)
		}
		opposed := call.Function.Name == "opposed_attack"
		req := rules.AttackRequest{
			ActorID:        args.AttackerID,
			ActorTraitType: args.AttackerTraitType,
			ActorTraitSlug: args.AttackerTraitSlug,
			ActorModifier:  args.AttackerModifier,
			Weapon:         args.Weapon,
			Thrown:         args.Thrown,
			AddOne:         args.AddOne,
		}
		if opposed {
			req.TargetID = args.DefenderID
			req.TargetTraitType = args.DefenderTraitType
			req.TargetTraitSlug = args.DefenderTraitSlug
			req.TargetModifier = args.DefenderModifier
			if req.TargetID == "" || req.TargetTraitType == "" || req.TargetTraitSlug == "" {
				return "", fmt.Errorf("opposed attack requires defender_id, defender_trait_type and defender_trait_slug")
			}
		} else {
			req.TargetID = args.TargetID
			if req.TargetID == "" {
				return "", fmt.Errorf("unopposed attack requires target_id")
			}
			// An undefended target is a Poor difficulty unless the DM makes the
			// shot harder (§4.2.2).
			req.Reroll = args.Reroll
			req.DifficultyName = args.Difficulty
			if req.DifficultyName == "" {
				req.DifficultyName = "very easy"
			}
			difficulty, err := rules.DifficultyLevel(req.DifficultyName)
			if err != nil {
				return "", err
			}
			req.Difficulty = difficulty
		}
		if req.ActorID == "" || req.ActorTraitType == "" || req.ActorTraitSlug == "" {
			return "", fmt.Errorf("%s requires attacker_id, attacker_trait_type and attacker_trait_slug", call.Function.Name)
		}
		// Both sides are snapshotted ahead of the roll: an attack can spend the
		// attacker's Fudge Point, ammunition or thrown weapon and wound the
		// target, all in the one call.
		attacker, err := w.Character(req.ActorID)
		if err != nil {
			return "", err
		}
		target, err := w.Character(req.TargetID)
		if err != nil {
			return "", err
		}
		snaps := snapshot(attacker, target)
		var result *rules.AttackResult
		if opposed {
			result, err = w.OpposedAttack(req)
		} else {
			result, err = w.UnopposedAttack(req)
		}
		if err != nil {
			return "", err
		}
		return withStates(result.String(), snaps), nil
	case "add_coins", "remove_coins":
		var args struct {
			CharacterID string `json:"character_id"`
			Amount      int    `json:"amount"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse %s: %w", call.Function.Name, err)
		}
		ch, err := targetCharacter(w, args.CharacterID)
		if err != nil {
			return "", err
		}
		before := ch.ToString()
		sign := "+"
		balance := 0
		if call.Function.Name == "add_coins" {
			balance, err = ch.AddCoins(args.Amount)
		} else {
			sign = "-"
			balance, err = ch.RemoveCoins(args.Amount)
		}
		if err != nil {
			return "", err
		}
		return withState(fmt.Sprintf("%s|%s%d coins|purse:%d", ch.Name, sign, args.Amount, balance), before, ch), nil
	case "roll_on_table":
		var args struct {
			Table    string `json:"table"`
			Modifier int    `json:"modifier"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse roll_on_table: %w", err)
		}
		roll, value, err := w.RollTable(args.Table, args.Modifier)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s|roll%+d|mod%+d|%s", args.Table, roll, args.Modifier, value), nil
	default:
		return "", fmt.Errorf("unknown action tool %q", call.Function.Name)
	}
}
