package model

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	"github.com/stretchr/testify/require"
)

func TestCritFactorHybrid(t *testing.T) {
	require.InDelta(t, 1.0, critFactor(StatBlock{CritChance: 0}, 139, 775), 1e-9)
	require.InDelta(t, 1.50, critFactor(StatBlock{CritChance: 100}, 100, 100), 1e-9)
	require.InDelta(t, 1.50, critFactor(StatBlock{CritChance: 100}, 316, 386), 1e-3)
	require.InDelta(t, 1.511, critFactor(StatBlock{CritChance: 100}, 215, 359), 1e-3)
	require.InDelta(t, 1.868, critFactor(StatBlock{CritChance: 100}, 139, 775), 1e-3)
	full := critFactor(StatBlock{CritChance: 100}, 139, 775)
	half := critFactor(StatBlock{CritChance: 50}, 139, 775)
	require.InDelta(t, 1+0.5*(full-1), half, 1e-9)
	require.InDelta(t, full, critFactor(StatBlock{CritChance: 150}, 139, 775), 1e-9)
	require.InDelta(t, 1.64, critFactor(StatBlock{CritChance: 100, CritBonus: 14}, 50, 50), 1e-9)
}

func TestCritAbilityModRides(t *testing.T) {
	ca := spell.CombatArt{
		Name:       "T",
		Components: []spell.Component{{Kind: spell.DirectHit, MinDamage: 100, MaxDamage: 100}},
	}
	got := CAEffectiveDamage(StatBlock{CritChance: 100, AbilityMod: 68}, ca)
	require.InDelta(t, (100+68)*1.5, got, 1e-6)
}
