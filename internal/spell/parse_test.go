package spell

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseDamage(t *testing.T) {
	cases := []struct {
		name             string
		effects          []string
		wantMin, wantMax float64
		wantOK           bool
	}{
		{"melee", []string{"Inflicts 3,011 - 5,018 melee damage on target", "You must be sneaking"}, 3011, 5018, true},
		{"piercing single-line", []string{"Inflicts 800 - 1,200 piercing damage on target"}, 800, 1200, true},
		{"no damage", []string{"Increases Haste of caster by 30.6."}, 0, 0, false},
	}
	for _, c := range cases {
		min, max, ok := ParseDamage(c.effects)
		require.Equal(t, c.wantOK, ok, c.name)
		require.Equal(t, c.wantMin, min, c.name)
		require.Equal(t, c.wantMax, max, c.name)
	}
}
