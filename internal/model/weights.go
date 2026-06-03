package model

import "github.com/amdrake93/eq2-eof-itemdex/internal/spell"

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

// DeriveWeights returns the marginal DPS per +1 unit of each stat at the given
// baseline (with the reference weapon + combat arts), via forward difference.
// Saturated stats (e.g. dps-mod at its cap) yield ~0 by construction.
func DeriveWeights(base StatBlock, w Weapon, cas []spell.CombatArt) map[string]float64 {
	d0 := TotalDPS(base, w, cas)
	out := make(map[string]float64, len(WeightStats))
	for _, s := range WeightStats {
		out[s] = (TotalDPS(bump(base, s, epsilon), w, cas) - d0) / epsilon
	}
	return out
}
