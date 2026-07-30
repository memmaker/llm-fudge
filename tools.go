package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"fudge/prompts"
	"fudge/rules"
	"fudge/ui"

	"github.com/memmaker/gollum/llm"
)

// entityArgs names every tool argument that carries an entity id. It is what
// makes the archive an entity index rather than a pile of text: a call already
// says which entities it is about — it could not be made otherwise — so the ids
// are collected as it goes by and the record can be asked "what has happened to
// this merchant" instead of "does the word merchant appear anywhere".
//
// A fixed list of argument names, read off the arguments the call already
// carries, is deliberately the whole of the mechanism. The alternative was to
// have each of the two dozen cases in executeAction hand its ids back, which
// would spread the same knowledge across two dozen places and mean a new tool
// silently contributes nothing to the index. Anything not on this list is not an
// id: `where` is prose about a shelf, `name` is what a thing is called, and
// neither addresses anything.
var entityArgs = []string{
	"actor_id", "opponent_id", "attacker_id", "defender_id", "target_id",
	"character_id", "item", "from_character", "to_character",
	"from_location", "to_location", "location", "parent",
}

// callEntities reads the entity ids out of a tool call's arguments, plus the
// entities named by any world_update operations riding along with it.
//
// Malformed arguments come back empty rather than as an error. This runs while a
// turn is being recorded, after the player has already read the prose, and a
// record that is thinner than it could be is a far smaller problem than a turn
// that fails over its own bookkeeping.
func callEntities(call llm.ToolCall) []string {
	var raw map[string]any
	if err := json.Unmarshal([]byte(call.Function.Arguments), &raw); err != nil {
		return nil
	}

	seen := map[string]bool{}
	var ids []string
	add := func(id string) {
		id = strings.ToLower(strings.TrimSpace(id))
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		ids = append(ids, id)
	}

	for _, key := range entityArgs {
		if value, ok := raw[key].(string); ok {
			add(value)
		}
	}

	// world_update rides on output_narration as an array of operations, each
	// naming its own entity. Those are the ids the DM itself chose to talk about,
	// which makes them the most worth indexing of the lot.
	if ops, ok := raw["world_update"].([]any); ok {
		for _, op := range ops {
			fields, ok := op.(map[string]any)
			if !ok {
				continue
			}
			if entity, ok := fields["entity"].(string); ok {
				add(entity)
			}
		}
	}
	return ids
}

// playerLanguageNote repeats the system prompt's language rule at the one place
// the model is actually filling a field the player will read. The prompt says it
// once, in a section; a tool description says it while the argument is being
// written, which is where English creeps back in. Empty when the player reads
// English, so the default game's tools stay exactly as they were.
func playerLanguageNote(what string) string {
	if language == "" || language == prompts.DefaultLanguage {
		return ""
	}
	return fmt.Sprintf(" LANGUAGE: %s is read by the player, so write it in %s. Every other argument of this call stays English—see the Language section of your system prompt.", what, language)
}

// setupUserChoiceTool is the menu offered during setting and character creation,
// where the model is talking to the player as itself rather than narrating.
func setupUserChoiceTool() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.ToolFunction{Name: "user_choice", Description: "Present the user with 2-4 mutually exclusive options. ONLY use when: (1) the player's action has exactly 2-4 meaningful, dramatically distinct outcomes; (2) you cannot narrate forward without knowing which they choose; (3) the moment is critical enough to warrant explicit choice. REQUIREMENTS: Write a clear prompt/question before calling this tool. The menu appears after your text. Never include a 'Type your answer' option—it's added automatically." + playerLanguageNote("The options are"), Parameters: map[string]any{"type": "object", "properties": map[string]any{"options": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "List of 2-4 mutually exclusive options. Do NOT include a 'Type your answer' or freeform option—it is added automatically."}}, "required": []string{"options"}}}}
}

func settingCreationTools() []llm.Tool {
	return []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "create_setting", Description: "Initialize a new setting with a chosen name. Call this FIRST before writing any setting files. The name becomes the folder name and setting identifier (converted to lowercase, spaces to hyphens). Example: 'Haunted Deadlands' becomes 'haunted-deadlands'.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string", "description": "The setting name (will be slugified for folder)"}}, "required": []string{"name"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "write_setting_file", Description: "Write one setting file. CRITICAL SEQUENCE: After create_setting, call this tool once for EACH of these files in this order: (1) description.md, (2) species.rec, (3) attributes.rec, (4) skills.rec, (5) gifts.rec, (6) faults.rec, (7) powers.rec.\n\nFor .rec files: Format each entry separated by blank line. REQUIRED FIELDS per entry: slug (lowercase+digits+hyphens only, NO underscores), display_name (human-readable name), description (1 sentence). OPTIONAL: scale (species only; integer -10 to +10 where 0 is human-sized, so anything roughly person-shaped is 0). Slug format: use hyphens not underscores, e.g. \"dust-witch\" not \"dust_witch\". Example entry:\nslug: dust-witch\ndisplay_name: Dust Witch\ndescription: Occult wanderers channeling desert spirits.\nscale: 0\n\nFor description.md: Plain markdown text describing the setting's atmosphere, themes, and core concept (2-3 sentences).", Parameters: map[string]any{"type": "object", "properties": map[string]any{"filename": map[string]any{"type": "string"}, "mode": map[string]any{"type": "string", "enum": []string{"replace", "append"}}, "content": map[string]any{"type": "string"}}, "required": []string{"filename", "mode", "content"}}}},
		setupUserChoiceTool(),
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
		setupUserChoiceTool(),
	}
	return append(tools, inventoryTools()...)
}

