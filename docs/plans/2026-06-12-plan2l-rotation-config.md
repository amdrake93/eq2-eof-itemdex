# Plan 2l — Rotation Mechanics Revision + Character Config Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the disproven reuse/recovery constants with the live-measured rules (reuse 1%/pt cap 50, shared per-art 50% recast ceiling, cast-speed divisor, recovery subtractive), and move all per-character values (AA stats, art mods, buff contexts) from hardcoded Go into a TOML character config.

**Architecture:** New `internal/charconfig` package owns the TOML schema (strict validation, `[stats]`/`[art_mods]`/`[contexts]`); art mods land as a `RecastReduction` field on `spell.CombatArt` applied post-load, so no DPS-function signatures grow; the rotation derives slot timing from two new `StatBlock` fields (`CastSpeed`, `RecoverySpeed`). `internal/baseline` is deleted; `cmd/weights`/`cmd/bis` take `-character`. Cast speed is already in `bis.db` as raw `spelltimecastpct` rows (419 items) — exposing it is one entry in `modifierToField`, no DB rebuild.

**Tech Stack:** Go 1.26, `github.com/BurntSushi/toml` (new dep), testify.

**Spec:** `docs/design-plan2.md` §3.1 + §4 (commit `7d04194`).

**Pre-computed expectations used below:**
- Eviscerate-like: `60 × (1 − 3.8/100) = 57.72`
- Assassinate-like: mod 0.5 + reuse 3.8 → reduction `min(0.50, 0.5+0.038) = 0.50` → `300 × 0.5 = 150`
- Head Shot slot at cast_speed 37.4, recovery 100: `2.0/1.374 + 0 = 1.45560` (tol 1e-4)
- Old `recoverySecs` param → new `RecoverySpeed` stat mapping (slot math identical): `0 ↔ 100`, `0.25 ↔ 50`, `0.5 ↔ 0`

---

### Task 1: `internal/charconfig` — TOML loader + the committed character file

**Files:**
- Create: `characters/alex.toml`
- Create: `internal/charconfig/charconfig.go`
- Test: `internal/charconfig/charconfig_test.go`
- Modify: `go.mod`/`go.sum` (new dep)

- [ ] **Step 1: Add the dependency**

Run: `go get github.com/BurntSushi/toml@latest`
Expected: go.mod gains `github.com/BurntSushi/toml`. If the network is unavailable, report BLOCKED.

- [ ] **Step 2: Create `characters/alex.toml`**

```toml
# Character config — per-player inputs (docs/design-plan2.md §4).
# [stats] = AA/innate bonuses; [contexts.X] = the FULL buff package on you in
# that situation (self-buffs listed in every context they're up); gear is the
# optimizer's output, never config.

[character]
name     = "Alex"
class    = "assassin"
art_tier = "expert"

[stats]
cast_speed     = 37.4 # AA total (measured: Head Shot 2.0s -> 1.46s tooltip)
recovery_speed = 100  # AA total (tooltips read "Recovery: Instant")

[art_mods."Assassinate"]
recast_mult = 0.5 # AA halving; fills the art's 50% recast-reduction ceiling
[art_mods."Mortal Blade"]
recast_mult = 0.5 # AA halving; fills the ceiling

[contexts.solo]
multiattack = 34.2 # Villainy IV (maintained self-buff)

[contexts.raid]
multiattack = 34.2  # Villainy IV
dpsmod      = 114.2 # coercer 74 + inquis 30.2 + dirge 10 (live estimate 2026-06)
critchance  = 31.0  # buffed estimate; split the AA portion into [stats] when measured
```

- [ ] **Step 3: Write the failing tests** — `internal/charconfig/charconfig_test.go`:

```go
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
	require.InDelta(t, 114.2, raid.DPSMod, 1e-9)   // from context
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
```

- [ ] **Step 4: Run to verify failure** — `go test ./internal/charconfig/ -v` → FAIL (`undefined: Load`).

- [ ] **Step 5: Implement** — `internal/charconfig/charconfig.go`:

