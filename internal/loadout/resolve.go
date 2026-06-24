package loadout

import (
	"fmt"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
)

// Resolve turns a fetched Character into a loadout File. It is pure: catalogLookup
// returns a cataloged census.Item by id; adornLookup returns an adornment's
// census-keyed stat grants by id; optimizable decides whether a catalog slot is a
// swap candidate. Ids not found are returned in missItems/missAdorns so the caller
// can fetch them and re-run Resolve. Skipped slots (food/drink/mount) and empty
// item sockets (ID 0) are omitted.
func Resolve(
	ch census.Character,
	catalogLookup func(int64) (census.Item, bool),
	adornLookup func(int64) (map[string]float64, bool),
	optimizable func(catalogSlot string) bool,
) (f File, missItems []int64, missAdorns []int64) {
	f.CharacterName = ch.DisplayName
	f.LastUpdate = ch.LastUpdate

	for _, slot := range ch.EquipmentSlots {
		if SkipSlot(slot.Name) || slot.Item.ID == 0 {
			continue
		}
		it, ok := catalogLookup(slot.Item.ID)
		if !ok {
			missItems = append(missItems, slot.Item.ID)
		}

		adornStats := map[string]float64{}
		for _, aid := range slot.Item.FilledAdornmentIDs() {
			as, found := adornLookup(aid)
			if !found {
				missAdorns = append(missAdorns, aid)
				continue
			}
			for k, v := range as {
				adornStats[k] += v
			}
		}

		if !ok {
			continue // can't build a slot entry without the item; caller re-resolves
		}

		catalogSlot := ""
		if len(it.Slots) > 0 {
			catalogSlot = it.Slots[0].Name
		}
		if slot.Name == "secondary" {
			catalogSlot = "Secondary"
		}
		mods := ItemStatGrants(it)
		for k, v := range adornStats {
			mods[k] += v
		}
		var sb model.StatBlock
		sb.AddModifiers(mods)

		f.Slots = append(f.Slots, SlotEntry{
			CatalogSlot: catalogSlot,
			CharSlot:    slot.Name,
			ItemID:      slot.Item.ID,
			Name:        string(it.DisplayName),
			Optimizable: optimizable(catalogSlot),
			WeaponMin:   it.TypeInfo.MinBaseDamage,
			WeaponMax:   it.TypeInfo.MaxBaseDamage,
			WeaponDelay: it.TypeInfo.Delay,
			Stats:       sb,
		})
	}
	return f, missItems, missAdorns
}

// MarkUnresolved records ids the caller still could not fetch after re-resolve, so
// they surface in the file rather than being silently dropped.
func (f *File) MarkUnresolved(label string, ids []int64) {
	for _, id := range ids {
		f.Unresolved = append(f.Unresolved, fmt.Sprintf("%s:%d", label, id))
	}
}
