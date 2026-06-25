package bis

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
)

func TestTierFilters(t *testing.T) {
	leg := store.ScorableItem{Tier: "LEGENDARY", Name: "Leg Cloak"}
	treas := store.ScorableItem{Tier: "TREASURED", Name: "Treasured Ring"}
	fabled := store.ScorableItem{Tier: "FABLED", Name: "Fabled Blade"}
	avatar := store.ScorableItem{Tier: "MYTHICAL", Name: "Avatar Helm"}
	hunters := store.ScorableItem{Tier: "FABLED", Name: "Hunter's Cloak"}

	require.True(t, PreRaidFilter(leg))
	require.True(t, PreRaidFilter(treas))
	require.False(t, PreRaidFilter(fabled))

	require.True(t, RaidFilter(fabled))
	require.True(t, RaidFilter(leg))
	require.False(t, RaidFilter(avatar))

	require.True(t, BestFilter(avatar))

	require.False(t, BestFilter(hunters))
	require.False(t, RaidFilter(hunters))
	require.False(t, PreRaidFilter(hunters))
}