// addWeaponTool and addArmorTool are built on their own because they stand on
// both sides of the turn. They are constructors: the ratings they mint are what
// the attack calls do their arithmetic with, so a bandit described with a rifle
// and a breastplate has to be armed in the same response he is created and
// attacked in — the consult phase — or he fights bare-handed in his shirt.
// Everything else they are used for is loot, rewards and gifts, which is
// bookkeeping and waits for the prose like the rest of it.
func addWeaponTool() llm.Tool {
	characterID, location, locationDescription := itemPlacementArgs()
	return llm.Tool{Type: "function", Function: llm.ToolFunction{Name: "add_weapon", Description: "Give a character a weapon. Use this instead of add_item for anything meant to be attacked with—the damage rating you set here is what the attack tools do their arithmetic with, and a blade recorded as plain gear cannot be swung at all. Rate the weapon by what it is (Fudge damage factors): -1 bare fist, +0 a knife or small club, +1 a sword or hand axe, +2 a large one-handed or light two-handed weapon, +3 a heavy two-handed weapon, +1 more for a genuinely sharp edge. Firearms: +0 to +1 derringer, +2 to +3 handgun, +5 or more for a powerful two-hander, +10 for a bazooka. Set damage_attribute for anything driven by muscle (sword, axe, bow, thrown spear) so the wielder's own strength counts; leave it out for anything with its own power source, where the weapon hits the same however weak the hand holding it.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": characterID, "location": location, "location_description": locationDescription, "name": map[string]any{"type": "string", "description": "The weapon's name as the player will read it, e.g. \"Blaster Pistol\"."}, "description": map[string]any{"type": "string", "description": "One short concrete sentence describing this particular weapon."}, "damage": map[string]any{"type": "integer", "description": "Damage rating: the weapon's own contribution to a hit. See the guidance above."}, "ranged": map[string]any{"type": "boolean", "description": "True for anything fired or shot from a distance (gun, bow, crossbow). False for hand-to-hand weapons. Defaults to false."}, "damage_attribute": map[string]any{"type": "string", "description": "Slug of the attribute added to damage for muscle-powered weapons—the setting's strength-like attribute. Omit for firearms, energy weapons and anything else that does not depend on the wielder's build."}, "ammo_capacity": map[string]any{"type": "integer", "description": "How many shots it holds when full. Ranged only. Omit or 0 to not count shots at all, which is fine for a bow whose arrows you would rather not tally."}, "ammo": map[string]any{"type": "integer", "description": "How many shots are in it right now. Defaults to full."}, "ammo_item": map[string]any{"type": "string", "description": "The name of the item a reload spends, e.g. \"Power Cell\". Add that item separately with add_item—reloading fails when they carry none, and a character with a full magazine and no spares is one firefight from empty."}, "count": map[string]any{"type": "integer", "description": "How many are gained. Defaults to 1."}}, "required": []string{"name", "description", "damage"}}}}
}

func addArmorTool() llm.Tool {
	characterID, location, locationDescription := itemPlacementArgs()
	return llm.Tool{Type: "function", Function: llm.ToolFunction{Name: "add_armor", Description: "Give a character armor. Use this instead of add_item for anything worn as protection—the rating you set here is subtracted from every blow that lands on them. Rate it by what it is made of: +1 light non-metal (padded, leather), +2 heavy non-metal or light metal (mail shirt, flak vest), +3 medium metal, +4 heavy metal (full plate), +5 or more for powered or science-fiction armor. Armor goes on worn: it protects from the moment you record it. Pass worn false for a piece being hauled rather than used—salvage in a pack, a breastplate looted off a body and not yet put on.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": characterID, "location": location, "location_description": locationDescription, "name": map[string]any{"type": "string", "description": "The armor's name as the player will read it, e.g. \"Flak Vest\"."}, "description": map[string]any{"type": "string", "description": "One short concrete sentence describing this particular piece."}, "protection": map[string]any{"type": "integer", "description": "Protection rating: how much it takes off a blow. See the guidance above."}, "worn": map[string]any{"type": "boolean", "description": "Whether it is being worn. Defaults to true. Only worn armor protects, and only the single best worn piece counts—layering does not add up."}, "count": map[string]any{"type": "integer", "description": "How many are gained. Defaults to 1."}}, "required": []string{"name", "description", "protection"}}}}
}

// inventoryTools are shared by character creation and play: the same calls outfit
// a starting character, hand out loot and settle a bar tab. Weapons and armor go
// on through their own calls because they carry ratings combat reads — a sword
// added as plain gear cannot be swung.
// itemPlacementArgs are the three arguments every call that brings something
// into the world shares: whose it is, or else where it lies and where in there.
func itemPlacementArgs() (characterID, location, locationDescription map[string]any) {
	characterID = map[string]any{"type": "string", "description": "Whose possessions change. Omit it to mean the player character—during character creation their id does not exist yet."}
	// A destination is a character or a place, never both and never one field
	// holding either: both namespaces are slugs, so an NPC called Mira and a
	// room called Mira would be indistinguishable in the one place it matters.
	location = map[string]any{"type": "string", "description": "Create the item lying at this location instead of in somebody's hands—a crate already in the cellar, a body's gear on the ground, a sword in the stone. Give the location's slug, and give location_description with it. Leave both out to give the item to a character."}
	locationDescription = map[string]any{"type": "string", "description": "Where in that location the thing rests, in a few words: \"on the top shelf\", \"half-buried in ash\", \"nailed above the door\". Required whenever location is given."}
	return characterID, location, locationDescription
}

