package model

import (
	"math"

	"github.com/amdrake93/eq2-eof-itemdex/internal/constants"
	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
)

// CAEffectiveDamage is one cast's damage: potency-scaled base + capped ability
// mod, times crit. (Ability-mod cap = 50% of the potency-adjusted MAX damage.)
func CAEffectiveDamage(sb StatBlock, ca spell.CombatArt) float64 {
	pot := 1 + sb.Potency/100
	avgBase := (ca.MinDamage + ca.MaxDamage) / 2 * pot
	capBonus := constants.AbilityModCapFrac * ca.MaxDamage * pot
	bonus := math.Min(sb.AbilityMod, capBonus)
	return (avgBase + bonus) * critFactor(sb)
}

// aaCooldownReduction is the AA that halves the base recast of these two arts.
// Applied to base recast BEFORE the reuse reduction.
var aaCooldownReduction = map[string]float64{
	"Assassinate":  0.5,
	"Mortal Blade": 0.5,
}

func effRecast(sb StatBlock, ca spell.CombatArt) float64 {
	base := ca.RecastSecs
	if cdr, ok := aaCooldownReduction[spell.BaseName(ca.Name)]; ok {
		base *= cdr
	}
	return base * (1 - constants.ReuseHalveCoeff*math.Min(sb.Reuse, constants.ReuseHalvesAt)/100)
}

// RotationCADPS simulates the priority rotation: at each slot it casts the
// off-cooldown art with the highest damage-per-cast-time (eff/slot), since arts
// have different cast times and a slow high-damage art can be worse per second
// than a fast lower-damage one. recoverySecs is the post-cast recovery added to
// each art's own cast time to size its timeline slot; an art with no recorded
// cast time falls back to constants.CACastTimeSecs. Auto-attack runs in parallel
// (modeled separately), so casting does not displace it.
func RotationCADPS(sb StatBlock, cas []spell.CombatArt, durationSecs, recoverySecs float64) float64 {
	if durationSecs <= 0 || len(cas) == 0 {
		return 0
	}
	eff := make([]float64, len(cas))
	rec := make([]float64, len(cas))
	slot := make([]float64, len(cas))
	avail := make([]float64, len(cas))
	for i, ca := range cas {
		eff[i] = CAEffectiveDamage(sb, ca)
		rec[i] = effRecast(sb, ca)
		castSecs := float64(ca.CastSecsHundredths) / 100
		if castSecs <= 0 {
			castSecs = constants.CACastTimeSecs
		}
		slot[i] = castSecs + recoverySecs
	}
	var total, t float64
	for t < durationSecs {
		best, bestRate := -1, -1.0
		for i := range cas {
			if avail[i] <= t {
				if rate := eff[i] / slot[i]; rate > bestRate {
					best, bestRate = i, rate
				}
			}
		}
		if best < 0 {
			soonest := math.Inf(1)
			for i := range cas {
				if avail[i] < soonest {
					soonest = avail[i]
				}
			}
			if math.IsInf(soonest, 1) || soonest <= t {
				break
			}
			t = soonest
			continue
		}
		total += eff[best]
		avail[best] = t + rec[best]
		t += slot[best]
	}
	return total / durationSecs
}
