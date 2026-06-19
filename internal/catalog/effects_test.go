package catalog

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/stretchr/testify/require"
)

func eff(pairs ...any) []census.Effect {
	var out []census.Effect
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, census.Effect{Description: pairs[i].(string), Indentation: pairs[i+1].(int)})
	}
	return out
}

func TestParseEffects_StaticStat(t *testing.T) {
	stats, procs, _ := ParseEffects(eff(
		"When Equipped:", 0,
		"Increases Haste of caster by 25.0.", 1))
	require.Equal(t, map[string]float64{"attackspeed": 25.0}, stats)
	require.Empty(t, procs)
}

func TestParseEffects_Proc(t *testing.T) {
	stats, procs, _ := ParseEffects(eff(
		"When Equipped:", 0,
		"On a spell cast this spell may cast Harnessed Power of the Estate on caster.  Lasts for 30.0 seconds.  Triggers about 1.8 times per minute.", 1,
		"Decrease the caster's spell reuse time by 10%.", 2))
	require.Empty(t, stats)
	require.Len(t, procs, 1)
	require.InDelta(t, 1.8, procs[0].PerMinute, 1e-9)
	require.Contains(t, procs[0].Trigger, "On a spell cast")
}

func TestParseEffects_DamageProc(t *testing.T) {
	stats, procs, _ := ParseEffects(eff(
		"When Equipped:", 0,
		"On a successful melee attack this spell may cast Flame on target.  Triggers about 3.0 times per minute.", 1,
		"Inflicts 1,200 - 2,000 heat damage on target", 2))
	require.Empty(t, stats)
	require.Len(t, procs, 1)
	require.InDelta(t, 3.0, procs[0].PerMinute, 1e-9)
	require.Equal(t, "heat", procs[0].DmgType)
	require.InDelta(t, 1200, procs[0].MinDmg, 1e-9)
	require.InDelta(t, 2000, procs[0].MaxDmg, 1e-9)
}

func TestParseEffects_DecreaseIsNegative(t *testing.T) {
	stats, _, _ := ParseEffects(eff(
		"When Equipped:", 0,
		"Decreases Haste of caster by 5.0.", 1))
	require.Equal(t, map[string]float64{"attackspeed": -5.0}, stats)
}

func TestParseEffects_SkipsPercentTargetUnknown(t *testing.T) {
	stats, procs, audit := ParseEffects(eff(
		"When Equipped:", 0,
		"Increases Haste of caster by 10%.", 1,
		"Increases STA of target by 40.", 1,
		"Increases Bogus Stat of caster by 7.", 1))
	require.Empty(t, stats)
	require.Empty(t, procs)
	require.GreaterOrEqual(t, len(audit), 2)
}
