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
	approx(t, 100.0, CADPS(StatBlock{}, []spell.CombatArt{ca}))            // 1000/10
	approx(t, 110.0, CADPS(StatBlock{Potency: 10}, []spell.CombatArt{ca})) // base ×1.1
	approx(t, 200.0, CADPS(StatBlock{Reuse: 100}, []spell.CombatArt{ca}))  // recast halved
}

func TestAutoDPSDual(t *testing.T) {
	main := Weapon{AvgDamage: 100, DelaySecs: 2.0} // 50 dps unpenalized
	off := Weapon{AvgDamage: 60, DelaySecs: 3.0}   // 20 dps unpenalized

	// Dual-wield multiplies each weapon's delay by the penalty, so with all
	// other factors 1 the total is the naive sum divided by the penalty.
	approx(t, 70.0/constants.DualWieldDelayPenalty, AutoDPSDual(StatBlock{}, main, off))

	// It equals AutoDPS on penalty-scaled-delay weapons (the spec'd behavior).
	mainPen := Weapon{AvgDamage: 100, DelaySecs: 2.0 * constants.DualWieldDelayPenalty}
	offPen := Weapon{AvgDamage: 60, DelaySecs: 3.0 * constants.DualWieldDelayPenalty}
	approx(t, AutoDPS(StatBlock{Haste: 50}, mainPen)+AutoDPS(StatBlock{Haste: 50}, offPen),
		AutoDPSDual(StatBlock{Haste: 50}, main, off))

	// And it is strictly below the un-penalized sum.
	require.Less(t, AutoDPSDual(StatBlock{}, main, off),
		AutoDPS(StatBlock{}, main)+AutoDPS(StatBlock{}, off))
}

func TestAutoDPSDualNoOffhandUnpenalized(t *testing.T) {
	main := Weapon{AvgDamage: 100, DelaySecs: 2.0}
	// No off-hand weapon → not dual-wielding → main is NOT penalized; the result
	// equals the single-weapon path exactly. (Detected via off.DelaySecs, so an
	// imported single-wield/shield loadout is correct, not an assumed dual-wield.)
	approx(t, 50.0, AutoDPSDual(StatBlock{}, main, Weapon{}))
	approx(t, AutoDPS(StatBlock{}, main), AutoDPSDual(StatBlock{}, main, Weapon{}))
}
