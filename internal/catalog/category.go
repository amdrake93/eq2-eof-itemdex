package catalog

import "strings"

var slotCategory = map[string]string{
	"primary":   "weapons",
	"secondary": "weapons",
	"ranged":    "weapons",

	"head":      "armor",
	"shoulders": "armor",
	"chest":     "armor",
	"forearms":  "armor",
	"hands":     "armor",
	"legs":      "armor",
	"feet":      "armor",

	"neck":       "jewelry-charms",
	"ears":       "jewelry-charms",
	"ear":        "jewelry-charms",
	"wrist":      "jewelry-charms",
	"wrists":     "jewelry-charms",
	"ring":       "jewelry-charms",
	"finger":     "jewelry-charms",
	"charm":      "jewelry-charms",
	"waist":      "jewelry-charms",
	"belt":       "jewelry-charms",
	"cloak":      "jewelry-charms",
	"cloak/back": "jewelry-charms",
}

// CategoryForSlot maps a slot name to a catalog category. Unmapped slots go to
// "other" so nothing is silently dropped.
func CategoryForSlot(slot string) string {
	if c, ok := slotCategory[strings.ToLower(slot)]; ok {
		return c
	}
	return "other"
}

// armorSkillType maps Census typeinfo.skilltype → armor class.
// Keys are confirmed from live Census probe on 2026-06-03:
//
//	heavyarmor     → "Plate Armor"
//	mediumarmor    → "Chain Armor"
//	lightarmor     → "Leather Armor"
//	verylightarmor → "Cloth Armor"
var armorSkillType = map[string]string{
	"verylightarmor": "Cloth",
	"lightarmor":     "Leather",
	"mediumarmor":    "Chain",
	"heavyarmor":     "Plate",
}

// ArmorType maps a Census typeinfo.skilltype to Cloth/Leather/Chain/Plate.
// Returns "" for non-armor skill types.
func ArmorType(skillType string) string {
	return armorSkillType[strings.ToLower(skillType)]
}
