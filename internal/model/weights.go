package model

import "github.com/amdrake93/eq2-eof-itemdex/internal/constants"

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
	}
	return sb
}

// WeightStats is the fixed ordered set of stats the model derives weights for.
var WeightStats = []string{"haste", "multiattack", "critchance", "potency", "dpsmod", "reuse", "flurry", "abilitymod"}

// curveStats convert through the shared non-linear curve; their marginal weight
// is the sample-to-sample slope (a +1 forward diff would read lumpy under the floor).
var curveStats = map[string]bool{"haste": true, "multiattack": true}

// DeriveWeights returns the marginal DPS per +1 unit of each stat at the given
// baseline. dps computes total DPS for a stat block; the caller binds the
// loadout (dual-wield weapons + combat arts), enabling per-set re-derivation.
// Curve stats use the sample-to-sample slope; everything else uses a +1 forward
// diff. Saturated stats (e.g. dps-mod at cap) yield ~0 by construction.
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
	}
	return 0
}

func setStat(sb StatBlock, stat string, v float64) StatBlock {
	return bump(sb, stat, v-getStat(sb, stat))
}

// curveStatMarginal is the per-point value of a curve stat as the slope of the
// effect curve across the sample interval bracketing the baseline. Haste clamps
// at its stat cap (no value past it).
func curveStatMarginal(base StatBlock, stat string, dps func(StatBlock) float64) float64 {
	v := getStat(base, stat)
	if stat == "haste" && v >= constants.HasteStatCap {
		return 0
	}
	lo, hi := combatModBracket(v)
	if stat == "haste" && hi > constants.HasteStatCap {
		hi = constants.HasteStatCap
	}
	if hi <= lo {
		return 0
	}
	return (dps(setStat(base, stat, hi)) - dps(setStat(base, stat, lo))) / (hi - lo)
}
