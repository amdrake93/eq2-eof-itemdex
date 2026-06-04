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

// RotationCADPS simulates a priority rotation over durationSecs: at each cast
// slot (slotSecs = cast time + recovery apart) it fires the highest-effective-
// damage art that is off cooldown; cooldowns are reuse-reduced; when nothing is
// castable it jumps to the next availability. Auto-attack runs in parallel and
// is modeled separately, so casting does not reduce auto throughput.
func RotationCADPS(sb StatBlock, cas []spell.CombatArt, durationSecs, slotSecs float64) float64 {
	if durationSecs <= 0 || slotSecs <= 0 || len(cas) == 0 {
		return 0
	}
	eff := make([]float64, len(cas))
	rec := make([]float64, len(cas))
	avail := make([]float64, len(cas))
	for i, ca := range cas {
		eff[i] = CAEffectiveDamage(sb, ca)
		rec[i] = effRecast(sb, ca)
	}
	var total, t float64
	for t < durationSecs {
		best, bestDmg := -1, -1.0
		for i := range cas {
			if avail[i] <= t && eff[i] > bestDmg {
				best, bestDmg = i, eff[i]
			}
		}
		if best < 0 { // nothing off cooldown — jump to soonest availability
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
		total += bestDmg
		avail[best] = t + rec[best]
		t += slotSecs
	}
	return total / durationSecs
}
