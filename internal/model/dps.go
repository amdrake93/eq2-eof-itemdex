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

// flurryFactor is gear flurry only (haste overcap no longer converts to flurry).
func flurryFactor(sb StatBlock) float64 {
	return 1 + (sb.Flurry/100)*(constants.FlurryMultiplier-1)
}

func dpsModFactor(sb StatBlock) float64 {
	return 1 + HasteDpsModEffect(sb.DPSMod)/100
}

func effDelay(sb StatBlock, w Weapon) float64 {
	h := HasteDpsModEffect(sb.Haste)
	return w.DelaySecs / (1 + h/100)
}

// AutoDPS models sustained auto-attack damage per second.
func AutoDPS(sb StatBlock, w Weapon) float64 {
	if w.DelaySecs <= 0 {
		return 0
	}
	swings := w.AvgDamage / effDelay(sb, w)
	return swings * (1 + MultiAttackEffect(sb.MultiAttack)/100) * critFactor(sb) * flurryFactor(sb) * dpsModFactor(sb)
}

// CADPS is the simulated combat-art DPS over a standard fight (priority rotation).
// Each cast occupies cast time + recovery before the next cast.
func CADPS(sb StatBlock, cas []spell.CombatArt) float64 {
	slot := constants.CACastTimeSecs + constants.CARecoverySecs
	return RotationCADPS(sb, cas, constants.FightDurationSecs, slot)
}

// AutoDPSDual is dual-wield auto-attack: both weapons swing on their own delay.
// Main and off-hand are treated equally — the off-hand's only EoF penalty is not
// benefiting from the weapon-multiplier stat, which this model doesn't track, so
// it nets out the same for relative comparison.
func AutoDPSDual(sb StatBlock, main, off Weapon) float64 {
	return AutoDPS(sb, main) + AutoDPS(sb, off)
}

// TotalDPS = auto-attack + combat arts. Auto and CAs run in parallel.
func TotalDPS(sb StatBlock, w Weapon, cas []spell.CombatArt) float64 {
	return AutoDPS(sb, w) + CADPS(sb, cas)
}

// TotalDPSDual = dual-wield auto-attack + combat arts, in parallel.
func TotalDPSDual(sb StatBlock, main, off Weapon, cas []spell.CombatArt) float64 {
	return AutoDPSDual(sb, main, off) + CADPS(sb, cas)
}
