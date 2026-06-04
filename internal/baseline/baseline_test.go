package baseline

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProfiles(t *testing.T) {
	require.Equal(t, 34.2, Solo.MultiAttack)
	require.Equal(t, 0.0, Solo.DPSMod)
	require.Equal(t, 200.0, Raid.DPSMod)
	require.Equal(t, 1.30, CritMultiplier)
	require.Equal(t, 4.0, FlurryMultiplier)
	require.Equal(t, 200.0, HasteStatCap)
	require.Equal(t, 200.0, DPSModCap)
}
