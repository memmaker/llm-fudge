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
	return append(tools, creationInventoryTools()...)
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

func creationInventoryTools() []llm.Tool {
	characterID, location, locationDescription := itemPlacementArgs()
	return []llm.Tool{
		addWeaponTool(),
		addArmorTool(),
		{Type: "function", Function: llm.ToolFunction{Name: "equip_armor", Description: "Put armor on or take it off. Armor is already worn when you add it, so this is for the moments the fiction turns on it: stripping a corpse and shrugging into what it wore, peeling off a soaked coat, being stripped of a shield by captors. Armor that is not worn protects nobody.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": characterID, "name": map[string]any{"type": "string", "description": "The armor's name as it was added."}, "worn": map[string]any{"type": "boolean", "description": "True to put it on, false to take it off."}}, "required": []string{"name", "worn"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "add_item", Description: "Bring an item into the world: starting equipment at creation, a purchase, a reward, a gift from off-screen, something found that was never recorded before. It goes into a character's hands, or—with location and location_description—onto the ground somewhere. An item they already carry stacks onto the entry they have, so use the same name you used before. The description is what the player reads on their sheet, so write one concrete line about this particular thing—what it is, what shape it is in, where it came from—not a catalogue entry. This is for outfitting a character who is being rolled up: during play, items are created and moved by declaring them in the turn's <state> block. Items are recorded here and nowhere else: an object you only narrate does not exist.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": characterID, "location": location, "location_description": locationDescription, "name": map[string]any{"type": "string", "description": "The item's name as the player will read it, e.g. \"Blaster Pistol\". Name the thing itself, without the count."}, "description": map[string]any{"type": "string", "description": "One short concrete sentence describing this item."}, "count": map[string]any{"type": "integer", "description": "How many are gained. Defaults to 1."}}, "required": []string{"name", "description"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "add_coins", Description: "Credit a character's purse: starting money at creation, a reward, a sale, found treasure. Coins are the setting's ordinary currency, whatever it is called there.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": characterID, "amount": map[string]any{"type": "integer", "description": "How many coins are gained. Must be positive."}}, "required": []string{"amount"}}}},
	}
}

// attackTools resolve a blow from the roll through to the wound in one call.
// Everything they need is already on the sheets — the weapon's rating, the arm
// behind it, the armor in the way, both Scales — so the DM names the traits and
// the circumstances and the engine does the rest, rather than rolling a check and
// then guessing at a severity.
//
// The descriptions say what an argument means and nothing else. What a call is
// for, when to reach for it, and what to do with what comes back are doctrine,
// and doctrine is in the system prompt: it was in both for a while, and the
// duplicate copy was the single largest thing in a request after the prompt
// itself — paid on every turn, to say again what the model had already read.
func attackTools() []llm.Tool {
	attackerID := map[string]any{"type": "string", "description": "Who attacks, by character id."}
	traitType := map[string]any{"type": "string", "enum": []string{"attribute", "skill"}}
	attackTrait := map[string]any{"type": "string", "description": "The skill or attribute they attack with."}
	weapon := map[string]any{"type": "string", "description": "The weapon, by its carried name. Must be on their sheet with a weapon rating. Omit for bare-handed."}
	thrown := map[string]any{"type": "boolean", "description": "Thrown, so it leaves their hand. Pick a throwing skill."}
	addOne := map[string]any{"type": "boolean", "description": "Spend a Fudge Point for +1."}
	reroll := map[string]any{"type": "boolean", "description": "Spend a Fudge Point to roll twice and keep the better."}
	modifier := map[string]any{"type": "integer", "description": "Circumstances, -4 to +4. Never wound penalties."}

	return []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "opposed_attack", Description: "Strike at someone fighting back. Rolls, decides whether the blow landed, works weapon, armor and both Scales into a wound, and applies it. If the opponent does not exist yet, call create_character first in the same response.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"attacker_id": attackerID, "attacker_trait_type": traitType, "attacker_trait_slug": attackTrait, "attacker_modifier": modifier, "defender_id": map[string]any{"type": "string", "description": "Who is attacked, by character id."}, "defender_trait_type": traitType, "defender_trait_slug": map[string]any{"type": "string", "description": "How they defend: parry, dodge, shield, or weapon skill."}, "defender_modifier": modifier, "weapon": weapon, "thrown": thrown, "add_one": addOne}, "required": []string{"attacker_id", "attacker_trait_type", "attacker_trait_slug", "defender_id", "defender_trait_type", "defender_trait_slug"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "unopposed_attack", Description: "Strike at someone who cannot resist: unaware, asleep, bound, already down. Same one-call resolution as opposed_attack but against a difficulty instead of a defence. Use opposed_attack the moment the target can do anything about it.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"attacker_id": attackerID, "attacker_trait_type": traitType, "attacker_trait_slug": attackTrait, "attacker_modifier": modifier, "target_id": map[string]any{"type": "string", "description": "Who is attacked, by character id."}, "difficulty": map[string]any{"type": "string", "enum": rules.DifficultyNames(), "description": "How hard the blow is. Defaults to very easy."}, "weapon": weapon, "thrown": thrown, "add_one": addOne, "reroll": reroll}, "required": []string{"attacker_id", "attacker_trait_type", "attacker_trait_slug", "target_id"}}}},
	}
}