```go
// Package charconfig loads per-character TOML config (docs/design-plan2.md §4):
// AA/innate stats, per-art AA modifiers, and buff-package contexts. Server-wide
// combat mechanics stay in internal/constants — config is everything about one
// player and their group. Validation is strict: unknown keys are errors, so a
// typo'd stat cannot silently vanish.
package charconfig

import (
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
)

type Character struct {
	Name    string `toml:"name"`
	Class   string `toml:"class"`
	ArtTier string `toml:"art_tier"`
}

// StatGrants is a stat block as written in TOML ([stats] or one [contexts.X]).
type StatGrants struct {
	Haste         float64 `toml:"haste"`
	MultiAttack   float64 `toml:"multiattack"`
	CritChance    float64 `toml:"critchance"`
	Potency       float64 `toml:"potency"`
	DPSMod        float64 `toml:"dpsmod"`
	Reuse         float64 `toml:"reuse"`
	Flurry        float64 `toml:"flurry"`
	AbilityMod    float64 `toml:"abilitymod"`
	CastSpeed     float64 `toml:"cast_speed"`
	RecoverySpeed float64 `toml:"recovery_speed"`
}

// Block converts the grants to a model.StatBlock.
func (g StatGrants) Block() model.StatBlock {
	return model.StatBlock{
		Haste:         g.Haste,
		MultiAttack:   g.MultiAttack,
		CritChance:    g.CritChance,
		Potency:       g.Potency,
		DPSMod:        g.DPSMod,
		Reuse:         g.Reuse,
		Flurry:        g.Flurry,
		AbilityMod:    g.AbilityMod,
		CastSpeed:     g.CastSpeed,
		RecoverySpeed: g.RecoverySpeed,
	}
}

// ArtMod is a per-art AA effect ([art_mods."Name"]). RecastMult multiplies the
// art's base recast (0.5 = the AA halving); it counts against the art's shared
// 50% recast-reduction ceiling.
type ArtMod struct {
	RecastMult float64 `toml:"recast_mult"`
}

type Config struct {
	Character Character             `toml:"character"`
	Stats     StatGrants            `toml:"stats"`
	ArtMods   map[string]ArtMod     `toml:"art_mods"`
	Contexts  map[string]StatGrants `toml:"contexts"`
}

// ContextBlock is the model input for one context: [stats] + that context's
// buff package (gear is added by the caller/optimizer).
func (c Config) ContextBlock(name string) (model.StatBlock, error) {
	ctx, ok := c.Contexts[name]
	if !ok {
		return model.StatBlock{}, fmt.Errorf("context %q not found in character config", name)
	}
	return c.Stats.Block().Add(ctx.Block()), nil
}

// Load parses and validates a character config file.
func Load(path string) (Config, error) {
	var cfg Config
	md, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return Config{}, err
	}
	if undec := md.Undecoded(); len(undec) > 0 {
		return Config{}, fmt.Errorf("%s: unknown config keys: %v", path, undec)
	}
	if cfg.Character.Class != "assassin" {
		return Config{}, fmt.Errorf("%s: unsupported class %q (only assassin is implemented)", path, cfg.Character.Class)
	}
	if cfg.Character.ArtTier != "expert" {
		return Config{}, fmt.Errorf("%s: unsupported art_tier %q (only expert is implemented)", path, cfg.Character.ArtTier)
	}
	for name, m := range cfg.ArtMods {
		if m.RecastMult <= 0 || m.RecastMult > 1 {
			return Config{}, fmt.Errorf("%s: art_mods[%q]: recast_mult %v out of range (0,1]", path, name, m.RecastMult)
		}
	}
	if len(cfg.Contexts) == 0 {
		return Config{}, fmt.Errorf("%s: config must define at least one context", path)
	}
	return cfg, nil
}
```

NOTE: this references `model.StatBlock.CastSpeed`/`RecoverySpeed`, which exist only after Task 2. **Execute Task 2 first if compiling in task order matters to you — or implement Tasks 1–2 in either order but only run/commit after both are in.** Recommended execution: do Task 2's model fields FIRST, then this task compiles standalone. (Tasks are written in config-first order for readability; the executor should build model fields → charconfig.)

