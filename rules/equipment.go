package rules

import (
	"fmt"
	"strings"
)

// WeaponItem finds a weapon the character is carrying. It is how an attack turns
// the name the DM wrote into the ratings it fights with, so it refuses anything
// that is not a rated weapon on this particular sheet: a blade left at home, or
// one recorded as plain gear, cannot be swung.
func (c *Character) WeaponItem(name string) (*Item, error) {
	item, err := c.item(name)
	if err != nil {
		return nil, err
	}
	if item.Weapon == nil {
		return nil, fmt.Errorf("%s is %s, not a weapon: record it with add_weapon to fight with it", item.Name, item.Kind())
	}
	return item, nil
}

// ArmorItem finds a piece of armor the character is carrying.
func (c *Character) ArmorItem(name string) (*Item, error) {
	item, err := c.item(name)
	if err != nil {
		return nil, err
	}
	if item.Armor == nil {
		return nil, fmt.Errorf("%s is %s, not armor: record it with add_armor to wear it", item.Name, item.Kind())
	}
	return item, nil
}

func (c *Character) item(name string) (*Item, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("item name is required")
	}
	return c.FindItem(name, "")
}

// BestWornArmor picks the armor that protects the character, which is the
// heaviest piece they are actually wearing. Fudge totals one armor rating into
// the defensive damage factor, not a wardrobe: layering a mail shirt over a
// leather jerkin does not add up, the better of the two simply takes the blow.
// A character wearing nothing returns nil, which the arithmetic reads as zero.
func (c *Character) BestWornArmor() *Item {
	var best *Item
	for _, it := range sortedInventory(c.Inventory) {
		if it.Armor == nil || !it.Armor.Worn {
			continue
		}
		if best == nil || it.Armor.Protection > best.Armor.Protection {
			best = it
		}
	}
	return best
}

// EquipArmor puts a piece of armor on or takes it off. Armor is recorded worn,
// so this is for the moments the fiction turns on it: stripping a corpse,
// shrugging into salvage, being disarmed of a shield.
func (c *Character) EquipArmor(name string, worn bool) (*Item, error) {
	item, err := c.ArmorItem(name)
	if err != nil {
		return nil, err
	}
	item.Armor.Worn = worn
	return item, nil
}

// Reload fills a ranged weapon's magazine, spending one of the ammunition items
// it feeds on. A weapon whose ammunition was never recorded as a separate item
// simply refills — the DM chose not to count rounds, and refusing the reload
// would only strand a gun that has no supply to run out of. The second return
// value is how many ammunition items are left, or -1 when none are tracked.
func (c *Character) Reload(name string) (*Item, int, error) {
	item, err := c.WeaponItem(name)
	if err != nil {
		return nil, 0, err
	}
	wp := item.Weapon
	if !wp.TracksAmmo() {
		return nil, 0, fmt.Errorf("%s holds no magazine to reload", item.Name)
	}
	if wp.Ammo >= wp.AmmoCapacity {
		return nil, 0, fmt.Errorf("%s is already fully loaded (%d/%d)", item.Name, wp.Ammo, wp.AmmoCapacity)
	}
	if wp.AmmoItem == "" {
		wp.Ammo = wp.AmmoCapacity
		return item, -1, nil
	}
	// The ammunition comes off the sheet first: a reload the character cannot
	// supply must leave the magazine as empty as it was.
	_, left, err := c.RemoveItem(wp.AmmoItem, "", 1)
	if err != nil {
		return nil, 0, fmt.Errorf("%s cannot reload %s: %w", c.Name, item.Name, err)
	}
	wp.Ammo = wp.AmmoCapacity
	return item, left, nil
}

// itemDetail renders an item's ratings in condensed notation, empty for plain
// gear. It is what tells the DM that the pistol on the sheet is down to two
// shots, or that the jerkin it keeps describing is not being worn.
func itemDetail(it *Item) string {
	var parts []string
	if wp := it.Weapon; wp != nil {
		reach := "melee"
		if wp.Ranged {
			reach = "ranged"
		}
		parts = append(parts, reach, fmt.Sprintf("dmg%+d", wp.Damage))
		if wp.DamageAttribute != "" {
			parts = append(parts, "+"+wp.DamageAttribute)
		}
		if wp.TracksAmmo() {
			ammo := fmt.Sprintf("ammo:%d/%d", wp.Ammo, wp.AmmoCapacity)
			if wp.Ammo == 0 {
				ammo += " EMPTY"
			}
			parts = append(parts, ammo)
			if wp.AmmoItem != "" {
				parts = append(parts, "feeds:"+wp.AmmoItem)
			}
		}
	}
	if ar := it.Armor; ar != nil {
		parts = append(parts, fmt.Sprintf("armor%+d", ar.Protection))
		if ar.Worn {
			parts = append(parts, "worn")
		} else {
			parts = append(parts, "not-worn")
		}
	}
	return strings.Join(parts, "|")
}

// itemRatings renders an item's ratings for the player's own character sheet, in
// words rather than the notation the DM reads. Plain gear renders as nothing.
func itemRatings(it *Item) string {
	var parts []string
	if wp := it.Weapon; wp != nil {
		reach := "melee"
		if wp.Ranged {
			reach = "ranged"
		}
		parts = append(parts, fmt.Sprintf("%s, damage %+d", reach, wp.Damage))
		if wp.TracksAmmo() {
			parts = append(parts, fmt.Sprintf("ammo %d/%d", wp.Ammo, wp.AmmoCapacity))
		}
	}
	if ar := it.Armor; ar != nil {
		worn := "carried"
		if ar.Worn {
			worn = "worn"
		}
		parts = append(parts, fmt.Sprintf("protection %+d, %s", ar.Protection, worn))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ")
}
