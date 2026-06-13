# Plan 2m — Measured CA Damage Equation (Main-Stat Scaling) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the tooltip-measured CA damage equation — main-stat (AGI) curve multiplier, potency pool with the calibrated hidden bonus (⚠ flagged mystery), per-art potency riders, ability-mod uncapped — and make main stat a rankable gear stat.

**Architecture:** `MainStat`/`PotencyBonus` join `StatBlock` (census `strength` key feeds MainStat); the AGI conversion is an interpolated sample table of the 13 committed readings (NO equation assumed — the curve flattens below 600), capped 1100 → 65%, mirrored in `data/mainstat-readings.csv` with a sync test; `CAEffectiveDamage` becomes `base × (1+potPool/100) × (1+curveMS/100) + abilityMod` with the cap constant deleted; `[art_mods]` gains `potency_add`. The unexplained ~23.4-point potency-pool component stays a calibrated config value and is FLAGGED in every report.

**Tech Stack:** Go 1.26, existing `curveInterp` machinery, testify. No new deps.

**Spec:** `docs/design-plan2.md` §3.1 "Combat-art damage equation" + §4 + §12 (commit `109c8fe`).

**Key measured values used below:** AGI readings (raw → %): (73, 6.08), (156, 15.01), (625, 51.74), (664, 53.74), (695, 55.22), (738, 57.10), (780, 58.74), (819, 60.10), (859, 61.33), (899, 62.39), (941, 63.32), (983, 64.06), (1100, 65.00 hard cap). AGI tooltips show two decimals → the curve is **unfloored** (unlike haste/dps-mod). Alex's config: mainstat 156, potency 5, potency_bonus 24.6, both AA arts potency_add 15.

---

### Task 1: Model plumbing — MainStat/PotencyBonus fields, census mapping, the AGI sample curve, weights

**Files:**
- Modify: `internal/model/stats.go`
- Modify: `internal/model/curve.go` (new sample table + accessor)
- Modify: `internal/model/weights.go` (`WeightStats`, `curveStats`, `bump`/`getStat`, `curveStatMarginal`)
- Test: `internal/model/stats_test.go`, `internal/model/curve_test.go`, `internal/model/weights_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/model/stats_test.go`:

```go
func TestAddModifiersMainStat(t *testing.T) {
	var s StatBlock
	// census "strength" = "+N primary attributes" → AGI point-for-point for a scout
	s.AddModifiers(map[string]float64{"strength": 46})
	require.InDelta(t, 46.0, s.MainStat, 1e-9)
}

func TestAddIncludesMainStatAndPotencyBonus(t *testing.T) {
	sum := StatBlock{MainStat: 156, PotencyBonus: 24.6}.Add(StatBlock{MainStat: 40})
	require.InDelta(t, 196.0, sum.MainStat, 1e-9)
	require.InDelta(t, 24.6, sum.PotencyBonus, 1e-9)
}
```

Append to `internal/model/curve_test.go`:

```go
func TestMainStatEffect(t *testing.T) {
	require.Equal(t, 0.0, MainStatEffect(0))
	// Committed readings reproduced exactly (unfloored — AGI tooltips show 2dp):
	require.InDelta(t, 6.08, MainStatEffect(73), 1e-9)
	require.InDelta(t, 15.01, MainStatEffect(156), 1e-9)
	require.InDelta(t, 51.74, MainStatEffect(625), 1e-9)
	require.InDelta(t, 64.06, MainStatEffect(983), 1e-9)
	// Interpolated between samples: (738,57.10)-(780,58.74) midpoint
	require.InDelta(t, 57.92, MainStatEffect(759), 1e-9)
	// Hard cap 1100 → 65, overcap clamps:
	require.InDelta(t, 65.0, MainStatEffect(1100), 1e-9)
	require.InDelta(t, 65.0, MainStatEffect(5000), 1e-9)
}
```

Append to `internal/model/weights_test.go`:

