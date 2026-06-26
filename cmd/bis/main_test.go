package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/amdrake93/eq2-eof-itemdex/internal/bis"
	"github.com/amdrake93/eq2-eof-itemdex/internal/loadout"
)

func TestRenderLoadoutReportLinkedTable(t *testing.T) {
	f := loadout.File{CharacterName: "Biffels", LastUpdate: 123}
	reports := []bucketReport{{
		Title: "Get now — pre-raid accessible",
		Upgrades: []bis.SlotUpgrade{
			{
				Slot:          "Finger",
				EquippedName:  "WeakRing",
				EquippedID:    11,
				EquippedValue: 120,
				Best:          bis.UpgradeOption{ID: 20, Name: "BigRing", Delta: 300},
				Alt:           &bis.UpgradeOption{ID: 21, Name: "MidRing", Delta: 150},
			},
			{
				Slot:         "Finger",
				EquippedName: "Empty",
				Best:         bis.UpgradeOption{ID: 22, Name: "AnyRing", Delta: 400},
			},
		},
	}}

	md := renderLoadoutReport(f, 30000, 31000, reports)

	require.Contains(t, md, "| Slot | Wearing | Best upgrade | Alternative |")
	// Worn item is linked and shows its slot-DPS value.
	require.Contains(t, md, "[WeakRing](https://u.eq2wire.com/item/11) (120)")
	// Best is bold-linked with +ΔDPS and total-relative %.
	require.Contains(t, md, "**[BigRing](https://u.eq2wire.com/item/20) +300 (+1.0%)**")
	// Alternative cell present.
	require.Contains(t, md, "[MidRing](https://u.eq2wire.com/item/21) +150 (+0.5%)")
	// Empty row: plain text, no link, value 0, blank alternative cell.
	require.Contains(t, md, "| Finger | Empty (0) | **[AnyRing](https://u.eq2wire.com/item/22) +400 (+1.3%)** |  |")
	// No tier tags anywhere.
	require.NotContains(t, md, "[FABLED]")
}

func TestRenderLoadoutReportNoUpgradeRow(t *testing.T) {
	f := loadout.File{CharacterName: "Biffels", LastUpdate: 123}
	reports := []bucketReport{{
		Title: "Get now — pre-raid accessible",
		Upgrades: []bis.SlotUpgrade{
			{
				Slot:          "Finger",
				EquippedName:  "StrongRing",
				EquippedID:    10,
				EquippedValue: 300,
				// zero-value Best => no upgrade in this bucket
			},
		},
	}}

	md := renderLoadoutReport(f, 30000, 30000, reports)

	// Worn item still linked; best cell is an em dash; alternative cell blank.
	require.Contains(t, md, "[StrongRing](https://u.eq2wire.com/item/10) (300) | — |  |")
}
