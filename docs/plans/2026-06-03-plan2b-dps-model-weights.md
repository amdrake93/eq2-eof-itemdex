# Plan 2b — DPS Model & Derived Weights Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Build the relative-DPS model and, from it, derive the per-stat weights for the **solo** and **raid** baselines — the validatable core an experienced player can sanity-check at a glance.

**Architecture:** A pure, deterministic `internal/model` (StatBlock + census-modifier→model-stat mapping + the DPS equations) and `internal/baseline` (the two buffed profiles + locked combat constants). Weights come from numerically differentiating total DPS w.r.t. each stat at a baseline. A `weights` command prints the per-baseline weights for inspection.

**Tech Stack:** Go 1.26; reuses `internal/spell` (CombatArt) + Plan-1 conventions (testify, gofmt/golangci-lint). No new deps. Module `github.com/amdrake93/eq2-eof-itemdex`.

This is **Plan 2b of the model work** (a short **Plan 2c** will add item scoring, the `scores` table, per-slot ranking, and the explainable report — all mechanical once the weights exist). Spec: `docs/design-plan2.md` §3–§4, §11. Data layer: Plan 2a's `bis.db` (gear + combat arts).

**Modeling note — the equations are deliberate approximations for *relative ordering*, not absolute DPS** (per spec §1). Haste and DPS-mod use a **linear-to-cap** curve (vs the game's true diminishing-returns curve); this preserves the key behavior — marginal value tapers to ~0 at the cap — which is what the weights need. All constants live in one place (`internal/baseline`) so they're auditable and tunable during the expert-review loop (spec §9).

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/model/stats.go` (new) | `StatBlock` (model's DPS-relevant stat variables) + `MapModifiers` (census modifier key → StatBlock field) |
| `internal/baseline/baseline.go` (new) | combat constants + `Solo`/`Raid` baseline `StatBlock`s |
| `internal/model/dps.go` (new) | `AutoDPS`, `CADPS`, `TotalDPS` |
| `internal/model/weights.go` (new) | `DeriveWeights` (numeric ∂DPS/∂stat at a baseline) |
| `cmd/weights/main.go` (new) | print derived weights for both baselines (inspection) |

---

## Task 1: `StatBlock` + census-modifier mapping

**Files:**
- Create: `internal/model/stats.go`
- Test: `internal/model/stats_test.go`

The model only cares about the DPS-relevant, gear-variable stats. Map the census modifier keys to them; **drop** `critbonus` (ignored on server), resists/mitigation/hp (defensive), and attributes (don't vary meaningfully, held constant — spec).

- [ ] **Step 1: failing test**

```go
package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMapModifiers(t *testing.T) {
	// Census modifier keys → StatBlock. Values are percentages/points as Census reports.
	mods := map[string]float64{
		"attackspeed":       30, // haste
		"doubleattackchance": 12, // multi-attack (legacy key)
		"critchance":        5,
		"basemodifier":      8, // potency
		"dps":               40, // dps-mod
		"spelltimereusepct": 6, // reuse
		"flurry":            3,
		"critbonus":         9,  // IGNORED
		"strength":          40, // attribute, dropped
		"arcane":            500, // resist, dropped
	}
	var sb StatBlock
	sb.AddModifiers(mods)
	require.Equal(t, 30.0, sb.Haste)
	require.Equal(t, 12.0, sb.MultiAttack)
	require.Equal(t, 5.0, sb.CritChance)
	require.Equal(t, 8.0, sb.Potency)
	require.Equal(t, 40.0, sb.DPSMod)
	require.Equal(t, 6.0, sb.Reuse)
	require.Equal(t, 3.0, sb.Flurry)
	require.Equal(t, 0.0, sb.AbilityMod) // not in mods
}
```

- [ ] **Step 2: run, verify FAIL**: `go test ./internal/model/ -run TestMapModifiers -v` → `undefined: StatBlock`.

- [ ] **Step 3: implement `stats.go`**

```go
package model

// StatBlock holds the DPS-relevant, gear-variable combat stats the model uses.
// Values are in the same units Census reports (percent/points).
type StatBlock struct {
	Haste       float64 // attackspeed
	MultiAttack float64 // doubleattackchance (legacy key; = Multi-Attack)
	CritChance  float64 // critchance
	Potency     float64 // basemodifier
	DPSMod      float64 // dps
	Reuse       float64 // spelltimereusepct
	Flurry      float64 // flurry
	AbilityMod  float64 // ability-mod / +combat-art damage (if present as a modifier)
}

// modifierToField maps a Census modifier key to the StatBlock field it feeds.
// Keys NOT listed (critbonus, resists, mitigation, attributes, etc.) are
// intentionally ignored: critbonus is stripped on the server; resists/hp are
// defensive; attributes don't discriminate and are held constant.
var modifierToField = map[string]func(*StatBlock, float64){
	"attackspeed":        func(s *StatBlock, v float64) { s.Haste += v },
	"doubleattackchance": func(s *StatBlock, v float64) { s.MultiAttack += v },
	"critchance":         func(s *StatBlock, v float64) { s.CritChance += v },
	"basemodifier":       func(s *StatBlock, v float64) { s.Potency += v },
	"dps":                func(s *StatBlock, v float64) { s.DPSMod += v },
	"spelltimereusepct":  func(s *StatBlock, v float64) { s.Reuse += v },
	"flurry":             func(s *StatBlock, v float64) { s.Flurry += v },
	"abilitymod":         func(s *StatBlock, v float64) { s.AbilityMod += v },
}

// AddModifiers folds a set of Census modifiers into the StatBlock (additive).
func (s *StatBlock) AddModifiers(mods map[string]float64) {
	for k, v := range mods {
		if apply, ok := modifierToField[k]; ok {
			apply(s, v)
		}
	}
}

// Add returns the sum of two StatBlocks (baseline + an item's stats).
func (s StatBlock) Add(o StatBlock) StatBlock {
	return StatBlock{
		Haste: s.Haste + o.Haste, MultiAttack: s.MultiAttack + o.MultiAttack,
		CritChance: s.CritChance + o.CritChance, Potency: s.Potency + o.Potency,
		DPSMod: s.DPSMod + o.DPSMod, Reuse: s.Reuse + o.Reuse,
		Flurry: s.Flurry + o.Flurry, AbilityMod: s.AbilityMod + o.AbilityMod,
	}
}
```

- [ ] **Step 4: run, verify PASS**: `go test ./internal/model/ -v` → PASS.
- [ ] **Step 5: commit**

```bash
git add internal/model/stats.go internal/model/stats_test.go
git commit -m "feat: model StatBlock + census modifier mapping"
```

> **Note for the implementer:** confirm the exact Census key for ability-mod by scanning the catalog (`grep -ho '"[a-z]*mod[a-z]*"' data/*.csv | sort -u`, or query a known item). The spec calls it "ability mod / +CA damage"; the live key may be `abilitymod` (assumed here) or similar. If it differs, update the `"abilitymod"` map key + the StatBlock comment, and note it. Do NOT invent behavior — just fix the key name.

---

## Task 2: Baseline profiles + combat constants

**Files:**
- Create: `internal/baseline/baseline.go`
- Test: `internal/baseline/baseline_test.go`

- [ ] **Step 1: failing test**

```go
package baseline

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProfiles(t *testing.T) {
	require.Equal(t, 34.2, Solo.MultiAttack)       // Villainy self-buff
	require.Equal(t, 0.0, Solo.DPSMod)             // no group dps-mod solo
	require.Equal(t, 200.0, Raid.DPSMod)           // buff-capped in raid
	require.Equal(t, 1.30, CritMultiplier)
	require.Equal(t, 5.0, FlurryMultiplier)
	require.Equal(t, 100.0, HasteCapPct)
	require.Equal(t, 200.0, DPSModCap)
}
```

- [ ] **Step 2: run, verify FAIL**: `go test ./internal/baseline/ -v` → `undefined: Solo`.

- [ ] **Step 3: implement `baseline.go`**

```go
package baseline

import "github.com/amdrake93/eq2-eof-itemdex/internal/model"

// Locked combat constants (see docs/design-plan2.md §11). Single source of truth.
// All tagged "confirm vs guild leader / Varsoon parse" where uncertain.
const (
	CritMultiplier   = 1.30  // a crit deals +30%
	FlurryMultiplier = 5.0   // flurry applies a 5× burst to the auto-attack
	HasteCapPct      = 100.0 // haste effect cap; overcap converts to flurry
	HasteToFlurry    = 10.0  // 10 points of overcap haste → 1% flurry
	DPSModCap        = 200.0 // dps-mod cap (≈ +125% at cap); overcap wasted
	DPSModEffectAtCap = 1.25 // multiplier added to auto-attack at the cap
	AbilityModCapFrac = 0.50 // +CA-dmg cap = 50% of the potency-adjusted CA base
	ReuseHalvesAt     = 100.0 // 100% reuse → recast halved (the cap)
)

// Solo: the Assassin's self-buffs only (no group buffs). Documented best-guess;
// haste/crit/potency baselines are placeholders pending parse confirmation.
var Solo = model.StatBlock{
	MultiAttack: 34.2, // Villainy IV
	// Haste, CritChance, Potency, DPSMod, Reuse, Flurry, AbilityMod default 0
	// (self-haste buff is temporary → excluded from the sustained baseline).
}

// Raid: self + group package. DPS-mod buff-capped at 200; crit elevated by
// AAs/buffs (placeholder until parse). Haste still low (no maintained haste buff).
var Raid = model.StatBlock{
	MultiAttack: 34.2, // Villainy IV
	DPSMod:      200.0, // buff-capped (guild-leader reported)
	CritChance:  31.0,  // ~31% buffed in an MT group (research; confirm)
	// Potency, Haste, Reuse, Flurry, AbilityMod baseline 0 pending confirmation.
}
```

- [ ] **Step 4: run, verify PASS**: `go test ./internal/baseline/ -v` → PASS.
- [ ] **Step 5: commit**

```bash
git add internal/baseline/baseline.go internal/baseline/baseline_test.go
git commit -m "feat: solo/raid baselines + locked combat constants"
```

---

## Task 3: DPS equations

**Files:**
- Create: `internal/model/dps.go`
- Test: `internal/model/dps_test.go`

`TotalDPS(sb, weapon, cas)` — sb is the character's *total* combat stats (baseline + gear). Constants come from `internal/baseline`.

- [ ] **Step 1: failing test** (hand-computed from round numbers)

```go
package model

import (
	"math"
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	"github.com/stretchr/testify/require"
)

func approx(t *testing.T, want, got float64) {
	t.Helper()
	require.InDelta(t, want, got, 0.01)
}

func TestAutoDPS(t *testing.T) {
	// avgDmg=100, delay=2.0s. No stats → effDelay=2.0, all factors=1 → 50 dps.
	w := Weapon{AvgDamage: 100, DelaySecs: 2.0}
	approx(t, 50.0, AutoDPS(StatBlock{}, w))

	// Haste 100% → effDelay = 2/(1+1)=1.0 → 100 dps.
	approx(t, 100.0, AutoDPS(StatBlock{Haste: 100}, w))

	// MultiAttack 50% → ×1.5 → 75 dps (from the 50 base).
	approx(t, 75.0, AutoDPS(StatBlock{MultiAttack: 50}, w))

	// CritChance 100% → critFactor = 1 + 1.0*0.30 = 1.30 → 65 dps.
	approx(t, 65.0, AutoDPS(StatBlock{CritChance: 100}, w))
}

func TestCADPS(t *testing.T) {
	// One CA: avg 1000 (min 800/max 1200), recast 10s. No stats:
	// hit = 1000, /10s = 100 dps.
	ca := spell.CombatArt{Name: "X", MinDamage: 800, MaxDamage: 1200, RecastSecs: 10}
	approx(t, 100.0, CADPS(StatBlock{}, []spell.CombatArt{ca}))

	// Potency 10% → base ×1.1 = 1100 → 110 dps.
	approx(t, 110.0, CADPS(StatBlock{Potency: 10}, []spell.CombatArt{ca}))

	// Reuse 100% → recast halved to 5s → 200 dps.
	approx(t, 200.0, CADPS(StatBlock{Reuse: 100}, []spell.CombatArt{ca}))
}

var _ = math.Abs
```

- [ ] **Step 2: run, verify FAIL**: `go test ./internal/model/ -run 'TestAutoDPS|TestCADPS' -v` → `undefined: Weapon` / `AutoDPS`.

- [ ] **Step 3: implement `dps.go`**

```go
package model

import (
	"github.com/amdrake93/eq2-eof-itemdex/internal/baseline"
	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
)

// Weapon is the auto-attack-relevant view of an equipped weapon.
type Weapon struct {
	AvgDamage float64 // (min+max)/2 of the weapon's base damage
	DelaySecs float64 // attack delay in seconds
}

func critFactor(sb StatBlock) float64 {
	return 1 + (sb.CritChance/100)*(baseline.CritMultiplier-1)
}

// flurryPct = gear flurry + haste overcap converted at HasteToFlurry (10:1).
func flurryPct(sb StatBlock) float64 {
	over := sb.Haste - baseline.HasteCapPct
	if over < 0 {
		over = 0
	}
	return sb.Flurry + over/baseline.HasteToFlurry
}

func flurryFactor(sb StatBlock) float64 {
	return 1 + (flurryPct(sb)/100)*(baseline.FlurryMultiplier-1)
}

func dpsModFactor(sb StatBlock) float64 {
	d := sb.DPSMod
	if d > baseline.DPSModCap {
		d = baseline.DPSModCap // overcap wasted
	}
	return 1 + (d/baseline.DPSModCap)*baseline.DPSModEffectAtCap
}

func effDelay(sb StatBlock, w Weapon) float64 {
	h := sb.Haste
	if h > baseline.HasteCapPct {
		h = baseline.HasteCapPct // beyond cap → flurry (handled separately)
	}
	return w.DelaySecs / (1 + h/100)
}

// AutoDPS models sustained auto-attack damage per second.
func AutoDPS(sb StatBlock, w Weapon) float64 {
	if w.DelaySecs <= 0 {
		return 0
	}
	swings := w.AvgDamage / effDelay(sb, w)
	return swings * (1 + sb.MultiAttack/100) * critFactor(sb) * flurryFactor(sb) * dpsModFactor(sb)
}

// CADPS sums each combat art's damage/recast, with potency, the ability-mod
// cap (50% of the potency-adjusted base), reuse, and crit applied.
func CADPS(sb StatBlock, cas []spell.CombatArt) float64 {
	cf := critFactor(sb)
	pot := 1 + sb.Potency/100
	reuseFactor := 1 - (baseline.AbilityModCapFrac)*min(sb.Reuse, baseline.ReuseHalvesAt)/100
	var total float64
	for _, ca := range cas {
		if ca.RecastSecs <= 0 {
			continue
		}
		avgBase := (ca.MinDamage + ca.MaxDamage) / 2 * pot
		cap := baseline.AbilityModCapFrac * ca.MaxDamage * pot
		bonus := sb.AbilityMod
		if bonus > cap {
			bonus = cap
		}
		hit := (avgBase + bonus) * cf
		total += hit / (ca.RecastSecs * reuseFactor)
	}
	return total
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// TotalDPS = auto-attack + combat arts.
func TotalDPS(sb StatBlock, w Weapon, cas []spell.CombatArt) float64 {
	return AutoDPS(sb, w) + CADPS(sb, cas)
}
```
> Implementer: verify the hand-calc test values pass exactly; `reuseFactor` uses `ReuseHalvesAt`=100 and `AbilityModCapFrac` reused as the 0.5 reuse-halving coefficient — confirm `1 - 0.5*min(reuse,100)/100` gives 0.5 at reuse=100 (recast halved). If clearer, introduce a dedicated `ReuseHalveCoeff = 0.5` constant rather than reusing `AbilityModCapFrac`. (Go 1.26 has builtin `min`; if the local `min` shadows/conflicts, drop the local and use builtin.)

- [ ] **Step 4: run, verify PASS**: `go test ./internal/model/ -v` → PASS.
- [ ] **Step 5: commit**

```bash
git add internal/model/dps.go internal/model/dps_test.go
git commit -m "feat: relative DPS equations (auto-attack + combat arts)"
```

---

## Task 4: Derive stat weights

**Files:**
- Create: `internal/model/weights.go`
- Test: `internal/model/weights_test.go`

A weight = marginal DPS per +1 unit of a stat, at a baseline, via finite difference: `(TotalDPS(sb+ε·stat) − TotalDPS(sb)) / ε`.

- [ ] **Step 1: failing test**

```go
package model

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	"github.com/stretchr/testify/require"
)

func TestDeriveWeights(t *testing.T) {
	w := Weapon{AvgDamage: 100, DelaySecs: 2.0}
	cas := []spell.CombatArt{{Name: "X", MinDamage: 800, MaxDamage: 1200, RecastSecs: 10}}
	weights := DeriveWeights(StatBlock{}, w, cas)

	// All weights present and finite.
	for _, k := range []string{"haste", "multiattack", "critchance", "potency", "dpsmod", "reuse", "flurry", "abilitymod"} {
		_, ok := weights[k]
		require.True(t, ok, k)
	}
	// Sanity: at an empty baseline, more crit-chance increases DPS → positive weight.
	require.Greater(t, weights["critchance"], 0.0)
	// DPS-mod at cap (raidlike) → ~zero weight; below cap → positive. Here baseline is 0 → positive.
	require.Greater(t, weights["dpsmod"], 0.0)
}

func TestDPSModWeightZeroAtCap(t *testing.T) {
	w := Weapon{AvgDamage: 100, DelaySecs: 2.0}
	weights := DeriveWeights(StatBlock{DPSMod: 200}, w, nil) // at cap
	require.InDelta(t, 0.0, weights["dpsmod"], 1e-6)         // saturated → no marginal value
}
```

- [ ] **Step 2: run, verify FAIL**: `go test ./internal/model/ -run TestDerive -v` → `undefined: DeriveWeights`.

- [ ] **Step 3: implement `weights.go`**

```go
package model

import "github.com/amdrake93/eq2-eof-itemdex/internal/spell"

// epsilon is the finite-difference step (1 stat point).
const epsilon = 1.0

// statField returns a copy of sb with the named stat bumped by delta.
func bump(sb StatBlock, stat string, delta float64) StatBlock {
	switch stat {
	case "haste":
		sb.Haste += delta
	case "multiattack":
		sb.MultiAttack += delta
	case "critchance":
		sb.CritChance += delta
	case "potency":
		sb.Potency += delta
	case "dpsmod":
		sb.DPSMod += delta
	case "reuse":
		sb.Reuse += delta
	case "flurry":
		sb.Flurry += delta
	case "abilitymod":
		sb.AbilityMod += delta
	}
	return sb
}

// DeriveWeights returns the marginal DPS per +1 unit of each stat at the given
// baseline (with the reference weapon + combat arts), via forward difference.
// Saturated stats (e.g. dps-mod at its cap) yield ~0, by construction.
func DeriveWeights(base StatBlock, w Weapon, cas []spell.CombatArt) map[string]float64 {
	stats := []string{"haste", "multiattack", "critchance", "potency", "dpsmod", "reuse", "flurry", "abilitymod"}
	d0 := TotalDPS(base, w, cas)
	out := make(map[string]float64, len(stats))
	for _, s := range stats {
		out[s] = (TotalDPS(bump(base, s, epsilon), w, cas) - d0) / epsilon
	}
	return out
}
```

- [ ] **Step 4: run, verify PASS**: `go test ./internal/model/ -v` → PASS.
- [ ] **Step 5: commit**

```bash
git add internal/model/weights.go internal/model/weights_test.go
git commit -m "feat: derive per-stat DPS weights via finite differences"
```

---

## Task 5: `weights` command — print derived weights (validation milestone)

**Files:**
- Create: `cmd/weights/main.go`

This prints the solo + raid weights so they can be eyeballed against experience (spec §9 primary validation). Uses a reference weapon + the combat arts from `bis.db`.

- [ ] **Step 1: implement `main.go`**

```go
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/amdrake93/eq2-eof-itemdex/internal/baseline"
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", "bis.db", "sqlite db from builddb")
	flag.Parse()

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT name, min_dmg, max_dmg, recast_secs FROM combat_arts`)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var cas []spell.CombatArt
	for rows.Next() {
		var ca spell.CombatArt
		if err := rows.Scan(&ca.Name, &ca.MinDamage, &ca.MaxDamage, &ca.RecastSecs); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		cas = append(cas, ca)
	}
	_ = rows.Close()

	// Reference weapon: a generic 1H (avg 100, delay 4.0) so weights are comparable.
	ref := model.Weapon{AvgDamage: 100, DelaySecs: 4.0}

	for _, b := range []struct {
		name string
		sb   model.StatBlock
	}{{"SOLO", baseline.Solo}, {"RAID", baseline.Raid}} {
		fmt.Printf("\n== %s baseline weights (marginal DPS per +1 stat, ref weapon) ==\n", b.name)
		ws := model.DeriveWeights(b.sb, ref, cas)
		keys := make([]string, 0, len(ws))
		for k := range ws {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return ws[keys[i]] > ws[keys[j]] })
		for _, k := range keys {
			fmt.Printf("  %-12s %.4f\n", k, ws[k])
		}
	}
}
```

- [ ] **Step 2: build + lint**: `make build && make lint` → exit 0.
- [ ] **Step 3: run it** (needs `bis.db` from `go run ./cmd/builddb`):

Run: `go run ./cmd/builddb && go run ./cmd/weights`
Expected: two ranked weight tables (solo, raid). **This is the first validatable output** — eyeball it: e.g. is dps-mod's weight ~0 under raid (capped) but positive under solo? Does crit rank sensibly? Discrepancies → tune `internal/baseline` constants/baselines and re-run (spec §9 loop).

- [ ] **Step 4: commit**

```bash
git add cmd/weights/main.go
git commit -m "feat: weights command prints solo/raid stat weights for review"
```

---

## Self-Review

**Spec coverage (design-plan2.md → tasks):**
- §3 DPS model (auto + CAs; potency on CAs; ability-mod 50%-of-potency-adjusted-base cap; reuse; crit ×1.3; MA from doubleattackchance; flurry ×5; haste→flurry overcap; dps-mod cap) → Tasks 1–3.
- §3 derive-don't-declare weights → Task 4 (numeric ∂DPS/∂stat; saturation falls out, tested in `TestDPSModWeightZeroAtCap`).
- §4 two baselines (solo/raid, neutral inputs) → Task 2.
- §11 constants/translations (critbonus ignored, doubleattackchance=MA) → Task 1 mapping + Task 2 constants.
- §9 primary validation = inspectable output → Task 5 (the weights table is the first thing to sanity-check).
- **Deferred to Plan 2c** (correctly out of scope): item scoring (`Σ weight×stat`) + breakdowns, the `scores` table, per-slot top-3-Fabled/top-3-Legendary ranking, locked-items re-model, the full markdown report.

**Placeholder scan:** none structural. Two flagged confirmations (the live `abilitymod` key name in Task 1; the reuse-coefficient constant naming in Task 3) have explicit instructions, not hand-waves. Baseline numeric values are intentionally marked "confirm vs parse" — that's the spec's parameterized design, and the model self-corrects when they're updated.

**Type consistency:** `model.StatBlock` fields used identically across stats/dps/weights. `model.Weapon` defined in Task 3, used in Tasks 4–5. `spell.CombatArt` (from Plan 2a) consumed in dps/weights/cmd unchanged. `baseline.*` constants referenced consistently in `dps.go`. `DeriveWeights`/`TotalDPS`/`AutoDPS`/`CADPS` signatures stable across tasks.

**Modeling caveats (intentional, documented):** linear-to-cap haste/dps-mod curves approximate the game's diminishing-returns; weights are relative; attributes/attack-rating held constant. All tunable in `internal/baseline` for the expert-review loop.