```go
func TestWeightStatsIncludeMainStat(t *testing.T) {
	require.Contains(t, WeightStats, "mainstat")
	require.NotContains(t, WeightStats, "potencybonus") // calibrated config value, not a gear stat
}

func TestCurveStatMarginalMainStat(t *testing.T) {
	cas := []spell.CombatArt{{Name: "X", MinDamage: 800, MaxDamage: 1200, RecastSecs: 0}}
	dps := func(sb StatBlock) float64 { return CADPS(sb, cas) }
	// At mainstat 700 the bracket is the (695, 738) samples; positive marginal
	// on a CA-only dps closure (mainstat multiplies CA damage).
	m := curveStatMarginal(StatBlock{MainStat: 700, RecoverySpeed: 100}, "mainstat", dps)
	require.Greater(t, m, 0.0)
}

func TestCurveStatMarginalMainStatAtCap(t *testing.T) {
	cas := []spell.CombatArt{{Name: "X", MinDamage: 800, MaxDamage: 1200, RecastSecs: 0}}
	dps := func(sb StatBlock) float64 { return CADPS(sb, cas) }
	require.InDelta(t, 0.0, curveStatMarginal(StatBlock{MainStat: 1100, RecoverySpeed: 100}, "mainstat", dps), 1e-9)
}
```

