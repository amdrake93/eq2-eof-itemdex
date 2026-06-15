# Per-Component Damage Sim (Increment B) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the rotation sim consume each art's parsed `Components`: per-component damage with per-mechanic ability-mod placement, DoT tick windowing (clip vs hold), conditional termination, and Death Mark trigger scoring.

**Architecture:** `CAEffectiveDamage` is rewritten to sum over `ca.Components` (DirectHit gets full abmod, DoT ticks + Termination get none, TriggerProc gets ½ abmod × trigger count, RateProc is skipped). DoT tick count and the termination's presence are gated by a per-art **cadence**: termination arts HOLD to `max(effRecast, duration)` (full ticks + detonate); all other duration arts CLIP on `effRecast` (windowed ticks, no detonate). Arts with **no** parsed components fall back to the legacy single-line equation, so existing model tests and any un-repulled data are unaffected.

**Tech Stack:** Go, `math`, testify. Builds on Increment A (plan 2q) — `spell.Component`/`ComponentKind` and `CombatArt.Components`/`DurationSecs` already exist and are persisted/loaded.

**Spec:** `docs/design-plan2.md` §3.1 "Multi-component abilities". Branch: `dot-component-sim`.

**Calibrated rules being implemented (§3.1):**
- abmod: DirectHit full; DoT ticks + Termination zero; TriggerProc `0.5 × abmod` per trigger; RateProc unscored.
- rotation: `hasTermination` → HOLD (`cadence = max(effRecast, duration)`, ticks over full duration, detonate fires); else CLIP (`cadence = effRecast`, ticks over `min(effRecast, duration)`, no detonate).
- DoT applications = `(hasInstant ? 1 : 0) + floor(window / interval)`.
- Ground-truth calibration bases (tests): Gushing Wound melee **49.2–82.5**, piercing **69.6–116.5** (instant+every 4s), detonate **326.1–543.2**, duration 24s; Death Mark per-trigger **329–545**, 5 triggers.

---

## File Structure

- `internal/model/rotation.go` — **modify**: rewrite `CAEffectiveDamage` to per-component; add `hasTermination`, `dotTicks`, `artCadence` helpers; route `rotationTimeline` recast through `artCadence`.
- `internal/model/rotation_test.go` — **modify**: add per-component damage tests, cadence tests, Gushing Wound + Death Mark calibration tests. Existing tests stay green via the legacy fallback.

No new files; this is a focused change to the damage/rotation core. `spell.Component` fields used: `Kind`, `MinDamage`, `MaxDamage`, `IntervalSecs`, `HasInstant`, `Triggers`. `CombatArt.DurationSecs`/`Components` from Increment A.

---

### Task 1: Cadence + helpers (hold-on-termination, DoT tick count)

**Files:**
- Modify: `internal/model/rotation.go`
- Test: `internal/model/rotation_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/model/rotation_test.go`:

```go
func TestArtCadence_HoldOnTermination(t *testing.T) {
	dot := spell.CombatArt{
		Name: "Bleed Art", RecastSecs: 10, DurationSecs: 24,
		Components: []spell.Component{
			{Kind: spell.DoT, MinDamage: 70, MaxDamage: 117, IntervalSecs: 4, HasInstant: true},
		},
	}
	// No termination → clip → cadence is the plain effRecast (10s, no reuse).
	require.InDelta(t, 10.0, artCadence(StatBlock{}, dot), 1e-9)

	term := dot
	term.Components = append([]spell.Component{{Kind: spell.Termination, MinDamage: 300, MaxDamage: 500}}, dot.Components...)
	// Has termination → hold → cadence = max(effRecast 10, duration 24) = 24.
	require.InDelta(t, 24.0, artCadence(StatBlock{}, term), 1e-9)

	// Reuse can pull effRecast below duration, but a held art still waits for termination.
	require.InDelta(t, 24.0, artCadence(StatBlock{Reuse: 50}, term), 1e-9) // effRecast 5 < 24 → 24
}

func TestDotTicks(t *testing.T) {
	// instant + every 4s over a 24s window → 1 + floor(24/4) = 7.
	c := spell.Component{Kind: spell.DoT, IntervalSecs: 4, HasInstant: true}
	require.InDelta(t, 7.0, dotTicks(c, 24), 1e-9)
	// periodic-only (no instant) over 12s / 4s → 3.
	c2 := spell.Component{Kind: spell.DoT, IntervalSecs: 4, HasInstant: false}
	require.InDelta(t, 3.0, dotTicks(c2, 12), 1e-9)
	// clipped to a 5s window, instant + floor(5/4)=1 → 2.
	require.InDelta(t, 2.0, dotTicks(c, 5), 1e-9)
	// zero interval guards against div-by-zero.
	require.InDelta(t, 0.0, dotTicks(spell.Component{Kind: spell.DoT}, 24), 1e-9)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run 'TestArtCadence_HoldOnTermination|TestDotTicks' -v`
