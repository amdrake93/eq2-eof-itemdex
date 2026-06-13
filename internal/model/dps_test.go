package model

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/constants"
	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	"github.com/stretchr/testify/require"
)

func approx(t *testing.T, want, got float64) {
	t.Helper()
	require.InDelta(t, want, got, 0.01)
}

func TestAutoDPS(t *testing.T) {
	w := Weapon{AvgDamage: 100, DelaySecs: 2.0}
	approx(t, 50.0, AutoDPS(StatBlock{}, w))                     // 100/2, all factors 1
	approx(t, 50.0*1.67, AutoDPS(StatBlock{Haste: 100}, w))      // haste 100 → effect 67 → /1.67 delay
	approx(t, 50.0*1.52, AutoDPS(StatBlock{MultiAttack: 50}, w)) // MA 50 → effect 52 → ×1.52
	approx(t, 65.0, AutoDPS(StatBlock{CritChance: 100}, w))      // ×1.30
}

func TestEffDelayHasteFittedCurve(t *testing.T) {
	w := Weapon{AvgDamage: 160, DelaySecs: 4.0}
	approx(t, 4.0/1.67, effDelay(StatBlock{Haste: 100}, w)) // effect 67 → 4/1.67
}

func TestCADPS(t *testing.T) {
	ca := spell.CombatArt{Name: "X", MinDamage: 800, MaxDamage: 1200, RecastSecs: 10}
	// Smoothing: 10s-recast window [595,605] straddles the 60th/61st-cast boundary at
	// t=600, so the smoothed average is ~0.74 above the naive 1000/10=100.0. Same
	// fractional shift applies to the ×1.1 potency and ×2 recast-halved cases.
	approx(t, 100.74, CADPS(StatBlock{}, []spell.CombatArt{ca}, 600))            // smoothed; unsmoothed = 100.0
	approx(t, 110.81, CADPS(StatBlock{Potency: 10}, []spell.CombatArt{ca}, 600)) // base ×1.1; smoothed shift ~×1.1
	approx(t, 200.74, CADPS(StatBlock{Reuse: 100}, []spell.CombatArt{ca}, 600))  // recast halved; smoothed
}

func TestAutoDPSDual(t *testing.T) {
	main := Weapon{AvgDamage: 100, DelaySecs: 2.0} // 50 dps unpenalized
	off := Weapon{AvgDamage: 60, DelaySecs: 3.0}   // 20 dps unpenalized

	// Dual-wield multiplies each weapon's delay by the penalty, so with all
	// other factors 1 the total is the naive sum divided by the penalty.
	approx(t, 70.0/constants.DualWieldDelayPenalty, AutoDPSDual(StatBlock{}, main, off, 1.0))

	// It equals AutoDPS on penalty-scaled-delay weapons (the spec'd behavior).
	mainPen := Weapon{AvgDamage: 100, DelaySecs: 2.0 * constants.DualWieldDelayPenalty}
	offPen := Weapon{AvgDamage: 60, DelaySecs: 3.0 * constants.DualWieldDelayPenalty}
	approx(t, AutoDPS(StatBlock{Haste: 50}, mainPen)+AutoDPS(StatBlock{Haste: 50}, offPen),
		AutoDPSDual(StatBlock{Haste: 50}, main, off, 1.0))

	// And it is strictly below the un-penalized sum.
	require.Less(t, AutoDPSDual(StatBlock{}, main, off, 1.0),
		AutoDPS(StatBlock{}, main)+AutoDPS(StatBlock{}, off))
}

func TestAutoDPSDualNoOffhandUnpenalized(t *testing.T) {
	main := Weapon{AvgDamage: 100, DelaySecs: 2.0}
	// No off-hand weapon → not dual-wielding → main is NOT penalized; the result
	// equals the single-weapon path exactly. (Detected via off.DelaySecs, so an
	// imported single-wield/shield loadout is correct, not an assumed dual-wield.)
	approx(t, 50.0, AutoDPSDual(StatBlock{}, main, Weapon{}, 1.0))
	approx(t, AutoDPS(StatBlock{}, main), AutoDPSDual(StatBlock{}, main, Weapon{}, 1.0))
}

func TestAutoDPSClassMultAndAGI(t *testing.T) {
	main := Weapon{AvgDamage: 100, DelaySecs: 2.0}
	off := Weapon{AvgDamage: 60, DelaySecs: 3.0}
	// classMult 2.0 doubles the auto sum vs 1.0.
	base := AutoDPSDual(StatBlock{}, main, off, 1.0)
	require.InDelta(t, 2.0*base, AutoDPSDual(StatBlock{}, main, off, 2.0), 1e-9)
	// AGI scales a single weapon: MainStat 625 → +51.74% per swing.
	require.InDelta(t, 1.5174, AutoDPS(StatBlock{MainStat: 625}, main)/AutoDPS(StatBlock{}, main), 1e-3)
}

func TestAutoWeaponMultiplierCalibration(t *testing.T) {
	// /weaponstat 2026-06-13, Blood Fire census max 290.
	// dps-mod 0, AGI MainStat 625 (curve→51.74%), classMult 2.0 → actual max 882.
	m0 := AutoWeaponMultiplier(StatBlock{MainStat: 625, DPSMod: 0}, 2.0)
	require.InDelta(t, 882.0, 290*m0, 6) // 290×1.5174×1.0×2.0 = 880.1
	// dps-mod stat 73.2 (curve→51%), AGI MainStat 983 (→64.06%), classMult 2.0 → actual max 1442.
	m1 := AutoWeaponMultiplier(StatBlock{MainStat: 983, DPSMod: 73.2}, 2.0)
	require.InDelta(t, 1442.0, 290*m1, 8) // 290×1.6406×1.51×2.0 = 1436.7
}
