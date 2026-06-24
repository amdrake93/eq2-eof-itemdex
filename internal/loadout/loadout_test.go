package loadout

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
