# Plan 2c — CA Rotation Model Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Replace the naive "sum every combat art" CA term with a realistic **priority-rotation simulation**, map the `all` stat to Ability Modifier, and re-derive the solo/raid weights for validation.

**Architecture:** Two corrections surfaced by expert review of Plan 2b's weights. (1) `all` (Census displayname "All") IS Ability Modifier — map it. (2) EoF has **no global cooldown**; arts are a *priority* system (cast the highest-damage art that's off cooldown), and all *ranks* of one art line share a cooldown — so collapse to the highest rank, then simulate casting over a fixed fight length. CADPS becomes the simulated total over the fight, not an unbounded sum.

**Tech Stack:** Go 1.26; pure (no network/deps). Reuses `internal/model`, `internal/spell`, `internal/constants`, `internal/baseline`. Module `github.com/amdrake93/eq2-eof-itemdex`.

This is **Plan 2c of the model work**; **Plan 2d** (final) adds item scoring + the per-slot report. Spec: `docs/design-plan2.md`. Conventions from prior plans carry over (testify, gofmt/golangci-lint, no-cycle: `model`↮`baseline`).

**Key facts (from expert review):**
- No GCD; arts ≈ same cast time → priority = "biggest off-cooldown hit."
- `<Name> <Rank>` ranks share one cooldown → use highest rank only.
- Reuse is disproportionately strong: it lets huge long-recast nukes (Assassinate / stealth attacks) fire more often. The sim derives this; we don't assert it.
- Fight length = **600s (10 min)** — covers long fights yet short enough that one extra Assassinate matters.

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/model/stats.go` (modify) | map `all` → `AbilityMod` |
| `internal/constants/constants.go` (modify) | add `FightDurationSecs=600`, `CACastTimeSecs=0.5` |
| `internal/spell/rotation.go` (new) | `HighestRanks` — collapse `<Name> <Rank>` to the top rank per line |
| `internal/model/rotation.go` (new) | `CAEffectiveDamage` + `RotationCADPS` (the priority sim) |
| `internal/model/dps.go` (modify) | `CADPS` delegates to `RotationCADPS`; extract per-CA damage |
| `cmd/weights/main.go` (modify) | collapse CAs via `HighestRanks` before deriving weights |

---

## Task 1: Map `all` → Ability Modifier

**Files:**
- Modify: `internal/model/stats.go`
- Test: `internal/model/stats_test.go`

`all` (displayname "All") is the Ability Modifier — it boosts *all* abilities (combat arts/spells/heals), which is why Census keys it `all`. It's on 783 EoF gear items.

- [ ] **Step 1: failing test** (append to `stats_test.go`)

```go
func TestAllMapsToAbilityMod(t *testing.T) {
	var sb StatBlock
	sb.AddModifiers(map[string]float64{"all": 62})
	require.Equal(t, 62.0, sb.AbilityMod)
}
```

- [ ] **Step 2: run, verify FAIL**: `go test ./internal/model/ -run TestAllMapsToAbilityMod -v` (AbilityMod stays 0 — `all` currently unmapped).

- [ ] **Step 3: edit the map in `stats.go`** — change the `abilitymod` entry's key to `all` (Census's actual key), keeping the field comment accurate:

```go
	"all":         func(s *StatBlock, v float64) { s.AbilityMod += v }, // displayname "All" = Ability Modifier
```
(Remove the old `"abilitymod"` entry — that key doesn't exist in the data. Update the `AbilityMod` field comment to: `// all (displayname "All") = Ability Modifier`.)

