# Plan 2u: Hybrid Crit Model Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the flat ×1.30 crit model with the measured range-shift floor (hybrid): a crit deals `max(rangeMax+1, (1.5+critBonus)·roll)`, computed per damage source from its range.

**Architecture:** The model computes the *expected* crit multiplier per source from its final damage range `[lo,hi]` via a closed form (uniform rolls). Combat arts apply it per component (each on its ability-mod-inclusive scaled range); auto-attack applies it on the weapon's damage range (which must be threaded into the `Weapon` struct). Crit chance clamps at 100%; a new `CritBonus` field (0 today) is reserved for raid buffs.

**Tech Stack:** Go 1.26; existing `internal/model`, `internal/constants`, `internal/store`, `internal/bis`.

**Design reference:** `docs/plans/2026-06-19-crit-model-design.md` (approved). Calibration: `data/autoattacktest.txt` (106 non-crit + 14 crit → measured critMult 1.85 vs formula 1.87).

**Key facts for the implementer:**
- `critFactor` currently has signature `critFactor(sb StatBlock) float64` returning `1 + critChance·(CritMultiplier−1)`. It is called in `dps.go AutoDPS` and twice in `rotation.go CAEffectiveDamage`. Its signature changes to `critFactor(sb StatBlock, lo, hi float64) float64`; **all three call sites change in the same task** (Task 2) so the package keeps compiling.
- `critFactor` is **scale-invariant** in `[lo,hi]` (ignoring the negligible +1), so auto-attack can pass the **base weapon** `min/max` directly. Ability-mod is a *flat* add that breaks scale-invariance, so combat-art DirectHit/TriggerProc components must pass their **scaled + abmod-inclusive** range.
- Only one existing test asserts a crit value: `dps_test.go` `TestAutoDPS` line ~21. All other model tests use `CritChance: 0` (→ factor 1.0, unchanged) or don't touch crit.

---

## Task 1: Thread weapon min/max through the model

Pure plumbing — adds `MinDamage`/`MaxDamage` to `Weapon` and populates them everywhere a `Weapon` is built. No behavior change (swing damage still uses `AvgDamage`), so the full suite stays green.

**Files:**
- Modify: `internal/model/dps.go` (Weapon struct)
- Modify: `internal/store/store.go` (`loadWeapon`, `ScorableItem`, `LoadScorableItems`)
- Modify: `internal/bis/set.go` (`offWeapon`, `restOff`)

- [ ] **Step 1: Add fields to the Weapon struct**

In `internal/model/dps.go`, change:
```go
type Weapon struct {
	AvgDamage float64 // (min+max)/2 of the weapon's base damage
	DelaySecs float64 // attack delay in seconds
}
```
to:
```go
type Weapon struct {
	AvgDamage float64 // (min+max)/2 of the weapon's base damage
	MinDamage float64 // weapon base damage range low (for the range-shift crit, §11)
	MaxDamage float64 // weapon base damage range high
	DelaySecs float64 // attack delay in seconds
}
```

- [ ] **Step 2: Populate min/max in store.loadWeapon**

In `internal/store/store.go`, change the return in `loadWeapon` (around line 228):
```go
	return model.Weapon{AvgDamage: (mn + mx) / 2, DelaySecs: delay}, name, nil
```
to:
```go
	return model.Weapon{AvgDamage: (mn + mx) / 2, MinDamage: mn, MaxDamage: mx, DelaySecs: delay}, name, nil
```

- [ ] **Step 3: Add WeaponMin/WeaponMax to ScorableItem and populate them**

In `internal/store/store.go`, in the `ScorableItem` struct add two fields after `WeaponAvg`:
```go
	WeaponAvg   float64
	WeaponMin   float64
	WeaponMax   float64
	WeaponDelay float64
```
Then in `LoadScorableItems`, change:
```go
		if delay > 0 {
			it.WeaponAvg = (mn + mx) / 2
			it.WeaponDelay = delay
		}
```
to:
```go
		if delay > 0 {
			it.WeaponAvg = (mn + mx) / 2
			it.WeaponMin = mn
			it.WeaponMax = mx
			it.WeaponDelay = delay
		}
```

