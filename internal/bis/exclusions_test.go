package bis

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
	"github.com/stretchr/testify/require"
)

func TestAccessibilityPredicates(t *testing.T) {
	avatar := store.ScorableItem{Name: "Fragment of the Chime", Tier: "MYTHICAL"}
	soulfire := store.ScorableItem{Name: "Soulfire Sabre", Tier: "MYTHICAL"}
	hunters := store.ScorableItem{Name: "Sky Hunter's Stiletto", Tier: "LEGENDARY"}
	fabled := store.ScorableItem{Name: "Bee Sting", Tier: "FABLED"}

	require.True(t, IsAvatar(avatar))
	require.False(t, IsAvatar(soulfire), "Soulfire mythicals are accessible")
	require.False(t, IsAvatar(fabled))

	require.True(t, IsHunters(hunters))
	require.False(t, IsHunters(fabled))

	require.False(t, Curated(fabled)) // empty curated list by default
}
