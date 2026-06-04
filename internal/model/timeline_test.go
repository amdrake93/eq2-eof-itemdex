package model

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	"github.com/stretchr/testify/require"
)

func TestRotationCA_ReportsCastingTime(t *testing.T) {
	// One non-AA art (name avoids the Assassinate/Mortal Blade CDR map),
	// RecastSecs 10, over a 100s fight with a 0.75s slot. It fires at
	// t=0,10,...,90 → 10 casts (idle-jump to next availability between casts).
	cas := []spell.CombatArt{
		{Name: "Test Strike", MinDamage: 800, MaxDamage: 1200, RecastSecs: 10},
	}
	r := RotationCA(StatBlock{}, cas, 100, 0.75)
	require.InDelta(t, 7.5, r.CastingSecs, 0.01)
	require.Greater(t, r.DamageTotal, 0.0)
}

func TestRotationCA_EmptyListZeroResult(t *testing.T) {
	r := RotationCA(StatBlock{}, nil, 600, 0.75)
	require.Equal(t, RotationResult{}, r)
}

func TestTotalDPSDual_NoCAsEqualsPureAuto(t *testing.T) {
	main := Weapon{AvgDamage: 100, DelaySecs: 2}
	off := Weapon{AvgDamage: 100, DelaySecs: 2}
	sb := StatBlock{}
	require.InDelta(t, AutoDPSDual(sb, main, off), TotalDPSDual(sb, main, off, nil), 0.01)
}

func TestTotalDPSDual_CastingDisplacesAuto(t *testing.T) {
	main := Weapon{AvgDamage: 100, DelaySecs: 2}
	off := Weapon{AvgDamage: 60, DelaySecs: 3}
	cas := []spell.CombatArt{
		{Name: "Test Strike", MinDamage: 1000, MaxDamage: 1000, RecastSecs: 1},
		{Name: "Test Slash", MinDamage: 500, MaxDamage: 500, RecastSecs: 1},
	}
	sb := StatBlock{}
	naive := AutoDPSDual(sb, main, off) + CADPS(sb, cas)
	require.Less(t, TotalDPSDual(sb, main, off, cas), naive)
}

func TestRotationCADPS_WrapperMatchesRotationCA(t *testing.T) {
	cas := []spell.CombatArt{
		{Name: "Test Strike", MinDamage: 800, MaxDamage: 1200, RecastSecs: 10},
	}
	want := RotationCA(StatBlock{}, cas, 100, 0.75).DamageTotal / 100
	require.InDelta(t, want, RotationCADPS(StatBlock{}, cas, 100, 0.75), 0.01)
}