- [ ] **Step 4: Pass min/max where bis builds an off-hand Weapon**

In `internal/bis/set.go`, the two places that build a `model.Weapon` from a `ScorableItem` (`offWeapon` around line 50, and the candidate weapon around line 75) read like:
```go
		return model.Weapon{AvgDamage: it.WeaponAvg, DelaySecs: it.WeaponDelay}
```
and
```go
		w := model.Weapon{AvgDamage: c.WeaponAvg, DelaySecs: c.WeaponDelay}
```
Change each to also pass min/max:
```go
		return model.Weapon{AvgDamage: it.WeaponAvg, MinDamage: it.WeaponMin, MaxDamage: it.WeaponMax, DelaySecs: it.WeaponDelay}
```
and
```go
		w := model.Weapon{AvgDamage: c.WeaponAvg, MinDamage: c.WeaponMin, MaxDamage: c.WeaponMax, DelaySecs: c.WeaponDelay}
```
(Use `grep -n 'model.Weapon{' internal/bis/set.go` to find the exact lines; update every construction that has a `ScorableItem` in scope. Leave the two `model.Weapon{}` empty-zero returns as-is.)

- [ ] **Step 5: Build and run the full suite (no behavior change expected)**

Run: `go build ./... && go test ./... 2>&1 | tail -8`
Expected: builds; all packages PASS (min/max are carried but not yet consumed).

- [ ] **Step 6: Commit**

```bash
git add internal/model/dps.go internal/store/store.go internal/bis/set.go
git commit -m "Model: thread weapon min/max into Weapon for range-shift crit (plan 2u)"
```

---

## Task 2: Hybrid crit model (critFactor + constant + CritBonus + callers)

**Files:**
- Modify: `internal/constants/constants.go`
- Modify: `internal/model/stats.go` (CritBonus field)
- Modify: `internal/model/dps.go` (`critFactor`, `AutoDPS`)
- Modify: `internal/model/rotation.go` (`CAEffectiveDamage`)
- Modify/Test: `internal/model/crit_test.go` (new), `internal/model/dps_test.go` (update one value)

- [ ] **Step 1: Write the failing unit test for the hybrid critFactor**

Create `internal/model/crit_test.go`:
```go
package model

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	"github.com/stretchr/testify/require"
)

func TestCritFactorHybrid(t *testing.T) {
	// 0% crit → no effect.
	require.InDelta(t, 1.0, critFactor(StatBlock{CritChance: 0}, 139, 775), 1e-9)

	// Single-valued / zero-width range → the 1.5× branch always wins → ×1.50 at 100%.
	require.InDelta(t, 1.50, critFactor(StatBlock{CritChance: 100}, 100, 100), 1e-9)

	// Narrow range (ratio 1.2:1, < 1.5) → 1.5× branch always wins → ×1.50.
	require.InDelta(t, 1.50, critFactor(StatBlock{CritChance: 100}, 316, 386), 1e-3)

	// Typical CA range 1.667:1 → floor grazes the bottom → ~×1.511.
	require.InDelta(t, 1.511, critFactor(StatBlock{CritChance: 100}, 215, 359), 1e-3)

	// Wide weapon range 5.58:1 (Modinthalis 139–775) → floor-boosted → ~×1.868
	// (measured 1.85 in data/autoattacktest.txt).
	require.InDelta(t, 1.868, critFactor(StatBlock{CritChance: 100}, 139, 775), 1e-3)

	// Crit chance scales linearly: 50% → halfway to the 100% multiplier.
	full := critFactor(StatBlock{CritChance: 100}, 139, 775)
	half := critFactor(StatBlock{CritChance: 50}, 139, 775)
	require.InDelta(t, 1+0.5*(full-1), half, 1e-9)

	// Crit chance clamps at 100%.
	require.InDelta(t, full, critFactor(StatBlock{CritChance: 150}, 139, 775), 1e-9)

	// Crit bonus adds to the base factor: +14 → c = 1.64 on a single-valued hit.
	require.InDelta(t, 1.64, critFactor(StatBlock{CritChance: 100, CritBonus: 14}, 50, 50), 1e-9)
}

func TestCritAbilityModRides(t *testing.T) {
	// A single-valued DirectHit with ability-mod, at 100% crit: the crit multiplies
	// the WHOLE hit including abmod (measured SoC+abmod 121→182 = ×1.5).
	// base 100, no potency/AGI (scaling=1 via PotencyBonus 0), abmod 68, 100% crit.
	ca := spell.CombatArt{
		Name:      "T",
		Components: []spell.Component{{Kind: spell.DirectHit, MinDamage: 100, MaxDamage: 100}},
	}
	got := CAEffectiveDamage(StatBlock{CritChance: 100, AbilityMod: 68}, ca)
	require.InDelta(t, (100+68)*1.5, got, 1e-6) // 252
}
```

