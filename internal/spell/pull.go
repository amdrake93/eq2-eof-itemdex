package spell

import (
	"context"
	"fmt"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
)

// assassinClassID is the Assassin's class id in the SPELL collection (40);
// note it differs from the item collection's class id (15) — a Census quirk.
const assassinClassID = 40

const caShowFields = "name,level,tier_name,type,beneficial,cast_secs_hundredths,recast_secs,classes,effect_list"

// minDamageArtLevel is the level floor for a real rotation art — a level-70
// assassin's damaging abilities are all level 57+; below that is vestigial
// low-level filler that never scales.
const minDamageArtLevel = 57

// CombatArt is a damaging Assassin ability with the fields the DPS model needs.
type CombatArt struct {
	Name               string
	Level              int
	MinDamage          float64
	MaxDamage          float64
	RecastSecs         float64
	CastSecsHundredths int
	RecastReduction    float64 // per-art AA recast reduction (1 − recast_mult), set from character config; counts against the shared 50% ceiling
	PotencyAdd         float64 // per-art AA potency rider (config [art_mods]), pooled with potency
}

// AssassinCombatArts pulls the Assassin's Expert-tier combat arts usable by
// level 70, keeping only damaging ones (a parseable effect_list damage line).
func AssassinCombatArts(ctx context.Context, c *census.Client) ([]CombatArt, error) {
	query := fmt.Sprintf(
		"classes.assassin.id=%d&type=arts&tier_name=Expert&level=%%3C71&c:limit=500&c:show=%s",
		assassinClassID, caShowFields)
	body, err := c.Get(ctx, "get", "spell", query)
	if err != nil {
		return nil, err
	}
	spells, err := DecodeSpells(body)
	if err != nil {
		return nil, err
	}
	return FilterCombatArts(spells), nil
}

// FilterCombatArts keeps only the arts an assassin presses in a melee rotation:
// level 57+ (below that is vestigial low-level filler), damaging (parseable
// damage line), and not a buff (beneficial == 0). Ranged bow shots ARE kept —
// they fire with no minimum range and don't cost melee auto-attacks, so they're
// free bonus CA damage that fills the rotation's idle time.
func FilterCombatArts(spells []Spell) []CombatArt {
	var arts []CombatArt
	for _, s := range spells {
		if s.Level < minDamageArtLevel {
			continue
		}
		if s.Beneficial != 0 {
			continue
		}
		min, max, ok := ParseDamage(effectStrings(s.Effects))
		if !ok {
			continue
		}
		arts = append(arts, CombatArt{
			Name:               s.Name,
			Level:              s.Level,
			MinDamage:          min,
			MaxDamage:          max,
			RecastSecs:         s.RecastSecs,
			CastSecsHundredths: s.CastSecsHundredths,
		})
	}
	return arts
}

func effectStrings(effs []Effect) []string {
	out := make([]string, len(effs))
	for i, e := range effs {
		out[i] = e.Description
	}
	return out
}
