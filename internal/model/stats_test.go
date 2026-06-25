package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMapModifiers(t *testing.T) {
	mods := map[string]float64{
		"attackspeed":        30,
		"doubleattackchance": 12,
		"critchance":         5,
		"basemodifier":       8,
		"dps":                40,
		"spelltimereusepct":  6,
		"flurry":             3,
		"critbonus":          9,
		"strength":           40,
		"arcane":             500,
	}
	var sb StatBlock
	sb.AddModifiers(mods)
	require.Equal(t, 30.0, sb.Haste)
	require.Equal(t, 12.0, sb.MultiAttack)
	require.Equal(t, 5.0, sb.CritChance)
	require.Equal(t, 8.0, sb.Potency)
	require.Equal(t, 40.0, sb.DPSMod)
	require.Equal(t, 6.0, sb.Reuse)
	require.Equal(t, 3.0, sb.Flurry)
	require.Equal(t, 0.0, sb.AbilityMod)
}

func TestAllMapsToAbilityMod(t *testing.T) {
	var sb StatBlock
	sb.AddModifiers(map[string]float64{"all": 62})
	require.Equal(t, 62.0, sb.AbilityMod)
}

func TestAddModifiersCastSpeed(t *testing.T) {
	var s StatBlock
	s.AddModifiers(map[string]float64{"spelltimecastpct": 1.6})
	require.InDelta(t, 1.6, s.CastSpeed, 1e-9)
}

func TestAddIncludesTimingStats(t *testing.T) {
	a := StatBlock{CastSpeed: 1.5, RecoverySpeed: 100}
	b := StatBlock{CastSpeed: 0.3}
	sum := a.Add(b)
	require.InDelta(t, 1.8, sum.CastSpeed, 1e-9)
	require.InDelta(t, 100.0, sum.RecoverySpeed, 1e-9)
}

func TestAddModifiersMainStat(t *testing.T) {
	var s StatBlock
	// census "strength" = "+N primary attributes" → AGI point-for-point for a scout
	s.AddModifiers(map[string]float64{"strength": 46})
	require.InDelta(t, 46.0, s.MainStat, 1e-9)
}

func TestAddIncludesMainStatAndPotencyBonus(t *testing.T) {
	sum := StatBlock{MainStat: 156, PotencyBonus: 24.6}.Add(StatBlock{MainStat: 40})
	require.InDelta(t, 196.0, sum.MainStat, 1e-9)
	require.InDelta(t, 24.6, sum.PotencyBonus, 1e-9)
}

func TestAddMaxesHasteEffect(t *testing.T) {
	a := StatBlock{Haste: 10, HasteEffect: 25}
	b := StatBlock{Haste: 5, HasteEffect: 21}
	sum := a.Add(b)
	require.InDelta(t, 15, sum.Haste, 1e-9)       // stackable haste sums
	require.InDelta(t, 25, sum.HasteEffect, 1e-9) // item-effect haste maxes (25 > 21)
	require.InDelta(t, 40, sum.EffectiveHaste(), 1e-9)
}