- [ ] **Step 2: Run it to confirm it fails to compile (signature/field not present yet)**

Run: `go test ./internal/model/ -run TestCritFactorHybrid 2>&1 | tail -5`
Expected: compile error (`critFactor` takes 1 arg; `CritBonus` undefined).

- [ ] **Step 3: Bump the constant**

In `internal/constants/constants.go`, change:
```go
	CritMultiplier   = 1.30  // a crit deals +30%
```
to:
```go
	CritMultiplier   = 1.50  // base crit factor: a crit is max(rangeMax+1, 1.50·roll) (measured 2026-06-19, spec §11)
```

- [ ] **Step 4: Add the CritBonus field to StatBlock**

In `internal/model/stats.go`, add to the `StatBlock` struct (after `PotencyBonus`):
```go
	PotencyBonus  float64 // calibrated hidden potency-pool points (config-only; ⚠ spec §12 open mystery)
	CritBonus     float64 // buff/gear bonus added to the 1.50 base crit factor (percent points; 0 today, raid-context future — §16)
```
And add it to the `Add` method's returned struct:
```go
		PotencyBonus:  s.PotencyBonus + o.PotencyBonus,
		CritBonus:     s.CritBonus + o.CritBonus,
	}
```

- [ ] **Step 5: Replace critFactor with the hybrid closed form**

In `internal/model/dps.go`, ensure `"math"` is in the imports (add it if absent), then replace:
```go
func critFactor(sb StatBlock) float64 {
	return 1 + (sb.CritChance/100)*(constants.CritMultiplier-1)
}
```
with:
```go
// critFactor is the expected crit damage multiplier for a hit whose final damage
// range is [lo, hi] (after potency, AGI, and any ability-mod). A crit re-rolls as
// max(hi, c·roll) — the higher of the range ceiling (a floor a crit can't fall
// below) and c× the roll — where c = 1.50 + crit bonus (measured 2026-06-19, §11).
// Narrow ranges → the c× branch always wins (flat ×c); wide ranges (weapons) →
// the ceiling lifts low rolls, pushing the average above c. Uniform rolls assumed
// (data-validated, data/autoattacktest.txt). Crit chance clamps at 100%.
func critFactor(sb StatBlock, lo, hi float64) float64 {
	p := math.Min(sb.CritChance, 100) / 100
	if p <= 0 {
		return 1
	}
	c := constants.CritMultiplier + sb.CritBonus/100
	m := c // single-valued / narrow: the c× branch wins everywhere
	if hi > lo {
		t := hi / c // roll where c·roll overtakes the floor
		if lo < t {
			avg := (lo + hi) / 2
			m = (hi*(t-lo) + c*(hi*hi-t*t)/2) / ((hi - lo) * avg)
		}
	}
	return 1 + p*(m-1)
}
```