- [ ] **Step 6: Run to verify pass** — `go test ./internal/charconfig/ -v` → PASS (7 tests). `make lint` → clean.

- [ ] **Step 7: Commit**

```bash
git add characters/alex.toml internal/charconfig/ go.mod go.sum
git commit -m "Rotation config: charconfig TOML loader + committed alex.toml"
```

---

### Task 2: Model stat plumbing — `CastSpeed`/`RecoverySpeed` fields, census mapping, weight stats

**Files:**
- Modify: `internal/model/stats.go`
- Modify: `internal/model/weights.go` (`WeightStats`, `bump`, `getStat`)
- Test: `internal/model/stats_test.go`, `internal/model/weights_test.go`

**Execute BEFORE Task 1's build/commit** (charconfig consumes these fields).

- [ ] **Step 1: Write the failing tests**

Append to `internal/model/stats_test.go`:

```go
func TestAddModifiersCastSpeed(t *testing.T) {
	var s StatBlock
	s.AddModifiers(map[string]float64{"spelltimecastpct": 1.6})
	require.InDelta(t, 1.6, s.CastSpeed, 1e-9)
}

func TestAddIncludesTimingStats(t *testing.T) {
	a := StatBlock{CastSpeed: 1.5, RecoverySpeed: 100}
	b := StatBlock{CastSpeed: 0.3}
	sum := a.Add(b)
	require.InDelta(t, 1.8, sum.CastSpeed, 1e-9)
	require.InDelta(t, 100.0, sum.RecoverySpeed, 1e-9)
}
```

Append to `internal/model/weights_test.go`:

```go
func TestWeightStatsIncludeCastSpeedNotRecovery(t *testing.T) {
	require.Contains(t, WeightStats, "castspeed")
	require.NotContains(t, WeightStats, "recoveryspeed") // not a gear stat in the EoF pool

	w := Weapon{AvgDamage: 100, DelaySecs: 2.0}
	cas := []spell.CombatArt{{Name: "X", MinDamage: 800, MaxDamage: 1200, RecastSecs: 0}}
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, w) + CADPS(sb, cas) }
	weights := DeriveWeights(StatBlock{}, dps)
	_, ok := weights["castspeed"]
	require.True(t, ok)
	// A zero-recast (spammable) art makes the CA timeline cast-bound, so faster
	// casts must add DPS.
	require.Greater(t, weights["castspeed"], 0.0)
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./internal/model/ -run 'TestAddModifiersCastSpeed|TestAddIncludes|TestWeightStats' -v` → FAIL (unknown fields).

- [ ] **Step 3: Implement**

`internal/model/stats.go` — add to the `StatBlock` struct:

```go
	CastSpeed     float64 // spelltimecastpct — divides CA cast times (gear + AAs)
	RecoverySpeed float64 // AA-only (no EoF gear carries it) — shrinks the 0.5s post-cast recovery
```

Add to `modifierToField`:

```go
	"spelltimecastpct":   func(s *StatBlock, v float64) { s.CastSpeed += v },
```

Add to `Add`:

```go
		CastSpeed:     s.CastSpeed + o.CastSpeed,
		RecoverySpeed: s.RecoverySpeed + o.RecoverySpeed,
```

`internal/model/weights.go` — `WeightStats` becomes:

```go
var WeightStats = []string{"haste", "multiattack", "critchance", "potency", "dpsmod", "reuse", "flurry", "abilitymod", "castspeed"}
```

Add cases to `bump` and `getStat` (both switches):

```go
	case "castspeed":
		sb.CastSpeed += delta      // bump; getStat: return sb.CastSpeed
	case "recoveryspeed":
		sb.RecoverySpeed += delta  // bump; getStat: return sb.RecoverySpeed
```

(`recoveryspeed` gets switch cases so `setStat` works generically, but is deliberately NOT in `WeightStats`.)