func inventoryTools() []llm.Tool {
	characterID, location, locationDescription := itemPlacementArgs()
	return []llm.Tool{
		addWeaponTool(),
		addArmorTool(),
		{Type: "function", Function: llm.ToolFunction{Name: "equip_armor", Description: "Put armor on or take it off. Armor is already worn when you add it, so this is for the moments the fiction turns on it: stripping a corpse and shrugging into what it wore, peeling off a soaked coat, being stripped of a shield by captors. Armor that is not worn protects nobody.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": characterID, "name": map[string]any{"type": "string", "description": "The armor's name as it was added."}, "worn": map[string]any{"type": "boolean", "description": "True to put it on, false to take it off."}}, "required": []string{"name", "worn"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "add_item", Description: "Bring an item into the world: starting equipment at creation, a purchase, a reward, a gift from off-screen, something found that was never recorded before. It goes into a character's hands, or—with location and location_description—onto the ground somewhere. An item they already carry stacks onto the entry they have, so use the same name you used before. The description is what the player reads on their sheet, so write one concrete line about this particular thing—what it is, what shape it is in, where it came from—not a catalogue entry. This is for things that did NOT exist a moment ago: anything already on a sheet or lying somewhere is moved with move_item, which carries its ratings and wording across intact. Items are recorded here and nowhere else: an object you only narrate does not exist.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": characterID, "location": location, "location_description": locationDescription, "name": map[string]any{"type": "string", "description": "The item's name as the player will read it, e.g. \"Blaster Pistol\". Name the thing itself, without the count."}, "description": map[string]any{"type": "string", "description": "One short concrete sentence describing this item."}, "count": map[string]any{"type": "integer", "description": "How many are gained. Defaults to 1."}}, "required": []string{"name", "description"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "remove_item", Description: "Destroy an item: drunk, eaten, burned, smashed, fired off as the last charge, dissolved, spent for good. It ceases to exist. This is NOT how something changes hands—an item sold, given, dropped, stolen or left behind still exists and belongs somewhere, so move_item it to whoever or wherever has it now. Destroying something that merely moved is how a world loses things. Works on a character's inventory, or on a location's floor with location and location_description. The call fails if there are not that many—read the error and narrate what is actually there rather than insisting.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": characterID, "location": location, "location_description": locationDescription, "name": map[string]any{"type": "string", "description": "The item's name as it was recorded."}, "count": map[string]any{"type": "integer", "description": "How many are destroyed. Defaults to 1."}}, "required": []string{"name"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "add_coins", Description: "Credit a character's purse: starting money at creation, a reward, a sale, found treasure. Coins are the setting's ordinary currency, whatever it is called there.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": characterID, "amount": map[string]any{"type": "integer", "description": "How many coins are gained. Must be positive."}}, "required": []string{"amount"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "remove_coins", Description: "Debit a character's purse: a purchase, a bribe, a toll, a theft. A purse never goes negative, so the call fails when they cannot afford it—when it does, they cannot pay, and the scene goes differently.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": characterID, "amount": map[string]any{"type": "integer", "description": "How many coins are spent. Must be positive."}}, "required": []string{"amount"}}}},
	}
}

// attackTools resolve a blow from the roll through to the wound in one call.
// Everything they need is already on the sheets — the weapon's rating, the arm
// behind it, the armor in the way, both Scales — so the DM names the traits and
// the circumstances and the engine does the rest, rather than rolling a check and
// then guessing at a severity.
func attackTools() []llm.Tool {
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
	}
}

// reloadWeaponTool is built on its own because it is on both sides of the turn:
// a gun has to be loaded before it can be fired, which is the consult phase, and
// a magazine slapped home in the prose has to be written down, which is the
// bookkeeping phase.
func reloadWeaponTool() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.ToolFunction{Name: "reload_weapon", Description: "Reload a ranged weapon, spending one of the ammunition items it feeds on. Every shot fired counts down the magazine—hits and misses alike—and an empty weapon simply will not fire until this is called. It costs the character their action in the fiction, so narrate the moment: a fumbled magazine mid-firefight is a scene, not bookkeeping. It fails when they carry no ammunition left, and then they are out for good and must find more, take a weapon off someone, or fight with something else.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": map[string]any{"type": "string", "description": "Whose weapon is reloaded. Omit to mean the player character."}, "weapon": map[string]any{"type": "string", "description": "The weapon's name as it is carried."}}, "required": []string{"weapon"}}}}
}

// createLocationTool is built on its own because the round that has to end a
// turn is offered it beside the narration: a scene that has moved somewhere new
// cannot be narrated until the place exists, and that round has no other call
// left to make it with.
func createLocationTool() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.ToolFunction{Name: "create_location", Description: "Register a place the story has reached: a room, a street, a clearing, a ship's hold. Every narration names the location it happened in, and only a location that exists can be named—so the moment your prose moves the scene somewhere new, create it in that same response, before output_narration. The id is minted from the name and comes back in the reply; it is also listed in LOCATIONS every turn, so read slugs off there rather than inventing them. Do NOT create a second location for a place that already has one—one room, one slug. Split a place into child locations only when the fiction actually treats them separately: a tavern and its cellar, a keep and its courtyard, not every corner of one room.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string", "description": "The place's name as the player would hear it, e.g. \"The Rusty Anchor\"."}, "description": map[string]any{"type": "string", "description": "What this place permanently IS, in one or two concrete sentences: its shape, its light, what it is for. Not what is currently happening in it and not who is in it—those change, and they belong in world_update."}, "parent": map[string]any{"type": "string", "description": "The slug of the location this one sits inside, if any: a cellar's parent is the tavern above it. The parent must already exist. Nesting is for orientation and does not affect reach—an item is only ever picked up from the exact location a character stands in."}}, "required": []string{"name", "description"}}}}
}