- [ ] **Step 6: Update the AutoDPS call site**

In `internal/model/dps.go` `AutoDPS`, change:
```go
	return swings * (1 + MultiAttackEffect(sb.MultiAttack)/100) * autoDamageMult(sb) * critFactor(sb) * flurryFactor(sb)
```
to (auto crit from the weapon's base range — scale-invariant):
```go
	return swings * (1 + MultiAttackEffect(sb.MultiAttack)/100) * autoDamageMult(sb) * critFactor(sb, w.MinDamage, w.MaxDamage) * flurryFactor(sb)
```

- [ ] **Step 7: Rewrite CAEffectiveDamage for per-component crit**

In `internal/model/rotation.go`, replace the whole `CAEffectiveDamage` body:
```go
func CAEffectiveDamage(sb StatBlock, ca spell.CombatArt) float64 {
	potPool := 1 + (sb.Potency+sb.PotencyBonus+ca.PotencyAdd)/100
	mainStat := 1 + MainStatEffect(sb.MainStat)/100
	scaling := potPool * mainStat

	if len(ca.Components) == 0 {
		avgBase := (ca.MinDamage + ca.MaxDamage) / 2 * scaling
		return (avgBase + sb.AbilityMod) * critFactor(sb)
	}

	hold := hasTermination(ca)
	window := ca.DurationSecs
	if !hold {
		window = math.Min(effRecast(sb, ca), ca.DurationSecs)
	}

	var total float64
	for _, c := range ca.Components {
		base := (c.MinDamage + c.MaxDamage) / 2 * scaling
		switch c.Kind {
		case spell.DirectHit:
			total += base + sb.AbilityMod
		case spell.DoT:
			total += base * dotTicks(c, window)
		case spell.Termination:
			if hold { // detonate fires only when the DoT runs to termination
				total += base
			}
		case spell.TriggerProc:
			total += (base + 0.5*sb.AbilityMod) * float64(c.Triggers)
		case spell.RateProc:
			// deferred — proc-rate scoring not modeled (spec §3.1 deferred)
		}
	}
	return total * critFactor(sb)
}
```
with (each component crits on its own ability-mod-inclusive scaled range):
```go
func CAEffectiveDamage(sb StatBlock, ca spell.CombatArt) float64 {
	potPool := 1 + (sb.Potency+sb.PotencyBonus+ca.PotencyAdd)/100
	mainStat := 1 + MainStatEffect(sb.MainStat)/100
	scaling := potPool * mainStat

	if len(ca.Components) == 0 {
		lo := ca.MinDamage*scaling + sb.AbilityMod
		hi := ca.MaxDamage*scaling + sb.AbilityMod
		return (lo + hi) / 2 * critFactor(sb, lo, hi)
	}

	hold := hasTermination(ca)
	window := ca.DurationSecs
	if !hold {
		window = math.Min(effRecast(sb, ca), ca.DurationSecs)
	}

	var total float64
	for _, c := range ca.Components {
		loBase := c.MinDamage * scaling
		hiBase := c.MaxDamage * scaling
		switch c.Kind {
		case spell.DirectHit: // ability mod in full, rides the crit
			lo, hi := loBase+sb.AbilityMod, hiBase+sb.AbilityMod
			total += (lo + hi) / 2 * critFactor(sb, lo, hi)
		case spell.DoT: // no ability mod; crit per tick
			total += (loBase + hiBase) / 2 * dotTicks(c, window) * critFactor(sb, loBase, hiBase)
		case spell.Termination: // detonate fires only when the DoT runs to termination
			if hold {
				total += (loBase + hiBase) / 2 * critFactor(sb, loBase, hiBase)
			}
		case spell.TriggerProc: // half ability mod per trigger, rides the crit
			lo, hi := loBase+0.5*sb.AbilityMod, hiBase+0.5*sb.AbilityMod
			total += (lo + hi) / 2 * float64(c.Triggers) * critFactor(sb, lo, hi)
		case spell.RateProc:
			// deferred — proc-rate scoring not modeled (spec §13 deferred)
		}
	}
	return total
}
```

- [ ] **Step 8: Update the one existing crit assertion in dps_test.go**

In `internal/model/dps_test.go` `TestAutoDPS`, give the weapon a real range and update the crit line. Change:
```go
	w := Weapon{AvgDamage: 100, DelaySecs: 2.0}
	approx(t, 50.0, AutoDPS(StatBlock{}, w))                     // 100/2, all factors 1
	approx(t, 50.0*1.67, AutoDPS(StatBlock{Haste: 100}, w))      // haste 100 → effect 67 → /1.67 delay
	approx(t, 50.0*1.52, AutoDPS(StatBlock{MultiAttack: 50}, w)) // MA 50 → effect 52 → ×1.52
	approx(t, 65.0, AutoDPS(StatBlock{CritChance: 100}, w))      // ×1.30
```
to:
```go
	w := Weapon{AvgDamage: 100, MinDamage: 50, MaxDamage: 150, DelaySecs: 2.0} // 3:1 range
	approx(t, 50.0, AutoDPS(StatBlock{}, w))                     // 100/2, all factors 1
	approx(t, 50.0*1.67, AutoDPS(StatBlock{Haste: 100}, w))      // haste 100 → effect 67 → /1.67 delay
	approx(t, 50.0*1.52, AutoDPS(StatBlock{MultiAttack: 50}, w)) // MA 50 → effect 52 → ×1.52
	approx(t, 84.375, AutoDPS(StatBlock{CritChance: 100}, w))    // 3:1 hybrid crit → ×1.6875
```

- [ ] **Step 9: Run the model tests to green**

Run: `go test ./internal/model/ 2>&1 | tail -15`
Expected: PASS (TestCritFactorHybrid, TestCritAbilityModRides, TestAutoDPS, and all others).

- [ ] **Step 10: Run the full suite**

Run: `go test ./... 2>&1 | tail -8`
Expected: all packages PASS.

- [ ] **Step 11: Commit**

```bash
git add internal/constants/constants.go internal/model/stats.go internal/model/dps.go internal/model/rotation.go internal/model/crit_test.go internal/model/dps_test.go
git commit -m "Model: hybrid range-shift crit (max(rangeMax+1, 1.5·roll)) per source (plan 2u)"
```

---

## Task 3: Calibration test pinning the mechanic to the 2026-06-19 reads

**Files:**
- Modify: `internal/model/crit_test.go` (add a calibration test)

- [ ] **Step 1: Add the calibration test**

Append to `internal/model/crit_test.go`:
```go
// TestCritCalibration2026_06_19 pins the crit model to the live tooltip/log reads.
// Range-shift floor confirmed via the auto-attack pile-up at max+1 (data/autoattacktest.txt:
// 11/14 crits exactly 776 on a 139–775 weapon; empirical avg crit/non-crit = 1.85).
func TestCritCalibration2026_06_19(t *testing.T) {
	full := func(lo, hi float64) float64 { return critFactor(StatBlock{CritChance: 100}, lo, hi) }

	// Single-valued (Strike of Consistency, Quick Strike DoT): flat ×1.50.
	require.InDelta(t, 1.50, full(61, 61), 1e-9)

	// Narrow range below 1.5:1 (Hilt Strike 316–386): still flat ×1.50 — its crits
	// exceeded any pure range-shift ceiling, ruling out range-shift-only.
	require.InDelta(t, 1.50, full(316, 386), 1e-3)

	// Typical CA 1.667:1 (Quick Strike 285–475): floor grazes bottom → ~×1.511.
	require.InDelta(t, 1.511, full(285, 475), 1e-3)

	// Wide weapon (Modinthalis 139–775): floor dominates → ~×1.868 (measured 1.85,
	// within ~1% — the modeled value is conservative vs the slightly low-skewed log).
	require.InDelta(t, 1.85, full(139, 775), 0.03)
}
```

- [ ] **Step 2: Run it**

Run: `go test ./internal/model/ -run TestCritCalibration2026_06_19 -v 2>&1 | tail -5`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/model/crit_test.go
git commit -m "Test: pin crit model to 2026-06-19 calibration reads (plan 2u)"
```

---

## Task 4: Amend the spec (SPEC.md)

**Files:**
- Modify: `docs/SPEC.md` (§11 Crit, §15 Constants, §16, §12 note)

- [ ] **Step 1: Rewrite §11 Crit**

Open `docs/SPEC.md`, find the `### Crit` subsection under §11 (the block with `critFactor = 1 + (crit/100) · (CritMultiplier − 1)  CritMultiplier = 1.30` and its "Known divergence" quote). Replace the entire subsection with:
```markdown
### Crit

A critical hit re-rolls as the **higher of** the range ceiling and a flat multiple of the roll (measured 2026-06-19, naked/controlled tooltip + combat-log reads):

```
crit = max( rangeMax + 1 , (1.50 + critBonus) · roll )
```

applied to the **final** per-hit damage — after potency × AGI **and** ability-mod (ability-mod rides the crit). `critBonus` is a buff/gear term (0 at baseline; raid-context, §16). The model computes the **expected** crit multiplier per source from its final range `[lo,hi]` (`critFactor`, `dps.go`), assuming uniform rolls:

- range ratio ≤ 1.5:1 (single-valued, narrow CAs) → the `1.50×` branch always wins → **×1.50**
- 1.667:1 (the typical CA range) → the floor grazes the low rolls → **~×1.51**
- wide weapon ranges (auto-attack) → the floor lifts most low rolls → **>×1.50** (Modinthalis 139–775 → ~×1.87)

Combat arts apply it per component on each component's ability-mod-inclusive scaled range (§13); auto-attack applies it on the weapon's damage range (`Weapon.MinDamage/MaxDamage`). Crit chance is clamped at 100% (a crit can't happen more than every hit).

**Provenance:** flat ×1.30 disproven; range-shift-only disproven (Hilt Strike crits exceeded its ceiling); the `max(...)` hybrid confirmed by the auto-attack floor pile-up (11/14 crits exactly `max+1` on a 5.58:1 weapon) and validated at critMult 1.85 measured vs 1.87 modeled (`data/autoattacktest.txt`). Uniform rolls is the one documented assumption (distribution consistent with uniform at the measured sample size).
```

- [ ] **Step 2: Update §15 Constants**

In the §15 constants table/list, change the `CritMultiplier` row from `1.30` (flat +30%) to:
```markdown
`CritMultiplier = 1.50` — base crit factor (the `1.50×` branch of the range-shift floor model; measured 2026-06-19, §11). Not a flat expected multiplier.
```

- [ ] **Step 3: Update §16 (remove the resolved divergence, add refinements)**

In `docs/SPEC.md` §16, under "Known code divergences" remove the **Crit model** item (resolved by plan 2u). Add to the appropriate subsections:
- Under a data/refinement heading: *"Auto roll-distribution / auto average damage — `data/autoattacktest.txt` (106 non-crit) is consistent with uniform rolls (χ²≈0.11) but hints a mild low-lean (mean ~6% under the midpoint). More swings would confirm uniform and check whether the model's `(min+max)/2` average auto damage runs slightly high. Auto-damage, not crit."*
- *"`critBonus` raid stat — raid buffs raise the crit factor above 1.50 (auto crit read ~1.64 in the buffed raid log). A `StatBlock.CritBonus` field exists (0 today); wire it into `[contexts.raid]` when measured."*
- Under the class-agnostic/future or adornment note: *"Crit-adornment question — crit chance's derived weight rises under the hybrid (esp. via wide-range auto), so a crit adornment vs other stats becomes a real choice once the adornment layer (backlog) exists."*

- [ ] **Step 4: Add the §12 weapon-range note**

In `docs/SPEC.md` §12 (Auto-Attack Model), add one sentence noting that the weapon's `min/max` now feed the model (`Weapon.MinDamage/MaxDamage`, loaded from the existing `weapon_min_dmg/weapon_max_dmg` columns) to drive the range-shift crit, where the wide weapon range makes auto crit exceed ×1.50.

- [ ] **Step 5: Verify spec edits + no code touched**

Run: `grep -n 'CritMultiplier\|range-shift\|max( rangeMax' docs/SPEC.md | head` and `grep -c '1.30' docs/SPEC.md`
Expected: §11/§15 show the new 1.50 hybrid; no stray `1.30` crit references remain in §11/§15. Then `go test ./... 2>&1 | tail -3` — still PASS (no code changed this task).

- [ ] **Step 6: Commit**

```bash
git add docs/SPEC.md
git commit -m "Spec: §11/§15/§16/§12 — hybrid range-shift crit model (plan 2u)"
```

---

## Task 5: Final verification + weight regeneration

**Files:** none (verification only)

- [ ] **Step 1: Full suite + vet**

Run: `go vet ./... && go test ./... 2>&1 | tail -8`
Expected: vet clean; all packages PASS.

- [ ] **Step 2: Confirm the crit weight rose**

Run: `go run ./cmd/weights --db bis.db --character characters/alex.toml 2>&1 | tail -40`
Expected: runs cleanly; `critchance` weight is now **higher** than the pre-change report (was ~13.45 pre-raid). Record the new value in the commit message. (If the command needs other flags, inspect `cmd/weights/main.go`.)

- [ ] **Step 3: Regenerate the BiS report (sanity — picks should be ~stable)**

Run: `go run ./cmd/bis --db bis.db --character characters/alex.toml --out bis-report.md 2>&1 | tail -5`
Expected: runs cleanly; per-slot picks largely unchanged (gear crit is near-constant), with crit-chance contributions larger in the breakdowns. (`bis-report.md` is untracked — leave it untracked.)

- [ ] **Step 4: Confirm only intended files changed**

Run: `git status --porcelain`
Expected: clean working tree except untracked `bis-report.md` and `data/autoattacktest.txt` is committed (from the earlier merge). No stray modifications.

- [ ] **Step 5: Commit (if any verification notes worth recording)**

No code commit needed if Steps 1–4 are clean. If `cmd/weights` exposed a needed flag fix or similar, commit it separately with a clear message.

---

## Self-Review

**Spec coverage** (design doc → tasks):
- Weapon min/max threading → Task 1. ✓
- `critFactor` hybrid closed form + constant 1.50 + CritBonus + 100% clamp → Task 2 (Steps 3–5). ✓
- Auto crit from weapon range → Task 2 Step 6. ✓
- Per-component CA crit on abmod-inclusive ranges → Task 2 Step 7. ✓
- abmod rides the crit → Task 2 Step 1 (TestCritAbilityModRides). ✓
- Calibration to reads → Task 3. ✓
- SPEC §11/§15/§16/§12 → Task 4. ✓
- "No code change" guarantee in spec task → Task 4 Step 5; final verify Task 5. ✓

**Placeholder scan:** none — every step has exact code/commands. The §11/§16 spec prose is given verbatim; §16 edits reference exact items to remove/add.

**Type/name consistency:** `critFactor(sb, lo, hi)` signature used identically in dps.go (Step 5), AutoDPS (Step 6), CAEffectiveDamage (Step 7), and all tests. `Weapon.MinDamage/MaxDamage`, `ScorableItem.WeaponMin/WeaponMax`, `StatBlock.CritBonus` named consistently across tasks. `constants.CritMultiplier` reused (value changed, name kept).