- [ ] **Step 4: Run to verify pass** — `go test ./internal/model/ -v` → all PASS (existing tests unaffected: new fields default 0; the weight test needs Task 3's rotation only if CADPS pacing changed — it hasn't yet, and a faster cast already fits more zero-recast casts via the existing per-art cast handling... **NOTE**: until Task 3 wires `CastSpeed` into the slot computation, `weights["castspeed"]` is 0, so `require.Greater(..., 0.0)` FAILS. **Mark that single assertion as the cross-task red bar**: run it, observe FAIL, leave the test in place, and proceed to Task 3 — it goes green there. Commit Tasks 2+3 together if you want every commit green, or commit Task 2 with the castspeed-weight test commented IN but the task explicitly continuing to Task 3 in the same commit. Recommended: **one commit covering Tasks 2+3.**)

- [ ] **Step 5: Proceed to Task 3 (same commit).**

---

### Task 3: Rotation mechanics — measured reuse/cast/recovery rules

**Files:**
- Modify: `internal/constants/constants.go`
- Modify: `internal/spell/pull.go` (CombatArt field)
- Modify: `internal/model/rotation.go`
- Modify: `internal/model/dps.go:41-45` (CADPS)
- Modify: `internal/bis/render.go:124-125` (assumptions line — references removed constants, must change in this commit)
- Test: `internal/model/rotation_test.go`, `internal/model/timeline_test.go`, `internal/model/dps_test.go`

- [ ] **Step 1: Update test expectations first**

`internal/model/rotation_test.go` — existing calls use the old 4-arg signature `RotationCADPS(sb, cas, dur, recoverySecs)`. Convert every call with the equivalence mapping (slot math identical): `recoverySecs 0 → sb.RecoverySpeed = 100`, `0.25 → 50`, `0.5 → 0` (leave sb.RecoverySpeed zero). All numeric expectations stay the same. Example conversions:

```go
// was: RotationCADPS(StatBlock{}, cas, 10, 0)
RotationCADPS(StatBlock{RecoverySpeed: 100}, cas, 10)
// was: RotationCADPS(StatBlock{Reuse: 100}, cas, 600, 0.5)
RotationCADPS(StatBlock{Reuse: 100}, cas, 600)
// was: RotationCADPS(StatBlock{}, fast, 600, 0.25)
RotationCADPS(StatBlock{RecoverySpeed: 50}, fast, 600)
```

In `TestRotationCADPS_ReuseHelps`, update the comment: reuse 100 is now clamped to the 50-stat cap, which still halves recast — the expectation is unchanged, the mechanism is the cap.

Then ADD new tests to `rotation_test.go`:

```go
func TestEffRecastMeasuredRules(t *testing.T) {
	evis := spell.CombatArt{Name: "Eviscerate V", RecastSecs: 60}
	// Full-strength conversion: 60 × (1 − 3.8/100) = 57.72 (live tooltip: 57.8 @ 3.8 reuse)
	require.InDelta(t, 57.72, effRecast(StatBlock{Reuse: 3.8}, evis), 1e-9)
	// Reuse stat caps at 50 → recast can halve but never beat the ceiling.
	require.InDelta(t, 30.0, effRecast(StatBlock{Reuse: 50}, evis), 1e-9)
	require.InDelta(t, 30.0, effRecast(StatBlock{Reuse: 100}, evis), 1e-9)
}

func TestEffRecastSharedCeiling(t *testing.T) {
	assassinate := spell.CombatArt{Name: "Assassinate II", RecastSecs: 300, RecastReduction: 0.5}
	// AA halving fills the entire 50% ceiling: reuse adds nothing (live: pinned 2m30s).
	require.InDelta(t, 150.0, effRecast(StatBlock{}, assassinate), 1e-9)
	require.InDelta(t, 150.0, effRecast(StatBlock{Reuse: 3.8}, assassinate), 1e-9)
	require.InDelta(t, 150.0, effRecast(StatBlock{Reuse: 50}, assassinate), 1e-9)

	// A partial art mod leaves headroom under the ceiling for reuse.
	partial := spell.CombatArt{Name: "Y", RecastSecs: 100, RecastReduction: 0.3}
	require.InDelta(t, 60.0, effRecast(StatBlock{Reuse: 10}, partial), 1e-9) // 0.3+0.1
	require.InDelta(t, 50.0, effRecast(StatBlock{Reuse: 30}, partial), 1e-9) // 0.3+0.3 → ceiling
}

func TestSlotTiming(t *testing.T) {
	headShot := spell.CombatArt{Name: "Head Shot IV", RecastSecs: 60, CastSecsHundredths: 200}
	// Divisor cast speed (live: 2.0s → 1.46s @ 37.4%), recovery gone at 100.
	require.InDelta(t, 2.0/1.374, slotSecs(StatBlock{CastSpeed: 37.4, RecoverySpeed: 100}, headShot), 1e-9)
	// No timing stats: fallback cast 0.5 + full base recovery 0.5.
	quick := spell.CombatArt{Name: "Q", RecastSecs: 10}
	require.InDelta(t, 1.0, slotSecs(StatBlock{}, quick), 1e-9)
	// Recovery is subtractive: 50% → 0.25s left.
	require.InDelta(t, 0.75, slotSecs(StatBlock{RecoverySpeed: 50}, quick), 1e-9)
}
```

