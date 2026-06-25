package charconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	"github.com/stretchr/testify/require"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "c.toml")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	return p
}

const minimalValid = `
[character]
name = "T"
class = "assassin"
art_tier = "expert"
[contexts.solo]
multiattack = 10
`

func TestLoadCommittedConfig(t *testing.T) {
	cfg, err := Load("../../characters/biffels/config.toml")
	require.NoError(t, err)

	require.Equal(t, "assassin", cfg.Character.Class)
	require.InDelta(t, 37.4, cfg.Stats.CastSpeed, 1e-9)
	require.InDelta(t, 100.0, cfg.Stats.RecoverySpeed, 1e-9)
	require.InDelta(t, 0.5, cfg.ArtMods["Assassinate"].RecastMult, 1e-9)
	require.InDelta(t, 0.5, cfg.ArtMods["Mortal Blade"].RecastMult, 1e-9)

	raid, err := cfg.ContextBlock("raid")
	require.NoError(t, err)
	require.InDelta(t, 114.2, raid.DPSMod, 1e-9) // from context
	require.InDelta(t, 34.2, raid.MultiAttack, 1e-9)
	require.InDelta(t, 37.4, raid.CastSpeed, 1e-9) // from [stats], folded in

	solo, err := cfg.ContextBlock("solo")
	require.NoError(t, err)
	require.InDelta(t, 0.0, solo.DPSMod, 1e-9)
	require.InDelta(t, 100.0, solo.RecoverySpeed, 1e-9)
}

func TestContextBlockUnknownContext(t *testing.T) {
	cfg, err := Load(writeConfig(t, minimalValid))
	require.NoError(t, err)
	_, err = cfg.ContextBlock("raid")
	require.ErrorContains(t, err, `context "raid" not found`)
}

func TestLoadRejectsUnknownStatKey(t *testing.T) {
	_, err := Load(writeConfig(t, `
[character]
name = "T"
class = "assassin"
art_tier = "expert"
[stats]
cast_sped = 37.4
[contexts.solo]
multiattack = 10
`))
	require.ErrorContains(t, err, "cast_sped") // typo must not silently vanish
}

func TestLoadRejectsUnsupportedClass(t *testing.T) {
	_, err := Load(writeConfig(t, `
[character]
name = "T"
class = "brigand"
art_tier = "expert"
[contexts.solo]
multiattack = 10
`))
	require.ErrorContains(t, err, "unsupported class")
}

func TestLoadRejectsBadRecastMult(t *testing.T) {
	for _, v := range []string{"0", "1.5", "-0.2"} {
		_, err := Load(writeConfig(t, `
[character]
name = "T"
class = "assassin"
art_tier = "expert"
[art_mods."X"]
recast_mult = `+v+`
[contexts.solo]
multiattack = 10
`))
		require.ErrorContains(t, err, "recast_mult", v)
	}
}

func TestLoadRejectsNoContexts(t *testing.T) {
	_, err := Load(writeConfig(t, `
[character]
name = "T"
class = "assassin"
art_tier = "expert"
`))
	require.ErrorContains(t, err, "at least one context")
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("nope.toml")
	require.Error(t, err)
}

// --- Task 4: ApplyArtMods ---

func TestApplyArtMods(t *testing.T) {
	arts := []spell.CombatArt{
		{Name: "Assassinate II", RecastSecs: 300},
		{Name: "Eviscerate V", RecastSecs: 60},
	}
	out, err := ApplyArtMods(arts, map[string]ArtMod{"Assassinate": {RecastMult: 0.5}})
	require.NoError(t, err)
	require.InDelta(t, 0.5, out[0].RecastReduction, 1e-9) // matched by base name (rank-insensitive)
	require.InDelta(t, 0.0, out[1].RecastReduction, 1e-9)
	require.InDelta(t, 0.0, arts[0].RecastReduction, 1e-9) // input not mutated
}

func TestApplyArtModsTypoFailsLoudly(t *testing.T) {
	arts := []spell.CombatArt{{Name: "Assassinate II", RecastSecs: 300}}
	_, err := ApplyArtMods(arts, map[string]ArtMod{"Assasinate": {RecastMult: 0.5}})
	require.ErrorContains(t, err, "Assasinate") // a typo must not silently un-halve the big hit
}

// --- Review fix 1: stat-range validation ---

