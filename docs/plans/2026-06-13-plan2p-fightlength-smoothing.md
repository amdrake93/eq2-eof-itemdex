# Plan 2p — Fight-Length Smoothing + Configurable Fight Length

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the single-fixed-600s cast-boundary quantization that makes reuse scores lumpy (two near-identical reuse charms scored 240 vs 177), by averaging CADPS over a recast-wide window around a configurable target fight length — via one sim pass over a recorded cast timeline.

**Architecture:** The priority sim is refactored to record each cast's start time + cumulative CA damage (it's prefix-consistent, so one run to the window's top yields `cumCA(t)` for every `t`). `CADPS(sb, cas, fightLen)` averages `cumCA(t)/t` over K samples across `[fightLen − R/2, fightLen + R/2]`, `R` = longest effective recast (auto-computed). `fightLen` threads through `TotalDPS`/`TotalDPSDual`/`ItemDelta`/the bis `Set` (mirroring the existing `classAutoMult` param) and is set from a new `-fight` flag (default 600). Smoothed CADPS should make the wide-span marginal band-aids redundant — verified by reverting them.

**Tech Stack:** Go 1.26, testify. No new deps.

**Spec:** `docs/design-plan2.md` §3.1 "Fight-length smoothing" (committed this session).

**Key facts:** single short-recast-art sims are smoothing-invariant (no big boundary), so simple `TestCADPS` cases keep their values; the real 19-art pool shifts because Assassinate's ~150s boundary gets averaged. K = 9 (internal constant). Window centered on `fightLen`, lower bound clamped `> 0`.

---

### Task 1: Smoothing core + thread `fightLen` (model + bis + cmd — ONE commit, compile boundary)

`CADPS`'s new signature cascades through `TotalDPS`/`TotalDPSDual`/`ItemDelta`/`Set`/cmd, so this is one compile-bound commit (same shape as the classAutoMult change).

**Files:**
- Modify: `internal/model/rotation.go`, `internal/model/dps.go`, `internal/model/itemdelta.go`, `internal/constants/constants.go`
- Modify: `internal/bis/set.go`, `internal/bis/report.go`, `internal/bis/build.go`, `internal/bis/render.go`
- Modify: `cmd/weights/main.go`, `cmd/bis/main.go`
- Test: `internal/model/rotation_test.go`, `internal/model/timeline_test.go`, `internal/model/dps_test.go`, `internal/model/itemdelta_test.go`, `internal/model/weights_test.go`, `internal/bis/set_test.go`, `internal/bis/build_test.go`, `internal/bis/buildset_test.go`, `internal/bis/report_test.go`

#### Model core

- [ ] **Step 1: Write the failing tests** (append to `internal/model/rotation_test.go`)

```go
func TestCADPSSmoothingInvariantForShortRecast(t *testing.T) {
	// A single short-recast art has no big-cast boundary, so smoothing is a
	// no-op: CADPS ≈ the raw single-length rate.
	cas := []spell.CombatArt{{Name: "X", MinDamage: 800, MaxDamage: 1200, RecastSecs: 10}}
	approx(t, 100.0, CADPS(StatBlock{}, cas, 600)) // 1000 dmg / 10s recast
}

func TestCADPSSmoothsBigCastBoundary(t *testing.T) {
	// One big art on a 150s recast, fight target 150 (right on its 2nd-cast
	// boundary). Raw single-length is a coin-flip on the 2nd cast; smoothed
	// CADPS lands strictly between the 1-cast and 2-cast rates.
	cas := []spell.CombatArt{{Name: "Big", MinDamage: 10000, MaxDamage: 10000, RecastSecs: 150}}
	oneCast := 10000.0 / 150.0  // only the t=0 cast credited
	twoCast := 20000.0 / 150.0  // t=0 and t=150 both credited
	got := CADPS(StatBlock{}, cas, 150)
	require.Greater(t, got, oneCast)
	require.Less(t, got, twoCast)
}

func TestCumCAAt(t *testing.T) {
	starts := []float64{0, 150, 300}
	cum := []float64{100, 200, 300}
	require.InDelta(t, 0.0, cumCAAt(starts, cum, 0), 1e-9)   // nothing before t=0
	require.InDelta(t, 100.0, cumCAAt(starts, cum, 150), 1e-9) // only the t=0 cast (start<150)
	require.InDelta(t, 200.0, cumCAAt(starts, cum, 300), 1e-9)
	require.InDelta(t, 300.0, cumCAAt(starts, cum, 1000), 1e-9)
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/model/ -run 'TestCADPS|TestCumCAAt' 2>&1 | head`
Expected: compile error (`cumCAAt` undefined; `CADPS` arg-count mismatch).

