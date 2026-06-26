package bis

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
	"github.com/stretchr/testify/require"
)

func testLoadout() store.Loadout { return store.Loadout{} }

// seedMain gives a set the standard test main-hand weapon (160 avg, 4.0 delay,
// single-wield) so auto-attack DPS is non-zero — the equipped-slot equivalent of
// the old Loadout.Main.
func seedMain(set *Set) {
	set.Equipped[mainHandSlot] = []store.ScorableItem{{ID: 1000, Slot: mainHandSlot, WeaponAvg: 160, WeaponDelay: 4}}
}

func TestSetDPSAndCandidateDelta(t *testing.T) {
	set := NewSet(model.StatBlock{}, testLoadout(), 1.0, 600)
	seedMain(set)

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
	set := NewSet(model.StatBlock{}, testLoadout(), 1.0, 600)
	set.Equipped["Head"] = []store.ScorableItem{{ID: 1, Slot: "Head", Stats: model.StatBlock{Potency: 10}}}
	set.Equipped["Chest"] = []store.ScorableItem{{ID: 2, Slot: "Chest", Stats: model.StatBlock{Potency: 25}}}

	require.InDelta(t, 35.0, set.restBase("").Potency, 1e-9)
	require.InDelta(t, 25.0, set.restBase("Head").Potency, 1e-9)
}

func TestSetAppliesClassAutoMult(t *testing.T) {
	mk := func(am float64) float64 {
		set := NewSet(model.StatBlock{}, store.Loadout{}, am, 600)
		seedMain(set)
		return set.DPS()
	}
	require.InDelta(t, 2.0*mk(1.0), mk(2.0), 1e-9) // no CAs/arts in fixture → DPS is pure auto
}

func TestReplaceInstanceDeltaHoldsOtherInstanceFixed(t *testing.T) {
	set := NewSet(model.StatBlock{}, testLoadout(), 1.0, 600)
	seedMain(set)
	strong := store.ScorableItem{ID: 1, Slot: "Finger", Stats: model.StatBlock{MultiAttack: 40}}
	weak := store.ScorableItem{ID: 2, Slot: "Finger", Stats: model.StatBlock{MultiAttack: 5}}
	set.Equipped["Finger"] = []store.ScorableItem{strong, weak}

	cand := store.ScorableItem{ID: 3, Slot: "Finger", Stats: model.StatBlock{MultiAttack: 30}}

	// Replacing the WEAK ring (idx 1) with a better one is a positive gain.
	upWeak := set.ReplaceInstanceDelta("Finger", 1, cand)
	require.Greater(t, upWeak, 0.0)

	// Replacing the STRONG ring (idx 0) with the same candidate is a smaller gain
	// (or a loss) — the two instances are evaluated independently.
	upStrong := set.ReplaceInstanceDelta("Finger", 0, cand)
	require.Greater(t, upWeak, upStrong)

	// Filling an empty position (idx -1) ADDS the candidate alongside both rings.
	add := set.ReplaceInstanceDelta("Finger", -1, cand)
	require.Greater(t, add, 0.0)
}

func TestEquippedInstanceValueIsMarginalContribution(t *testing.T) {
	set := NewSet(model.StatBlock{}, testLoadout(), 1.0, 600)
	seedMain(set)
	ring := store.ScorableItem{ID: 1, Slot: "Finger", Stats: model.StatBlock{MultiAttack: 40}}
	set.Equipped["Finger"] = []store.ScorableItem{ring}

	// The worn ring contributes positive DPS vs the slot without it.
	require.Greater(t, set.EquippedInstanceValue("Finger", 0), 0.0)
}