// reloadWeaponTool is built on its own because it is on both sides of the turn:
// a gun has to be loaded before it can be fired, which is the consult phase, and
// a magazine slapped home in the prose has to be written down, which is the
// bookkeeping phase.
func reloadWeaponTool() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.ToolFunction{Name: "reload_weapon", Description: "Reload a ranged weapon, spending one of the ammunition items it feeds on. It costs the character their action in the fiction. Fails when they carry no ammunition left.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"character_id": map[string]any{"type": "string", "description": "Whose weapon is reloaded. Omit to mean the player character."}, "weapon": map[string]any{"type": "string", "description": "The weapon's name as it is carried."}}, "required": []string{"weapon"}}}}
}

// createCharacterTool mints an NPC mid-turn, and it survives the state block for
// one reason: a check cannot name somebody who does not exist, so an NPC the
// player has just turned on has to be created in the same response the roll is
// made in. That is the rule for every tool still on this list — a tool exists
// when a query depends on the state it sets. Everything else waits for the
// state block, where it is declared rather than commanded.
func createCharacterTool() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.ToolFunction{Name: "create_character", Description: "Create an NPC mid-turn, only when a check or attack in this same response is about to name them. An NPC who merely walks into your prose needs no call: declare them in the <state> block instead. Traits must come from the Setting Information list.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string", "description": "The id they keep permanently, e.g. \"merchant-alric\". Must not already be in WORLD STATE."}, "name": map[string]any{"type": "string"}, "species": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}, "location": map[string]any{"type": "string"}, "attributes": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, "skills": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, "gifts": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "faults": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, "required": []string{"id", "name", "species"}}}}
}