func TestLoadRejectsNegativeCastSpeedInStats(t *testing.T) {
	_, err := Load(writeConfig(t, `
[character]
name = "T"
class = "assassin"
art_tier = "expert"
[stats]
cast_speed = -150
[contexts.solo]
multiattack = 10
`))
	require.ErrorContains(t, err, "cast_speed")
}

func TestLoadRejectsNegativeDPSModInContext(t *testing.T) {
	_, err := Load(writeConfig(t, `
[character]
name = "T"
class = "assassin"
art_tier = "expert"
[contexts.raid]
dpsmod = -10
`))
	require.ErrorContains(t, err, "dpsmod")
	require.ErrorContains(t, err, "raid")
}

func TestLoadMainStatAndPotencyFields(t *testing.T) {
	cfg, err := Load("../../characters/biffels/config.toml")
	require.NoError(t, err)
	require.InDelta(t, 156.0, cfg.Stats.MainStat, 1e-9)
	require.InDelta(t, 5.0, cfg.Stats.Potency, 1e-9)
	require.InDelta(t, 24.6, cfg.Stats.PotencyBonus, 1e-9)
	require.InDelta(t, 15.0, cfg.ArtMods["Assassinate"].PotencyAdd, 1e-9)
	require.InDelta(t, 15.0, cfg.ArtMods["Mortal Blade"].PotencyAdd, 1e-9)

	raid, err := cfg.ContextBlock("raid")
	require.NoError(t, err)
	require.InDelta(t, 156.0, raid.MainStat, 1e-9) // [stats] folds into contexts
	require.InDelta(t, 24.6, raid.PotencyBonus, 1e-9)
}

func TestApplyArtModsPotencyRider(t *testing.T) {
	arts := []spell.CombatArt{{Name: "Assassinate II", RecastSecs: 300}}
	out, err := ApplyArtMods(arts, map[string]ArtMod{"Assassinate": {RecastMult: 0.5, PotencyAdd: 15}})
	require.NoError(t, err)
	require.InDelta(t, 0.5, out[0].RecastReduction, 1e-9)
	require.InDelta(t, 15.0, out[0].PotencyAdd, 1e-9)
}

func TestLoadRejectsNegativePotencyAdd(t *testing.T) {
	_, err := Load(writeConfig(t, `
[character]
name = "T"
class = "assassin"
art_tier = "expert"
[art_mods."X"]
recast_mult = 0.5
potency_add = -5
[contexts.solo]
multiattack = 10
`))
	require.ErrorContains(t, err, "potency_add")
}

// --- Task 1: LoadClass ---

func TestLoadClassAssassin(t *testing.T) {
	cd, err := LoadClass("../../classes", "assassin")
	require.NoError(t, err)
	require.InDelta(t, 2.0, cd.AutoAttackMultiplier, 1e-9)
}

func TestLoadClassMissingFile(t *testing.T) {
	_, err := LoadClass("../../classes", "wizard")
	require.Error(t, err)
}

func TestLoadClassRejectsMissingMultiplier(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "x.toml"), []byte("# empty\n"), 0o644))
	_, err := LoadClass(dir, "x")
	require.ErrorContains(t, err, "auto_attack_multiplier")
}

func TestLoadClassRejectsUnknownKey(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "x.toml"),
		[]byte("auto_attack_multiplier = 2.0\nbogus = 1\n"), 0o644))
	_, err := LoadClass(dir, "x")
	require.ErrorContains(t, err, "bogus")
}

func TestLoadCensusFields(t *testing.T) {
	p := writeConfig(t, `
[character]
name = "Alex"
class = "assassin"
art_tier = "expert"
census_name = "Biffels"
world = 618
[contexts.solo]
mainstat = 156
`)
	cfg, err := Load(p)
	require.NoError(t, err)
	require.Equal(t, "Biffels", cfg.Character.CensusName)
	require.Equal(t, 618, cfg.Character.World)
}

func TestLoadCensusFieldsOptional(t *testing.T) {
	p := writeConfig(t, `
[character]
name = "Alex"
class = "assassin"
art_tier = "expert"
[contexts.solo]
mainstat = 156
`)
	cfg, err := Load(p)
	require.NoError(t, err)
	require.Equal(t, "", cfg.Character.CensusName)
	require.Equal(t, 0, cfg.Character.World)
}
