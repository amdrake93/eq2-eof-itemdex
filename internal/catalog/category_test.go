package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCategoryForSlot(t *testing.T) {
	cases := map[string]string{
		"Primary":   "weapons",
		"Secondary": "weapons",
		"Ranged":    "weapons",
		"Head":      "armor",
		"Chest":     "armor",
		"Forearms":  "armor",
		"Feet":      "armor",
		"Neck":      "jewelry-charms",
		"Ears":      "jewelry-charms",
		"Ring":      "jewelry-charms",
		"Charm":     "jewelry-charms",
		"Waist":     "jewelry-charms",
		"Cloak":     "jewelry-charms",
		"Mount":     "other",
	}
	for slot, want := range cases {
		assert.Equal(t, want, CategoryForSlot(slot), "CategoryForSlot(%q)", slot)
	}
}

func TestArmorType(t *testing.T) {
	// skilltype keys confirmed from live Census probe 2026-06-03:
	//   heavyarmor     → "Plate Armor"
	//   mediumarmor    → "Chain Armor"
	//   lightarmor     → "Leather Armor"
	//   verylightarmor → "Cloth Armor"
	cases := map[string]string{
		"heavyarmor":     "Plate",
		"mediumarmor":    "Chain",
		"lightarmor":     "Leather",
		"verylightarmor": "Cloth",
		"crushing":       "",
	}
	for skill, want := range cases {
		assert.Equal(t, want, ArmorType(skill), "ArmorType(%q)", skill)
	}
}