// ================================= Queries =================================
//
// What is left of the tool interface, and the whole of what a turn resolves
// with. Every one of these answers a question the DM cannot answer itself — how
// the dice fell, what a table gave, what an NPC said six turns ago, which option
// the player picked — so information runs engine to DM, and they have to come
// before the prose: a roll made afterwards is a roll the prose already decided.
//
// Nothing that merely writes something down is here any more. A declarative
// block has no way to hand a result back, which is exactly why these stayed
// while the bookkeeping calls went: they are the calls whose answer the next
// token depends on.
func queryTools() []llm.Tool {
	tools := []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "unopposed_check", Description: "Roll an uncertain action nothing is actively resisting: climbing a wall, picking a lock, recalling lore. If an opponent resists it, use opposed_check instead.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"actor_id": map[string]any{"type": "string"}, "trait_type": map[string]any{"type": "string", "enum": []string{"attribute", "skill"}}, "trait_slug": map[string]any{"type": "string"}, "difficulty": map[string]any{"type": "string", "enum": rules.DifficultyNames(), "description": "How hard the task is."}, "situational_modifier": map[string]any{"type": "integer", "description": "Circumstances, roughly -4 to +4, 0 the common case."}, "add_one": map[string]any{"type": "boolean", "description": "Spend one Fudge Point for +1."}, "reroll": map[string]any{"type": "boolean", "description": "Spend one Fudge Point to roll twice and keep the better."}}, "required": []string{"actor_id", "trait_type", "trait_slug", "difficulty"}}}},
		createCharacterTool(),
		{Type: "function", Function: llm.ToolFunction{Name: "opposed_check", Description: "Resolve a conflict between two characters that is not a blow: a grapple, a chase, a deception, a stare-down, resisting a spell. Never wounds anybody; for an actual attack use opposed_attack. If an NPC does not exist yet, call create_character first in the same response.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"actor_id": map[string]any{"type": "string", "description": "Whoever initiated."}, "actor_trait_type": map[string]any{"type": "string", "enum": []string{"attribute", "skill"}}, "actor_trait_slug": map[string]any{"type": "string"}, "actor_modifier": map[string]any{"type": "integer"}, "opponent_id": map[string]any{"type": "string", "description": "Whoever reacts."}, "opponent_trait_type": map[string]any{"type": "string", "enum": []string{"attribute", "skill"}}, "opponent_trait_slug": map[string]any{"type": "string"}, "opponent_modifier": map[string]any{"type": "integer"}, "add_one": map[string]any{"type": "boolean"}}, "required": []string{"actor_id", "actor_trait_type", "actor_trait_slug", "opponent_id", "opponent_trait_type", "opponent_trait_slug"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "roll_on_table", Description: "Roll for an outcome no character is choosing, rather than deciding it yourself. npc-reaction: how an uncharacterized NPC takes the party. npc-morale: whether an NPC presses on, wavers or breaks. weather: conditions outdoors. situational-roll: how luck breaks with no actor to check.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"table": map[string]any{"type": "string", "enum": rules.TableNames()}, "modifier": map[string]any{"type": "integer", "description": "Whatever the fiction says should tilt it, roughly -4 to +4."}}, "required": []string{"table"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "recall", Description: "Look up a turn from earlier in this session, for something you cannot see above: what an NPC actually said, the terms of a deal, what a room looked like the first time.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"entity": map[string]any{"type": "string", "description": "An entity id from WORLD STATE, returning every turn it took part in. The sharpest question here. The id exactly, not a name."}, "query": map[string]any{"type": "string", "description": "Words to search across input, records and narration. For subjects rather than entities."}, "turn": map[string]any{"type": "integer", "description": "A turn number, e.g. the 12 from an @t12 marker."}, "to": map[string]any{"type": "integer", "description": "End of a turn range. Requires turn."}, "include_prose": map[string]any{"type": "boolean", "description": "Return the narration itself. Costly: ask only when the wording matters."}, "limit": map[string]any{"type": "integer", "description": "How many turns to return. Defaults to a few."}}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "user_choice", Description: "Offer the player 2-4 mutually exclusive options, only when you cannot narrate forward without knowing which they take and the moment is worth stopping for. Not a generic \"What do you do?\". Write the prose that poses the question in the same response; the menu is shown underneath it, and their answer comes back as this tool's result." + playerLanguageNote("The options are"), Parameters: map[string]any{"type": "object", "properties": map[string]any{"options": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "2-4 mutually exclusive options. Never a 'Type your answer' one; it is added automatically."}}, "required": []string{"options"}}}},
		reloadWeaponTool(),
	}

	return append(tools, attackTools()...)
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
		return rules.Flow("location", d.location.ID, "added", count, "item", item.ID, "type", item.Kind(), "lies", item.Where, "total", item.Count), nil
	}
	before := d.character.ToString()
	item, err := d.character.Accept(proto, count)
	if err != nil {
		return "", err
	}
	return withState(rules.Flow("character", d.character.ID, "added", count, "item", item.ID, "type", item.Kind(), "carrying", item.Count), before, d.character), nil
}

