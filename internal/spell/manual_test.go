package spell

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManualArts(t *testing.T) {
	byName := map[string]CombatArt{}
	for _, a := range ManualArts() {
		byName[a.Name] = a
	}

	hs, ok := byName["Hilt Strike"]
	require.True(t, ok)
	require.Equal(t, 20.0, hs.RecastSecs)
	require.Equal(t, 50, hs.CastSecsHundredths)
	require.Len(t, hs.Components, 1)
	require.Equal(t, DirectHit, hs.Components[0].Kind)
	require.Equal(t, 262.0, hs.Components[0].MinDamage)
	require.Equal(t, 315.0, hs.Components[0].MaxDamage)

	soc, ok := byName["Strike of Consistency"]
	require.True(t, ok)
	require.Equal(t, 12.0, soc.RecastSecs)
	require.Equal(t, 199.0, soc.Components[0].MinDamage)
	require.Equal(t, 199.0, soc.Components[0].MaxDamage)

	// Returns a copy — mutating the result must not corrupt the package data.
	ManualArts()[0].Components[0].MinDamage = -1
	require.Equal(t, 262.0, ManualArts()[0].Components[0].MinDamage)
}
