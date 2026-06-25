package bis

import "github.com/amdrake93/eq2-eof-itemdex/internal/store"

// The accessibility-tier candidate filters, shared by the from-scratch tier runs
// (cmd/bis) and the imported-loadout tiered upgrade report (§6, §7). They compose
// as nested supersets: PreRaid ⊂ Raid ⊂ Best.

// BestFilter keeps everything except Hunter's sets and curated exclusions.
func BestFilter(it store.ScorableItem) bool { return !IsHunters(it) && !Curated(it) }

// RaidFilter additionally excludes avatar (MYTHICAL non-Soulfire) gear.
func RaidFilter(it store.ScorableItem) bool { return !IsAvatar(it) && BestFilter(it) }

// PreRaidFilter keeps only LEGENDARY/TREASURED items within the raid-eligible set.
func PreRaidFilter(it store.ScorableItem) bool {
	return (it.Tier == "LEGENDARY" || it.Tier == "TREASURED") && RaidFilter(it)
}
