package classify

import (
	"time"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
)

const VarsoonWorldID = 614

// EoF unlock window on Varsoon (content-gated by expansion-exclusive collectables):
// lower bound = White Oak Acorn (EoF-exclusive); upper bound = Tuft of Dark Brown
// Brute Fur (RoK-exclusive), exclusive.
var (
	EoFStart = time.Date(2023, 4, 11, 0, 0, 0, 0, time.UTC)
	EoFEnd   = time.Date(2023, 8, 8, 0, 0, 0, 0, time.UTC)
	KoSStart = time.Date(2022, 12, 11, 0, 0, 0, 0, time.UTC)
	KoSEnd   = EoFStart
)

// VarsoonDiscovery returns the first-discovery time on Varsoon (world 614), if any.
func VarsoonDiscovery(it census.Item) (time.Time, bool) {
	for _, w := range it.Extended.Discovered.Worlds {
		if w.ID == VarsoonWorldID {
			return time.Unix(int64(w.Timestamp), 0).UTC(), true
		}
	}
	return time.Time{}, false
}

func inWindow(t, start, end time.Time) bool {
	return !t.Before(start) && t.Before(end)
}

// IsEoF reports whether the item's Varsoon first-discovery falls in the EoF window.
func IsEoF(it census.Item) bool {
	t, ok := VarsoonDiscovery(it)
	return ok && inWindow(t, EoFStart, EoFEnd)
}

// IsKoS reports whether the item's Varsoon first-discovery falls in the KoS window.
func IsKoS(it census.Item) bool {
	t, ok := VarsoonDiscovery(it)
	return ok && inWindow(t, KoSStart, KoSEnd)
}
