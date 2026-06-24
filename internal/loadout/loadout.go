// Package loadout reads/writes imported-character loadout files and resolves
// equipped items + adornments to stat blocks for the bis sim (SPEC §4, §7).
package loadout

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
