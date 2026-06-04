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

	low := ItemDelta(StatBlock{}, main, off, arts, flurry, nil)
	high := ItemDelta(StatBlock{DPSMod: 200}, main, off, arts, flurry, nil)
	require.Greater(t, high, low)
}

func TestItemDeltaCappedStatZero(t *testing.T) {
	main := Weapon{AvgDamage: 160, DelaySecs: 4}
	off := Weapon{AvgDamage: 158, DelaySecs: 4.4}
	var arts []spell.CombatArt
	d := ItemDelta(StatBlock{Haste: 200}, main, off, arts, StatBlock{Haste: 50}, nil)
	require.InDelta(t, 0.0, d, 1e-9)
}

func TestItemDeltaOffHandWeapon(t *testing.T) {
	main := Weapon{AvgDamage: 160, DelaySecs: 4}
	var arts []spell.CombatArt
	w := Weapon{AvgDamage: 150, DelaySecs: 4}
	d := ItemDelta(StatBlock{}, main, Weapon{}, arts, StatBlock{}, &w)
	require.Greater(t, d, 0.0)
}