(NOTE: the marginal tests exercise `CAEffectiveDamage`'s mainstat multiplier, which lands in Task 2 — they stay RED until then. Tasks 1+2 are one commit.)

- [ ] **Step 2: Run to verify failures** — `go test ./internal/model/ -run 'MainStat' -v` → FAIL (unknown fields/functions).

- [ ] **Step 3: Implement**

`internal/model/stats.go` — `StatBlock` gains:

```go
	MainStat      float64 // strength key = "+N primary attributes" → AGI for a scout; multiplies CA damage via its curve
	PotencyBonus  float64 // calibrated hidden potency-pool points (config-only; ⚠ spec §12 open mystery)
```

`modifierToField` gains:

```go
	"strength":           func(s *StatBlock, v float64) { s.MainStat += v }, // "+N primary attributes" (explicit agility/wisdom/intelligence keys excluded — data-suspicious, spec §11)
```

`Add()` gains both fields (same pattern as the others).

`internal/model/curve.go` — add below `multiAttackSamples`:

```go
// mainStatSamples: AGI → CA-damage % (13 live readings, data/mainstat-readings.csv;
// the sync test in internal/fit pins this table to the CSV). UNFLOORED — AGI
// tooltips display two decimals ("Agility increases your damage by 64.06%").
// The curve flattens below ~600 (73→6.08 sits under the high-range trend), so
// no equation is assumed — interpolation only. Hard cap 1100 → 65% (confirmed).
// High range fits a quadratic peaking ≈1142 — the third "peak just past the
// cap" dev signature — commentary only, not modeled.
var mainStatSamples = []curvePoint{
	{0, 0}, {73, 6.08}, {156, 15.01}, {625, 51.74}, {664, 53.74}, {695, 55.22},
	{738, 57.10}, {780, 58.74}, {819, 60.10}, {859, 61.33}, {899, 62.39},
	{941, 63.32}, {983, 64.06}, {1100, 65},
}

// MainStatEffect is the CA-damage % for a main-stat value: interpolated,
// unfloored, clamped at the 1100 cap (curveInterp clamps past the top sample).
func MainStatEffect(stat float64) float64 { return curveInterp(mainStatSamples, stat) }
```

`internal/model/weights.go`:
- `WeightStats` gains `"mainstat"` (append at the end).
- `curveStats` map gains `"mainstat": true`.
- `bump`/`getStat` gain cases `"mainstat"` and `"potencybonus"` (PotencyBonus gets switch cases for generic completeness but stays out of `WeightStats`).
- `curveStatMarginal`'s switch gains:

```go
	case "mainstat":
		lo, hi = curveBracket(mainStatSamples, v)
```

(no explicit cap guard needed: at/above the 1100 top sample `curveBracket` returns `(top, top)` → the `hi <= lo` guard yields 0, the multi-attack pattern.)

- [ ] **Step 4: Run** — `go test ./internal/model/ -v`: stats/curve tests PASS; the two marginal tests stay RED (CAEffectiveDamage not yet wired). Proceed to Task 2 — same commit.

---

### Task 2: The measured CA damage equation

**Files:**
- Modify: `internal/model/rotation.go:10-18` (`CAEffectiveDamage`)
- Modify: `internal/constants/constants.go:13` (delete `AbilityModCapFrac`)
- Modify: `internal/spell/pull.go` (`CombatArt` field)
- Modify: `internal/bis/render.go:120-121` (references the deleted constant — must change in this commit)
- Test: `internal/model/rotation_test.go:66-69`

- [ ] **Step 1: Update/extend tests first**

In `internal/model/rotation_test.go`, replace `TestCAEffectiveDamage_Potency` with:

```go
func TestCAEffectiveDamageMeasuredEquation(t *testing.T) {
	ca := spell.CombatArt{Name: "X", MinDamage: 800, MaxDamage: 1200}
	require.InDelta(t, 1000.0, CAEffectiveDamage(StatBlock{}, ca), 0.01)            // avg 1000, no stats
	require.InDelta(t, 1100.0, CAEffectiveDamage(StatBlock{Potency: 10}, ca), 0.01) // ×1.1

	// PotencyBonus pools additively with potency (⚠ §12: includes the unexplained innate).
	require.InDelta(t, 1300.0, CAEffectiveDamage(StatBlock{Potency: 10, PotencyBonus: 20}, ca), 0.01)

	// Per-art potency rider pools too (the cooldown AA's +15 to Assassinate/Mortal Blade).
	rider := spell.CombatArt{Name: "Y", MinDamage: 800, MaxDamage: 1200, PotencyAdd: 15}
	require.InDelta(t, 1450.0, CAEffectiveDamage(StatBlock{Potency: 10, PotencyBonus: 20}, rider), 0.01)

	// Main stat multiplies on top: mainstat 625 → 51.74% → ×1.5174.
	require.InDelta(t, 1517.4, CAEffectiveDamage(StatBlock{MainStat: 625}, ca), 0.01)

	// Ability mod adds IN FULL — the old 50%-of-adjusted-base cap is disproven.
	// Old rule would cap 738 at 0.5×1200=600; measured tooltips show the full add.
	require.InDelta(t, 1738.0, CAEffectiveDamage(StatBlock{AbilityMod: 738}, ca), 0.01)

	// Full stack, hand-computed: avg 1000 × (1+(57.7+24.6)/100) × (1+0.6406) + 738
	// = 1000 × 1.823 × 1.6406 + 738 = 2990.81 + 738 = 3728.81
	full := StatBlock{Potency: 57.7, PotencyBonus: 24.6, MainStat: 983, AbilityMod: 738}
	require.InDelta(t, 3728.81, CAEffectiveDamage(full, ca), 0.05)
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./internal/model/ -run TestCAEffectiveDamage -v` → FAIL (unknown field `PotencyAdd`, capped ability-mod math).

- [ ] **Step 3: Implement**

`internal/spell/pull.go` — `CombatArt` gains:

```go
	PotencyAdd      float64 // per-art AA potency rider (config [art_mods]), pooled with potency
```

`internal/constants/constants.go` — delete the `AbilityModCapFrac` line entirely.

`internal/model/rotation.go` — replace `CAEffectiveDamage`:

```go
// CAEffectiveDamage is one cast's damage under the measured equation (spec
// §3.1, tooltip-calibrated 2026-06-12 across 4 gear/AA states × 3 probe arts):
// the potency pool (displayed potency + the calibrated PotencyBonus — ⚠ spec
// §12 open mystery — + the art's AA rider) and the main-stat curve each
// multiply the base; ability mod adds IN FULL (the old 50%-of-adjusted-base
// cap is disproven — Quick Strike at AM 738 tooltips the whole add). A small
// measured per-art enhancer (≈ AM × base_max/3400) is documented, not modeled.
func CAEffectiveDamage(sb StatBlock, ca spell.CombatArt) float64 {
	potPool := 1 + (sb.Potency+sb.PotencyBonus+ca.PotencyAdd)/100
	mainStat := 1 + MainStatEffect(sb.MainStat)/100
	avgBase := (ca.MinDamage + ca.MaxDamage) / 2 * potPool * mainStat
	return (avgBase + sb.AbilityMod) * critFactor(sb)
}
```

`internal/bis/render.go:120-121` — the crit/flurry/cap line loses the cap clause; the mystery + mainstat lines are added (full new `writeAssumptions` body shown in Task 4 Step 2 — apply just the compile-fixing first line now if executing tasks separately, or the whole body if combining):

```go
	fmt.Fprintf(b, "- crit ×%.2f; flurry ×%.1f; ability-mod adds in full (50%% cap disproven by tooltip probes)\n",
		constants.CritMultiplier, constants.FlurryMultiplier)
```

- [ ] **Step 4: Run the full suite** — `go test ./... -count=1` → ALL PASS, including Task 1's marginal tests (CADPS now responds to mainstat). `make lint` → clean. (`dps_test`'s CADPS≈200 expectation is unaffected: zero MainStat/PotencyBonus → multiplier 1, AM 0 → cap removal moot.)

