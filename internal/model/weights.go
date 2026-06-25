package model

import (
	"math"

	"github.com/amdrake93/eq2-eof-itemdex/internal/constants"
)

// epsilon is the finite-difference step (1 stat point/percent).
const epsilon = 1.0

func bump(sb StatBlock, stat string, delta float64) StatBlock {
	switch stat {
	case "haste":
		sb.Haste += delta
	case "multiattack":
		sb.MultiAttack += delta
	case "critchance":
		sb.CritChance += delta
	case "potency":
		sb.Potency += delta
	case "dpsmod":
		sb.DPSMod += delta
	case "reuse":
		sb.Reuse += delta
	case "flurry":
		sb.Flurry += delta
	case "abilitymod":
		sb.AbilityMod += delta
	case "castspeed":
		sb.CastSpeed += delta
	case "recoveryspeed":
		sb.RecoverySpeed += delta
	case "mainstat":
		sb.MainStat += delta
	case "potencybonus":
		sb.PotencyBonus += delta
	}
	return sb
}

// WeightStats is the fixed ordered set of stats the model derives weights for.
var WeightStats = []string{"haste", "multiattack", "critchance", "potency", "dpsmod", "reuse", "flurry", "abilitymod", "castspeed", "mainstat"}

// curveStats convert through a non-linear curve; their marginal weight is a
// bracket slope rather than a +1 forward diff (which reads lumpy under the
// in-game flooring). Multi-attack and main stat use their sample tables; haste
// and dps-mod bracket between the fitted curve's integer-effect crossings. Main
// stat uses the multi-attack treatment (sample table — its readings are
// unfloored, so the bracket slope is simply the exact local slope of the
// piecewise model).
var curveStats = map[string]bool{"haste": true, "multiattack": true, "dpsmod": true, "mainstat": true}

// DeriveWeights returns the marginal DPS per +1 unit of each stat at the given
// baseline. dps computes total DPS for a stat block; the caller binds the
// loadout (dual-wield weapons + combat arts), enabling per-set re-derivation.
// Curve stats use a bracket slope (table samples for multi-attack,
// fitted-curve integer crossings for haste/dps-mod); everything else uses a +1
// forward diff. Saturated stats (e.g. dps-mod at cap) yield ~0 by construction.
func DeriveWeights(base StatBlock, dps func(StatBlock) float64) map[string]float64 {
	d0 := dps(base)
	out := make(map[string]float64, len(WeightStats))
	for _, s := range WeightStats {
		if curveStats[s] {
			out[s] = curveStatMarginal(base, s, dps)
			continue
		}
		out[s] = (dps(bump(base, s, epsilon)) - d0) / epsilon
	}
	return out
}

func getStat(sb StatBlock, stat string) float64 {
	switch stat {
	case "haste":
		return sb.Haste
	case "multiattack":
		return sb.MultiAttack
	case "critchance":
		return sb.CritChance
	case "potency":
		return sb.Potency
	case "dpsmod":
		return sb.DPSMod
	case "reuse":
		return sb.Reuse
	case "flurry":
		return sb.Flurry
	case "abilitymod":
		return sb.AbilityMod
	case "castspeed":
		return sb.CastSpeed
	case "recoveryspeed":
		return sb.RecoverySpeed
	case "mainstat":
		return sb.MainStat
	case "potencybonus":
		return sb.PotencyBonus
	}
	return 0
}

func setStat(sb StatBlock, stat string, v float64) StatBlock {
	return bump(sb, stat, v-getStat(sb, stat))
}

// statAtEffect inverts the unfloored fitted curve: the stat on the rising
// branch where f(stat) = e. Effects beyond f(cap) resolve to the cap.
// Assumes B > 0 (a real diminishing-returns fit).
func statAtEffect(e float64) float64 {
	disc := HasteDpsModA*HasteDpsModA - 4*HasteDpsModB*e
	if disc <= 0 {
		return constants.HasteStatCap
	}
	s := (HasteDpsModA - math.Sqrt(disc)) / (2 * HasteDpsModB)
	return math.Min(s, constants.HasteStatCap)
}

// curveStatMarginal is the per-point value of a curve stat as the DPS slope
// across an interval whose endpoints land exactly on whole-percent effects, so
// the in-game flooring contributes no noise. Multi-attack and main stat use
// their sample tables; haste/dps-mod use the fitted equation's integer
// crossings (available anywhere on the curve) and clamp to 0 at the shared
// 300 cap.
func curveStatMarginal(base StatBlock, stat string, dps func(StatBlock) float64) float64 {
	v := getStat(base, stat)
	if stat == "haste" {
		v = base.EffectiveHaste() // curve position uses stackable + item-effect haste (spec §11)
	}

	var lo, hi float64
	switch stat {
	case "multiattack":
		lo, hi = curveBracket(multiAttackSamples, v)
	case "haste", "dpsmod":
		if v >= constants.HasteStatCap { // == constants.DPSModCap (shared curve)
			return 0
		}
		n := math.Floor(hasteDpsModUnfloored(v))
		const nudge = 1e-9 // keep floor() on the intended side of each crossing
		lo = statAtEffect(n + nudge)
		hi = statAtEffect(n + 1 + nudge)
	case "mainstat":
		lo, hi = curveBracket(mainStatSamples, v)
	}

	if hi <= lo {
		return 0
	}
	// For haste, lo/hi are EFFECTIVE-haste stat values; evaluate DPS at those totals
	// by adjusting only the stackable Haste field (HasteEffect is fixed in base).
	loSet, hiSet := lo, hi
	if stat == "haste" {
		loSet, hiSet = lo-base.HasteEffect, hi-base.HasteEffect
	}
	return (dps(setStat(base, stat, hiSet)) - dps(setStat(base, stat, loSet))) / (hi - lo)
}
