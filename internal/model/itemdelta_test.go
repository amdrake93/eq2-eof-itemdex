package model

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	"github.com/stretchr/testify/require"
)

func TestItemDeltaInteractionMultiplicative(t *testing.T) {
	main := Weapon{AvgDamage: 160, DelaySecs: 4}
	off := Weapon{AvgDamage: 158, DelaySecs: 4.4}
	var arts []spell.CombatArt
	flurry := StatBlock{Flurry: 10}

	low := ItemDelta(StatBlock{}, main, off, arts, flurry, nil, nil, 1.0, 600)
	high := ItemDelta(StatBlock{DPSMod: 200}, main, off, arts, flurry, nil, nil, 1.0, 600)
	require.Greater(t, high, low)
}

func TestItemDeltaCappedStatZero(t *testing.T) {
	main := Weapon{AvgDamage: 160, DelaySecs: 4}
	off := Weapon{AvgDamage: 158, DelaySecs: 4.4}
	var arts []spell.CombatArt
	d := ItemDelta(StatBlock{Haste: 300}, main, off, arts, StatBlock{Haste: 50}, nil, nil, 1.0, 600)
	require.InDelta(t, 0.0, d, 1e-9)
}

func TestItemDeltaOffHandWeapon(t *testing.T) {
	main := Weapon{AvgDamage: 160, DelaySecs: 4}
	var arts []spell.CombatArt
	w := Weapon{AvgDamage: 150, DelaySecs: 4}
	d := ItemDelta(StatBlock{}, main, Weapon{}, arts, StatBlock{}, nil, &w, 1.0, 600)
	require.Greater(t, d, 0.0)
}

func TestItemDeltaMainHandWeapon(t *testing.T) {
	base := StatBlock{}
	restMain := Weapon{AvgDamage: 100, MinDamage: 60, MaxDamage: 140, DelaySecs: 4}
	bigMain := Weapon{AvgDamage: 200, MinDamage: 120, MaxDamage: 280, DelaySecs: 4}
	// Swapping in a stronger main-hand (via newMain) raises DPS.
	d := ItemDelta(base, restMain, Weapon{}, nil, StatBlock{}, &bigMain, nil, 1.0, 600)
	require.Greater(t, d, 0.0)
}