`internal/model/timeline_test.go` — same signature conversion: `TestRotationCADPS_RecoveryPacesThroughput` uses `RotationCADPS(sb, cas, 600)` with `sb{RecoverySpeed: 50}` for the old `0.25` and `sb{RecoverySpeed: 100}` for the old `0.0`; `TestCADPS_UsesRecoveryPacedRotation` becomes `want := RotationCADPS(sb, cas, constants.FightDurationSecs)` (same `sb` on both sides — still a pure pass-through check).

`internal/model/dps_test.go` — the `CADPS(StatBlock{Reuse: 100}, ...)` ≈ 200 expectation is unchanged (reuse 100 clamps to 50 → recast still halves; the slot grows 0.75→1.0 with zero RecoverySpeed but stays below the 5s recast, so cast count is identical). Update only if the run disagrees — then investigate, do not fudge.

- [ ] **Step 2: Run to verify the right failures** — `go test ./internal/model/` → compile errors on the old signature + `undefined: slotSecs`, `unknown field RecastReduction`.

- [ ] **Step 3: Implement**

`internal/constants/constants.go` — delete `ReuseHalvesAt`/`ReuseHalveCoeff`/`CARecoverySecs`, add:

```go
	ReuseCapStat            = 50.0 // reuse converts 1%/pt and the stat caps at 50 (= the 50% ceiling); measured: Eviscerate 60s → 57.8s @ 3.8 reuse
	RecastReductionCeiling  = 0.50 // per-art ceiling shared by ALL recast-reduction sources (AA mods + reuse); measured: Assassinate pinned at 2m30s with reuse gear
	CARecoveryBaseSecs      = 0.5  // server base post-cast recovery, reduced subtractively by the character's recovery-speed stat (100 → "Recovery: Instant")
```

Update the comment block above `FightDurationSecs` that mentions `CACastTimeSecs + CARecoverySecs = 0.75s` to: `// Rotation-sim parameters. Each cast occupies effCast + effRecovery (both reduced by the character's timing stats).`

`internal/spell/pull.go` — add to `CombatArt`:

```go
	RecastReduction float64 // per-art AA recast reduction (1 − recast_mult), set from character config; counts against the shared 50% ceiling
```

`internal/model/rotation.go` — delete the `aaCooldownReduction` var entirely; replace `effRecast` and the slot computation:

```go
// effRecast applies the measured recast rules: every reduction source (per-art
// AA mods + the reuse stat at 1%/pt, stat-capped at 50) shares one per-art
// ceiling of 50% of base. An AA-halved art (Assassinate, Mortal Blade) arrives
// with the ceiling already full, so reuse does nothing for it.
func effRecast(sb StatBlock, ca spell.CombatArt) float64 {
	reduction := ca.RecastReduction + math.Min(sb.Reuse, constants.ReuseCapStat)/100
	return ca.RecastSecs * (1 - math.Min(constants.RecastReductionCeiling, reduction))
}

// slotSecs is the CA-timeline slot one cast occupies: cast time divided by the
// cast-speed stat (measured divisor: Head Shot 2.0s → 1.46s @ 37.4%), plus the
// 0.5s base recovery shrunk subtractively by recovery speed (100 → instant).
func slotSecs(sb StatBlock, ca spell.CombatArt) float64 {
	castSecs := float64(ca.CastSecsHundredths) / 100
	if castSecs <= 0 {
		castSecs = constants.CACastTimeSecs
	}
	effCast := castSecs / (1 + sb.CastSpeed/100)
	effRecovery := constants.CARecoveryBaseSecs * (1 - math.Min(sb.RecoverySpeed, 100)/100)
	return effCast + effRecovery
}
```

`RotationCADPS` drops the `recoverySecs` parameter — new signature and slot init:

```go
func RotationCADPS(sb StatBlock, cas []spell.CombatArt, durationSecs float64) float64 {
	...
	for i, ca := range cas {
		eff[i] = CAEffectiveDamage(sb, ca)
		rec[i] = effRecast(sb, ca)
		slot[i] = slotSecs(sb, ca)
	}
	...
```

(also update the function's doc comment: timing comes from the stat block's cast/recovery speeds; remove the recoverySecs sentence). `internal/model/dps.go` `CADPS`:

```go
// CADPS is the simulated combat-art DPS over a standard fight (priority rotation).
// Slot pacing (cast + recovery) comes from the stat block's timing stats.
func CADPS(sb StatBlock, cas []spell.CombatArt) float64 {
	return RotationCADPS(sb, cas, constants.FightDurationSecs)
}
```

`internal/bis/render.go:124-125` — replace the reuse/recovery line:

```go
	fmt.Fprintf(b, "- reuse: 1%%/pt to the %.0f-stat cap, sharing each art's %.0f%%-of-base recast ceiling with AA art mods; cast speed divides cast times; recovery base %.2fs (reduced by recovery speed); fight = %.0fs\n",
		constants.ReuseCapStat, constants.RecastReductionCeiling*100, constants.CARecoveryBaseSecs, constants.FightDurationSecs)
```