// remove destroys a stack, or part of one, wherever it is held.
func (d *itemDestination) remove(name string, count int) (string, error) {
	if d.location != nil {
		item, left, err := d.location.RemoveItem(name, d.where, count)
		if err != nil {
			return "", err
		}
		return rules.Flow("location", d.location.ID, "removed", count, "item", item.ID, "remaining", left), nil
	}
	before := d.character.ToString()
	item, left, err := d.character.RemoveItem(name, count)
	if err != nil {
		return "", err
	}
	return withState(rules.Flow("character", d.character.ID, "removed", count, "item", item.ID, "carrying", left), before, d.character), nil
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
// means "as you were last told", the same convention the state block uses.
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
		successStr := "no"
		if result.Success {
			successStr = "ok"
		}
		// A rerolled check names the roll it threw away, so the DM can narrate the
		// Fudge Point doing something: the fumble that was, and then was not.
		parts := []any{
			"actor", actor.Name,
			"roll", result.Roll,
		}
		if result.Rerolled {
			parts = append(parts, "rerolled", result.DiscardedRoll)
		}
		parts = append(parts,
			"total", result.Total,
			"vs", args.Difficulty,
			"result", successStr,
		)
		if result.CritSuccess {
			parts = append(parts, "crit", "success")
		} else if result.CritFailure {
			parts = append(parts, "crit", "failure")
		}
		return withState(rules.Flow(parts...), before, actor), nil
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
		outcomeStr := "tie"
		if result.ActorWins {
			outcomeStr = actor.Name
		} else {
			outcomeStr = opponent.Name
		}
		parts := []any{
			"actor", actor.Name,
			"actor_total", result.ActorTotal,
			"opponent", opponent.Name,
			"opponent_total", result.OpponentTotal,
			"degree", fmt.Sprintf("deg%+d", result.RelativeDegree),
			"winner", outcomeStr,
		}
		if result.ActorCrit {
			parts = append(parts, "crit", actor.Name)
		} else if result.OpponentCrit {
			parts = append(parts, "crit", opponent.Name)
		}
		// A wound on either side arrives through apply_damage, not from here.
		return withState(rules.Flow(parts...), before, actor), nil
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
			ID          string         `json:"id"`
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
			// Empty during character creation, where the engine still derives the
			// id from the name: the player character is minted before there is a
			// DM reading ids off a block.
			ID:          req.ID,
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
	case "add_item":
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
		return dest.add(&rules.Item{Name: args.Name, Description: args.Description}, args.Count)
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
		if args.AmmoItem != "" && !rules.ValidID(args.AmmoItem) {
			return "", fmt.Errorf("ammo_item %q is not a valid item id — lowercase words joined by single hyphens, e.g. revolver-rounds", args.AmmoItem)
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
		return withState(rules.Flow("character", ch.Name, "armor", item.Name, "state", state), before, ch), nil
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
		parts := []any{
			"character", ch.Name,
			"reloaded", item.Name,
			"ammo", fmt.Sprintf("%d/%d", item.Weapon.Ammo, item.Weapon.AmmoCapacity),
		}
		if ammoLeft >= 0 {
			parts = append(parts, "spare_ammo_left", ammoLeft)
		}
		return withState(rules.Flow(parts...), before, ch), nil
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
	case "add_coins":
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
		balance, err := ch.AddCoins(args.Amount)
		if err != nil {
			return "", err
		}
		return withState(rules.Flow("character", ch.Name, "transaction", "+"+strconv.Itoa(args.Amount), "purse", balance), before, ch), nil
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

// entityArgs names every argument of a query tool that carries an entity id. It
// is what keeps the archive an entity index rather than a pile of text: a call
// already says which entities it is about — it could not be made otherwise — so
// the ids are collected as it goes by, and the record can be asked "what has
// happened to this merchant" instead of "does the word merchant appear
// anywhere". The state block's ids need no such list; the applier holds them.
var entityArgs = []string{
	"actor_id", "opponent_id", "attacker_id", "defender_id", "target_id",
	"character_id", "id", "location",
}

// toolEntities reads the entity ids out of a tool call's arguments. Malformed
// arguments come back empty rather than as an error: a record thinner than it
// could be is a far smaller problem than a turn that fails over its own index.
func toolEntities(call llm.ToolCall) []string {
	var raw map[string]any
	if err := json.Unmarshal([]byte(call.Function.Arguments), &raw); err != nil {
		return nil
	}
	seen := map[string]bool{}
	var ids []string
	for _, key := range entityArgs {
		value, ok := raw[key].(string)
		if !ok {
			continue
		}
		id := strings.ToLower(strings.TrimSpace(value))
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}