Expected: FAIL — `artCadence` / `dotTicks` / `hasTermination` undefined.

- [ ] **Step 3: Implement the helpers**

Add to `internal/model/rotation.go` (after `effRecast`):

```go
// hasTermination reports whether the art carries an on-termination detonate
// component (the switch that makes a DoT held-to-full-duration rather than
// clipped on cooldown).
func hasTermination(ca spell.CombatArt) bool {
	for _, c := range ca.Components {
		if c.Kind == spell.Termination {
			return true
		}
	}
	return false
}

// artCadence is the scheduling interval between casts. A termination art is
// HELD to its full duration so the detonate lands (max with effRecast — a long
// cooldown still gates it); every other art (including clipped DoTs) recasts on
// its plain effRecast.
func artCadence(sb StatBlock, ca spell.CombatArt) float64 {
	er := effRecast(sb, ca)
	if hasTermination(ca) {
		return math.Max(er, ca.DurationSecs)
	}
	return er
}

// dotTicks is the number of applications a DoT component delivers inside an
// active window: one instant tick (if present) plus one per completed interval.
func dotTicks(c spell.Component, windowSecs float64) float64 {
	if c.IntervalSecs <= 0 {
		return 0
	}
	ticks := math.Floor(windowSecs / c.IntervalSecs)
	if c.HasInstant {
		ticks++
	}
	return ticks
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/model/ -run 'TestArtCadence_HoldOnTermination|TestDotTicks' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/model/rotation.go internal/model/rotation_test.go
git commit -m "Sim: hasTermination + artCadence (hold-on-termination) + dotTicks helpers"
```

---

### Task 2: Per-component CAEffectiveDamage (abmod placement, windowing, conditional termination, trigger scoring)

**Files:**
- Modify: `internal/model/rotation.go`
- Test: `internal/model/rotation_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/model/rotation_test.go`:

```go
func TestCAEffectiveDamage_PerComponent(t *testing.T) {
	// No-stats StatBlock → scaling = 1, crit = 1, so damage is just the base math.
	sb := StatBlock{AbilityMod: 100}

	// DirectHit gets full abmod.
	dh := spell.CombatArt{Components: []spell.Component{
		{Kind: spell.DirectHit, MinDamage: 800, MaxDamage: 1200},
	}}
	require.InDelta(t, 1100.0, CAEffectiveDamage(sb, dh), 0.01) // avg 1000 + 100 abmod

	// DoT held to full duration: instant + 6 ticks = 7 applications, NO abmod.
	gw := spell.CombatArt{
		RecastSecs: 30, DurationSecs: 24,
		Components: []spell.Component{
			{Kind: spell.DirectHit, MinDamage: 800, MaxDamage: 1200},
			{Kind: spell.DoT, MinDamage: 50, MaxDamage: 50, IntervalSecs: 4, HasInstant: true},
			{Kind: spell.Termination, MinDamage: 300, MaxDamage: 300},
		},
	}
	// DirectHit 1000+100 + DoT 50×7 + detonate 300 (fires: termination held) = 1100+350+300 = 1750.
	require.InDelta(t, 1750.0, CAEffectiveDamage(sb, gw), 0.01)

	// Clipped DoT (no termination): effRecast 10 < duration 24 → window 10 → instant + floor(10/4)=2 → 3 ticks.
	clip := spell.CombatArt{
		RecastSecs: 10, DurationSecs: 24,
		Components: []spell.Component{
			{Kind: spell.DirectHit, MinDamage: 800, MaxDamage: 1200},
			{Kind: spell.DoT, MinDamage: 50, MaxDamage: 50, IntervalSecs: 4, HasInstant: true},
		},
	}
	// 1100 + 50×3 = 1250 (no detonate — none present, and it never terminates anyway).
	require.InDelta(t, 1250.0, CAEffectiveDamage(sb, clip), 0.01)

	// TriggerProc: (base + 0.5×abmod) × triggers, no DirectHit.
	dm := spell.CombatArt{
		RecastSecs: 30, DurationSecs: 72,
		Components: []spell.Component{
			{Kind: spell.TriggerProc, MinDamage: 400, MaxDamage: 600, Triggers: 5},
		},
	}
	// (500 + 0.5×100) × 5 = 550 × 5 = 2750.
	require.InDelta(t, 2750.0, CAEffectiveDamage(sb, dm), 0.01)

	// RateProc is not scored.
	rp := spell.CombatArt{Components: []spell.Component{
		{Kind: spell.RateProc, MinDamage: 100, MaxDamage: 100, PerMinute: 3},
	}}
	require.InDelta(t, 0.0, CAEffectiveDamage(sb, rp), 0.01)

	// Backward-compat: no components → legacy single-line (full abmod on the one line).
	legacy := spell.CombatArt{MinDamage: 800, MaxDamage: 1200}
	require.InDelta(t, 1100.0, CAEffectiveDamage(sb, legacy), 0.01)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run TestCAEffectiveDamage_PerComponent -v`