- [ ] **Step 3: Implement the timeline sim + smoothed CADPS** in `internal/model/rotation.go`

Replace `RotationCADPS` with a timeline recorder + a thin wrapper, and add the smoothing. Add `fightSmoothingSamples` near the top:

```go
// fightSmoothingSamples is K: how many fight lengths CADPS averages across the
// recast-wide window to smooth cast-boundary quantization (spec §3.1).
const fightSmoothingSamples = 9

// rotationTimeline runs the priority sim out to maxLen, recording each fired
// cast's start time and the cumulative CA damage through it. The sim is
// prefix-consistent — a fight of length s credits exactly the casts with start
// time < s — so one run yields cumCA(t) for every t ≤ maxLen.
func rotationTimeline(sb StatBlock, cas []spell.CombatArt, maxLen float64) (starts, cum []float64) {
	eff := make([]float64, len(cas))
	rec := make([]float64, len(cas))
	slot := make([]float64, len(cas))
	avail := make([]float64, len(cas))
	for i, ca := range cas {
		eff[i] = CAEffectiveDamage(sb, ca)
		rec[i] = effRecast(sb, ca)
		slot[i] = slotSecs(sb, ca)
	}
	var total, t float64
	for t < maxLen {
		best, bestRate := -1, -1.0
		for i := range cas {
			if avail[i] <= t {
				if rate := eff[i] / slot[i]; rate > bestRate {
					best, bestRate = i, rate
				}
			}
		}
		if best < 0 {
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
		total += eff[best]
		starts = append(starts, t)
		cum = append(cum, total)
		avail[best] = t + rec[best]
		t += slot[best]
	}
	return starts, cum
}

// cumCAAt is the cumulative CA damage from casts started strictly before s.
func cumCAAt(starts, cum []float64, s float64) float64 {
	out := 0.0
	for i, st := range starts {
		if st < s {
			out = cum[i]
		} else {
			break
		}
	}
	return out
}

// maxEffRecast is the longest effective recast in the art set — the smoothing
// window width R (one full big-cast boundary cycle).
func maxEffRecast(sb StatBlock, cas []spell.CombatArt) float64 {
	r := 0.0
	for _, ca := range cas {
		if er := effRecast(sb, ca); er > r {
			r = er
		}
	}
	return r
}

// RotationCADPS is total CA damage over a single fixed fight length / that
// length (unsmoothed). Retained for direct single-length tests.
func RotationCADPS(sb StatBlock, cas []spell.CombatArt, durationSecs float64) float64 {
	if durationSecs <= 0 || len(cas) == 0 {
		return 0
	}
	starts, cum := rotationTimeline(sb, cas, durationSecs)
	return cumCAAt(starts, cum, durationSecs) / durationSecs
}
```

In `internal/model/dps.go`, replace `CADPS` with the smoothed, fight-length-parameterized version:

```go
// CADPS is the fight-length-smoothed combat-art DPS for a target fight length.
// A single fixed length quantizes the last cast of long-cooldown arts (spec
// §3.1); CADPS averages cumCA(t)/t over K samples spanning [fightLen − R/2,
// fightLen + R/2], R = longest effective recast, computed from one sim pass to
// the window's top. Short-recast-only art sets have R≈0 → effectively unsmoothed.
func CADPS(sb StatBlock, cas []spell.CombatArt, fightLen float64) float64 {
	if fightLen <= 0 || len(cas) == 0 {
		return 0
	}
	r := maxEffRecast(sb, cas)
	lo := math.Max(fightLen-r/2, 1.0)
	hi := fightLen + r/2
	starts, cum := rotationTimeline(sb, cas, hi)
	if hi <= lo {
		return cumCAAt(starts, cum, fightLen) / fightLen
	}
	var sum float64
	for i := 0; i < fightSmoothingSamples; i++ {
		s := lo + float64(i)*(hi-lo)/float64(fightSmoothingSamples-1)
		sum += cumCAAt(starts, cum, s) / s
	}
	return sum / fightSmoothingSamples
}
```

Add `"math"` to `dps.go` imports if not already present (it is, via effDelay? check — `dps.go` may not import math; add if needed).

