package model

import (
	"testing"

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
	approx(t, 50.0*1.66, AutoDPS(StatBlock{Haste: 100}, w))      // haste 100 → effect 66 → /1.66 delay
	approx(t, 50.0*1.52, AutoDPS(StatBlock{MultiAttack: 50}, w)) // MA 50 → effect 52 → ×1.52
	approx(t, 65.0, AutoDPS(StatBlock{CritChance: 100}, w))      // ×1.30
}

func TestEffDelayHasteInterpolated(t *testing.T) {
	w := Weapon{AvgDamage: 160, DelaySecs: 4.0}
	approx(t, 2.4096, effDelay(StatBlock{Haste: 100}, w)) // effect 66 → 4/1.66
}

func TestCADPS(t *testing.T) {
	ca := spell.CombatArt{Name: "X", MinDamage: 800, MaxDamage: 1200, RecastSecs: 10}
	approx(t, 100.0, CADPS(StatBlock{}, []spell.CombatArt{ca}))            // 1000/10
	approx(t, 110.0, CADPS(StatBlock{Potency: 10}, []spell.CombatArt{ca})) // base ×1.1
	approx(t, 200.0, CADPS(StatBlock{Reuse: 100}, []spell.CombatArt{ca}))  // recast halved
}

func TestAutoDPSDual(t *testing.T) {
	main := Weapon{AvgDamage: 100, DelaySecs: 2.0} // 50 dps
	off := Weapon{AvgDamage: 60, DelaySecs: 3.0}   // 20 dps
	approx(t, 70.0, AutoDPSDual(StatBlock{}, main, off))
	approx(t, AutoDPS(StatBlock{Haste: 50}, main)+AutoDPS(StatBlock{Haste: 50}, off),
		AutoDPSDual(StatBlock{Haste: 50}, main, off))
}