Expected: FAIL — current `CAEffectiveDamage` ignores `Components` (e.g. `dh`/`gw` return wrong values, `dm` returns ~0+abmod from empty Min/Max).

- [ ] **Step 3: Rewrite CAEffectiveDamage**

In `internal/model/rotation.go`, replace the whole `CAEffectiveDamage` function with:

```go
// CAEffectiveDamage is one cast's damage. Arts carrying parsed Components sum
// per-component under the calibrated rules (spec §3.1): DirectHit takes ability
// mod in full; DoT ticks and Termination take none; a TriggerProc takes half the
// ability mod per trigger; RateProc is not scored. Potency pool and the main-stat
// curve multiply every component's base; crit multiplies the total. DoT tick count
// and whether the detonate fires are gated by the art's cadence — a termination
// art is HELD to its full duration (full ticks + detonate); any other DoT is
// CLIPPED on effRecast (ticks only within that window, no detonate). Arts without
// parsed components fall back to the legacy single damage line (full abmod).
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

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/model/ -run TestCAEffectiveDamage_PerComponent -v`
Expected: PASS.

- [ ] **Step 5: Run the existing model suite (legacy fallback must stay green)**

Run: `go test ./internal/model/ -v`
Expected: PASS — `TestCAEffectiveDamageMeasuredEquation` and the rotation/weights tests build arts with no `Components`, so they take the legacy path unchanged.

- [ ] **Step 6: Commit**

```bash
git add internal/model/rotation.go internal/model/rotation_test.go
git commit -m "Sim: per-component CAEffectiveDamage (abmod placement, DoT windowing, conditional detonate, trigger scoring; legacy fallback)"
```

---

### Task 3: Route the rotation through artCadence (hold-on-termination scheduling)

**Files:**
- Modify: `internal/model/rotation.go`
- Test: `internal/model/rotation_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/model/rotation_test.go`:

```go
func TestRotationHoldsTerminationArt(t *testing.T) {
	// A termination DoT with effRecast 10 but duration 24 must recast on 24 (held),
	// not 10. Over a 48s fight it fires at t=0 and t=24 → exactly 2 casts.
	term := spell.CombatArt{
		Name: "Held", RecastSecs: 10, DurationSecs: 24, CastSecsHundredths: 100,
		Components: []spell.Component{
			{Kind: spell.DirectHit, MinDamage: 1000, MaxDamage: 1000},
			{Kind: spell.Termination, MinDamage: 500, MaxDamage: 500},
		},
	}
	starts, _ := rotationTimeline(StatBlock{RecoverySpeed: 100}, []spell.CombatArt{term}, 48)
	require.Len(t, starts, 2)
	require.InDelta(t, 0.0, starts[0], 1e-9)
	require.InDelta(t, 24.0, starts[1], 1e-9) // held to duration, not 10
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run TestRotationHoldsTerminationArt -v`
Expected: FAIL — `rotationTimeline` still uses `effRecast` (10s), so it fires at 0, 10, 20, 30, 40 → 5 casts, not 2.

- [ ] **Step 3: Use artCadence for the recast in rotationTimeline**

