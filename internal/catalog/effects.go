package catalog

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
)

// Proc is a triggered item effect, cataloged but not scored (the deferred proc
// layer reads these; SPEC §16).
type Proc struct {
	Trigger   string
	PerMinute float64
	DmgType   string
	MinDmg    float64
	MaxDmg    float64
	Raw       string
}

// AuditLine records a When-Equipped wording and how the parser classified it.
type AuditLine struct {
	Description string
	Kind        string // "stat" | "proc" | "skip"
	Detail      string
}

// effectStatKey maps an effect stat display-name to the census modifier key the
// pipeline uses. Captured faithfully; the model's modifierToField decides what is
// actually scored (skills/attributes captured here but unused until modeled).
var effectStatKey = map[string]string{
	"haste":                  "attackspeed",
	"multi attack":           "doubleattackchance",
	"crit chance":            "critchance",
	"potency":                "basemodifier",
	"dps":                    "dps",
	"flurry":                 "flurry",
	"ability modifier":       "all",
	"reuse":                  "spelltimereusepct",
	"casting speed":          "spelltimecastpct",
	"agility":                "strength",
	"all primary attributes": "strength",
	"combat skills":          "combatskills",
	"slashing":               "slashing",
	"piercing":               "piercing",
	"crushing":               "crushing",
	"ranged":                 "ranged",
	"aggression":             "aggression",
}

var (
	effStatRe  = regexp.MustCompile(`^(Increases|Decreases) (.+?) of caster by ([\d,]+(?:\.\d+)?)\.?$`)
	effPctRe   = regexp.MustCompile(`of caster by [\d,.]+%`)
	procRateRe = regexp.MustCompile(`Triggers about ([\d.]+) times per minute`)
	procDmgRe  = regexp.MustCompile(`Inflicts\s+([\d,]+)\s*-\s*([\d,]+)\s+(\w+)\s+damage`)
	triggerRe  = regexp.MustCompile(`(?i)may cast|Triggers about|On a spell cast|On a successful|when (?:struck|striking)`)
)

func toFloat(s string) float64 {
	f, _ := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	return f
}

// ParseEffects classifies each direct When-Equipped child: a trigger line (and
// its deeper children) becomes a Proc; an unambiguous "Increases|Decreases <stat>
// of caster by N" (no %) becomes a stat keyed by effectStatKey; everything else
// is skipped and logged.
func ParseEffects(effects []census.Effect) (map[string]float64, []Proc, []AuditLine) {
	stats := map[string]float64{}
	var procs []Proc
	var audit []AuditLine

	for i := 0; i < len(effects); i++ {
		e := effects[i]
		if e.Indentation != 1 {
			continue
		}
		desc := strings.TrimSpace(e.Description)

		if triggerRe.MatchString(desc) {
			p := Proc{Trigger: desc, Raw: desc}
			if m := procRateRe.FindStringSubmatch(desc); m != nil {
				p.PerMinute = toFloat(m[1])
			}
			j := i + 1
			for j < len(effects) && effects[j].Indentation > 1 {
				child := strings.TrimSpace(effects[j].Description)
				p.Raw += " | " + child
				if m := procDmgRe.FindStringSubmatch(child); m != nil {
					p.MinDmg, p.MaxDmg, p.DmgType = toFloat(m[1]), toFloat(m[2]), m[3]
				}
				j++
			}
			procs = append(procs, p)
			audit = append(audit, AuditLine{desc, "proc", "trigger"})
			i = j - 1
			continue
		}

		if m := effStatRe.FindStringSubmatch(desc); m != nil && !effPctRe.MatchString(desc) {
			name := strings.ToLower(strings.TrimSpace(m[2]))
			key, ok := effectStatKey[name]
			if !ok {
				audit = append(audit, AuditLine{desc, "skip", "unknown stat: " + name})
				continue
			}
			v := toFloat(m[3])
			if m[1] == "Decreases" {
				v = -v
			}
			stats[key] += v
			audit = append(audit, AuditLine{desc, "stat", key})
			continue
		}

		if effPctRe.MatchString(desc) {
			audit = append(audit, AuditLine{desc, "skip", "percent unit"})
			continue
		}
		audit = append(audit, AuditLine{desc, "skip", "unrecognized"})
	}
	return stats, procs, audit
}
