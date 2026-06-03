package spell

import (
	"regexp"
	"strconv"
	"strings"
)

// damageRe matches "Inflicts 3,011 - 5,018 <type> damage..." capturing min and max.
var damageRe = regexp.MustCompile(`Inflicts\s+([\d,]+)\s*-\s*([\d,]+)\s+\w+\s+damage`)

func toFloat(s string) float64 {
	v, _ := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64) //nolint:errcheck // input is regex-validated digits/commas
	return v
}

// ParseDamage scans effect descriptions for the first damage line and returns
// its min/max. ok is false when no damage line is present (buff/utility art).
func ParseDamage(effects []string) (min, max float64, ok bool) {
	for _, e := range effects {
		if m := damageRe.FindStringSubmatch(e); m != nil {
			return toFloat(m[1]), toFloat(m[2]), true
		}
	}
	return 0, 0, false
}