In `internal/model/rotation.go`, inside `rotationTimeline`, change the per-art recast precompute from `effRecast` to `artCadence`:

```go
	for i, ca := range cas {
		eff[i] = CAEffectiveDamage(sb, ca)
		rec[i] = artCadence(sb, ca)
		slot[i] = slotSecs(sb, ca)
	}
```

(Only the `rec[i]` line changes — `effRecast` → `artCadence`.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/model/ -run TestRotationHoldsTerminationArt -v`
Expected: PASS.

- [ ] **Step 5: Run the model suite**

Run: `go test ./internal/model/ -v`
Expected: PASS — arts without components have `hasTermination == false`, so `artCadence == effRecast` and existing rotation/timeline/weights tests are unchanged.

- [ ] **Step 6: Commit**

```bash
git add internal/model/rotation.go internal/model/rotation_test.go
git commit -m "Sim: schedule recast via artCadence so termination arts hold to full duration"
```

---

### Task 4: Gushing Wound calibration test (ground-truth bases)

**Files:**
- Test: `internal/model/rotation_test.go`

- [ ] **Step 1: Write the calibration test**

Add to `internal/model/rotation_test.go` (uses the naked-recovered ground-truth bases from spec §3.1; hand-computed against the full-gear state):

```go
func TestGushingWoundCalibration(t *testing.T) {
	gw := spell.CombatArt{
		Name: "Gushing Wound VI", RecastSecs: 30, DurationSecs: 24,
		Components: []spell.Component{
			{Kind: spell.Termination, DamageType: "piercing", MinDamage: 326.1, MaxDamage: 543.2, TriggeredSpell: "Untreated Bleeding"},
			{Kind: spell.DirectHit, DamageType: "melee", MinDamage: 49.2, MaxDamage: 82.5},
			{Kind: spell.DoT, DamageType: "piercing", MinDamage: 69.6, MaxDamage: 116.5, IntervalSecs: 4, HasInstant: true},
		},
	}
	// Full-gear state: pool 58.7+24.6 = 83.3 → ×1.833; AGI 64.17% → ×1.6417; abmod 694; crit 0.
	sb := StatBlock{Potency: 58.7, PotencyBonus: 24.6, MainStat: agiFor(t, 64.17), AbilityMod: 694}
	scaling := 1.833 * 1.6417 // 3.0093

	avg := func(a, b float64) float64 { return (a + b) / 2 }
	directHit := avg(49.2, 82.5)*scaling + 694 // full abmod
	dotTickAvg := avg(69.6, 116.5) * scaling
	dot := dotTickAvg * 7 // instant + 6 ticks over 24s, no abmod
	detonate := avg(326.1, 543.2) * scaling // held → fires, no abmod
	want := directHit + dot + detonate

	require.InDelta(t, want, CAEffectiveDamage(sb, gw), 1.0)
	// Held: cadence is max(effRecast 30, duration 24) = 30.
	require.InDelta(t, 30.0, artCadence(sb, gw), 1e-9)
}

// agiFor returns a MainStat value whose curve output equals the given AGI%, so
// the test can pin scaling to the measured tooltip percentage rather than a raw
// stat (the curve is calibrated separately).
func agiFor(t *testing.T, wantPct float64) float64 {
	t.Helper()
	lo, hi := 0.0, 1100.0
	for i := 0; i < 60; i++ {
		mid := (lo + hi) / 2
		if MainStatEffect(mid) < wantPct {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./internal/model/ -run TestGushingWoundCalibration -v`
Expected: PASS. (If `MainStatEffect`'s curve can't reach 64.17% via `agiFor`, the binary search returns the cap — adjust by using `MainStat: 983` directly, the measured raw AGI for ~64% from the §3.1 ledger, and recompute `scaling` with `1 + MainStatEffect(983)/100`.)

- [ ] **Step 3: Commit**

```bash
git add internal/model/rotation_test.go
git commit -m "Test: Gushing Wound full-component calibration (ground-truth bases)"
```

---

### Task 5: Death Mark trigger calibration test

**Files:**
- Test: `internal/model/rotation_test.go`

- [ ] **Step 1: Write the test**

Add to `internal/model/rotation_test.go`:

```go
func TestDeathMarkCalibration(t *testing.T) {
	dm := spell.CombatArt{
		Name: "Death Mark IV", RecastSecs: 30, DurationSecs: 72,
		Components: []spell.Component{
			{Kind: spell.TriggerProc, DamageType: "piercing", MinDamage: 329, MaxDamage: 545, Triggers: 5, TriggeredSpell: "Agonizing Pain"},
		},
	}
	// Half-gear state from the calibration: pool 33.9+24.6 = 58.5 → ×1.585; AGI 51.42% → ×1.5142; abmod 376.
	sb := StatBlock{Potency: 33.9, PotencyBonus: 24.6, MainStat: agiFor(t, 51.42), AbilityMod: 376}
	scaling := 1.585 * 1.5142 // 2.400
	perTrigger := (329+545)/2.0*scaling + 0.5*376 // ½ abmod per trigger
	want := perTrigger * 5

	require.InDelta(t, want, CAEffectiveDamage(sb, dm), 5.0)
	// No termination → recast on plain effRecast (30s), not held.
	require.InDelta(t, 30.0, artCadence(sb, dm), 1e-9)
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./internal/model/ -run TestDeathMarkCalibration -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/model/rotation_test.go
git commit -m "Test: Death Mark trigger calibration (½ abmod × 5 triggers)"
```

---

### Task 6: Full suite + report smoke

**Files:** none (verification only).

- [ ] **Step 1: Run the entire test suite**

Run: `go test ./...`
Expected: all PASS. The rotation now consumes components for DB-loaded arts; the legacy fallback keeps every existing test green.

- [ ] **Step 2: Smoke the bis report against the real DB**

Run: `go run ./cmd/bis -character characters/alex.toml -db bis.db 2>&1 | head -40`
Expected: runs without error and prints a report. (The DB already holds components from the Increment-A `builddb`; Gushing Wound / Death Mark / Impale / Bleed / Quick Strike now contribute their full component damage. Sanity-check that DPS numbers are higher than before for the multi-component arts and nothing crashed.)

- [ ] **Step 3: Note known data caveat (no code change)**

Low-level-learned arts (Gushing Wound, Death Mark, Impale, Bleed, Quick Strike) carry census bases that disagree with naked reads by ~7–11% on their non-DirectHit components (spec §3.1 "Component bases"). The *mechanics* are calibrated (proven by Tasks 4–5 with ground-truth bases); the *production base accuracy* for these arts is the backlog-§9 manual-scaling follow-up, out of scope here. Leave a note in the PR/branch summary.

---

## Self-Review

**Spec coverage** (§3.1):
- Per-component sum, potency×mainstat per component → Task 2. ✅
- abmod placement: DirectHit full / DoT+Termination zero / TriggerProc ½×N / RateProc unscored → Task 2 + tests. ✅
- DoT applications `(instant?1:0)+floor(window/interval)` → Task 1 (`dotTicks`) + Task 2. ✅
- Clip vs hold: termination → hold `max(effRecast,duration)`, else clip `effRecast` → Task 1 (`artCadence`) + Task 3. ✅
- Conditional detonate (fires iff held/completes) → Task 2 (`if hold`). ✅
- Death Mark TriggerProc scoring (all 5 triggers) → Task 2 + Task 5. ✅
- Highest-rank census bases in the rotation; ground-truth bases in calibration tests → Task 6 uses the DB (HighestRanks already applied at load); Tasks 4–5 hand-code ground truth. ✅
- **Deferred, correctly absent:** RateProc scoring, detrimental no-DirectHit DoT abmod, clip-vs-hold optimization, overtime-potency AAs, AoE.

**Placeholder scan:** Task 4 Step 2 notes a fallback if `agiFor`'s binary search hits the curve cap (use raw `MainStat: 983`) — a real contingency with concrete instructions, not a placeholder. No TBD/TODO.

**Type consistency:** `hasTermination(ca)`, `artCadence(sb, ca)`, `dotTicks(c, windowSecs)` signatures are consistent across Tasks 1→2→3→4→5. `CAEffectiveDamage(sb, ca)` keeps its existing signature. `spell.Component` field names (`Kind`, `MinDamage`, `MaxDamage`, `IntervalSecs`, `HasInstant`, `Triggers`, `DamageType`, `TriggeredSpell`, `PerMinute`) and `ComponentKind` constants (`DirectHit`, `DoT`, `Termination`, `TriggerProc`, `RateProc`) match Increment A's `component.go`.
