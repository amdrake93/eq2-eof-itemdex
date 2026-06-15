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

func TestParseComponents_DirectHitAndDoT(t *testing.T) {
	// Impale: a DirectHit + a DoT-with-instant (both ind 0).
	impale := []Effect{
		{Description: "Inflicts 73 - 122 piercing damage on target", Indentation: 0},
		{Description: "Inflicts 20 - 33 piercing damage on target instantly and every 4 seconds.", Indentation: 0},
	}
	comps := ParseComponents(impale, 24.0)
	require.Len(t, comps, 2)
	require.Equal(t, DirectHit, comps[0].Kind)
	require.Equal(t, 122.0, comps[0].MaxDamage)
	require.Equal(t, DoT, comps[1].Kind)
	require.True(t, comps[1].HasInstant)
	require.Equal(t, 4.0, comps[1].IntervalSecs)

	// Quick Strike: DirectHit + periodic-only DoT (single value, no instant).
	quick := []Effect{
		{Description: "Inflicts 8 - 13 melee damage on target", Indentation: 0},
		{Description: "Inflicts 2 slashing damage on target every 4 seconds.", Indentation: 0},
	}
	comps = ParseComponents(quick, 12.0)
	require.Len(t, comps, 2)
	require.Equal(t, DirectHit, comps[0].Kind)
	require.Equal(t, DoT, comps[1].Kind)
	require.False(t, comps[1].HasInstant)
	require.Equal(t, 2.0, comps[1].MinDamage)
	require.Equal(t, 2.0, comps[1].MaxDamage)
}

func TestParseComponents_Termination(t *testing.T) {
	// Gushing Wound: termination (+inlined detonate child) + DirectHit + DoT.
	gushing := []Effect{
		{Description: "Applies Untreated Bleeding on termination.", Indentation: 0},
		{Description: "Inflicts 6 - 10 piercing damage on target.", Indentation: 1},
		{Description: "Inflicts 0 - 1 melee damage on target", Indentation: 0},
		{Description: "Inflicts 1 - 2 piercing damage on target instantly and every 4 seconds.", Indentation: 0},
	}
	comps := ParseComponents(gushing, 24.0)
	require.Len(t, comps, 3)
	require.Equal(t, Termination, comps[0].Kind)
	require.Equal(t, "Untreated Bleeding", comps[0].TriggeredSpell)
	require.Equal(t, 10.0, comps[0].MaxDamage)
	require.Equal(t, DirectHit, comps[1].Kind)
	require.Equal(t, "melee", comps[1].DamageType)
	require.Equal(t, DoT, comps[2].Kind)
	require.True(t, comps[2].HasInstant)
}

func TestParseComponents_Procs(t *testing.T) {
	// Death Mark IV: TriggerProc — damage + count at ind 2; outer "Grants 1
	// trigger" at ind 1 (for Marked, which has no inlined damage) must be ignored.
	deathMark := []Effect{
		{Description: "When damaged with a melee weapon this spell has a 5% chance to cast Marked on target.  Lasts for 36.0 seconds.", Indentation: 0},
		{Description: "When damaged with a melee weapon this spell will cast Agonizing Pain on target.", Indentation: 1},
		{Description: "Inflicts 295 - 492 piercing damage on target", Indentation: 2},
		{Description: "Grants a total of 5 triggers of the spell.", Indentation: 2},
		{Description: "Grants a total of 1 trigger of the spell.", Indentation: 1},
	}
	comps := ParseComponents(deathMark, 72.0)
	require.Len(t, comps, 1)
	require.Equal(t, TriggerProc, comps[0].Kind)
	require.Equal(t, "Agonizing Pain", comps[0].TriggeredSpell)
	require.Equal(t, 492.0, comps[0].MaxDamage)
	require.Equal(t, 5, comps[0].Triggers)

	// Whirling Blades IV: RateProc — a hit cast ~2/min.
	whirling := []Effect{
		{Description: "On a melee hit this spell may cast Swipe on target of attack.  Triggers about 2.0 times per minute.", Indentation: 0},
		{Description: "Inflicts 252 - 421 melee damage on target", Indentation: 1},
	}
	comps = ParseComponents(whirling, 0)
	require.Len(t, comps, 1)
	require.Equal(t, RateProc, comps[0].Kind)
	require.Equal(t, "Swipe", comps[0].TriggeredSpell)
	require.Equal(t, 2.0, comps[0].PerMinute)

	// Apply Poison: RateProc that casts a DoT (interval/instant captured).
	applyPoison := []Effect{
		{Description: "On a melee hit this spell may cast Assassin's Hemotoxin on target of attack.  Lasts for 24.0 seconds.  Triggers about 3.0 times per minute.", Indentation: 0},
		{Description: "Inflicts 217 poison damage on target instantly and every 4 seconds.", Indentation: 1},
	}
	comps = ParseComponents(applyPoison, 0)
	require.Len(t, comps, 1)
	require.Equal(t, RateProc, comps[0].Kind)
	require.Equal(t, 3.0, comps[0].PerMinute)
	require.Equal(t, 4.0, comps[0].IntervalSecs)
	require.True(t, comps[0].HasInstant)
}
