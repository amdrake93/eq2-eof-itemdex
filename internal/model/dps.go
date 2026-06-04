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

// CADPS is the simulated combat-art DPS over a standard fight (priority rotation).
func CADPS(sb StatBlock, cas []spell.CombatArt) float64 {
	return RotationCADPS(sb, cas, constants.FightDurationSecs, constants.CACastTimeSecs)
}

// AutoDPSDual is dual-wield auto-attack: both weapons swing on their own delay.
// Main and off-hand are treated equally — the off-hand's only EoF penalty is not
// benefiting from the weapon-multiplier stat, which this model doesn't track, so
// it nets out the same for relative comparison.
func AutoDPSDual(sb StatBlock, main, off Weapon) float64 {
	return AutoDPS(sb, main) + AutoDPS(sb, off)
}

// TotalDPS = auto-attack + combat arts.
func TotalDPS(sb StatBlock, w Weapon, cas []spell.CombatArt) float64 {
	return AutoDPS(sb, w) + CADPS(sb, cas)
}
