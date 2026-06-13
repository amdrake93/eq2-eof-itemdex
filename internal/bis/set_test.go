package bis

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
	"github.com/stretchr/testify/require"
)

func testLoadout() store.Loadout {
	return store.Loadout{
		Main: model.Weapon{AvgDamage: 160, DelaySecs: 4},
	}
}

func TestSetDPSAndCandidateDelta(t *testing.T) {
	set := NewSet(model.StatBlock{}, testLoadout())

	// This fixture has no off-hand, so it is single-wielding: the dual-wield
	// delay penalty does NOT apply (it's gated on an equipped off-hand weapon).
	require.InDelta(t, 40.0, set.DPS(), 1e-6) // 160/4, unpenalized

	chest := store.ScorableItem{ID: 1, Slot: "Chest", Stats: model.StatBlock{Flurry: 10}}
	require.Greater(t, set.CandidateDelta("Chest", chest), 0.0)

	off := store.ScorableItem{ID: 2, Slot: "Secondary", WeaponAvg: 150, WeaponDelay: 4, Stats: model.StatBlock{}}
	require.True(t, off.IsWeapon())
	require.Greater(t, set.CandidateDelta("Secondary", off), 0.0)
}

func TestSetRestBaseExcludesSlot(t *testing.T) {
	set := NewSet(model.StatBlock{}, testLoadout())
	set.Equipped["Head"] = []store.ScorableItem{{ID: 1, Slot: "Head", Stats: model.StatBlock{Potency: 10}}}
	set.Equipped["Chest"] = []store.ScorableItem{{ID: 2, Slot: "Chest", Stats: model.StatBlock{Potency: 25}}}

	require.InDelta(t, 35.0, set.restBase("").Potency, 1e-9)
	require.InDelta(t, 25.0, set.restBase("Head").Potency, 1e-9)
}