- [ ] **Step 5: Commit (Tasks 1+2)**

```bash
git add internal/model/ internal/constants/constants.go internal/spell/pull.go internal/bis/render.go
git commit -m "CA equation: measured potency pool + mainstat curve; ability-mod cap disproven and removed"
```

---

### Task 3: Config + readings CSV + sync test

**Files:**
- Modify: `internal/charconfig/charconfig.go` (StatGrants/ArtMod fields, validation, ApplyArtMods rider)
- Modify: `characters/alex.toml`
- Create: `data/mainstat-readings.csv`
- Create: `internal/fit/mainstat_sync_test.go`
- Test: `internal/charconfig/charconfig_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/charconfig/charconfig_test.go`:

```go
func TestLoadMainStatAndPotencyFields(t *testing.T) {
	cfg, err := Load("../../characters/alex.toml")
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
```

Create `internal/fit/mainstat_sync_test.go`:

```go
package fit

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/stretchr/testify/require"
)

// The main-stat sample table in internal/model must mirror the committed
// readings byte-for-byte: append a reading to the CSV without updating the
// table (or vice versa) and this fails.
func TestMainStatSamplesMatchReadings(t *testing.T) {
	rs, err := LoadReadings("../../data/mainstat-readings.csv")
	require.NoError(t, err)
	require.NotEmpty(t, rs)

	msg := "main-stat table is stale — sync internal/model/curve.go mainStatSamples with data/mainstat-readings.csv"
	for _, r := range rs {
		require.Equal(t, r.Stat, "agi", "unexpected stat in mainstat readings")
		require.InDelta(t, r.Effect, model.MainStatEffect(r.Raw), 1e-9, msg)
	}
}
```

- [ ] **Step 2: Run to verify failures** — `go test ./internal/charconfig/ ./internal/fit/ -v` → FAIL (unknown fields, missing CSV).

- [ ] **Step 3: Implement**

`data/mainstat-readings.csv` (13 rows; same schema as curve-readings — `fit.LoadReadings` parses it as-is; effect carries two decimals because AGI tooltips are unfloored):

```csv
stat,raw,effect,era
agi,73,6.08,live
agi,156,15.01,live
agi,625,51.74,live
agi,664,53.74,live
agi,695,55.22,live
agi,738,57.10,live
agi,780,58.74,live
agi,819,60.10,live
agi,859,61.33,live
agi,899,62.39,live
agi,941,63.32,live
agi,983,64.06,live
agi,1100,65,live
```

`internal/charconfig/charconfig.go`:
- `StatGrants` gains `MainStat float64 \`toml:"mainstat"\`` and `PotencyBonus float64 \`toml:"potency_bonus"\`` (Potency already exists); `Block()` and `nonNegative()` gain both.
- `ArtMod` gains `PotencyAdd float64 \`toml:"potency_add"\``; validation in `Load` rejects negative:

```go
		if m.PotencyAdd < 0 {
			return Config{}, fmt.Errorf("%s: art_mods[%q]: potency_add %v is negative", path, name, m.PotencyAdd)
		}
```

- `ApplyArtMods` also sets `out[i].PotencyAdd = m.PotencyAdd` next to the RecastReduction line.

`characters/alex.toml` — `[stats]` gains:

```toml
mainstat       = 156  # innate + AA agility (naked-with-AAs reading)
potency        = 5    # AA potency (displayed in the character window)
potency_bonus  = 24.6 # calibrated hidden potency-pool points (⚠ spec §12 OPEN MYSTERY — naked-tooltip procedure)
```

and both art-mod blocks gain `potency_add = 15 # the same AA's potency rider, per its own text`.

- [ ] **Step 4: Run** — `go test ./... -count=1` → all PASS; `make lint` → clean.

- [ ] **Step 5: Commit**

```bash
git add internal/charconfig/ characters/alex.toml data/mainstat-readings.csv internal/fit/mainstat_sync_test.go
git commit -m "CA equation: config mainstat/potency_bonus/potency_add; mainstat readings CSV + sync test"
```

---

### Task 4: The flagged mystery in the report + end-to-end verification