- [ ] **Step 4: Run** — `go test ./internal/... -count=1` → model/bis/spell/charconfig all PASS, **including Task 2's castspeed-weight assertion** (CastSpeed now shrinks slots → more casts of the spammable art → positive weight). `cmd/` does not compile yet only if signatures leaked — it shouldn't (CADPS kept its signature; cmds don't call RotationCADPS directly — verify with `go build ./...`; `internal/baseline` still compiles untouched).

- [ ] **Step 5: Commit (Tasks 2+3 together)**

```bash
git add internal/model/ internal/constants/ internal/spell/pull.go internal/bis/render.go internal/charconfig/ characters/ go.mod go.sum
git commit -m "Rotation config: measured reuse/cast/recovery rules + timing stats (cast speed rankable)"
```

(If Task 1 was committed separately already, this commit covers Tasks 2+3 only.)

---

### Task 4: `ApplyArtMods` — config art mods onto the loaded art pool

**Files:**
- Modify: `internal/charconfig/charconfig.go` (append)
- Test: `internal/charconfig/charconfig_test.go` (append)

- [ ] **Step 1: Write the failing test** (append):

```go
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
```

Add `"github.com/amdrake93/eq2-eof-itemdex/internal/spell"` to the test imports.

- [ ] **Step 2: Run to verify failure** — `go test ./internal/charconfig/ -v` → FAIL (`undefined: ApplyArtMods`).

- [ ] **Step 3: Implement** (append to charconfig.go; add the `spell` import):

```go
// ApplyArtMods returns a copy of the art pool with each config art mod applied
// to the art whose base name (rank-stripped) matches. Every mod must match an
// art — a typo'd name failing loudly beats silently un-halving Assassinate.
func ApplyArtMods(arts []spell.CombatArt, mods map[string]ArtMod) ([]spell.CombatArt, error) {
	out := make([]spell.CombatArt, len(arts))
	copy(out, arts)

	matched := make(map[string]bool, len(mods))
	for i := range out {
		if m, ok := mods[spell.BaseName(out[i].Name)]; ok {
			out[i].RecastReduction = 1 - m.RecastMult
			matched[spell.BaseName(out[i].Name)] = true
		}
	}

	for name := range mods {
		if !matched[name] {
			return nil, fmt.Errorf("art_mods[%q] matches no loaded combat art", name)
		}
	}
	return out, nil
}
```

- [ ] **Step 4: Run to verify pass** — `go test ./internal/charconfig/ -v` → PASS. `make lint` → clean.

- [ ] **Step 5: Commit**

```bash
git add internal/charconfig/
git commit -m "Rotation config: ApplyArtMods maps config art mods onto the art pool"
```

---

### Task 5: Wire the cmds — `-character` flag, contexts replace baselines, per-art cast times actually load

**Files:**
- Modify: `internal/store/store.go:146-170` (`LoadLoadout` — add `cast_secs_hundredths`)
- Modify: `cmd/weights/main.go`
- Modify: `cmd/bis/main.go`
- Delete: `internal/baseline/` (both files)
- Test: `internal/store/loadout_test.go` (cast-time assertion)

- [ ] **Step 1: store — load per-art cast times (latent-bug fix)**

In `LoadLoadout`, the combat-arts query currently drops `cast_secs_hundredths`, so the whole bis pipeline paces every art at the 0.5s fallback. Change the query + scan:

```go
	rows, err := d.db.Query(`SELECT name, min_dmg, max_dmg, recast_secs, cast_secs_hundredths FROM combat_arts`)
	...
		if err := rows.Scan(&ca.Name, &ca.MinDamage, &ca.MaxDamage, &ca.RecastSecs, &ca.CastSecsHundredths); err != nil {
```

In `internal/store/loadout_test.go`, find the existing loadout test (it seeds combat_arts rows) and extend its seeded row + assertion to include a nonzero `cast_secs_hundredths` (e.g. 200) and `require.Equal(t, 200, lo.Arts[0].CastSecsHundredths)` — read the existing test first and follow its seeding pattern exactly.

Run: `go test ./internal/store/ -v` → PASS after the change (FAIL before, with the new assertion).

- [ ] **Step 2: cmd/weights — config-driven contexts**

Rewrite `cmd/weights/main.go`'s flag/baseline section (keep `loadCAs`/`loadWeapon` helpers, but add `cast_secs_hundredths` to the `loadCAs` query + scan exactly as in Step 1):

```go
	dbPath := flag.String("db", "bis.db", "sqlite db from builddb")
	character := flag.String("character", "characters/alex.toml", "character config (TOML)")
	flag.Parse()

	cfg, err := charconfig.Load(*character)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load character:", err)
		os.Exit(1)
	}
	...
	cas = spell.HighestRanks(cas)
	cas, err = charconfig.ApplyArtMods(cas, cfg.ArtMods)
	if err != nil {
		fmt.Fprintln(os.Stderr, "apply art mods:", err)
		os.Exit(1)
	}
```

and the baseline loop becomes a deterministic iteration over config contexts:

```go
	names := make([]string, 0, len(cfg.Contexts))
	for n := range cfg.Contexts {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		block, err := cfg.ContextBlock(name)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("\n== %s context weights (marginal DPS per +1 stat; dual-wield, %d combat arts) ==\n", strings.ToUpper(name), len(cas))
		...
		ws := model.DeriveWeights(block, dps)
		... (unchanged sorting/printing)
	}
```

Imports: drop `internal/baseline`, add `internal/charconfig`, `strings`.

- [ ] **Step 3: cmd/bis — tiers reference contexts by name**

In `cmd/bis/main.go`: add the same `-character` flag + `charconfig.Load` + after `db.LoadLoadout()` apply art mods:

```go
	lo.Arts, err = charconfig.ApplyArtMods(lo.Arts, cfg.ArtMods)
	if err != nil {
		fmt.Fprintln(os.Stderr, "apply art mods:", err)
		os.Exit(1)
	}
```

Resolve the two context blocks up front (hard error if the config lacks them — the report's tiers need exactly these):

```go
	solo, err := cfg.ContextBlock("solo")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	raid, err := cfg.ContextBlock("raid")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
```

Then mechanical swaps: `baseline.Solo` → `solo`, `baseline.Raid` → `raid` (three tier entries + the locked-items block at line ~145). Imports: drop `internal/baseline`, add `internal/charconfig`.

- [ ] **Step 4: Delete `internal/baseline`**

```bash
git rm -r internal/baseline
```

(Confirmed: its only consumers were the two cmds; the constants it re-exported are reachable via `internal/constants` everywhere else.)

- [ ] **Step 5: Verify** — `go build ./...` clean; `go test ./... -count=1` all PASS; `make lint` clean. Then run both cmds:

Run: `go run ./cmd/weights` → prints `RAID context weights` / `SOLO context weights` with a `castspeed` row present. `go run ./cmd/weights -character nope.toml` → clean error, exit 1.

- [ ] **Step 6: Commit**

```bash
git add internal/store/ cmd/weights/ cmd/bis/ internal/baseline
git commit -m "Rotation config: cmds take -character; contexts replace baselines; per-art cast times load (latent bug)"
```

---

### Task 6: End-to-end verification + report regeneration

**Files:** none new (regenerated `bis-report.md` stays untracked)

- [ ] **Step 1: Full gates** — `make test && make lint` → all green. `go run ./cmd/fitcurve` → constants unchanged (curve untouched by this plan).

- [ ] **Step 2: Regenerate** — `go run ./cmd/bis` → writes `bis-report.md`. Verify the Assumptions section shows the new reuse/cast/recovery line and the converged-weights tables include `castspeed`.

- [ ] **Step 3: Check the spec's predictions** (report in your summary, with numbers):
1. **Reuse artifact**: the raid converged-set reuse weight was **−4.81** pre-change; with reuse blind to Assassinate/Mortal Blade boundary casts the spec predicts it shrinks toward ~0 or goes positive. Quote the new value.
2. **dpsmod/haste** weights shouldn't move much (curve untouched).
3. **castspeed** appears with a small weight (rotation is cooldown-bound; cast speed only compresses cast-bound stretches).
4. Quote both contexts' full weight tables for the controller.

- [ ] **Step 4: Commit** — only if any file changed (test pins); otherwise nothing to commit (the report is untracked by design).

---

## Self-review notes

- **Spec coverage:** charconfig TOML + strict validation + class-agnostic schema ✓ (T1), stats/contexts decomposition + ContextBlock ✓ (T1), art-mod taxonomy with loud typo failure ✓ (T4), measured reuse rules + shared ceiling ✓ (T3), cast divisor + recovery subtractive ✓ (T3), `spelltimecastpct` translation ✓ (T2 — via `modifierToField`; **no DB rebuild needed**, the 419 raw rows are already stored), castspeed in WeightStats / recovery excluded ✓ (T2), baselines dissolved + `-character` on both cmds ✓ (T5), render line ✓ (T3), report regen + reuse-artifact prediction check ✓ (T6). Bonus latent-bug fix: per-art cast times now actually load (T5 Step 1) — previously every art paced at the 0.5s fallback in the bis pipeline.
- **Compile boundaries:** T1↔T2 (charconfig needs the new StatBlock fields — executor builds model fields first); T2↔T3 (the castspeed-weight assertion needs the rotation wiring — one commit for 2+3); T5 bundles both cmds with the baseline deletion. No commit may be non-compiling.
- **Type consistency:** `charconfig.Config{Character, Stats StatGrants, ArtMods map[string]ArtMod, Contexts map[string]StatGrants}`, `ContextBlock(name)`, `ApplyArtMods(arts, mods)`, `spell.CombatArt.RecastReduction`, `model.StatBlock.CastSpeed/RecoverySpeed`, `slotSecs`/`effRecast` — names match across tasks.
- **YAGNI check:** no per-class machinery, no reserved art-mod fields implemented (`damage_add` etc. stay schema-reserved in the spec only), no recovery gear stat, fitcurve untouched.
