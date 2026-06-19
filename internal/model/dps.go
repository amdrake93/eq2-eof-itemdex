package model

import (
	"math"

	"github.com/amdrake93/eq2-eof-itemdex/internal/constants"
	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
)

// Weapon is the auto-attack-relevant view of an equipped weapon.
type Weapon struct {
	AvgDamage float64 // (min+max)/2 of the weapon's base damage
	MinDamage float64 // weapon base damage range low (for the range-shift crit, §11)
	MaxDamage float64 // weapon base damage range high
	DelaySecs float64 // attack delay in seconds
}

// critFactor is the expected crit damage multiplier for a hit whose final damage
// range is [lo, hi] (after potency, AGI, and any ability-mod). A crit re-rolls as
// max(hi, c·roll) — the higher of the range ceiling (a floor a crit can't fall
// below) and c× the roll — where c = 1.50 + crit bonus (measured 2026-06-19, §11).
// Narrow ranges → the c× branch always wins (flat ×c); wide ranges (weapons) →
// the ceiling lifts low rolls, pushing the average above c. Uniform rolls assumed
// (data-validated, data/autoattacktest.txt). Crit chance clamps at 100%.
func critFactor(sb StatBlock, lo, hi float64) float64 {
	p := math.Min(sb.CritChance, 100) / 100
	if p <= 0 {
		return 1
	}
	c := constants.CritMultiplier + sb.CritBonus/100
	m := c
	if hi > lo {
		t := hi / c
		if lo < t {
			avg := (lo + hi) / 2
			m = (hi*(t-lo) + c*(hi*hi-t*t)/2) / ((hi - lo) * avg)
		}
	}
	return 1 + p*(m-1)
}

// flurryFactor is gear flurry only (haste overcap no longer converts to flurry).
func flurryFactor(sb StatBlock) float64 {
	return 1 + (sb.Flurry/100)*(constants.FlurryMultiplier-1)
}

func dpsModFactor(sb StatBlock) float64 {
	return 1 + HasteDpsModEffect(sb.DPSMod)/100
}

// autoDamageMult is the per-swing damage multiplier from the wielder's stats:
// main-stat (AGI, same curve as CAs) × dps-mod. (The class auto multiplier is
// applied separately at the AutoDPSDual/TotalDPS boundary.)
func autoDamageMult(sb StatBlock) float64 {
	return (1 + MainStatEffect(sb.MainStat)/100) * dpsModFactor(sb)
}

// AutoWeaponMultiplier is the full multiplier on census-base per-swing damage:
// main-stat × dps-mod × class auto multiplier. It is the model-verification
// anchor for in-game /weaponstat readings (see TestAutoWeaponMultiplierCalibration),
// not a production code path — AutoDPS composes these factors itself.
func AutoWeaponMultiplier(sb StatBlock, classAutoMult float64) float64 {
	return autoDamageMult(sb) * classAutoMult
}

func effDelay(sb StatBlock, w Weapon) float64 {
	h := HasteDpsModEffect(sb.Haste)
	return w.DelaySecs / (1 + h/100)
}

// AutoDPS models sustained auto-attack damage per second for one weapon. AGI and
// dps-mod scale per-swing damage (autoDamageMult). It does NOT apply the class
// auto multiplier — callers MUST (AutoDPSDual/TotalDPS do); a new caller that
// omits it silently under-reports for any class whose multiplier isn't 1.0.
func AutoDPS(sb StatBlock, w Weapon) float64 {
	if w.DelaySecs <= 0 {
		return 0
	}
	swings := w.AvgDamage / effDelay(sb, w)
	return swings * (1 + MultiAttackEffect(sb.MultiAttack)/100) * autoDamageMult(sb) * critFactor(sb, w.MinDamage, w.MaxDamage) * flurryFactor(sb)
}

// CADPS is the fight-length-smoothed combat-art DPS for a target fight length.
// A single fixed length quantizes the last cast of long-cooldown arts (spec
// §3.1); CADPS averages cumCA(t)/t over K samples spanning [fightLen − R/2,
// fightLen + R/2], R = longest effective recast, computed from one sim pass to
// the window's top. Short-recast-only art sets have R≈0 → effectively unsmoothed.
func CADPS(sb StatBlock, cas []spell.CombatArt, fightLen float64) float64 {
	if fightLen <= 0 || len(cas) == 0 {
		return 0
	}
	r := maxEffRecast(sb, cas)
	lo := math.Max(fightLen-r/2, 1.0)
	hi := fightLen + r/2
	starts, cum := rotationTimeline(sb, cas, hi)
	if hi <= lo {
		return cumCAAt(starts, cum, fightLen) / fightLen
	}
	var sum float64
	for i := 0; i < fightSmoothingSamples; i++ {
		s := lo + float64(i)*(hi-lo)/float64(fightSmoothingSamples-1)
		sum += cumCAAt(starts, cum, s) / s
	}
	return sum / fightSmoothingSamples
}

// AutoDPSDual is dual-wield auto-attack. Equipping an off-hand WEAPON imposes
// EQ2's ~33% delay penalty on BOTH weapons (DualWieldDelayPenalty), on top of
// haste and independent of it (measured 2026-06-13). The penalty is detected,
// not assumed: it applies only when a real off-hand weapon is present
// (off.DelaySecs > 0), so an empty off-hand or a non-weapon off-hand
// (shield/symbol) is correctly unpenalized — important for imported loadouts
// that may not be dual-wielding. Main and off are otherwise treated equally —
// the off-hand's weapon-multiplier-stat penalty isn't tracked and nets out for
// relative comparison. classAutoMult is the class-intrinsic auto-attack
// multiplier (sources from classes/<class>.toml).
func AutoDPSDual(sb StatBlock, main, off Weapon, classAutoMult float64) float64 {
	if off.DelaySecs > 0 {
		main.DelaySecs *= constants.DualWieldDelayPenalty
		off.DelaySecs *= constants.DualWieldDelayPenalty
	}
	return classAutoMult * (AutoDPS(sb, main) + AutoDPS(sb, off))
}

// TotalDPS = auto-attack + combat arts. Auto and CAs run in parallel.
// classAutoMult is the class-intrinsic auto-attack multiplier.
func TotalDPS(sb StatBlock, w Weapon, cas []spell.CombatArt, classAutoMult, fightLen float64) float64 {
	return classAutoMult*AutoDPS(sb, w) + CADPS(sb, cas, fightLen)
}

// TotalDPSDual = dual-wield auto-attack + combat arts, in parallel. Assumes a
// dual-wield context (the EoF Assassin always dual-wields): it routes through
// AutoDPSDual, which applies the ×1.33 off-hand delay penalty — so it must NOT
// model a true single-wield/2H loadout (use TotalDPS for that).
// classAutoMult is the class-intrinsic auto-attack multiplier.
func TotalDPSDual(sb StatBlock, main, off Weapon, cas []spell.CombatArt, classAutoMult, fightLen float64) float64 {
	return AutoDPSDual(sb, main, off, classAutoMult) + CADPS(sb, cas, fightLen)
}
