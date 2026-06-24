package loadout

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
)

func TestFileRoundTrip(t *testing.T) {
	f := File{
		CharacterName: "Biffels (Wuoshi)",
		LastUpdate:    1782258823.5,
		Slots: []SlotEntry{
			{CatalogSlot: "Back", CharSlot: "cloak", ItemID: 264598753, Name: "Cloak of Flames",
				Optimizable: true, Stats: model.StatBlock{Haste: 25}},
			{CatalogSlot: "Charm", CharSlot: "activate1", ItemID: 4135486725, Name: "Clicky",
				Optimizable: false, Stats: model.StatBlock{CritChance: 3}},
			{CatalogSlot: "Primary", CharSlot: "primary", ItemID: 1606057721, Name: "Sabre",
				Optimizable: true, WeaponMin: 80, WeaponMax: 160, WeaponDelay: 3.0,
				Stats: model.StatBlock{Potency: 5}},
		},
		Unresolved: []string{"event_slot:3649577502"},
	}
	path := filepath.Join(t.TempDir(), "biffels-loadout.toml")
	require.NoError(t, Write(path, f))

	got, err := Read(path)
	require.NoError(t, err)
	require.Equal(t, f.CharacterName, got.CharacterName)
	require.InDelta(t, f.LastUpdate, got.LastUpdate, 1e-3)
	require.Len(t, got.Slots, 3)
	require.Equal(t, "Back", got.Slots[0].CatalogSlot)
	require.InDelta(t, 25, got.Slots[0].Stats.Haste, 1e-9)
	require.True(t, got.Slots[0].Optimizable)
	require.False(t, got.Slots[1].Optimizable)
	require.InDelta(t, 3.0, got.Slots[2].WeaponDelay, 1e-9)
	require.Equal(t, []string{"event_slot:3649577502"}, got.Unresolved)
}

func TestSkipSlot(t *testing.T) {
	skip := []string{"food", "drink", "mount_adornment", "mount_armor"}
	for _, s := range skip {
		require.True(t, SkipSlot(s), "expected %q skipped", s)
	}
	keep := []string{"primary", "secondary", "head", "cloak", "left_ring", "ears2", "ranged", "ammo", "activate1", "event_slot", "waist"}
	for _, s := range keep {
		require.False(t, SkipSlot(s), "expected %q kept", s)
	}
}
