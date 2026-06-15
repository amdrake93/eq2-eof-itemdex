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

func TestParseDamageLine(t *testing.T) {
	cases := []struct {
		desc string
		want damageLine
		ok   bool
	}{
		{"Inflicts 73 - 122 piercing damage on target", damageLine{min: 73, max: 122, dmgType: "piercing"}, true},
		{"Inflicts 0 - 1 melee damage on target", damageLine{min: 0, max: 1, dmgType: "melee"}, true},
		{"Inflicts 1 - 2 piercing damage on target instantly and every 4 seconds.", damageLine{min: 1, max: 2, dmgType: "piercing", periodic: true, hasInstant: true, intervalSecs: 4}, true},
		{"Inflicts 2 slashing damage on target every 4 seconds.", damageLine{min: 2, max: 2, dmgType: "slashing", periodic: true, intervalSecs: 4}, true},
		{"Inflicts 217 poison damage on target instantly and every 4 seconds.", damageLine{min: 217, max: 217, dmgType: "poison", periodic: true, hasInstant: true, intervalSecs: 4}, true},
		{"Inflicts 252 - 421 melee damage on targets in Area of Effect", damageLine{min: 252, max: 421, dmgType: "melee", aoe: true}, true},
		{"Applies Untreated Bleeding on termination.", damageLine{}, false},
		{"Increases Haste of caster by 30.6.", damageLine{}, false},
	}
	for _, c := range cases {
		got, ok := parseDamageLine(c.desc)
		require.Equal(t, c.ok, ok, c.desc)
		if c.ok {
			require.Equal(t, c.want, got, c.desc)
		}
	}
}
