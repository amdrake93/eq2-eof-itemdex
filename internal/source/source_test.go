package source

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/catalog"
	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/stretchr/testify/require"
)

func TestWriteEffectArtifacts(t *testing.T) {
	// Item 101: Cloak of Flames — static haste stat
	// Item 102: Cloak of Unrest — spell-cast proc
	items := []census.Item{
		{
			ID: 101,
			EffectList: []census.Effect{
				{Description: "When Equipped:", Indentation: 0},
				{Description: "Increases Haste of caster by 25.0.", Indentation: 1},
			},
		},
		{
			ID: 102,
			EffectList: []census.Effect{
				{Description: "When Equipped:", Indentation: 0},
				{Description: "On a spell cast this spell may cast Harnessed Power on caster.  Triggers about 1.8 times per minute.", Indentation: 1},
				{Description: "Inflicts 500 - 900 heat damage on target", Indentation: 2},
			},
		},
	}

	dir := t.TempDir()
	require.NoError(t, WriteEffectArtifacts(items, dir))

	// item-effects.csv must contain the haste row for item 101
	efData, err := os.ReadFile(filepath.Join(dir, "item-effects.csv"))
	require.NoError(t, err)
	efStr := string(efData)
	require.Contains(t, efStr, "101")
	require.Contains(t, efStr, "attackspeed")
	require.Contains(t, efStr, "25")

	// Round-trip the effect stats CSV
	efFile, err := os.Open(filepath.Join(dir, "item-effects.csv"))
	require.NoError(t, err)
	defer efFile.Close()
	stats, err := catalog.ReadEffectStatsCSV(efFile)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.Equal(t, 101, stats[0].ItemID)
	require.Equal(t, "attackspeed", stats[0].Stat)
	require.InDelta(t, 25.0, stats[0].Value, 1e-9)

	// item-procs.csv must contain the proc row for item 102
	procData, err := os.ReadFile(filepath.Join(dir, "item-procs.csv"))
	require.NoError(t, err)
	procStr := string(procData)
	require.Contains(t, procStr, "102")
	require.Contains(t, procStr, "1.8")

	// Round-trip the procs CSV
	procFile, err := os.Open(filepath.Join(dir, "item-procs.csv"))
	require.NoError(t, err)
	defer procFile.Close()
	procs, err := catalog.ReadProcsCSV(procFile)
	require.NoError(t, err)
	require.Len(t, procs, 1)
	require.Equal(t, 102, procs[0].ItemID)
	require.InDelta(t, 1.8, procs[0].PerMinute, 1e-9)

	// effect-audit.md must exist and reference both items
	auditData, err := os.ReadFile(filepath.Join(dir, "effect-audit.md"))
	require.NoError(t, err)
	auditStr := string(auditData)
	require.Contains(t, auditStr, "101")
	require.Contains(t, auditStr, "102")
}

func TestMergeEffectsAccumulates(t *testing.T) {
	// Seed with a prior result for item 1 (a haste stat already collected).
	prior := EffectAccumulator{
		Stats: []catalog.EffectStat{{ItemID: 1, Stat: "attackspeed", Value: 25}},
		Procs: nil,
		Audit: map[int][]catalog.AuditLine{
			1: {{Description: "Increases Haste of caster by 25.0.", Kind: "stat", Detail: "attackspeed"}},
		},
	}

	newItems := []census.Item{
		{
			ID: 2,
			EffectList: []census.Effect{
				{Description: "When Equipped:", Indentation: 0},
				{Description: "Increases Haste of caster by 10.0.", Indentation: 1},
			},
		},
		{
			ID: 3,
			EffectList: []census.Effect{
				{Description: "When Equipped:", Indentation: 0},
				{Description: "On a spell cast this spell may cast Harnessed Power on caster.  Triggers about 1.8 times per minute.", Indentation: 1},
				{Description: "Inflicts 500 - 900 heat damage on target", Indentation: 2},
			},
		},
	}

	got := MergeEffects(prior, newItems)

	// Stats: prior item 1 haste retained + new item 2 haste appended.
	require.Contains(t, got.Stats, catalog.EffectStat{ItemID: 1, Stat: "attackspeed", Value: 25})
	require.Contains(t, got.Stats, catalog.EffectStat{ItemID: 2, Stat: "attackspeed", Value: 10})

	// Procs: the new item 3 spell-cast proc is cataloged.
	require.Len(t, got.Procs, 1)
	require.Equal(t, 3, got.Procs[0].ItemID)
	require.InDelta(t, 1.8, got.Procs[0].PerMinute, 1e-9)

	// Audit: grouped by item, prior item 1 line retained, new items added.
	require.Contains(t, got.Audit, 1)
	require.Contains(t, got.Audit, 2)
	require.Contains(t, got.Audit, 3)
}

func TestLoadFromCacheWhenPresent(t *testing.T) {
	dir := t.TempDir()
	items := []census.Item{
		{
			ID:          1,
			DisplayName: "Sword",
			Slots:       []census.Slot{{Name: "Primary"}},
			TypeInfo:    census.TypeInfo{Classes: map[string]census.ClassReq{}},
			Modifiers:   map[string]census.Modifier{},
		},
	}
	f, err := os.Create(filepath.Join(dir, "weapons.csv"))
	require.NoError(t, err)
	require.NoError(t, catalog.WriteCSV(f, items))
	require.NoError(t, f.Close())

	got, err := LoadCache(dir)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "Sword", string(got[0].DisplayName))
}

func TestCacheExists(t *testing.T) {
	dir := t.TempDir()
	if CacheExists(dir) {
		t.Fatal("empty dir should not be a cache")
	}
	f, err := os.Create(filepath.Join(dir, "weapons.csv"))
	require.NoError(t, err)
	require.NoError(t, f.Close())
	if !CacheExists(dir) {
		t.Fatal("dir with weapons.csv should be a cache")
	}
}
