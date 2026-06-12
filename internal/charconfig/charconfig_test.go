package charconfig

import (
	"os"
	"path/filepath"
	"testing"

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

func TestLoadCommittedAlexConfig(t *testing.T) {
	cfg, err := Load("../../characters/alex.toml")
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