// narrationTool is the one call a turn cannot end without, so it is built on its
// own: a turn that has spent every resolution round it is allowed still has to be
// narrated, and it is offered this and nothing else.
func narrationTool() llm.Tool {
	// Under --img-gen the scene's image prompt rides along as a third argument of
	// output_narration rather than a tool of its own: a separate tool is a
	// separate round-trip, and the DM ended its turn with the narration instead of
	// ever getting there. Required, so every narrated scene produces one.
	narrationDescription := "Show the turn's narration to the player. This is the ONLY channel to them: they read the prose argument and nothing else—no dice, no tool names, no tool results, and no text you write outside this call. Text written as a plain reply reaches nobody. Every turn ends with exactly one output_narration, and the turn is not over until you call it. Call it only once you know what happened: if a check, an attack, a new NPC or a table roll decides the outcome, make those calls first with no narration, read the results when they come back, and narrate in your next response. It must be your LAST tool call, after everything you resolved, so it can reflect their outcome. What the prose then implies about possessions, wounds and who is standing where is not your job here—you get one more response after this one, with those calls on the table and this prose quoted back to you, and that is where the books are squared. Argument prose: the narration the player reads. Argument location (REQUIRED): the slug of the place this scene happens in. Argument world_update: operations recording what your prose established about how people feel, what they want, and what is going on—never about where anything is or what anybody carries."
	narrationProperties := map[string]any{
		"prose":    map[string]any{"type": "string", "description": "The turn's narration exactly as the player will read it: normal, descriptive, engaging prose in full sentences. It must carry everything the tool calls established, since the player sees none of them. No titles or speaker prefixes (no \"DM:\"), no meta-commentary, no mention of dice, difficulty or mechanics, and no \"What do you do?\"—narrate and stop." + playerLanguageNote("This prose")},
		"location": map[string]any{"type": "string", "description": "Required. The slug of the place this scene happens in, taken from the location entities in WORLD STATE. It is how the engine knows where the player character is standing, so it must be where the prose actually leaves them—if they walked somewhere during the turn, name where they ended up. A slug that does not exist is refused and the player sees nothing, so when the scene moves somewhere new, call create_location in this same response first and use the id it returns."},
		"world_update": map[string]any{
			"type":        "array",
			"description": "Operations recording what your prose established about the things the engine does not model: how an NPC feels, what they want, what they believe, what has been promised, what is closing in. Each operation names an entity by its id from WORLD STATE and writes into that entity's `narrative` map—which is the ONLY place these land. Position, possessions, wounds, coins and traits are the engine's: they have their own calls (move_item, move_character, apply_damage, add_coins), and naming one of them here is discarded. Restating an unchanged narrative fact is harmless, so write anything you are unsure of rather than gambling that it was recorded. Leave the array out entirely on a turn that established nothing the engine does not already hold—an empty update is a normal turn, not a failure. First call of a game: a full baseline of the opening scene, since nothing is recorded yet.",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"op":     map[string]any{"type": "string", "enum": []string{"create", "set", "delete"}, "description": "set writes properties onto an entity that already exists—any character, item or location the engine holds, or a narrative entity you created earlier. create mints a NEW narrative-only entity, for something that exists nowhere else: a rumour, a debt, a faction's standing, a prophecy. It cannot mint a character, an item or a location—those have their own calls, which compute what the rules need. delete drops a narrative entity whose thread is finished; it cannot delete anything the engine holds."},
					"entity": map[string]any{"type": "string", "description": "The id of the entity this is about, exactly as it appears in WORLD STATE—never a name you slugified yourself. For create, the new id you are minting: lowercase, hyphenated, and unique against every id already in WORLD STATE."},
					"properties": map[string]any{
						"type":        "object",
						"description": "The narrative facts, as ordinary key/value pairs: {\"mood\": \"frightened\", \"wants\": \"the debt paid\", \"believes\": \"the player is a thief\"}. Merged field by field, so what you do not mention is left alone. A value of null retracts that property. On create, include a `type` saying what kind of thing this is (rumor, debt, faction, prophecy). Required for create and set; leave it off for delete.",
					},
				},
				"required": []string{"op", "entity"},
			},
		},
	}
	narrationRequired := []string{"prose", "location"}

	if imgGen {
		narrationDescription += " Argument img_gen (REQUIRED): the prompt for an image generation AI depicting the scene this prose just narrated. Write it for every narration—it is not optional and not something to save for dramatic moments."
		narrationProperties["img_gen"] = map[string]any{"type": "string", "description": "Prompt for an image generation AI depicting the scene the prose narrates. Self-contained: the image AI sees only this text, none of your context. Describe the location and its lighting and atmosphere in detail; then EVERY character present, each with their clothing, appearance, where they stand in the scene, their pose, and what they are doing; then the state and items that show—wounds, drawn weapons, carried or worn gear, doors open or shut, fire, debris. Plain visual description, present tense, no story, no names the picture cannot show."}
		narrationRequired = append(narrationRequired, "img_gen")
	}

	return llm.Tool{Type: "function", Function: llm.ToolFunction{Name: "output_narration", Description: narrationDescription, Parameters: map[string]any{"type": "object", "properties": narrationProperties, "required": narrationRequired}}}
}

