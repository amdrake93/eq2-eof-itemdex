package model

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	"github.com/stretchr/testify/require"
)

func TestRotationCADPS_Priority(t *testing.T) {
	// A: 100 dmg, 3s recast. B: 50 dmg, 1s recast. cast 1s + recovery 0 = 1s slot, fight 10s.
	// Hand-trace (priority = bigger hit, fire A whenever up, B fills):
	//   t0 A, t1 B, t2 B, t3 A, t4 B, t5 B, t6 A, t7 B, t8 B, t9 A  → A×4=400, B×6=300 = 700 /10s = 70.
	cas := []spell.CombatArt{
		{Name: "A", MinDamage: 100, MaxDamage: 100, RecastSecs: 3, CastSecsHundredths: 100},
		{Name: "B", MinDamage: 50, MaxDamage: 50, RecastSecs: 1, CastSecsHundredths: 100},
	}
	require.InDelta(t, 70.0, RotationCADPS(StatBlock{RecoverySpeed: 100}, cas, 10), 0.01)
}

func TestRotationCADPS_LowPriorityNeverFires(t *testing.T) {
	cas := []spell.CombatArt{
		{Name: "Big", MinDamage: 100, MaxDamage: 100, RecastSecs: 1, CastSecsHundredths: 100}, // always up (recast == slot)
		{Name: "Weak", MinDamage: 1, MaxDamage: 1, RecastSecs: 1, CastSecsHundredths: 100},
	}
	// cast 1s + recovery 0 = 1s slot; 10 slots all fire Big (100) → 1000/10 = 100; Weak never wins a slot.
	require.InDelta(t, 100.0, RotationCADPS(StatBlock{RecoverySpeed: 100}, cas, 10), 0.01)
}

func TestRotationCADPS_ReuseHelps(t *testing.T) {
	// reuse 100 is clamped to the 50-stat cap, which still halves recast.
	cas := []spell.CombatArt{{Name: "Nuke", MinDamage: 1000, MaxDamage: 1000, RecastSecs: 10}}
	base := RotationCADPS(StatBlock{}, cas, 600)
	fast := RotationCADPS(StatBlock{Reuse: 100}, cas, 600) // reuse capped at 50 → recast still halved → ~2× casts
	require.Greater(t, fast, base*1.8)
}

func TestRotationPerArtCastTime(t *testing.T) {
	// Two always-up arts (recast 0), equal damage, different cast times.
	// fast: cast 0.5s + recovery 0.25 = 0.75s/cast → 800 casts → 100/0.75
	// slow: cast 2.0s + recovery 0.25 = 2.25s/cast → ~266 casts → 100/2.25
	fast := []spell.CombatArt{{Name: "Fast", MinDamage: 100, MaxDamage: 100, RecastSecs: 0, CastSecsHundredths: 50}}
	slow := []spell.CombatArt{{Name: "Slow", MinDamage: 100, MaxDamage: 100, RecastSecs: 0, CastSecsHundredths: 200}}

	fastDPS := RotationCADPS(StatBlock{RecoverySpeed: 50}, fast, 600)
	slowDPS := RotationCADPS(StatBlock{RecoverySpeed: 50}, slow, 600)
	require.Greater(t, fastDPS, slowDPS, "the slower-cast art should yield less CADPS")
	require.InDelta(t, 100.0/0.75, fastDPS, 1e-6)
	// 600s isn't a whole multiple of the 2.25s slot, so the discrete sim fires one
	// partial-slot cast more than the continuous rate; tolerate up to one cast.
	require.InDelta(t, 100.0/2.25, slowDPS, 100.0/600)
}

func TestRotationPrioritizesByDPSPerCastTime(t *testing.T) {
	// Slow art has higher raw damage but worse per-second; fast art is lower raw
	// but better per-second. With DPS-per-cast-time priority the FAST art wins
	// every slot, so CADPS is its rate — not the slow art's higher-raw rate.
	cas := []spell.CombatArt{
		{Name: "Slow", MinDamage: 1287, MaxDamage: 1287, RecastSecs: 0, CastSecsHundredths: 150}, // slot 1.75 → 735/s
		{Name: "Fast", MinDamage: 891, MaxDamage: 891, RecastSecs: 0, CastSecsHundredths: 50},    // slot 0.75 → 1188/s
	}
	dps := RotationCADPS(StatBlock{RecoverySpeed: 50}, cas, 600)
	require.InDelta(t, 891.0/0.75, dps, 0.01, "picks higher DPS-per-cast-time (fast), not higher raw damage (slow)")
}

func TestCAEffectiveDamage_Potency(t *testing.T) {
	ca := spell.CombatArt{MinDamage: 800, MaxDamage: 1200}
	require.InDelta(t, 1000.0, CAEffectiveDamage(StatBlock{}, ca), 0.01)            // avg 1000, no stats
	require.InDelta(t, 1100.0, CAEffectiveDamage(StatBlock{Potency: 10}, ca), 0.01) // ×1.1
}

func TestEffRecastMeasuredRules(t *testing.T) {
	evis := spell.CombatArt{Name: "Eviscerate V", RecastSecs: 60}
	// Full-strength conversion: 60 × (1 − 3.8/100) = 57.72 (live tooltip: 57.8 @ 3.8 reuse)
	require.InDelta(t, 57.72, effRecast(StatBlock{Reuse: 3.8}, evis), 1e-9)
	// Reuse stat caps at 50 → recast can halve but never beat the ceiling.
	require.InDelta(t, 30.0, effRecast(StatBlock{Reuse: 50}, evis), 1e-9)
	require.InDelta(t, 30.0, effRecast(StatBlock{Reuse: 100}, evis), 1e-9)
}

func TestEffRecastSharedCeiling(t *testing.T) {
	assassinate := spell.CombatArt{Name: "Assassinate II", RecastSecs: 300, RecastReduction: 0.5}
	// AA halving fills the entire 50% ceiling: reuse adds nothing (live: pinned 2m30s).
	require.InDelta(t, 150.0, effRecast(StatBlock{}, assassinate), 1e-9)
	require.InDelta(t, 150.0, effRecast(StatBlock{Reuse: 3.8}, assassinate), 1e-9)
	require.InDelta(t, 150.0, effRecast(StatBlock{Reuse: 50}, assassinate), 1e-9)

	// A partial art mod leaves headroom under the ceiling for reuse.
	partial := spell.CombatArt{Name: "Y", RecastSecs: 100, RecastReduction: 0.3}
	require.InDelta(t, 60.0, effRecast(StatBlock{Reuse: 10}, partial), 1e-9) // 0.3+0.1
	require.InDelta(t, 50.0, effRecast(StatBlock{Reuse: 30}, partial), 1e-9) // 0.3+0.3 → ceiling
}

func TestSlotTiming(t *testing.T) {
	headShot := spell.CombatArt{Name: "Head Shot IV", RecastSecs: 60, CastSecsHundredths: 200}
	// Divisor cast speed (live: 2.0s → 1.46s @ 37.4%), recovery gone at 100.
	require.InDelta(t, 2.0/1.374, slotSecs(StatBlock{CastSpeed: 37.4, RecoverySpeed: 100}, headShot), 1e-9)
	// No timing stats: fallback cast 0.5 + full base recovery 0.5.
	quick := spell.CombatArt{Name: "Q", RecastSecs: 10}
	require.InDelta(t, 1.0, slotSecs(StatBlock{}, quick), 1e-9)
	// Recovery is subtractive: 50% → 0.25s left.
	require.InDelta(t, 0.75, slotSecs(StatBlock{RecoverySpeed: 50}, quick), 1e-9)
}