#### Thread `fightLen`

- [ ] **Step 4: Thread the param through dps.go + itemdelta.go**

`internal/model/dps.go`:
- `TotalDPS(sb, w, cas, classAutoMult, fightLen float64)` → `classAutoMult*AutoDPS(sb, w) + CADPS(sb, cas, fightLen)`
- `TotalDPSDual(sb, main, off, cas, classAutoMult, fightLen float64)` → `AutoDPSDual(sb, main, off, classAutoMult) + CADPS(sb, cas, fightLen)`

`internal/model/itemdelta.go`: `ItemDelta(restBase, main, restOff, arts, itemStats, newOff, classAutoMult, fightLen float64)` — pass `fightLen` to both `TotalDPSDual` calls.

#### bis

- [ ] **Step 5: bis Set + build + report + render**

`internal/bis/set.go`: add `FightLen float64` to `Set`; `NewSet(profile, lo, autoMult, fightLen float64)` sets it; `DPS()` → `model.TotalDPSDual(..., s.AutoMult, s.FightLen)`; both `CandidateDelta` `ItemDelta` calls gain `s.FightLen`.

`internal/bis/build.go`: `BuildSet(..., autoMult, fightLen float64)` → `NewSet(profile, lo, autoMult, fightLen)`.

`internal/bis/report.go` `ConvergedWeights`: the `dps` closure → `model.TotalDPSDual(sb, set.Main, off, set.Arts, set.AutoMult, set.FightLen)`.

`internal/bis/render.go` assumptions line: replace the trailing `fight = %.0fs` (sourced from `constants.FightDurationSecs`) with the configured target — pass the fight length into `Render`/`writeAssumptions` (add a param) OR read it from the first report's Set. Simplest: add a `fightLen float64` param to `Render` and `writeAssumptions`, print `fight target = %.0fs (smoothed)`.

#### cmd

- [ ] **Step 6: `-fight` flag**

`cmd/bis/main.go` and `cmd/weights/main.go`: add `fight := flag.Float64("fight", constants.FightDurationSecs, "target fight length in seconds (smoothed)")`. Pass `*fight` to every `BuildSet(...)` (bis, 3 tiers + locked block), to the `dps` closure's `TotalDPSDual` (weights), and to `bis.Render(reports, *fight)`. Import `internal/constants` in the cmds if needed.

#### Tests

- [ ] **Step 7: Update all test call sites**

Add the `fightLen` arg (use `600` or `constants.FightDurationSecs`) to every changed call across `dps_test.go`, `timeline_test.go`, `itemdelta_test.go`, `weights_test.go`, `set_test.go` (`NewSet`), `build_test.go` (`NewSet`/`BuildSet`), `buildset_test.go`, `report_test.go`. For `CADPS(sb, cas)` calls add `, 600`.

**Expected-value handling:** simple single-short-recast-art `TestCADPS` cases are smoothing-invariant (R = that recast; cumCA(t)/t flat) → values unchanged. The `Reuse:100` case (recast halved) likewise. If any test using the realistic/multi-art pool shifts, update it with a comment noting smoothing as the cause — but the model package's `TestCADPS` uses single arts, so expect no value changes there. `weights_test.go` curve-marginal tests use `AutoDPS` (unchanged) or single-art `CADPS` (invariant) — verify they still pass; update only the call signatures.

- [ ] **Step 8: Full gates**

Run: `go test ./... -count=1 && make lint && go build ./...`
Expected: all pass, lint clean, build clean.

- [ ] **Step 9: Commit**

```bash
git add internal/model/ internal/constants/ internal/bis/ cmd/weights/ cmd/bis/
git commit -m "CADPS: fight-length smoothing over a recast-wide window; configurable -fight (one sim pass via recorded timeline)"
```

---

### Task 2: Revert the wide-span marginal band-aids, verify

The wide spans (`reuseMarginalHalfSpan`, `castSpeedMarginalSpan`) existed only to average out the quantization that CADPS now smooths at the source. Revert to the plain +1 diff and verify the marginals stay clean.

**Files:**
- Modify: `internal/model/weights.go`
- Test: `internal/model/weights_test.go`

- [ ] **Step 1: Revert to +1 diff**

In `internal/model/weights.go`, remove the `castspeed` and `reuse` special-case branches in `DeriveWeights` (and the `castSpeedMarginalSpan` / `reuseMarginalHalfSpan` constants), so both fall through to the standard `+1` finite difference like the other linear stats.