**Files:**
- Modify: `internal/bis/render.go` (`writeAssumptions`)

- [ ] **Step 1: Verify the compile-fix line from Task 2 landed**, then replace the full `writeAssumptions` with:

```go
func writeAssumptions(b *strings.Builder) {
	b.WriteString("---\n\n## Assumptions & Constants\n\n")
	fmt.Fprintf(b, "- crit ×%.2f; flurry ×%.1f; ability-mod adds in full (50%% cap disproven by tooltip probes)\n",
		constants.CritMultiplier, constants.FlurryMultiplier)
	fmt.Fprintf(b, "- haste & dps-mod: fitted quadratic %.6f·s − %.8f·s², hard cap %.0f stat → %.0f%%\n",
		model.HasteDpsModA, model.HasteDpsModB, constants.HasteStatCap, model.HasteDpsModEffect(constants.HasteStatCap))
	fmt.Fprintf(b, "- main stat (AGI): interpolated 13-reading curve, hard cap 1100 → %.0f%%; multiplies CA damage (auto-attack scaling unverified — not modeled)\n",
		model.MainStatEffect(1100))
	b.WriteString("- ⚠ CA potency pool includes a per-character calibrated `potency_bonus` whose source is UNEXPLAINED (~23.4 pts survive naked/AA-less/buff-less — spec §12 'potency-pool mystery', actively hunted)\n")
	fmt.Fprintf(b, "- reuse: 1%%/pt to the %.0f-stat cap, sharing each art's %.0f%%-of-base recast ceiling with AA art mods; cast speed divides cast times; recovery base %.2fs (reduced by recovery speed); fight = %.0fs\n",
		constants.ReuseCapStat, constants.RecastReductionCeiling*100, constants.CARecoveryBaseSecs, constants.FightDurationSecs)
	b.WriteString("- Set built by coordinate-ascent to convergence (caps/interactions resolved at the live set baseline).\n")
	b.WriteString("- Main-hand is fixed (Soulfire Sabre); its weapon damage AND full stat line are included in the baseline.\n")
}
```

- [ ] **Step 2: Full gates** — `make test && make lint` → green. `go run ./cmd/fitcurve` → haste/dpsmod constants unchanged (mainstat CSV is a separate file; the joint fit must NOT have moved).

- [ ] **Step 3: Regenerate and report** — `go run ./cmd/weights` then `go run ./cmd/bis` (bis-report.md stays untracked). In your summary paste:
1. Both cmd/weights context tables (a `mainstat` row must appear; expect it LARGE — it multiplies all CA damage and gear carries up to ~46/item)
2. All three converged weight tables from the report
3. The report's Assumptions section (the ⚠ mystery line must be present)
4. Top BiS changes you can spot — the spec predicts the 46-AGI mythical pieces (Chime set etc.) climb in BEST-OF-BEST now that their main stat scores

- [ ] **Step 4: Commit**

```bash
git add internal/bis/render.go
git commit -m "CA equation: report flags the potency-pool mystery; mainstat curve line"
```

---

## Self-review notes

- **Spec coverage:** measured equation ✓ (T2), mainstat sample table unfloored + cap ✓ (T1), strength→MainStat mapping with agility-keys exclusion ✓ (T1 — exclusion = simply no mapping entry, already absent), readings CSV + sync test ✓ (T3), potency pool incl. calibrated bonus + per-art riders ✓ (T2/T3), ability-mod cap deletion ✓ (T2), mainstat in WeightStats as curve stat with bracket marginals ✓ (T1), config fields + validation + alex.toml ✓ (T3), ⚠ mystery flagged in report ✓ (T4), auto-attack NOT scaled (spec: unverified) ✓ (no AutoDPS change anywhere).
- **Compile boundaries:** T1+T2 single commit (marginal tests + render's cap-constant reference); T3/T4 independent.
- **Type consistency:** `MainStatEffect`, `mainStatSamples`, `StatBlock.MainStat/PotencyBonus`, `CombatArt.PotencyAdd`, `ArtMod.PotencyAdd`, stat keys `"mainstat"`/`"potencybonus"` — consistent across tasks.
- **Verified arithmetic:** full-stack test value 1000×1.823×1.6406+738 = 3728.81; midpoint interp (759−738)/(780−738) = 0.5 → 57.10+0.5×1.64 = 57.92.
