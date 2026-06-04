package spell

import (
	"regexp"
	"strings"
)

// rankSuffix matches a trailing space + roman numeral or arabic digits (the rank).
var rankSuffix = regexp.MustCompile(` (?:[IVXLCDM]+|\d+)$`)

// BaseName strips a trailing rank suffix ("Mortal Blade IV" -> "Mortal Blade").
func BaseName(name string) string {
	return strings.TrimSpace(rankSuffix.ReplaceAllString(name, ""))
}

// HighestRanks collapses combat arts to the highest-MaxDamage version per base
// name (all ranks of a line share one cooldown, so only the top rank is cast).
func HighestRanks(cas []CombatArt) []CombatArt {
	best := map[string]CombatArt{}
	for _, c := range cas {
		b := BaseName(c.Name)
		if cur, ok := best[b]; !ok || c.MaxDamage > cur.MaxDamage {
			best[b] = c
		}
	}
	out := make([]CombatArt, 0, len(best))
	for _, c := range best {
		out = append(out, c)
	}
	return out
}
