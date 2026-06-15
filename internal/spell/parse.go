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

// dmgLineRe matches an "Inflicts ..." damage description: amount (single or
// range), damage type, target scope, and an optional periodic clause. The AoE
// scope alternative is listed first so it wins over the "target" prefix.
var dmgLineRe = regexp.MustCompile(
	`^Inflicts\s+([\d,]+)(?:\s*-\s*([\d,]+))?\s+(\w+)\s+damage\s+on\s+(targets in Area of Effect|target)(?:\s+(instantly and every|every)\s+([\d.]+)\s+seconds)?`)

// damageLine is a parsed "Inflicts ..." description before kind/context resolution.
type damageLine struct {
	min, max     float64
	dmgType      string
	aoe          bool
	periodic     bool // has an "every N seconds" clause
	hasInstant   bool // "instantly and every" (vs bare "every")
	intervalSecs float64
}

// parseDamageLine extracts a damageLine from one effect description. ok is false
// for non-damage lines (buffs, termination/proc descriptors, conditions).
func parseDamageLine(desc string) (damageLine, bool) {
	m := dmgLineRe.FindStringSubmatch(desc)
	if m == nil {
		return damageLine{}, false
	}
	dl := damageLine{
		min:     toFloat(m[1]),
		dmgType: m[3],
		aoe:     strings.HasPrefix(m[4], "targets"),
	}
	if m[2] != "" {
		dl.max = toFloat(m[2])
	} else {
		dl.max = dl.min
	}
	if m[5] != "" {
		dl.periodic = true
		dl.hasInstant = m[5] == "instantly and every"
		dl.intervalSecs = toFloat(m[6])
	}
	return dl, true
}
