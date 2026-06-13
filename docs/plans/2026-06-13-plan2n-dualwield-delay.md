# Plan 2n — Dual-Wield Delay Penalty Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Apply EQ2's measured +33% dual-wield delay penalty (×1.33 on each weapon's delay) in `AutoDPSDual`, which currently sums two un-penalized auto-attack streams.

**Architecture:** One constant (`DualWieldDelayPenalty = 1.33`) in `internal/constants`, applied in `AutoDPSDual` by scaling each weapon's `DelaySecs` before the existing per-weapon `AutoDPS`/`effDelay` math (`Weapon` is a value type, so local copies are safe). Single-weapon `AutoDPS` is untouched — the penalty is dual-path only.

**Tech Stack:** Go 1.26, testify. No new deps.

**Spec:** `docs/design-plan2.md` §3.1 "Dual-wield delay penalty" (commit on branch). Value 1.33: measured 1.320–1.337 across two haste levels + documented +33%.

---

### Task 1: Constant + AutoDPSDual penalty

**Files:**
- Modify: `internal/constants/constants.go`
- Modify: `internal/model/dps.go` (`AutoDPSDual`)
- Test: `internal/model/dps_test.go`

- [ ] **Step 1: Update tests first**

In `internal/model/dps_test.go`, add `"github.com/amdrake93/eq2-eof-itemdex/internal/constants"` to imports. Replace the existing `TestAutoDPSDual` (currently asserting the un-penalized sum 70.0) with:

```go
func TestAutoDPSDual(t *testing.T) {
	main := Weapon{AvgDamage: 100, DelaySecs: 2.0} // 50 dps unpenalized
	off := Weapon{AvgDamage: 60, DelaySecs: 3.0}   // 20 dps unpenalized

	// Dual-wield multiplies each weapon's delay by the penalty, so with all
	// other factors 1 the total is the naive sum divided by the penalty.
	approx(t, 70.0/constants.DualWieldDelayPenalty, AutoDPSDual(StatBlock{}, main, off))

	// It equals AutoDPS on penalty-scaled-delay weapons (the spec'd behavior).
	mainPen := Weapon{AvgDamage: 100, DelaySecs: 2.0 * constants.DualWieldDelayPenalty}
	offPen := Weapon{AvgDamage: 60, DelaySecs: 3.0 * constants.DualWieldDelayPenalty}
	approx(t, AutoDPS(StatBlock{Haste: 50}, mainPen)+AutoDPS(StatBlock{Haste: 50}, offPen),
		AutoDPSDual(StatBlock{Haste: 50}, main, off))

	// And it is strictly below the un-penalized sum.
	require.Less(t, AutoDPSDual(StatBlock{}, main, off),
		AutoDPS(StatBlock{}, main)+AutoDPS(StatBlock{}, off))
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/model/ -run TestAutoDPSDual -v`
Expected: FAIL — current `AutoDPSDual` returns 70.0, test wants `70.0/1.33 ≈ 52.63`.

- [ ] **Step 3: Implement**

In `internal/constants/constants.go`, add to the locked-constants block (near the rotation params):

```go
	DualWieldDelayPenalty = 1.33  // equipping an off-hand multiplies each weapon's auto-attack delay ×1.33 (measured 1.32–1.34 across two haste levels; documented +33%)
```

In `internal/model/dps.go`, replace `AutoDPSDual`:

```go
// AutoDPSDual is dual-wield auto-attack. Equipping an off-hand imposes EQ2's
// ~33% delay penalty on BOTH weapons (DualWieldDelayPenalty), on top of haste
// and independent of it (measured 2026-06-13). The penalty lives only here, not
// in single-weapon AutoDPS. Main and off are otherwise treated equally — the
// off-hand's weapon-multiplier-stat penalty isn't tracked and nets out for
// relative comparison.
func AutoDPSDual(sb StatBlock, main, off Weapon) float64 {
	main.DelaySecs *= constants.DualWieldDelayPenalty
	off.DelaySecs *= constants.DualWieldDelayPenalty
	return AutoDPS(sb, main) + AutoDPS(sb, off)
}
```

(`main`/`off` are value parameters — scaling their `DelaySecs` mutates only the local copies.)

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/model/ -v`
Expected: all PASS. (Single-weapon `TestAutoDPS`/`TestEffDelayHasteFittedCurve` unaffected — they never call `AutoDPSDual`. `TotalDPSDual` now inherits the penalty automatically.)

- [ ] **Step 5: Full gates**

Run: `go test ./... -count=1 && make lint`
Expected: all packages PASS, lint clean. (`internal/bis` uses `TotalDPSDual` via the set builder — confirm its tests still pass; the penalty is a uniform scale so converged-set logic is unaffected, but the absolute DPS numbers in any value-pinned bis test may shift. If a bis test pins an absolute DPS/Δ value, update it with a comment noting the dual-wield penalty; if it only pins orderings/positivity, it passes unchanged. Report which.)

- [ ] **Step 6: Commit**

```bash
git add internal/constants/constants.go internal/model/dps.go internal/model/dps_test.go
git commit -m "Auto-attack: dual-wield delay penalty ×1.33 in AutoDPSDual (measured + documented +33%)"
```

---

### Task 2: Regenerate report + confirm the predicted weight shift

**Files:** none (regenerated `bis-report.md` stays untracked)

- [ ] **Step 1: Regenerate**

Run: `go run ./cmd/bis` then `go run ./cmd/weights`
Expected: both succeed; `bis-report.md` rewritten.

- [ ] **Step 2: Confirm the spec's prediction**

The penalty scales the auto term ~0.75× uniformly, so the spec predicts auto-side stat weights (haste, multiattack, flurry, dps-mod) tick **down** relative to CA stats (reuse, potency, etc.), which tick **up**. In your report summary, paste the three converged weight tables and state whether haste/MA/flurry/dps-mod dropped vs. the pre-change values (PRE-RAID/RAID/BoB):
- pre-change haste: 1.12 / 1.87 / 1.88
- pre-change multiattack: 0.96 / 1.48 / 1.48
- pre-change flurry: 4.58 / 7.04 / 8.16
- pre-change dpsmod: 0.87 / 0.65 / 0.69

A drop in those (and a relative rise in reuse/potency) confirms the change took effect as designed. No commit (report is untracked).

---

## Self-review notes

- **Spec coverage:** constant ✓ (T1), applied in `AutoDPSDual` only ✓ (T1), single-weapon path untouched ✓ (T1 Step 4 note), value 1.33 with provenance ✓ (spec), weight-shift prediction checked ✓ (T2). Auto-attack damage *multiplier* decomposition (the ~4.96×, AGI-on-auto) is explicitly NOT in scope — it's the agreed next work item (weapon-damage calibration).
- **No placeholders.** Exact test code, exact expected value (`70.0/1.33 ≈ 52.63`), exact constant.
- **Type consistency:** `constants.DualWieldDelayPenalty`, `AutoDPSDual(sb, main, off)` — unchanged signature, so `TotalDPSDual` and all `internal/bis` callers compile untouched.