- [ ] **Step 2: Verify the weights are clean**

Run: `go run ./cmd/weights`
Expected: `reuse` and `castspeed` weights are stable and sensible (reuse still the dominant pre-raid stat; no wild ±50 swings, no spurious negatives from boundary noise). If reuse/castspeed come back noisy/negative, the smoothing isn't fully removing the artifact — **STOP and report** (keep the spans, investigate) rather than shipping noisy weights.

- [ ] **Step 3: Update/trim tests**

Remove or update `weights_test.go` tests that asserted the wide-span behavior (`TestReuseWeightUsesCenteredClampedSpan`, `TestCastSpeedWeightUsesWideSpan`, `TestCurveStatMarginalDeadZoneBeforeCap` if it depended on the span). Replace with a plain-diff expectation or delete if they only tested the band-aid. Keep any test that asserts a genuine behavior (e.g. mainstat marginal). Run `go test ./internal/model/ -v`.

- [ ] **Step 4: Full gates + commit**

Run: `go test ./... -count=1 && make lint`

```bash
git add internal/model/weights.go internal/model/weights_test.go
git commit -m "Curve weights: revert wide-span reuse/castspeed marginals — fight-length smoothing supersedes them"
```

(If Step 2 said STOP, skip this task, leave the spans, and note it in the summary — the spec already says the revert is conditional on verification.)

---

### Task 3: Regenerate + confirm the lumpiness is gone

**Files:** none (regenerated `bis-report.md` stays untracked)

- [ ] **Step 1: Regenerate**

Run: `go run ./cmd/bis` then `go run ./cmd/weights`
Expected: both succeed.

- [ ] **Step 2: Confirm the artifact collapsed**

The whole point: two near-identical reuse charms scored **240.1 (Orb) vs 177.3 (Arachibutyrophobia)** before — a 63-DPS gap that was pure quantization noise. After smoothing they should be **close** (within a few DPS, ordered by their tiny real stat differences). In the summary, paste the Charm slot's top entries and confirm Orb/Arachibutyrophobia converged. Also confirm the Wrist slot's #1-by-ΔDPS now matches what's actually picked (no more reuse-stick ranked #1 but not chosen).

- [ ] **Step 3: Spot-check a non-default length**

Run: `go run ./cmd/bis -fight 270` and confirm it completes and produces a report (the "almost 3rd Assassinate" target). Note any sensible shift (reuse should look slightly more valuable at 270 if it helps reach that 3rd cast). No commit (report untracked).

---

## Self-review notes

- **Spec coverage:** smoothed CADPS = mean cumCA(t)/t over K window samples ✓ (T1); R = longest eff recast auto-computed ✓ (T1 `maxEffRecast`); one sim pass via recorded timeline ✓ (T1 `rotationTimeline`); configurable `-fight` default 600 ✓ (T1 Step 6); window lower-bound clamp >0 ✓ (`math.Max(..., 1.0)`); routes through CADPS so picks/ΔDPS/weights consistent ✓; wide-span band-aids reverted+verified ✓ (T2, conditional); lumpiness-gone verification ✓ (T3).
- **Compile boundary:** `CADPS`/`TotalDPS`/`TotalDPSDual`/`ItemDelta`/`BuildSet`/`NewSet` signatures all gain `fightLen`; bis+cmd break until updated → one commit (T1). Mirrors the classAutoMult threading exactly (purely additive — does not re-touch classAutoMult logic).
- **Type consistency:** `rotationTimeline(sb, cas, maxLen) (starts, cum []float64)`, `cumCAAt(starts, cum, s)`, `maxEffRecast(sb, cas)`, `CADPS(sb, cas, fightLen)`, `fightSmoothingSamples`, `Set.FightLen`, `Render(reports, fightLen)` — consistent across tasks.
- **Prefix-consistency claim:** the priority sim's cast at time `t` is credited iff `t < duration`; recording `(t, runningTotal)` and reading the last entry with `start < s` reproduces exactly what a length-`s` run would total — so one run to `hi` is faithful for all samples `≤ hi`. (Verified by `TestCumCAAt` + `TestCADPSSmoothsBigCastBoundary`.)
- **Risk note:** Task 2 (revert spans) is gated on Step 2 verification — if smoothing leaves residual noise, keep the spans and report; do not ship noisy weights.
