package model

import (
	"github.com/amdrake93/eq2-eof-itemdex/internal/constants"
	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
)

// Weapon is the auto-attack-relevant view of an equipped weapon.
type Weapon struct {
	AvgDamage float64 // (min+max)/2 of the weapon's base damage
	DelaySecs float64 // attack delay in seconds
}

func critFactor(sb StatBlock) float64 {
	return 1 + (sb.CritChance/100)*(constants.CritMultiplier-1)
}

// flurryPct = gear flurry + haste overcap converted at 10:1.
func flurryPct(sb StatBlock) float64 {
	over := sb.Haste - constants.HasteCapPct
	if over < 0 {
		over = 0
	}
	return sb.Flurry + over/constants.HasteToFlurry
}

func flurryFactor(sb StatBlock) float64 {
	return 1 + (flurryPct(sb)/100)*(constants.FlurryMultiplier-1)
}

func dpsModFactor(sb StatBlock) float64 {
	d := min(sb.DPSMod, constants.DPSModCap) // overcap wasted
	return 1 + (d/constants.DPSModCap)*constants.DPSModEffectAtCap
}

func effDelay(sb StatBlock, w Weapon) float64 {
	h := min(sb.Haste, constants.HasteCapPct) // beyond cap → flurry (handled in flurryPct)
	return w.DelaySecs / (1 + h/100)
}

// AutoDPS models sustained auto-attack damage per second.
func AutoDPS(sb StatBlock, w Weapon) float64 {
	if w.DelaySecs <= 0 {
		return 0
	}
	swings := w.AvgDamage / effDelay(sb, w)
	return swings * (1 + sb.MultiAttack/100) * critFactor(sb) * flurryFactor(sb) * dpsModFactor(sb)
}

// CADPS sums each combat art's damage/recast, applying potency, the ability-mod
// cap (50% of the potency-adjusted base), reuse, and crit.
func CADPS(sb StatBlock, cas []spell.CombatArt) float64 {
	cf := critFactor(sb)
	pot := 1 + sb.Potency/100
	reuseFactor := 1 - constants.ReuseHalveCoeff*min(sb.Reuse, constants.ReuseHalvesAt)/100
	var total float64
	for _, ca := range cas {
		if ca.RecastSecs <= 0 {
			continue
		}
		avgBase := (ca.MinDamage + ca.MaxDamage) / 2 * pot
		capBonus := constants.AbilityModCapFrac * ca.MaxDamage * pot
		bonus := min(sb.AbilityMod, capBonus)
		hit := (avgBase + bonus) * cf
		total += hit / (ca.RecastSecs * reuseFactor)
	}
	return total
}

// TotalDPS = auto-attack + combat arts.
func TotalDPS(sb StatBlock, w Weapon, cas []spell.CombatArt) float64 {
	return AutoDPS(sb, w) + CADPS(sb, cas)
}
