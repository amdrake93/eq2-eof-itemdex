package model

import (
	"math"

	"github.com/amdrake93/eq2-eof-itemdex/internal/constants"
	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
)

// CAEffectiveDamage is one cast's total damage (spec §3.1, tooltip-calibrated).
// The potency pool (displayed potency + calibrated PotencyBonus + the art's AA
// rider) and the main-stat curve multiply every component's base; crit multiplies
// the total. Arts with parsed Components sum per-component under the measured
// per-mechanic ability-mod rule: a DirectHit takes ability mod IN FULL; DoT ticks
// and Termination take none; a TriggerProc takes half the ability mod per trigger;
// RateProc is not scored. DoT tick count and whether the detonate fires are gated
// by the cadence — a termination art is HELD to its full duration (full ticks +
// detonate); any other DoT is CLIPPED on effRecast (ticks within that window, no
// detonate). Arts with no parsed components use the legacy single damage line
// (full abmod) — keeps existing callers/tests unchanged.
func CAEffectiveDamage(sb StatBlock, ca spell.CombatArt) float64 {
	potPool := 1 + (sb.Potency+sb.PotencyBonus+ca.PotencyAdd)/100
	mainStat := 1 + MainStatEffect(sb.MainStat)/100
	scaling := potPool * mainStat

	if len(ca.Components) == 0 {
		avgBase := (ca.MinDamage + ca.MaxDamage) / 2 * scaling
		return (avgBase + sb.AbilityMod) * critFactor(sb)
	}

	hold := hasTermination(ca)
	window := ca.DurationSecs
	if !hold {
		window = math.Min(effRecast(sb, ca), ca.DurationSecs)
	}

	var total float64
	for _, c := range ca.Components {
		base := (c.MinDamage + c.MaxDamage) / 2 * scaling
		switch c.Kind {
		case spell.DirectHit:
			total += base + sb.AbilityMod
		case spell.DoT:
			total += base * dotTicks(c, window)
		case spell.Termination:
			if hold { // detonate fires only when the DoT runs to termination
				total += base
			}
		case spell.TriggerProc:
			total += (base + 0.5*sb.AbilityMod) * float64(c.Triggers)
		case spell.RateProc:
			// deferred — proc-rate scoring not modeled (spec §3.1 deferred)
		}
	}
	return total * critFactor(sb)
}

// effRecast applies the measured recast rules (recalibrated 2026-06-18, spec §3.1):
// reuse is a DIVISOR like haste — recast = base/(1+reuse/100) — applied after the
// per-art AA reduction (a multiplier), and floored at 50% of base
// (RecastReductionCeiling). The divisor reaches that floor at 100% reuse. An
// AA-halved art (Assassinate 300→150, Mortal Blade 180→90) lands exactly on the
// floor, so reuse can't reduce it further. (The old subtractive 1%/pt + 50-stat
// cap was an under-determined fit from one low reuse point — disproven by six
// Eviscerate readings to 61.8%.)
func effRecast(sb StatBlock, ca spell.CombatArt) float64 {
	reduced := ca.RecastSecs * (1 - ca.RecastReduction) / (1 + sb.Reuse/100)
	floor := ca.RecastSecs * (1 - constants.RecastReductionCeiling)
	return math.Max(floor, reduced)
}

// hasTermination reports whether the art carries an on-termination detonate
// component — the switch that makes a DoT held to full duration rather than
// clipped on cooldown.
func hasTermination(ca spell.CombatArt) bool {
	for _, c := range ca.Components {
		if c.Kind == spell.Termination {
			return true
		}
	}
	return false
}

// artCadence is the scheduling interval between casts. A termination art is HELD
// to its full duration so the detonate lands (max with effRecast — a long
// cooldown still gates it); every other art (including clipped DoTs) recasts on
// its plain effRecast.
func artCadence(sb StatBlock, ca spell.CombatArt) float64 {
	er := effRecast(sb, ca)
	if hasTermination(ca) {
		return math.Max(er, ca.DurationSecs)
	}
	return er
}

// dotTicks is the number of applications a DoT component delivers inside an
// active window: one instant tick (if present) plus one per completed interval.
func dotTicks(c spell.Component, windowSecs float64) float64 {
	if c.IntervalSecs <= 0 {
		return 0
	}
	ticks := math.Floor(windowSecs / c.IntervalSecs)
	if c.HasInstant {
		ticks++
	}
	return ticks
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
		rec[i] = artCadence(sb, ca)
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
		if c := artCadence(sb, ca); c > r {
			r = c
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
