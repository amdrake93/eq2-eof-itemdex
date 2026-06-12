package model

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	"github.com/stretchr/testify/require"
)

func TestAACooldownReduction(t *testing.T) {
	t.Run("Assassinate halves base recast when RecastReduction set", func(t *testing.T) {
		ca := spell.CombatArt{Name: "Assassinate II", RecastSecs: 300, RecastReduction: 0.5}
		require.InDelta(t, 150.0, effRecast(StatBlock{Reuse: 0}, ca), 0.01)
	})

	t.Run("Mortal Blade halves base recast when RecastReduction set", func(t *testing.T) {
		ca := spell.CombatArt{Name: "Mortal Blade IV", RecastSecs: 180, RecastReduction: 0.5}
		require.InDelta(t, 90.0, effRecast(StatBlock{Reuse: 0}, ca), 0.01)
	})

	t.Run("AA halving fills ceiling so reuse adds nothing", func(t *testing.T) {
		ca := spell.CombatArt{Name: "Assassinate II", RecastSecs: 300, RecastReduction: 0.5}
		// ceiling already full (0.5 == RecastReductionCeiling); reuse stat clamped out
		require.InDelta(t, 150.0, effRecast(StatBlock{Reuse: 100}, ca), 0.01)
	})

	t.Run("non-AA art unchanged at reuse 0", func(t *testing.T) {
		ca := spell.CombatArt{Name: "Eviscerate V", RecastSecs: 60}
		require.InDelta(t, 60.0, effRecast(StatBlock{Reuse: 0}, ca), 0.01)
	})

	t.Run("non-AA art reuse still works normally", func(t *testing.T) {
		ca := spell.CombatArt{Name: "Eviscerate V", RecastSecs: 60}
		require.InDelta(t, 30.0, effRecast(StatBlock{Reuse: 100}, ca), 0.01)
	})
}
