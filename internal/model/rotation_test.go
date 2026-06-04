package model

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	"github.com/stretchr/testify/require"
)

func TestRotationCADPS_Priority(t *testing.T) {
	// A: 100 dmg, 3s recast. B: 50 dmg, 1s recast. castTime 1s, fight 10s.
	// Hand-trace (priority = bigger hit, fire A whenever up, B fills):
	//   t0 A, t1 B, t2 B, t3 A, t4 B, t5 B, t6 A, t7 B, t8 B, t9 A  → A×4=400, B×6=300 = 700 /10s = 70.
	cas := []spell.CombatArt{
		{Name: "A", MinDamage: 100, MaxDamage: 100, RecastSecs: 3},
		{Name: "B", MinDamage: 50, MaxDamage: 50, RecastSecs: 1},
	}
	require.InDelta(t, 70.0, RotationCADPS(StatBlock{}, cas, 10, 1), 0.01)
}

func TestRotationCADPS_LowPriorityNeverFires(t *testing.T) {
	cas := []spell.CombatArt{
		{Name: "Big", MinDamage: 100, MaxDamage: 100, RecastSecs: 1}, // always up (recast == castTime)
		{Name: "Weak", MinDamage: 1, MaxDamage: 1, RecastSecs: 1},
	}
	// 10 slots all fire Big (100) → 1000/10 = 100; Weak never wins a slot.
	require.InDelta(t, 100.0, RotationCADPS(StatBlock{}, cas, 10, 1), 0.01)
}

func TestRotationCADPS_ReuseHelps(t *testing.T) {
	cas := []spell.CombatArt{{Name: "Nuke", MinDamage: 1000, MaxDamage: 1000, RecastSecs: 10}}
	base := RotationCADPS(StatBlock{}, cas, 600, 0.5)
	fast := RotationCADPS(StatBlock{Reuse: 100}, cas, 600, 0.5) // recast halved → ~2× casts
	require.Greater(t, fast, base*1.8)
}

func TestCAEffectiveDamage_Potency(t *testing.T) {
	ca := spell.CombatArt{MinDamage: 800, MaxDamage: 1200}
	require.InDelta(t, 1000.0, CAEffectiveDamage(StatBlock{}, ca), 0.01)            // avg 1000, no stats
	require.InDelta(t, 1100.0, CAEffectiveDamage(StatBlock{Potency: 10}, ca), 0.01) // ×1.1
}
