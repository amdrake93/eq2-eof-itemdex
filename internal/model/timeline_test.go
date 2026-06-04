package model

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/constants"
	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	"github.com/stretchr/testify/require"
)

func TestTotalDPSDual_ParallelSum(t *testing.T) {
	// Auto-attack and CA casting run in parallel: casting does NOT displace auto.
	// TotalDPSDual must equal auto + CADPS exactly.
	main := Weapon{AvgDamage: 100, DelaySecs: 2}
	off := Weapon{AvgDamage: 60, DelaySecs: 3}
	cas := []spell.CombatArt{
		{Name: "Test Strike", MinDamage: 1000, MaxDamage: 1000, RecastSecs: 1},
		{Name: "Test Slash", MinDamage: 500, MaxDamage: 500, RecastSecs: 1},
	}
	sb := StatBlock{}
	want := AutoDPSDual(sb, main, off) + CADPS(sb, cas)
	require.InDelta(t, want, TotalDPSDual(sb, main, off, cas), 1e-9)
}

func TestTotalDPSDual_NoCAsEqualsPureAuto(t *testing.T) {
	main := Weapon{AvgDamage: 100, DelaySecs: 2}
	off := Weapon{AvgDamage: 100, DelaySecs: 2}
	sb := StatBlock{}
	require.InDelta(t, AutoDPSDual(sb, main, off), TotalDPSDual(sb, main, off, nil), 1e-9)
}

func TestRotationCADPS_RecoveryPacesThroughput(t *testing.T) {
	// An always-up CA (small recast). A larger slot (cast+recovery) yields fewer
	// casts and thus lower CADPS than a slot without recovery.
	cas := []spell.CombatArt{
		{Name: "Test Strike", MinDamage: 1000, MaxDamage: 1000, RecastSecs: 0.1},
	}
	sb := StatBlock{}
	withRecovery := RotationCADPS(sb, cas, 600, 0.75)
	withoutRecovery := RotationCADPS(sb, cas, 600, 0.5)
	require.Less(t, withRecovery, withoutRecovery)
	require.InDelta(t, 1.5, withoutRecovery/withRecovery, 0.05)
}

func TestCADPS_UsesCastPlusRecoverySlot(t *testing.T) {
	cas := []spell.CombatArt{
		{Name: "Test Strike", MinDamage: 800, MaxDamage: 1200, RecastSecs: 10},
	}
	sb := StatBlock{}
	slot := constants.CACastTimeSecs + constants.CARecoverySecs
	want := RotationCADPS(sb, cas, constants.FightDurationSecs, slot)
	require.InDelta(t, want, CADPS(sb, cas), 1e-9)
}
