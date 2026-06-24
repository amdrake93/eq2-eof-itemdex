// Package loadout reads/writes imported-character loadout files and resolves
// equipped items + adornments to stat blocks for the bis sim (SPEC §4, §7).
package loadout

import (
	"os"

	"github.com/BurntSushi/toml"
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
)

// skippedCharSlots are census character-equipment slots whose stats are NOT
// counted on import: food/drink change ad hoc; mount slots aren't player gear.
var skippedCharSlots = map[string]bool{
	"food":            true,
	"drink":           true,
	"mount_adornment": true,
	"mount_armor":     true,
}

// SkipSlot reports whether a census character slot name is excluded from import.
func SkipSlot(charSlot string) bool { return skippedCharSlots[charSlot] }

// File is the on-disk imported loadout (characters/<name>-loadout.toml).
type File struct {
	CharacterName string      `toml:"character_name"`
	LastUpdate    float64     `toml:"last_update"`
	Slots         []SlotEntry `toml:"slots"`
	Unresolved    []string    `toml:"unresolved"` // "charSlot:itemID" census could not resolve
}

// SlotEntry is one equipped slot's resolved data (item base + filled adornments).
type SlotEntry struct {
	CatalogSlot string          `toml:"catalog_slot"` // item's census slot_list name; the Set Equipped key
	CharSlot    string          `toml:"char_slot"`    // census character-equipment slot name
	ItemID      int64           `toml:"item_id"`
	Name        string          `toml:"name"`
	Optimizable bool            `toml:"optimizable"`
	WeaponMin   float64         `toml:"weapon_min,omitempty"`
	WeaponMax   float64         `toml:"weapon_max,omitempty"`
	WeaponDelay float64         `toml:"weapon_delay,omitempty"`
	Stats       model.StatBlock `toml:"stats"`
}

// Write serializes the loadout file as TOML.
func Write(path string, f File) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	return toml.NewEncoder(out).Encode(f)
}

// Read parses a loadout file.
func Read(path string) (File, error) {
	var f File
	_, err := toml.DecodeFile(path, &f)
	return f, err
}
