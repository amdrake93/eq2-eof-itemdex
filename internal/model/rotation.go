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

// effRecast applies the measured recast rules: every reduction source (per-art
// AA mods + the reuse stat at 1%/pt, stat-capped at 50) shares one per-art
// ceiling of 50% of base. An AA-halved art (Assassinate, Mortal Blade) arrives
// with the ceiling already full, so reuse does nothing for it.
func effRecast(sb StatBlock, ca spell.CombatArt) float64 {
	reduction := ca.RecastReduction + math.Min(sb.Reuse, constants.ReuseCapStat)/100
	return ca.RecastSecs * (1 - math.Min(constants.RecastReductionCeiling, reduction))
}

// slotSecs is the CA-timeline slot one cast occupies: cast time divided by the
// cast-speed stat (measured divisor: Head Shot 2.0s → 1.46s @ 37.4%), plus the
// 0.5s base recovery shrunk subtractively by recovery speed (100 → instant).
func slotSecs(sb StatBlock, ca spell.CombatArt) float64 {
	castSecs := float64(ca.CastSecsHundredths) / 100
	if castSecs <= 0 {
		castSecs = constants.CACastTimeSecs
	}
	effCast := castSecs / (1 + sb.CastSpeed/100)
	effRecovery := constants.CARecoveryBaseSecs * (1 - math.Min(sb.RecoverySpeed, 100)/100)
	return effCast + effRecovery
}

// RotationCADPS simulates the priority rotation: at each slot it casts the
// off-cooldown art with the highest damage-per-cast-time (eff/slot), since arts
// have different cast times and a slow high-damage art can be worse per second
// than a fast lower-damage one. Slot pacing (cast + recovery) comes from the
// stat block's cast/recovery speeds. Auto-attack runs in parallel (modeled
// separately), so casting does not displace it.
func RotationCADPS(sb StatBlock, cas []spell.CombatArt, durationSecs float64) float64 {
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
		slot[i] = slotSecs(sb, ca)
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