- [ ] **Step 4: run, verify PASS**: `go test ./internal/model/ -v` → PASS (existing `TestMapModifiers` still passes — it doesn't include `all`).

- [ ] **Step 5: commit**

```bash
git add internal/model/stats.go internal/model/stats_test.go
git commit -m "fix: map 'all' modifier to Ability Modifier"
```

---

## Task 2: Fight-length + cast-time constants

**Files:**
- Modify: `internal/constants/constants.go`
- Test: `internal/constants/constants_test.go` (create if absent)

- [ ] **Step 1: failing test**

```go
package constants

import "testing"

func TestRotationConstants(t *testing.T) {
	if FightDurationSecs != 600 {
		t.Fatalf("FightDurationSecs = %v, want 600", FightDurationSecs)
	}
	if CACastTimeSecs != 0.5 {
		t.Fatalf("CACastTimeSecs = %v, want 0.5", CACastTimeSecs)
	}
}
```

- [ ] **Step 2: run, verify FAIL**: `go test ./internal/constants/ -v` → undefined.

- [ ] **Step 3: add to `constants.go`**

```go
	// Rotation-sim parameters.
	FightDurationSecs = 600.0 // 10-minute fight (long-fight-aware; short enough that one extra big nuke matters)
	CACastTimeSecs    = 0.5   // combat arts share ~0.5s cast time
```

- [ ] **Step 4: run, verify PASS**: `go test ./internal/constants/ -v` → PASS.
- [ ] **Step 5: commit**

```bash
git add internal/constants/constants.go internal/constants/constants_test.go
git commit -m "feat: rotation-sim constants (fight length, cast time)"
```

---

## Task 3: Collapse combat arts to highest rank

**Files:**
- Create: `internal/spell/rotation.go`
- Test: `internal/spell/rotation_test.go`

`<Name> <Rank>` ranks share a cooldown → keep only the highest rank (≈ highest `MaxDamage`) per base name. Base name = the name with a trailing roman-numeral/digit rank stripped.

- [ ] **Step 1: failing test**

```go
package spell

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHighestRanks(t *testing.T) {
	cas := []CombatArt{
		{Name: "Mortal Blade", MaxDamage: 1000, RecastSecs: 30},
		{Name: "Mortal Blade IV", MaxDamage: 4000, RecastSecs: 30},
		{Name: "Mortal Blade II", MaxDamage: 2000, RecastSecs: 30},
		{Name: "Assassinate", MaxDamage: 8000, RecastSecs: 300},
		{Name: "Assassinate II", MaxDamage: 12000, RecastSecs: 300},
		{Name: "Quick Strike", MaxDamage: 500, RecastSecs: 5}, // no rank suffix = rank I
	}
	got := HighestRanks(cas)
	require.Len(t, got, 3) // Mortal Blade line, Assassinate line, Quick Strike
	byBase := map[string]float64{}
	for _, c := range got {
		byBase[BaseName(c.Name)] = c.MaxDamage
	}
	require.Equal(t, 4000.0, byBase["Mortal Blade"])
	require.Equal(t, 12000.0, byBase["Assassinate"])
	require.Equal(t, 500.0, byBase["Quick Strike"])
}

func TestBaseName(t *testing.T) {
	require.Equal(t, "Mortal Blade", BaseName("Mortal Blade IV"))
	require.Equal(t, "Assassinate", BaseName("Assassinate II"))
	require.Equal(t, "Quick Strike", BaseName("Quick Strike"))
	require.Equal(t, "Gut", BaseName("Gut X"))
}
```

- [ ] **Step 2: run, verify FAIL**: `go test ./internal/spell/ -run 'TestHighestRanks|TestBaseName' -v` → undefined.

- [ ] **Step 3: implement `rotation.go`**

```go
package spell

import (
	"regexp"
	"strings"
)

// rankSuffix matches a trailing space + roman numeral or arabic digits (the rank).
var rankSuffix = regexp.MustCompile(` (?:[IVXLCDM]+|\d+)$`)

// BaseName strips a trailing rank suffix ("Mortal Blade IV" -> "Mortal Blade").
func BaseName(name string) string {
	return strings.TrimSpace(rankSuffix.ReplaceAllString(name, ""))
}

// HighestRanks collapses combat arts to the highest-damage version per base
// name (all ranks of a line share one cooldown, so only the top rank is cast).
func HighestRanks(cas []CombatArt) []CombatArt {
	best := map[string]CombatArt{}
	for _, c := range cas {
		b := BaseName(c.Name)
		if cur, ok := best[b]; !ok || c.MaxDamage > cur.MaxDamage {
			best[b] = c
		}
	}
	out := make([]CombatArt, 0, len(best))
	for _, c := range best {
		out = append(out, c)
	}
	return out
}
```
> Note: the roman-numeral strip is a heuristic; a base name genuinely ending in a roman-looking token is possible but rare for EoF Assassin arts. If a collapse looks wrong during the live run (Task 6), refine `rankSuffix`. The map iteration order is nondeterministic — fine, since the rotation sim picks by damage, not order; but if a stable output is wanted, sort `out` by `MaxDamage` desc.

- [ ] **Step 4: run, verify PASS**: `go test ./internal/spell/ -v` → PASS.
- [ ] **Step 5: commit**

```bash
git add internal/spell/rotation.go internal/spell/rotation_test.go
git commit -m "feat: collapse combat arts to highest rank per line"
```

---

## Task 4: Priority-rotation simulator

**Files:**
- Create: `internal/model/rotation.go`
- Test: `internal/model/rotation_test.go`

Simulate a fight: step by cast time; each slot fire the highest-effective-damage art that's off cooldown; reuse shortens recast; idle-jump when nothing's available (auto-attack covers gaps, modeled separately). CADPS = total CA damage / duration.

- [ ] **Step 1: failing test** (hand-computed)

```go
package model

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	"github.com/stretchr/testify/require"
)

func TestRotationCADPS_Priority(t *testing.T) {
	// A: 100 dmg, 3s recast. B: 50 dmg, 1s recast. castTime 1s, fight 10s.
	// Priority fires A whenever up, B fills. Hand-trace: A×4 (t=0,3,6,9), B×6 → 700 dmg /10s = 70.
	cas := []spell.CombatArt{
		{Name: "A", MinDamage: 100, MaxDamage: 100, RecastSecs: 3},
		{Name: "B", MinDamage: 50, MaxDamage: 50, RecastSecs: 1},
	}
	require.InDelta(t, 70.0, RotationCADPS(StatBlock{}, cas, 10, 1), 0.01)
}

func TestRotationCADPS_LowPriorityNeverFires(t *testing.T) {
	// Big always-up art dominates; the weak one never gets a slot.
	cas := []spell.CombatArt{
		{Name: "Big", MinDamage: 100, MaxDamage: 100, RecastSecs: 1}, // always up (recast == castTime)
		{Name: "Weak", MinDamage: 1, MaxDamage: 1, RecastSecs: 1},
	}
	// 10 slots all fire Big (100) → 1000/10 = 100; Weak contributes nothing.
	require.InDelta(t, 100.0, RotationCADPS(StatBlock{}, cas, 10, 1), 0.01)
}

func TestRotationCADPS_ReuseHelps(t *testing.T) {
	cas := []spell.CombatArt{{Name: "Nuke", MinDamage: 1000, MaxDamage: 1000, RecastSecs: 10}}
	base := RotationCADPS(StatBlock{}, cas, 600, 0.5)
	fast := RotationCADPS(StatBlock{Reuse: 100}, cas, 600, 0.5) // recast halved -> fires ~2x as often
	require.Greater(t, fast, base*1.8)
}
```

- [ ] **Step 2: run, verify FAIL**: `go test ./internal/model/ -run TestRotationCADPS -v` → undefined.

- [ ] **Step 3: implement `rotation.go`**

```go
package model

import (
	"math"

	"github.com/amdrake93/eq2-eof-itemdex/internal/constants"
	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
)

// CAEffectiveDamage is one cast's damage: potency-scaled base + capped ability
// mod, times crit. (Cap = 50% of the potency-adjusted MAX damage.)
func CAEffectiveDamage(sb StatBlock, ca spell.CombatArt) float64 {
	pot := 1 + sb.Potency/100
	avgBase := (ca.MinDamage + ca.MaxDamage) / 2 * pot
	capBonus := constants.AbilityModCapFrac * ca.MaxDamage * pot
	bonus := math.Min(sb.AbilityMod, capBonus)
	return (avgBase + bonus) * critFactor(sb)
}

func effRecast(sb StatBlock, ca spell.CombatArt) float64 {
	return ca.RecastSecs * (1 - constants.ReuseHalveCoeff*math.Min(sb.Reuse, constants.ReuseHalvesAt)/100)
}

// RotationCADPS simulates a priority rotation over durationSecs: at each cast
// slot (castTimeSecs apart) it fires the highest-effective-damage art that is
// off cooldown; cooldowns are reuse-reduced; when nothing is available it jumps
// to the next availability (auto-attack covers idle, modeled separately).
func RotationCADPS(sb StatBlock, cas []spell.CombatArt, durationSecs, castTimeSecs float64) float64 {
	if durationSecs <= 0 || castTimeSecs <= 0 || len(cas) == 0 {
		return 0
	}
	eff := make([]float64, len(cas))
	rec := make([]float64, len(cas))
	avail := make([]float64, len(cas)) // next time each art is castable
	for i, ca := range cas {
		eff[i] = CAEffectiveDamage(sb, ca)
		rec[i] = effRecast(sb, ca)
	}
	var total, t float64
	for t < durationSecs {
		best, bestDmg := -1, -1.0
		for i := range cas {
			if avail[i] <= t && eff[i] > bestDmg {
				best, bestDmg = i, eff[i]
			}
		}
		if best < 0 { // nothing off cooldown — jump to soonest availability
			soonest := math.Inf(1)
			for i := range cas {
				if avail[i] < soonest {
					soonest = avail[i]
				}
			}
			if math.IsInf(soonest, 1) || soonest <= t {
				break
			}
			t = soonest
			continue
		}
		total += bestDmg
		avail[best] = t + rec[best]
		t += castTimeSecs
	}
	return total / durationSecs
}
```

- [ ] **Step 4: run, verify PASS**: `go test ./internal/model/ -run TestRotationCADPS -v` → PASS (all three).
- [ ] **Step 5: commit**

```bash
git add internal/model/rotation.go internal/model/rotation_test.go
git commit -m "feat: priority-rotation CA simulator"
```

---

## Task 5: Wire the rotation sim into the model

**Files:**
- Modify: `internal/model/dps.go`
- Test: `internal/model/dps_test.go` (existing `TestCADPS` stays — single-CA sim == sum)

Replace the old summing `CADPS` body with a wrapper over `RotationCADPS` using the constants. For a single art, the sim fires it every recast — identical to the old `damage/recast`, so `TestCADPS` still passes.

- [ ] **Step 1: verify the existing single-CA test still expresses the contract**

`TestCADPS` (Task 2b-T3) uses one art (recast 10) and expects 100 / 110 (potency) / 200 (reuse). Under the sim over 600s at 0.5s cast time, a single art fires every 10s (or 5s with reuse) → identical values. Keep the test as-is.

- [ ] **Step 2: replace `CADPS` in `dps.go`**

Delete the old `CADPS` body (the `for ca := range cas { ... avgBase ... }` loop) and the now-duplicated per-CA math (it moved to `CAEffectiveDamage` in `rotation.go`). Replace with:

```go
// CADPS is the simulated combat-art DPS over a standard fight (priority rotation).
func CADPS(sb StatBlock, cas []spell.CombatArt) float64 {
	return RotationCADPS(sb, cas, constants.FightDurationSecs, constants.CACastTimeSecs)
}
```
Add the `constants` import to `dps.go` if not present; remove the now-unused `min` references if `dps.go` no longer needs them (the CA math now lives in `rotation.go`). `TotalDPS` is unchanged (still `AutoDPS + CADPS`).

- [ ] **Step 3: run, verify PASS**: `go test ./internal/model/ -v` → ALL pass (TestCADPS single-CA values unchanged; rotation + auto + weights tests green).
- [ ] **Step 4: `make lint`; commit**

```bash
git add internal/model/dps.go
git commit -m "refactor: CADPS uses the rotation simulator"
```

---

## Task 6: Collapse CAs in the weights command + re-derive

**Files:**
- Modify: `cmd/weights/main.go`

The weights command loads all CA ranks from `bis.db`; collapse them to the rotation set before deriving weights.

- [ ] **Step 1: collapse on load**

In `cmd/weights/main.go`, after loading `cas` from the DB, add:
```go
	cas = spell.HighestRanks(cas)
```
(immediately before the weight-derivation loop; `spell` is already imported).

- [ ] **Step 2: build + lint**: `make build && make lint` → exit 0.

- [ ] **Step 3: re-run and inspect** (bis.db exists; if not, `go run ./cmd/builddb`):

Run: `go run ./cmd/weights`
Expected: both baselines' weights, now over the **collapsed** rotation set (~15–25 arts) with the **600s priority sim**. **Paste the output.** Sanity-read vs Plan 2b's broken weights:
- CA-vs-auto balance should ease (auto stats — haste/MA/flurry — rise relative to before).
- Reuse should stay strong (now *earned* by the sim: more big-nuke casts).
- `abilitymod` now has a real, non-trivial weight (it's `all`, on gear).
- dps-mod still ~0 under raid (capped).

This is the re-validation milestone — compare against experience and flag anything still off.

- [ ] **Step 4: commit**

```bash
git add cmd/weights/main.go
git commit -m "feat: collapse CAs to rotation set before deriving weights"
```

---

## Self-Review

**Spec/correction coverage:**
- `all` → Ability Modifier (the missed gear stat) → Task 1.
- No-GCD priority rotation + rank-collapse + 600s fight + reuse value → Tasks 2–5.
- Re-derived, re-validatable weights → Task 6.
- **Deferred to Plan 2d** (final): item scoring (`Σ weight×stat` + breakdowns), `scores` table, per-slot top-3-Fabled/top-3-Legendary report, locked-items re-model.

**Placeholder scan:** none — the rank-strip heuristic and map-order note have explicit guidance, not hand-waves. Hand-calc test values (70; 100; reuse>1.8×) are computed and exact.

**Type consistency:** `spell.CombatArt` fields (`Name`,`MinDamage`,`MaxDamage`,`RecastSecs`) used identically in `HighestRanks`, `CAEffectiveDamage`, `RotationCADPS`. `CADPS(sb, cas)` signature preserved (wrapper) so `TotalDPS` and `DeriveWeights` are untouched and existing tests pass. `constants.FightDurationSecs`/`CACastTimeSecs` and `baseline.AbilityModCapFrac`/`ReuseHalveCoeff`/`ReuseHalvesAt` referenced consistently. No new import cycle (`rotation.go` imports `baseline` like `dps.go` already does — wait: `dps.go` imports `constants` not `baseline`; `rotation.go` imports `baseline` for `AbilityModCapFrac`/`ReuseHalveCoeff`/`ReuseHalvesAt`, which are re-exported from `constants` — to stay cycle-safe, import `constants` directly in `rotation.go` instead of `baseline`).

> **Build note (cycle safety):** `internal/baseline` imports `internal/model`, so `internal/model` files must NOT import `internal/baseline`. `dps.go` already uses `internal/constants` for its constants. **`rotation.go` must likewise import `internal/constants`** (use `constants.AbilityModCapFrac`, `constants.ReuseHalveCoeff`, `constants.ReuseHalvesAt`) — NOT `internal/baseline`. Confirm those three constants live in `internal/constants` (they were extracted there in Plan 2b-T3); if any is missing, add it there and re-export from `baseline`.