// narrationOnlyTools is the tool set a turn ends on. Withholding the engine is
// what makes the ending certain: there is nothing left to resolve with, so the
// only move is the one that hands the player their turn.
//
// create_location stays, because it is not a way to keep resolving — it settles
// nothing and rolls nothing — and without it a turn whose scene has moved
// somewhere new cannot be narrated at all: the narration would name a slug that
// does not exist, be refused, and the turn would be lost on its last round for
// want of a call it was not allowed to make.
func narrationOnlyTools() []llm.Tool {
	return []llm.Tool{narrationTool(), createLocationTool()}
}

// createCharacterTool is built on its own because it stands on both sides of the
// turn: a check cannot name an NPC who does not exist yet, and an NPC the prose
// walked into the scene without rolling for still has to be registered afterwards.
func createCharacterTool() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.ToolFunction{Name: "create_character", Description: "Create a new NPC. CRITICAL: call this BEFORE any check involving that NPC—the character must exist before you can make a check against them. If a player targets an NPC for the first time, create them immediately, THEN call the appropriate check (opposed_check or unopposed_check) in the same response. The character ID is generated from the name and comes back in the reply as its id: field—that is the handle every later call to this character needs, so read the id off it instead of deriving one from the name. It is in WORLD STATE from the next turn onward, as an entity of its own, for as long as they exist. Only use traits (attributes, skills, gifts, faults) listed in the Setting Information section of your system prompt. Species is required.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}, "species": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}, "location": map[string]any{"type": "string"}, "attributes": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, "skills": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, "gifts": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "faults": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, "required": []string{"name", "species"}}}}
}

