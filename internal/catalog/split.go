package catalog

import "github.com/amdrake93/eq2-eof-itemdex/internal/census"

// maxLifeKeys are the Census modifier keys that represent maximum health.
var maxLifeKeys = []string{"maxhpperc", "health", "hp", "maxhealth"}

// SplitByCategory groups items by their first slot's category.
func SplitByCategory(items []census.Item) map[string][]census.Item {
	groups := map[string][]census.Item{}
	for _, it := range items {
		slot := ""
		if len(it.Slots) > 0 {
			slot = it.Slots[0].Name
		}
		cat := CategoryForSlot(slot)
		groups[cat] = append(groups[cat], it)
	}
	return groups
}

// WithMaxLife returns items carrying any maximum-health modifier (any class).
func WithMaxLife(items []census.Item) []census.Item {
	var out []census.Item
	for _, it := range items {
		for _, k := range maxLifeKeys {
			if _, ok := it.Modifiers[k]; ok {
				out = append(out, it)
				break
			}
		}
	}
	return out
}
