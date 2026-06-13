package model

import (
	"math"

	"github.com/amdrake93/eq2-eof-itemdex/internal/constants"
	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
)

// CAEffectiveDamage is one cast's damage under the measured equation (spec
// §3.1, tooltip-calibrated 2026-06-12 across 4 gear/AA states × 3 probe arts):
// the potency pool (displayed potency + the calibrated PotencyBonus + the
// art's AA rider) and the main-stat curve each multiply the base; ability mod
// adds IN FULL (the old 50%-of-adjusted-base cap is disproven — Quick Strike
// at AM 738 tooltips the whole add). A small measured per-art enhancer
// (≈ AM × base_max/3400) is documented, not modeled. The PotencyBonus
// component is calibrated, its source unexplained — spec §12 'potency-pool
// mystery'.
func CAEffectiveDamage(sb StatBlock, ca spell.CombatArt) float64 {
	potPool := 1 + (sb.Potency+sb.PotencyBonus+ca.PotencyAdd)/100
	mainStat := 1 + MainStatEffect(sb.MainStat)/100
	avgBase := (ca.MinDamage + ca.MaxDamage) / 2 * potPool * mainStat
	return (avgBase + sb.AbilityMod) * critFactor(sb)
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

// fightSmoothingSamples is K: how many fight lengths CADPS averages across the
// recast-wide window to smooth cast-boundary quantization (spec §3.1).
const fightSmoothingSamples = 9

// rotationTimeline runs the priority sim out to maxLen, recording each fired
// cast's start time and the cumulative CA damage through it. The sim is
// prefix-consistent — a fight of length s credits exactly the casts with start
// time < s — so one run yields cumCA(t) for every t ≤ maxLen.
func rotationTimeline(sb StatBlock, cas []spell.CombatArt, maxLen float64) (starts, cum []float64) {
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
	for t < maxLen {
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
		starts = append(starts, t)
		cum = append(cum, total)
		avail[best] = t + rec[best]
		t += slot[best]
	}
	return starts, cum
}

// cumCAAt is the cumulative CA damage from casts started strictly before s.
func cumCAAt(starts, cum []float64, s float64) float64 {
	out := 0.0
	for i, st := range starts {
		if st < s {
			out = cum[i]
		} else {
			break
		}
	}
	return out
}

// maxEffRecast is the longest effective recast in the art set — the smoothing
// window width R (one full big-cast boundary cycle).
func maxEffRecast(sb StatBlock, cas []spell.CombatArt) float64 {
	r := 0.0
	for _, ca := range cas {
		if er := effRecast(sb, ca); er > r {
			r = er
		}
	}
	return r
}

// RotationCADPS is total CA damage over a single fixed fight length / that
// length (unsmoothed). Retained for direct single-length tests.
func RotationCADPS(sb StatBlock, cas []spell.CombatArt, durationSecs float64) float64 {
	if durationSecs <= 0 || len(cas) == 0 {
		return 0
	}
	starts, cum := rotationTimeline(sb, cas, durationSecs)
	return cumCAAt(starts, cum, durationSecs) / durationSecs
}