// ============================== Consult-first ==============================
//
// The calls a turn is allowed while it still does not know what happened. Every
// one of them answers a question the DM cannot answer itself — how the dice
// fell, what id the engine minted, what an NPC said six turns ago — and the
// narration is written out of the answers. Information runs engine → DM here, so
// these have to come before the prose: a roll made afterwards is a roll the prose
// has already decided.
//
// This is the whole tool set of a turn's resolution rounds. Nothing that merely
// writes something down belongs in it — see recordTools.
func consultTools() []llm.Tool {
	tools := []llm.Tool{
		narrationTool(),
		{Type: "function", Function: llm.ToolFunction{Name: "unopposed_check", Description: "Roll an action that nothing is actively resisting: climbing a wall, picking a lock, recalling lore, crafting. Use it whenever the outcome is uncertain and no opponent opposes it—if one does, use opposed_check instead. Skip it entirely for trivial actions with no real chance of failure and no stakes; just narrate those. You name the difficulty and the engine rolls; narrate only what comes back, and never invent a roll or adjust the result. situational_modifier is roughly -4 to +4 for circumstances (terrain, lighting, preparation, exhaustion, poor tools); 0 should be the common case and ±3/±4 is for genuinely extreme conditions. add_one and reroll each spend one of the character's Fudge Points, and asking for both spends two.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"actor_id": map[string]any{"type": "string"}, "trait_type": map[string]any{"type": "string", "enum": []string{"attribute", "skill"}}, "trait_slug": map[string]any{"type": "string"}, "difficulty": map[string]any{"type": "string", "enum": rules.DifficultyNames(), "description": "How hard the task is. The engine converts this to a target number."}, "situational_modifier": map[string]any{"type": "integer"}, "add_one": map[string]any{"type": "boolean", "description": "Spend one Fudge Point for +1 on the roll."}, "reroll": map[string]any{"type": "boolean", "description": "Spend one Fudge Point to roll twice and keep the better roll. The result names the roll that was thrown away, which is worth narrating—the slip that almost happened, the grip that almost went."}}, "required": []string{"actor_id", "trait_type", "trait_slug", "difficulty"}}}},
		createCharacterTool(),
		{Type: "function", Function: llm.ToolFunction{Name: "opposed_check", Description: "Resolve a direct conflict between two characters that is not a blow being struck: a grapple, a chase, a deception, a stare-down, resisting a spell, wrestling for a dropped gun. Use when two characters have conflicting intentions. For an actual attack use opposed_attack instead—it settles the roll, the weapon, the armor and the wound in one call, and this tool never wounds anybody. CRITICAL SEQUENCE: (1) If an NPC doesn't exist yet, call create_character FIRST in the same response; (2) THEN immediately call opposed_check with both actor_id and opponent_id (use the character's actual ID). actor_id is whoever initiated the action; opponent_id is whoever reacts. Do NOT narrate the defense separately—the check outcome tells the story. Narrate using only the tool result.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"actor_id": map[string]any{"type": "string"}, "actor_trait_type": map[string]any{"type": "string", "enum": []string{"attribute", "skill"}}, "actor_trait_slug": map[string]any{"type": "string"}, "actor_modifier": map[string]any{"type": "integer"}, "opponent_id": map[string]any{"type": "string"}, "opponent_trait_type": map[string]any{"type": "string", "enum": []string{"attribute", "skill"}}, "opponent_trait_slug": map[string]any{"type": "string"}, "opponent_modifier": map[string]any{"type": "integer"}, "add_one": map[string]any{"type": "boolean"}}, "required": []string{"actor_id", "actor_trait_type", "actor_trait_slug", "opponent_id", "opponent_trait_type", "opponent_trait_slug"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "roll_on_table", Description: "Roll on a random table when the fiction needs an outcome that no character is choosing. Use it instead of deciding these yourself—that is the only way they are genuinely random. npc-reaction: how an NPC you have not characterized yet feels about the party on first meeting. npc-morale: whether an NPC presses on, wavers or breaks, checked when a fight turns against them. weather: conditions on the road or at the start of a day outdoors. situational-roll: how luck breaks when there is no actor to check at all. The modifier is roughly -4 to +4 for whatever the established fiction says should tilt it (a reputation, a bribe, the season, a rout already in progress); 0 is the common case. Narrate the result the table gives you.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"table": map[string]any{"type": "string", "enum": rules.TableNames()}, "modifier": map[string]any{"type": "integer"}}, "required": []string{"table"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "recall", Description: "Look up a turn from earlier in this session. Everything ever played is kept—what the player typed, what the engine resolved, what you narrated word for word—but only the last few turns ride in WORLD STATE, so this is how you reach the rest. Use it when you need something you cannot see: what an NPC actually said, the exact terms of a deal, what a room looked like the first time. Prefer entity: every turn is indexed by the ids that took part in it, so asking for an entity finds every turn it was involved in, exactly, whether or not your prose happened to use the word. Every narrative property carries the turn it was last set (@t12), so to learn how a fact came about, recall that turn. Keyword search is for what is not tied to one entity—a subject anybody might have mentioned. Ask for include_prose when you need the words themselves rather than what changed. This costs you one of the turn's calls like anything else, so spend it on something you will use—and if it comes back empty, then it did not happen: say so in the fiction rather than inventing it.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"entity": map[string]any{"type": "string", "description": "An entity id from WORLD STATE. Returns every turn that entity took part in—the best way to ask what has passed between the player and somebody, or what has happened to a thing. Exact: it must be the id, not a name and not a fragment of one."}, "query": map[string]any{"type": "string", "description": "Words to search for across player input, what the engine recorded, and narration. For subjects rather than entities—a rumour, a topic, a turn of phrase. Omit when asking by entity or turn number."}, "turn": map[string]any{"type": "integer", "description": "A turn number to fetch, e.g. the 12 from an @t12 marker on a narrative property."}, "to": map[string]any{"type": "integer", "description": "End of a turn range, when you want several turns around one moment. Requires turn."}, "include_prose": map[string]any{"type": "boolean", "description": "Return the narration itself, not just what changed. Costly—ask only when the wording matters."}, "limit": map[string]any{"type": "integer", "description": "How many turns to return. Defaults to a few."}}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "user_choice", Description: "Present the player with 2-4 mutually exclusive options. ONLY use when: (1) the player's action has exactly 2-4 meaningful, dramatically distinct outcomes; (2) you cannot narrate forward without knowing which they choose; (3) the moment is critical enough to warrant explicit choice. For normal gameplay where the player types an action, just narrate the outcome. Do NOT use this as a generic \"What do you do?\" prompt. REQUIREMENTS: call output_narration in the same response, with prose that poses the question—the menu is shown underneath it. The player's answer comes back to you as this tool's result, and the turn continues: narrate what their choice leads to. Never include a 'Type your answer' option—it's added automatically." + playerLanguageNote("The options are"), Parameters: map[string]any{"type": "object", "properties": map[string]any{"options": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "List of 2-4 mutually exclusive options. Do NOT include a 'Type your answer' or freeform option—it is added automatically."}}, "required": []string{"options"}}}},
		createLocationTool(),
		reloadWeaponTool(),
		addWeaponTool(),
		addArmorTool(),
	}

	return append(tools, attackTools()...)
}

// =============================== Record-after ===============================
//
// The calls a turn makes once the prose exists and the player has read it. None
// of them tells the DM anything it did not already decide: they write down what
// the narration committed to — who took the lantern, who walked out, whose purse
// is lighter, who is lying dead on the boards. Information runs DM → engine.
//
// They are withheld until the bookkeeping phase for one reason. Emitted before
// the prose, a bookkeeping call is a prediction of text that does not exist yet,
// in this context or anywhere else; emitted after it, with the narration quoted
// back, it is a reading of text that does. The second is a far easier task, and
// it is the same model doing it either way.
//
// Three calls appear in both sets, because they are preconditions as well as
// consequences: a character must exist before a roll can name them, a place must
// exist before an NPC can be moved into it, and a gun must be loaded before it
// can be fired.
func recordTools() []llm.Tool {
	tools := []llm.Tool{
		createCharacterTool(),
		createLocationTool(),
		reloadWeaponTool(),
		{Type: "function", Function: llm.ToolFunction{Name: "apply_damage", Description: "Wound a character by something no attack roll settles: a fall, a fire, an explosion, poison, drowning, a collapsing floor, an execution. NOT for blows struck in a fight—opposed_attack and unopposed_attack wound the target themselves, and calling this after one of them wounds the same character twice over. The engine's account of what it already resolved this turn is listed for you above: if a wound is in it, it is done. You name the wound the fiction calls for and the engine does the arithmetic: it accumulates scratches, escalates wound levels and applies the resulting penalty. Death is never a side effect: characters stop at near death and are killed only by an explicit declare_dead.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": map[string]any{"type": "string"}, "severity": map[string]any{"type": "string", "enum": rules.SeverityNames(), "description": "The wound this hit inflicts on an undamaged target. Existing wounds escalate from there."}}, "required": []string{"character_id", "severity"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "heal", Description: "Heal a character by reducing wound levels. Cannot heal the dead.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": map[string]any{"type": "string"}, "levels": map[string]any{"type": "integer"}}, "required": []string{"character_id", "levels"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "declare_dead", Description: "Permanently declare a character dead. Dead characters cannot act or be healed.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": map[string]any{"type": "string"}}, "required": []string{"character_id"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "move_item", Description: "Move an item to whoever or wherever has it now. This is the ONE call for every change of hands: picking something up, dropping it, handing it over, looting a body, stashing it, retrieving a knife you threw. It carries the thing itself across—its ratings, its wording, its ammunition—so a weapon retrieved is a weapon that still fights the same. Never destroy and re-create an item to move it: that loses its ratings and, if only half the pair lands, loses the item. Name the item and one destination; the source is worked out for you, and you only state it when the same name exists in more than one place, which the error will tell you.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"item": map[string]any{"type": "string", "description": "The item's id from WORLD STATE, e.g. \"throwing-knife\". Prefer the id: it is unique and it does not change when the thing moves, so it always resolves. Its name works too (\"Throwing Knife\"), but a name held by two stacks is refused until you name the id or the spot."}, "to_character": map[string]any{"type": "string", "description": "The character id receiving it. Give this or to_location, never both."}, "to_location": map[string]any{"type": "string", "description": "The location slug it comes to rest at. Give this or to_character, never both."}, "location_description": map[string]any{"type": "string", "description": "Where in that location it comes to rest, in a few words: \"quivering in the door frame\", \"under the overturned table\". Required with to_location—it is the one thing about the landing only your prose knows."}, "from_character": map[string]any{"type": "string", "description": "Only when the item's name exists in more than one place: the character id it is coming from."}, "from_location": map[string]any{"type": "string", "description": "Only when the item's name exists in more than one place: the location slug it is coming from."}, "from_location_description": map[string]any{"type": "string", "description": "Only when one location holds two lots of the same thing in different spots: the spot to take from, worded as it is recorded."}, "count": map[string]any{"type": "integer", "description": "How many move. Defaults to 1."}}, "required": []string{"item"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "move_character", Description: "Put an NPC somewhere. The world does not hold still, so use it when somebody walks out, arrives, is dragged off, flees, or follows the player to the next room—an NPC left where you created them will still be standing there ten turns later, and the engine will keep saying so. The player character is NOT moved with this: they go wherever your narration's location says the scene is.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": map[string]any{"type": "string", "description": "Who moves, by character id."}, "location": map[string]any{"type": "string", "description": "The location slug they are now in. It must already exist."}}, "required": []string{"character_id", "location"}}}},
	}

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

// writeNarration persists the prose the player was just shown. Like the image
// prompt beside it, each narration replaces the file rather than appending: this
// is the text on screen right now, and the run's full history is the archive's
// job, not this file's.
func writeNarration(prose string) error {
	return os.WriteFile("narration.txt", []byte(prose), 0o644)
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

// itemDestination is where an add_ or remove_ call reaches: somebody's hands or
// a spot on the ground. It exists so the four calls that create and destroy
// items do not each grow two versions of themselves, and so a location
// destination is validated identically wherever it turns up.
type itemDestination struct {
	world     *rules.World
	character *rules.Character
	location  *rules.Location
	where     string
}

// destination reads the holder a call names. A location wins when one is given,
// since character_id otherwise falls back to the player and every ground-level
// call would quietly land in the player's pockets instead.
func destination(w *rules.World, characterID, locationID, where string) (*itemDestination, error) {
	if locationID == "" {
		ch, err := targetCharacter(w, characterID)
		if err != nil {
			return nil, err
		}
		return &itemDestination{world: w, character: ch}, nil
	}
	if characterID != "" {
		return nil, fmt.Errorf("name one holder: character_id or location, not both")
	}
	loc, err := w.Location(locationID)
	if err != nil {
		return nil, err
	}
	// The spot is not demanded here. Creating something needs one and AddItem
	// says so itself; destroying something does not, because the name alone
	// usually finds it and the lookup asks for a spot only when two stacks of
	// that name are lying about.
	return &itemDestination{world: w, location: loc, where: where}, nil
}

// add puts a newly created stack where the call said. The character's state is
// returned with it, as every mutating call does; a location has no sheet, so its
// half reports the spot instead.
func (d *itemDestination) add(proto *rules.Item, count int) (string, error) {
	// Minted here rather than in each of the three calls that build a proto —
	// add_item, add_weapon, add_armor — since this is where all three converge and
	// where the world is in hand. An unminted proto is refused outright by Accept
	// and AddItem, so a route that forgets is caught rather than producing an item
	// no later call could address. A proto that merges into a stack already held
	// never spends the id it was given.
	if proto.ID == "" {
		proto.ID = d.world.MintItemID(proto.Name)
	}
	if d.location != nil {
		item, err := d.location.AddItem(proto, d.where, count)
		if err != nil {
			return "", err
		}
		return rules.Flow("location", d.location.Name, "added", count, "item", item.Name, "type", item.Kind(), "lies", item.Where, "total", item.Count), nil
	}
	before := d.character.ToString()
	item, err := d.character.Accept(proto, count)
	if err != nil {
		return "", err
	}
	return withState(rules.Flow("character", d.character.Name, "added", count, "item", item.Name, "type", item.Kind(), "carrying", item.Count), before, d.character), nil
}

// remove destroys a stack, or part of one, wherever it is held.
func (d *itemDestination) remove(name string, count int) (string, error) {
	if d.location != nil {
		item, left, err := d.location.RemoveItem(name, d.where, count)
		if err != nil {
			return "", err
		}
		return rules.Flow("location", d.location.Name, "removed", count, "item", item.Name, "remaining", left), nil
	}
	before := d.character.ToString()
	item, left, err := d.character.RemoveItem(name, count)
	if err != nil {
		return "", err
	}
	return withState(rules.Flow("character", d.character.Name, "removed", count, "item", item.Name, "carrying", left), before, d.character), nil
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
		return withState(rules.Flow("character", ch.Name, "took", severity, "now", result, "scratches", ch.Wounds.Scratches, "penalty", ch.Wounds.Penalty()), before, ch), nil
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
		return withState(rules.Flow("character", ch.Name, "healed", lvl, "now", result, "level", ch.Wounds.Level, "penalty", ch.Wounds.Penalty()), before, ch), nil
	case "declare_dead":
		ch, _, err := getCharacterFromJSON(w, call.Function.Arguments)
		if err != nil {
			return "", err
		}
		before := ch.ToString()
		ch.Wounds.DeclareDead()
		return withState(rules.Flow("character", ch.Name, "status", "DEAD"), before, ch), nil
	case "create_location":
		var args struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Parent      string `json:"parent"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse create_location: %w", err)
		}
		loc, err := w.CreateLocation(args.Name, args.Description, args.Parent)
		if err != nil {
			return "", err
		}
		line := fmt.Sprintf("%s|id:%s", loc.Name, loc.ID)
		if loc.Parent != "" {
			line += "|in:" + loc.Parent
		}
		return line, nil
	case "move_character":
		var args struct {
			CharacterID string `json:"character_id"`
			Location    string `json:"location"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse move_character: %w", err)
		}
		ch, err := w.MoveCharacter(args.CharacterID, args.Location)
		if err != nil {
			return "", err
		}
		return rules.Flow("character", ch.Name, "now_at", ch.Location), nil
	case "move_item":
		var args struct {
			Item          string `json:"item"`
			ToCharacter   string `json:"to_character"`
			ToLocation    string `json:"to_location"`
			Where         string `json:"location_description"`
			FromCharacter string `json:"from_character"`
			FromLocation  string `json:"from_location"`
			FromWhere     string `json:"from_location_description"`
			Count         int    `json:"count"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse move_item: %w", err)
		}
		res, err := w.MoveItem(rules.MoveRequest{
			Item:          args.Item,
			ToCharacter:   args.ToCharacter,
			ToLocation:    args.ToLocation,
			Where:         args.Where,
			FromCharacter: args.FromCharacter,
			FromLocation:  args.FromLocation,
			FromWhere:     args.FromWhere,
			Count:         args.Count,
		})
		if err != nil {
			return "", err
		}
		line := rules.Flow("moved", res.Count, "item", res.Item.Name, "from", res.From, "to", res.To, "lies", res.Item.Where)
		// Both sheets moved, but only the receiving one is stated: a move is one
		// fact, and the roster restates the other side next turn anyway.
		if args.ToCharacter != "" {
			if ch, err := w.Character(args.ToCharacter); err == nil {
				line += "\n" + ch.ToString()
			}
		}
		return line, nil
	case "add_item", "remove_item":
		var args struct {
			CharacterID string `json:"character_id"`
			Location    string `json:"location"`
			Where       string `json:"location_description"`
			Name        string `json:"name"`
			Description string `json:"description"`
			Count       int    `json:"count"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse %s: %w", call.Function.Name, err)
		}
		dest, err := destination(w, args.CharacterID, args.Location, args.Where)
		if err != nil {
			return "", err
		}
		if args.Count == 0 {
			args.Count = 1
		}
		if call.Function.Name == "add_item" {
			return dest.add(&rules.Item{Name: args.Name, Description: args.Description}, args.Count)
		}
		return dest.remove(args.Name, args.Count)
	case "add_weapon":
		var args struct {
			CharacterID     string `json:"character_id"`
			Location        string `json:"location"`
			Where           string `json:"location_description"`
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
		dest, err := destination(w, args.CharacterID, args.Location, args.Where)
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
		proto, err := rules.NewWeaponItem(args.Name, args.Description, rules.Weapon{
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
		return dest.add(proto, args.Count)
	case "add_armor":
		var args struct {
			CharacterID string `json:"character_id"`
			Location    string `json:"location"`
			Where       string `json:"location_description"`
			Name        string `json:"name"`
			Description string `json:"description"`
			Protection  int    `json:"protection"`
			Worn        *bool  `json:"worn"`
			Count       int    `json:"count"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse add_armor: %w", err)
		}
		dest, err := destination(w, args.CharacterID, args.Location, args.Where)
		if err != nil {
			return "", err
		}
		if args.Count == 0 {
			args.Count = 1
		}
		// Armor is worn unless the DM says otherwise: the common case by far is a
		// character who has it on, and armor recorded as cargo protects nobody.
		worn := args.Worn == nil || *args.Worn
		proto, err := rules.NewArmorItem(args.Name, args.Description, rules.Armor{Protection: args.Protection, Worn: worn})
		if err != nil {
			return "", err
		}
		return dest.add(proto, args.Count)
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
		return withState(rules.Flow("character", ch.Name, "transaction", sign+strconv.Itoa(args.Amount), "purse", balance), before, ch), nil
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
		return rules.Flow("table", args.Table, "rolled", roll, "modifier", args.Modifier, "result", value), nil
	default:
		return "", fmt.Errorf("unknown action tool %q", call.Function.Name)
	}
}
