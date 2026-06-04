package spell

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHighestRanks(t *testing.T) {
	cas := []CombatArt{
		{Name: "Mortal Blade", MaxDamage: 1000, RecastSecs: 30},
		{Name: "Mortal Blade IV", MaxDamage: 4000, RecastSecs: 30},
		{Name: "Mortal Blade II", MaxDamage: 2000, RecastSecs: 30},
		{Name: "Assassinate", MaxDamage: 8000, RecastSecs: 300},
		{Name: "Assassinate II", MaxDamage: 12000, RecastSecs: 300},
		{Name: "Quick Strike", MaxDamage: 500, RecastSecs: 5},
	}
	got := HighestRanks(cas)
	require.Len(t, got, 3)
	byBase := map[string]float64{}
	for _, c := range got {
		byBase[BaseName(c.Name)] = c.MaxDamage
	}
	require.Equal(t, 4000.0, byBase["Mortal Blade"])
	require.Equal(t, 12000.0, byBase["Assassinate"])
	require.Equal(t, 500.0, byBase["Quick Strike"])
}

func TestBaseName(t *testing.T) {
	require.Equal(t, "Mortal Blade", BaseName("Mortal Blade IV"))
	require.Equal(t, "Assassinate", BaseName("Assassinate II"))
	require.Equal(t, "Quick Strike", BaseName("Quick Strike"))
	require.Equal(t, "Gut", BaseName("Gut X"))
}
