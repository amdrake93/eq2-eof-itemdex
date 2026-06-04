package bis

import (
	"strings"

	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
)

// curatedExclusions is a hand-maintained set of item names that are discoverable
// on Varsoon but inaccessible to us, beyond the avatar/Hunter's rules below.
// Append item names here as they are identified.
var curatedExclusions = map[string]bool{}

// IsAvatar reports whether an item is contested-avatar gear: a Mythical that is
// not a Soulfire weapon. (All EoF mythicals are avatar drops except Soulfire.)
func IsAvatar(it store.ScorableItem) bool {
	return it.Tier == "MYTHICAL" && !strings.HasPrefix(it.Name, "Soulfire")
}

// IsHunters reports whether an item is from the inaccessible Hunter's sets
// (Cutthroat/Desert/Sky Hunter's ...).
func IsHunters(it store.ScorableItem) bool {
	return strings.Contains(it.Name, "Hunter's")
}

// Curated reports whether an item is on the hand-maintained exclusion list.
func Curated(it store.ScorableItem) bool {
	return curatedExclusions[it.Name]
}
