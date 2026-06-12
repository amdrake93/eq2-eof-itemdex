# Plan 2j — Haste/DPS-mod Curve Refit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the disproven piecewise haste/dps-mod curve (200-cap, patch-note anchor) with a quadratic equation fitted from 20 committed in-game readings, raise the cap to 300, and drop the raid baseline group DPS-mod from 200 to the measured 114.2.

**Architecture:** Readings are data (`data/curve-readings.csv`), the equation is derived (`internal/fit` + `cmd/fitcurve` print fitted constants; `internal/model/curve.go` records them; a sync test fails if the recorded constants drift from a re-fit of the CSV — that test *is* the refresh loop). Marginal weights for haste/dps-mod move from table-sample brackets to brackets at the fitted curve's integer-effect crossings, preserving the zero-flooring-noise property anywhere on the curve.

**Tech Stack:** Go 1.26, `encoding/csv`, `stretchr/testify`. No new dependencies.

**Spec:** `docs/design-plan2.md` §3.1 (commits `970483c`, `2e56e1d`).

**Fitted values (pre-computed from the 20 readings; the tool in Task 5 must reproduce these):**

| fit | form | a | b | RMS |
|---|---|---|---|---|
| joint (n=20) | quad | 0.800348 | 0.00127275 | 0.2868 |
| joint (n=20) | log | 99.99 | 106.35 | 1.9302 |
| haste (n=11) | quad | 0.801946 | 0.00127714 | 0.2536 |
| dpsmod (n=9) | quad | 0.798757 | 0.00126736 | 0.2923 |

Quadratic wins (6.7× lower RMS); haste/dpsmod subset fits agree → shared curve confirmed. Winner: `f(s) = 0.800348·s − 0.00127275·s²`, peak ≈ 314.4 (past the 300 cap), `f(300) = 125.56 → shows 125%`. Floored predictions match 18/20 readings exactly, the other two by one floor step.

Key derived values used in test expectations below: `f(24)=18.47→18`, `f(28.1)=21.48→21`, `f(48.3)=35.69→35`, `f(67.5)=48.23→48`, `f(100)=67.31→67`, `f(200)=109.16→109`, `f(281)=124.40→124`, `f(300)=125.56→125`.

---

### Task 1: Commit the readings dataset

**Files:**
- Create: `data/curve-readings.csv`

- [ ] **Step 1: Create the CSV**

`stat` is `haste`|`dpsmod`; `effect` is the UI's floored integer %; `era` is `varsoon` (original server, incl. playtest) or `live` (current TLE server).

```csv
stat,raw,effect,era
haste,24,18,varsoon
dpsmod,28.1,21,varsoon
dpsmod,48.3,35,varsoon
haste,48.7,36,live
haste,53,38,live
haste,57.3,41,live
haste,61.6,44,live
haste,67.5,48,varsoon
dpsmod,70.6,50,live
dpsmod,78.8,55,live
dpsmod,86.9,59,live
haste,87.4,60,live
haste,91.7,62,live
haste,96,65,live
haste,100.3,67,live
dpsmod,136.5,85,live
dpsmod,144.6,88,live
dpsmod,152.8,92,live
dpsmod,238.4,118,varsoon
haste,281,124,varsoon
```

- [ ] **Step 2: Verify row count**

Run: `wc -l data/curve-readings.csv`
Expected: `21` (header + 20 readings)

- [ ] **Step 3: Commit**

```bash
git add data/curve-readings.csv
git commit -m "Curve refit: commit the 20 haste/dps-mod readings (varsoon + live eras)"
```

---

### Task 2: `internal/fit` — readings loader

**Files:**
- Create: `internal/fit/readings.go`
- Test: `internal/fit/readings_test.go`

- [ ] **Step 1: Write the failing test**

```go
package fit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const readingsPath = "../../data/curve-readings.csv"

func TestLoadReadings(t *testing.T) {
	rs, err := LoadReadings(readingsPath)
	require.NoError(t, err)
	require.Len(t, rs, 20)

	require.Equal(t, Reading{Stat: "haste", Raw: 24, Effect: 18, Era: "varsoon"}, rs[0])
	require.Equal(t, Reading{Stat: "haste", Raw: 281, Effect: 124, Era: "varsoon"}, rs[19])

	require.Len(t, Filter(rs, "haste"), 11)
	require.Len(t, Filter(rs, "dpsmod"), 9)
}

func TestFitTargetIsFloorIntervalMidpoint(t *testing.T) {
	require.InDelta(t, 18.5, Reading{Effect: 18}.FitTarget(), 1e-12)
}

func TestLoadReadingsMissingFile(t *testing.T) {
	_, err := LoadReadings("nope.csv")
	require.Error(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/fit/ -v`
Expected: FAIL — `undefined: LoadReadings` (package doesn't exist yet)

- [ ] **Step 3: Write the implementation**

`internal/fit/readings.go`:

```go
// Package fit derives the shared haste/dps-mod conversion curve from in-game
// readings (data/curve-readings.csv). Shown effects are UI-floored integers, so
// each reading's least-squares target is shown+0.5 — the midpoint of its floor
// interval [shown, shown+1).
package fit

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
)

// Reading is one in-game observation: a raw stat total and the effect % the UI
// displayed for it.
type Reading struct {
	Stat   string  // "haste" or "dpsmod"
	Raw    float64 // raw stat total shown by the UI
	Effect float64 // shown effect % (floored integer)
	Era    string  // "varsoon" (original server) or "live" (current TLE)
}

// FitTarget is the reading's least-squares target: the midpoint of [shown, shown+1).
func (r Reading) FitTarget() float64 { return r.Effect + 0.5 }

// LoadReadings parses the readings CSV (header: stat,raw,effect,era).
func LoadReadings(path string) ([]Reading, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("%s: no readings", path)
	}

	out := make([]Reading, 0, len(rows)-1)
	for _, row := range rows[1:] {
		raw, err := strconv.ParseFloat(row[1], 64)
		if err != nil {
			return nil, fmt.Errorf("raw %q: %w", row[1], err)
		}
		effect, err := strconv.ParseFloat(row[2], 64)
		if err != nil {
			return nil, fmt.Errorf("effect %q: %w", row[2], err)
		}
		out = append(out, Reading{Stat: row[0], Raw: raw, Effect: effect, Era: row[3]})
	}
	return out, nil
}

// Filter returns the readings for one stat ("haste" or "dpsmod").
func Filter(rs []Reading, stat string) []Reading {
	out := make([]Reading, 0, len(rs))
	for _, r := range rs {
		if r.Stat == stat {
			out = append(out, r)
		}
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/fit/ -v`
Expected: PASS (3 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/fit/readings.go internal/fit/readings_test.go
git commit -m "Curve refit: fit package readings loader"
```

---

### Task 3: `internal/fit` — quadratic fit (closed-form least squares)

**Files:**
- Create: `internal/fit/fit.go`
- Test: `internal/fit/fit_test.go`

- [ ] **Step 1: Write the failing test**

`internal/fit/fit_test.go`:

```go
package fit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// syntheticQuad builds readings lying exactly on f(s) = a·s − b·s², with Effect
// set 0.5 below f so FitTarget() reproduces f exactly.
func syntheticQuad(a, b float64) []Reading {
	var rs []Reading
	for s := 10.0; s <= 200; s += 10 {
		rs = append(rs, Reading{Stat: "haste", Raw: s, Effect: a*s - b*s*s - 0.5, Era: "live"})
	}
	return rs
}

func TestFitQuadRecoversExactParams(t *testing.T) {
	q := FitQuad(syntheticQuad(0.8, 0.001))
	require.InDelta(t, 0.8, q.A, 1e-9)
	require.InDelta(t, 0.001, q.B, 1e-9)
	require.InDelta(t, 0.0, RMS(q, syntheticQuad(0.8, 0.001)), 1e-9)
}

func TestFitQuadOnCommittedReadings(t *testing.T) {
	rs, err := LoadReadings(readingsPath)
	require.NoError(t, err)

	q := FitQuad(rs)
	require.InDelta(t, 0.800348, q.A, 1e-4)
	require.InDelta(t, 0.00127275, q.B, 1e-6)
	require.InDelta(t, 0.2868, RMS(q, rs), 1e-3)

	// Peak just past the 300 cap; effect at cap displays as 125%.
	require.InDelta(t, 314.4, q.A/(2*q.B), 0.5)
	require.InDelta(t, 125.56, q.Eval(300), 0.05)
}

func TestQuadSubsetFitsAgree(t *testing.T) {
	rs, err := LoadReadings(readingsPath)
	require.NoError(t, err)

	h := FitQuad(Filter(rs, "haste"))
	d := FitQuad(Filter(rs, "dpsmod"))
	require.InDelta(t, h.A, d.A, 0.01,   "haste/dpsmod fits diverge — shared curve in doubt")
	require.InDelta(t, h.B, d.B, 5e-5, "haste/dpsmod fits diverge — shared curve in doubt")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/fit/ -v`
Expected: FAIL — `undefined: FitQuad`, `undefined: RMS`

- [ ] **Step 3: Write the implementation**

`internal/fit/fit.go`:

```go
package fit

import "math"

// QuadParams is the quadratic diminishing-returns form f(s) = A·s − B·s².
type QuadParams struct{ A, B float64 }

func (q QuadParams) Eval(s float64) float64 { return q.A*s - q.B*s*s }

// Curve is any fitted form evaluable at a raw stat value.
type Curve interface{ Eval(s float64) float64 }

// FitQuad least-squares fits the quadratic to (Raw, FitTarget) pairs. Both
// parameters are linear, so the 2×2 normal equations solve in closed form:
//
//	a·Σs² − b·Σs³ = Σs·y
//	a·Σs³ − b·Σs⁴ = Σs²·y
func FitQuad(rs []Reading) QuadParams {
	var s2, s3, s4, sy, s2y float64
	for _, r := range rs {
		s, y := r.Raw, r.FitTarget()
		s2 += s * s
		s3 += s * s * s
		s4 += s * s * s * s
		sy += s * y
		s2y += s * s * y
	}

	det := -s2*s4 + s3*s3
	return QuadParams{
		A: (-sy*s4 + s3*s2y) / det,
		B: (s2*s2y - s3*sy) / det,
	}
}

// RMS is the root-mean-square residual of a fitted curve against the readings'
// fit targets.
func RMS(c Curve, rs []Reading) float64 {
	var rss float64
	for _, r := range rs {
		d := c.Eval(r.Raw) - r.FitTarget()
		rss += d * d
	}
	return math.Sqrt(rss / float64(len(rs)))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/fit/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/fit/fit.go internal/fit/fit_test.go
git commit -m "Curve refit: closed-form quadratic least squares"
```

---

### Task 4: `internal/fit` — logarithmic fit and the bake-off

**Files:**
- Modify: `internal/fit/fit.go` (append)
- Test: `internal/fit/fit_test.go` (append)

- [ ] **Step 1: Write the failing test** (append to `fit_test.go`)

```go
func TestFitLogRecoversParams(t *testing.T) {
	truth := LogParams{A: 100, B: 110}
	var rs []Reading
	for s := 10.0; s <= 280; s += 15 {
		rs = append(rs, Reading{Stat: "dpsmod", Raw: s, Effect: truth.Eval(s) - 0.5, Era: "live"})
	}

	l := FitLog(rs)
	require.InEpsilon(t, truth.A, l.A, 0.02) // B scans a 1% grid; A follows
	require.InEpsilon(t, truth.B, l.B, 0.02)
}

func TestQuadraticBeatsLogOnCommittedReadings(t *testing.T) {
	rs, err := LoadReadings(readingsPath)
	require.NoError(t, err)

	q, l := FitQuad(rs), FitLog(rs)
	require.Less(t, RMS(q, rs), RMS(l, rs), "spec expects the quadratic to win the bake-off")
	require.InDelta(t, 1.93, RMS(l, rs), 0.05)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/fit/ -v`
Expected: FAIL — `undefined: LogParams`, `undefined: FitLog`

- [ ] **Step 3: Write the implementation** (append to `fit.go`)

```go
// LogParams is the logarithmic diminishing-returns form f(s) = A·ln(1 + s/B).
type LogParams struct{ A, B float64 }

func (l LogParams) Eval(s float64) float64 { return l.A * math.Log(1+s/l.B) }

// FitLog scans B over a 1% geometric grid (1 → 20000); for each B the best A is
// linear least squares over g = ln(1+s/B). Deterministic and plenty precise for
// a residual bake-off against the quadratic.
func FitLog(rs []Reading) LogParams {
	best := LogParams{}
	bestRSS := math.Inf(1)

	for b := 1.0; b < 20000; b *= 1.01 {
		var gg, gy float64
		for _, r := range rs {
			g := math.Log(1 + r.Raw/b)
			gg += g * g
			gy += g * r.FitTarget()
		}
		a := gy / gg

		var rss float64
		for _, r := range rs {
			d := a*math.Log(1+r.Raw/b) - r.FitTarget()
			rss += d * d
		}
		if rss < bestRSS {
			bestRSS = rss
			best = LogParams{A: a, B: b}
		}
	}
	return best
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/fit/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/fit/fit.go internal/fit/fit_test.go
git commit -m "Curve refit: logarithmic fit and quad-vs-log bake-off"
```

---

### Task 5: `cmd/fitcurve` — the re-fit tool

**Files:**
- Create: `cmd/fitcurve/main.go`

No unit test — the main is a thin printer over the tested `internal/fit`; Step 3 verifies its output against the pre-computed table.

- [ ] **Step 1: Write the tool**

```go
// fitcurve fits the shared haste/dps-mod conversion curve from
// data/curve-readings.csv and prints paste-ready constants for
// internal/model/curve.go. Append new readings to the CSV and re-run; the
// TestFittedConstantsMatchReadings sync test fails until the constants are
// updated.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/amdrake93/eq2-eof-itemdex/internal/fit"
)

func main() {
	path := flag.String("readings", "data/curve-readings.csv", "curve readings csv")
	flag.Parse()

	rs, err := fit.LoadReadings(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load readings:", err)
		os.Exit(1)
	}

	subsets := []struct {
		name string
		rs   []fit.Reading
	}{
		{"joint", rs},
		{"haste", fit.Filter(rs, "haste")},
		{"dpsmod", fit.Filter(rs, "dpsmod")},
	}
	for _, s := range subsets {
		q, l := fit.FitQuad(s.rs), fit.FitLog(s.rs)
		fmt.Printf("%-7s (n=%2d)  quad a=%.6f b=%.8f rms=%.4f peak=%.1f f(300)=%.2f\n",
			s.name, len(s.rs), q.A, q.B, fit.RMS(q, s.rs), q.A/(2*q.B), q.Eval(300))
		fmt.Printf("                log  a=%.4f   b=%.3f      rms=%.4f f(300)=%.2f\n",
			l.A, l.B, fit.RMS(l, s.rs), l.Eval(300))
	}

	joint := fit.FitQuad(rs)
	jointLog := fit.FitLog(rs)
	winner := "quadratic"
	if fit.RMS(jointLog, rs) < fit.RMS(joint, rs) {
		winner = "logarithmic — model expects quadratic, investigate before recording"
	}
	fmt.Printf("\nwinner: %s\n", winner)
	fmt.Println("\n// paste into internal/model/curve.go:")
	fmt.Printf("HasteDpsModA = %.6f\n", joint.A)
	fmt.Printf("HasteDpsModB = %.8f\n", joint.B)
}
```

- [ ] **Step 2: Build it**

Run: `go build ./cmd/fitcurve`
Expected: clean build

- [ ] **Step 3: Run and verify against the pre-computed table**

Run: `go run ./cmd/fitcurve`
Expected output (numbers must match to printed precision):

```
joint   (n=20)  quad a=0.800348 b=0.00127275 rms=0.2868 peak=314.4 f(300)=125.56
                log  a=99.9937   b=106.347      rms=1.9302 f(300)=134.04
haste   (n=11)  quad a=0.801946 b=0.00127714 rms=0.2536 peak=314.0 f(300)=125.64
                log  a=92.4620   b=97.237      rms=1.4133 f(300)=130.13
dpsmod  (n= 9)  quad a=0.798757 b=0.00126736 rms=0.2923 peak=315.1 f(300)=125.56
                log  a=111.6895   b=122.243      rms=1.4836 f(300)=138.45

winner: quadratic

// paste into internal/model/curve.go:
HasteDpsModA = 0.800348
HasteDpsModB = 0.00127275
```

(Log-fit values may differ in the last digit from grid stepping — quad values must match exactly.)

- [ ] **Step 4: Commit**

```bash
git add cmd/fitcurve/main.go
git commit -m "Curve refit: fitcurve tool (bake-off + paste-ready constants)"
```

---

### Task 6: Model swap — fitted curve + caps to 300

**Files:**
- Modify: `internal/model/curve.go` (replace `hasteDpsModSamples` block and `HasteDpsModEffect`)
- Modify: `internal/constants/constants.go:11-12`
- Test: `internal/model/curve_test.go`, `internal/model/conversion_test.go`

- [ ] **Step 1: Update test expectations first**

In `internal/model/curve_test.go`, replace `TestHasteDpsModEffect` and **delete** `TestCurveBracketHasteDpsMod` entirely (the sample table it exercises is being removed; `TestCurveBracketMultiAttack` stays):

```go
func TestHasteDpsModEffect(t *testing.T) {
	require.Equal(t, 0.0, HasteDpsModEffect(0))
	require.Equal(t, 0.0, HasteDpsModEffect(-5))
	// Committed readings reproduced exactly:
	require.Equal(t, 18.0, HasteDpsModEffect(24))
	require.Equal(t, 21.0, HasteDpsModEffect(28.1))
	require.Equal(t, 35.0, HasteDpsModEffect(48.3))
	require.Equal(t, 48.0, HasteDpsModEffect(67.5))
	require.Equal(t, 124.0, HasteDpsModEffect(281)) // the reading that disproved (200→125)
	// Fitted values between/beyond readings:
	require.Equal(t, 67.0, HasteDpsModEffect(100))   // f=67.31 (was 66 on the old piecewise)
	require.Equal(t, 109.0, HasteDpsModEffect(200))  // mid-curve now — NOT 125
	require.Equal(t, 125.0, HasteDpsModEffect(300))  // hard cap: f(300)=125.56
	require.Equal(t, 125.0, HasteDpsModEffect(5000)) // overcap clamps to f(300)
}
```

In `internal/model/conversion_test.go`, replace the two curve-dependent tests:

```go
func TestEffDelayHasteCurve(t *testing.T) {
	w := Weapon{AvgDamage: 100, DelaySecs: 4.0}
	// haste 200 → f = 0.800348·200 − 0.00127275·200² = 109.16 → floor 109
	require.InDelta(t, 4.0/2.09, effDelay(StatBlock{Haste: 200}, w), 1e-4)
	// haste 300 = hard cap → f(300) = 125.56 → floor 125
	require.InDelta(t, 4.0/2.25, effDelay(StatBlock{Haste: 300}, w), 1e-4)
	// haste 400 → clamped to the 300 cap
	require.InDelta(t, 4.0/2.25, effDelay(StatBlock{Haste: 400}, w), 1e-4)
}

func TestDPSModFactorCurveCapped(t *testing.T) {
	require.InDelta(t, 1.0, dpsModFactor(StatBlock{}), 1e-9)
	require.InDelta(t, 1.67, dpsModFactor(StatBlock{DPSMod: 100}), 1e-9) // f=67.31 → 67
	require.InDelta(t, 2.09, dpsModFactor(StatBlock{DPSMod: 200}), 1e-9) // f=109.16 → 109
	require.InDelta(t, 2.25, dpsModFactor(StatBlock{DPSMod: 300}), 1e-9) // hard cap
	require.InDelta(t, 2.25, dpsModFactor(StatBlock{DPSMod: 500}), 1e-9) // overcap clamps
}
```

(Keep the existing test names if they differ — match whatever the current function names are at `internal/model/conversion_test.go:10-21`; the assertions above are the content that changes.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/model/ -run 'TestHasteDpsModEffect|TestEffDelay|TestDPSMod' -v`
Expected: FAIL — old curve returns 125 at 200, 66 at 100, etc.

- [ ] **Step 3: Implement**

In `internal/constants/constants.go`, replace lines 11-12:

```go
	HasteStatCap      = 300.0 // haste stat hard cap; fitted curve gives f(300) ≈ 125.56 → shows 125%; overcap wasted (no flurry)
	DPSModCap         = 300.0 // dps-mod stat hard cap — shares the haste curve and cap
```

In `internal/model/curve.go`: delete the `hasteDpsModSamples` var (lines 17-21) and the old `HasteDpsModEffect` (line 64), add the constants import, and add:

```go
import (
	"math"

	"github.com/amdrake93/eq2-eof-itemdex/internal/constants"
)

// Fitted haste/dps-mod curve (2026-06 refit): quadratic f(s) = A·s − B·s²,
// joint floor-aware fit over the 20 readings in data/curve-readings.csv
// (RMS 0.29 vs 1.93 for the logarithmic alternative; haste-only and dpsmod-only
// fits agree within flooring noise → shared curve confirmed). Peak at
// A/2B ≈ 314, just past the 300 hard cap; f(300) ≈ 125.56 → shows as 125%.
// Re-derive after appending readings: go run ./cmd/fitcurve
// (internal/fit's TestFittedConstantsMatchReadings enforces the loop).
const (
	HasteDpsModA = 0.800348
	HasteDpsModB = 0.00127275
)

// hasteDpsModUnfloored is the fitted curve before UI flooring, clamped at the
// 300-stat hard cap (constants.HasteStatCap == constants.DPSModCap — one curve).
func hasteDpsModUnfloored(stat float64) float64 {
	if stat <= 0 {
		return 0
	}
	s := math.Min(stat, constants.HasteStatCap)
	return HasteDpsModA*s - HasteDpsModB*s*s
}

// HasteDpsModEffect is the in-game effect %: the fitted curve, floored to a
// whole percent (UI behavior).
func HasteDpsModEffect(stat float64) float64 { return math.Floor(hasteDpsModUnfloored(stat)) }
```

Note: `weights.go` still references `hasteDpsModSamples` at this point — it won't compile until Task 7's Step 3. Run only the curve/conversion tests via the package build failing is expected here **only if you run the full package**; to keep this task's commit green, apply the minimal `weights.go` bridge in the same commit as part of this step — replace `curveStatMarginal`'s haste/dpsmod sample references (lines 88-105) with the Task 7 implementation below. If you prefer strictly separate commits, squash Tasks 6 and 7 into one commit instead; do NOT commit a non-compiling tree.

- [ ] **Step 4: Run the model package tests**

Run: `go test ./internal/model/ -v`
Expected: curve/conversion tests PASS; weights/itemdelta tests FAIL with old expectations (fixed next task — if you bridged `weights.go` already, expect exactly the failures listed in Task 7 Step 2)

- [ ] **Step 5: Commit** (only if the tree compiles — see Step 3 note)

```bash
git add internal/model/curve.go internal/model/curve_test.go internal/model/conversion_test.go internal/constants/constants.go
git commit -m "Curve refit: fitted quadratic replaces piecewise haste/dps-mod samples; caps to 300"
```

---

### Task 7: Marginal weights via integer-effect-crossing brackets

**Files:**
- Modify: `internal/model/weights.go:82-110` (`curveStatMarginal`), comment at `weights.go:33-36`
- Test: `internal/model/weights_test.go`, `internal/model/itemdelta_test.go:21-27`

- [ ] **Step 1: Update test expectations first**

In `internal/model/weights_test.go`, replace the haste/dpsmod marginal tests (keep the two multi-attack tests unchanged):

```go
func TestDPSModWeightZeroAtCap(t *testing.T) {
	w := Weapon{AvgDamage: 100, DelaySecs: 2.0}
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, w) }
	weights := DeriveWeights(StatBlock{DPSMod: 300}, dps)
	require.InDelta(t, 0.0, weights["dpsmod"], 1e-6)
}

func TestCurveStatMarginalHaste(t *testing.T) {
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, Weapon{AvgDamage: 160, DelaySecs: 4}) }
	// At haste 0 the bracket is the curve's (0,1) effect crossings: (0, 1.2520).
	// dps: 40·1.00 → 40·1.01; marginal = 0.4 / 1.2520 = 0.3195
	require.InDelta(t, 0.3195, curveStatMarginal(StatBlock{}, "haste", dps), 1e-3)
}

func TestCurveStatMarginalHasteAtCap(t *testing.T) {
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, Weapon{AvgDamage: 160, DelaySecs: 4}) }
	require.InDelta(t, 0.0, curveStatMarginal(StatBlock{Haste: 300}, "haste", dps), 1e-9)
}

func TestCurveStatMarginalDPSMod(t *testing.T) {
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, Weapon{AvgDamage: 160, DelaySecs: 4}) }
	require.InDelta(t, 0.3195, curveStatMarginal(StatBlock{}, "dpsmod", dps), 1e-3)
}

func TestCurveStatMarginalDPSModAtCap(t *testing.T) {
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, Weapon{AvgDamage: 160, DelaySecs: 4}) }
	require.InDelta(t, 0.0, curveStatMarginal(StatBlock{DPSMod: 300}, "dpsmod", dps), 1e-9)
}

func TestCurveStatMarginalDPSModRaidBaseline(t *testing.T) {
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, Weapon{AvgDamage: 160, DelaySecs: 4}) }
	// At dps-mod 114.2 (raid baseline): f=74.80 → bracket = the (74,75) effect
	// crossings (112.63, 114.59); dps: 40·1.74 → 40·1.75; 0.4/1.9561 = 0.2045.
	// Nonzero — under the old 200-cap model the raid baseline read 0 here.
	require.InDelta(t, 0.2045, curveStatMarginal(StatBlock{DPSMod: 114.2}, "dpsmod", dps), 1e-3)
}

func TestCurveStatMarginalJustBelowCap(t *testing.T) {
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, Weapon{AvgDamage: 160, DelaySecs: 4}) }
	m := curveStatMarginal(StatBlock{Haste: 290}, "haste", dps)
	require.Greater(t, m, 0.0) // 290 was "capped → 0" under the old model
	require.Less(t, m, 0.1)    // but the curve is nearly flat at its peak
}

func TestDeriveWeightsDPSModIntegration(t *testing.T) {
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, Weapon{AvgDamage: 160, DelaySecs: 4}) }
	require.InDelta(t, 0.3195, DeriveWeights(StatBlock{}, dps)["dpsmod"], 1e-3)
	require.InDelta(t, 0.3195, DeriveWeights(StatBlock{}, dps)["haste"], 1e-3)
	require.InDelta(t, 0.0, DeriveWeights(StatBlock{DPSMod: 300}, dps)["dpsmod"], 1e-6)
}
```

In `internal/model/itemdelta_test.go:21-27`, the capped-stat test moves to the new cap:

```go
func TestItemDeltaCappedStatZero(t *testing.T) {
	main := Weapon{AvgDamage: 160, DelaySecs: 4}
	off := Weapon{AvgDamage: 158, DelaySecs: 4.4}
	var arts []spell.CombatArt
	d := ItemDelta(StatBlock{Haste: 300}, main, off, arts, StatBlock{Haste: 50}, nil)
	require.InDelta(t, 0.0, d, 1e-9)
}
```

- [ ] **Step 2: Run tests to verify the expected failures**

Run: `go test ./internal/model/ -v`
Expected: the tests above FAIL (old bracket math / old caps); everything else PASSES

- [ ] **Step 3: Implement**

In `internal/model/weights.go`: add `"math"` to imports; update the `curveStats` comment (line 33-35) to:

```go
// curveStats convert through a non-linear curve; their marginal weight is a
// bracket slope rather than a +1 forward diff (which reads lumpy under the
// in-game flooring). Multi-attack brackets between its table samples; haste and
// dps-mod bracket between the fitted curve's integer-effect crossings.
```

Replace `curveStatMarginal` (lines 82-110) with:

```go
// statAtEffect inverts the unfloored fitted curve: the stat on the rising
// branch where f(stat) = e. Effects beyond f(cap) resolve to the cap.
func statAtEffect(e float64) float64 {
	disc := HasteDpsModA*HasteDpsModA - 4*HasteDpsModB*e
	if disc <= 0 {
		return constants.HasteStatCap
	}
	s := (HasteDpsModA - math.Sqrt(disc)) / (2 * HasteDpsModB)
	return math.Min(s, constants.HasteStatCap)
}

// curveStatMarginal is the per-point value of a curve stat as the DPS slope
// across an interval whose endpoints land exactly on whole-percent effects, so
// the in-game flooring contributes no noise. Multi-attack uses its sample
// table; haste/dps-mod use the fitted equation's integer crossings (available
// anywhere on the curve) and clamp to 0 at the shared 300 cap.
func curveStatMarginal(base StatBlock, stat string, dps func(StatBlock) float64) float64 {
	v := getStat(base, stat)

	var lo, hi float64
	switch stat {
	case "multiattack":
		lo, hi = curveBracket(multiAttackSamples, v)
	case "haste", "dpsmod":
		if v >= constants.HasteStatCap { // == constants.DPSModCap (shared curve)
			return 0
		}
		n := math.Floor(hasteDpsModUnfloored(v))
		const nudge = 1e-9 // keep floor() on the intended side of each crossing
		lo = statAtEffect(n + nudge)
		hi = statAtEffect(n + 1 + nudge)
	}

	if hi <= lo {
		return 0
	}
	return (dps(setStat(base, stat, hi)) - dps(setStat(base, stat, lo))) / (hi - lo)
}
```

- [ ] **Step 4: Run the full model package**

Run: `go test ./internal/model/ -v`
Expected: PASS (all tests)

- [ ] **Step 5: Commit**

```bash
git add internal/model/weights.go internal/model/weights_test.go internal/model/itemdelta_test.go
git commit -m "Curve refit: haste/dps-mod marginals bracket at fitted integer-effect crossings"
```

---

### Task 8: Sync test — recorded constants must match a re-fit of the CSV

**Files:**
- Create: `internal/fit/sync_test.go`

This test is the enforcement of the refresh loop: append readings to the CSV without re-running `cmd/fitcurve` and updating `curve.go`, and the suite fails.

- [ ] **Step 1: Write the test**

```go
package fit

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/stretchr/testify/require"
)

func TestFittedConstantsMatchReadings(t *testing.T) {
	rs, err := LoadReadings(readingsPath)
	require.NoError(t, err)

	q := FitQuad(rs)
	msg := "model curve constants are stale — run `go run ./cmd/fitcurve` and update internal/model/curve.go"
	require.InDelta(t, model.HasteDpsModA, q.A, 1e-6, msg)
	require.InDelta(t, model.HasteDpsModB, q.B, 1e-8, msg)
}
```

- [ ] **Step 2: Run it**

Run: `go test ./internal/fit/ -run TestFittedConstantsMatchReadings -v`
Expected: PASS (constants were recorded from this exact dataset; tolerances cover the 6/8-decimal rounding in `curve.go`)

- [ ] **Step 3: Commit**

```bash
git add internal/fit/sync_test.go
git commit -m "Curve refit: sync test pins model constants to the readings CSV"
```

---

### Task 9: Raid baseline 114.2, report line, full verification

**Files:**
- Modify: `internal/baseline/baseline.go:26-32`
- Modify: `internal/bis/render.go:121`

- [ ] **Step 1: Update the raid baseline**

Replace the `Raid` block (and its doc comment) in `internal/baseline/baseline.go:26-32`:

```go
// Raid: self + group package. Group DPS-mod measured live (2026-06): Coercer 74
// + Inquisitor 30.2 + Dirge 10 = 114.2 — mid-curve, well below the 300 cap (the
// old "buffs reach the cap → 200" assumption died with the 200 cap and the comp
// losing its Berserker). Crit elevated by AAs/buffs. Haste still low (no
// maintained group haste buff). Refine per component as readings firm up.
var Raid = model.StatBlock{
	MultiAttack: 34.2,  // Villainy IV
	DPSMod:      114.2, // coercer 74 + inquis 30.2 + dirge 10 (live estimate)
	CritChance:  31.0,  // ~31% buffed in an MT group (research; confirm)
}
```

- [ ] **Step 2: Update the report assumptions line**

Replace `internal/bis/render.go:121`:

```go
	fmt.Fprintf(b, "- haste & dps-mod: fitted quadratic %.6f·s − %.8f·s², hard cap %.0f stat → %.0f%%\n",
		model.HasteDpsModA, model.HasteDpsModB, constants.HasteStatCap, model.HasteDpsModEffect(constants.HasteStatCap))
```

Add `"github.com/amdrake93/eq2-eof-itemdex/internal/model"` to `render.go`'s imports if it isn't already there.

- [ ] **Step 3: Full suite + lint**

Run: `make test && make lint`
Expected: all packages PASS, lint clean

- [ ] **Step 4: Eyeball the new weights**

Run: `go run ./cmd/weights`
Expected: RAID baseline now shows a **nonzero dpsmod weight** (was 0.0000 at the old capped baseline); haste weight shifted; no stat reads negative except possibly reuse (known quantization artifact).

- [ ] **Step 5: Regenerate the BiS report**

Run: `go run ./cmd/bis`
Expected: `bis-report.md` rewritten; the Assumptions section shows the fitted quadratic and `hard cap 300 stat → 125%`. The report is a generated artifact (untracked) — do not commit it; surface the headline ranking changes to the user instead.

- [ ] **Step 6: Commit**

```bash
git add internal/baseline/baseline.go internal/bis/render.go
git commit -m "Curve refit: raid group DPS-mod 114.2 (live comp); report shows fitted curve"
```

---

## Self-review notes

- **Spec coverage:** CSV dataset ✓ (T1), floor-aware fit vs shown+0.5 ✓ (T2-3), quad/log bake-off ✓ (T4-5), haste/dpsmod/joint shared-curve check ✓ (T3 subset test, T5 output), constants recorded in curve.go with residual+dataset annotation ✓ (T6), floor + f(min(stat,300)) clamp ✓ (T6), unfloored-curve marginals ✓ (T7), caps 300 ✓ (T6), raid baseline 114.2 ✓ (T9), render line ✓ (T9), report regen ✓ (T9). Spec's "remaining data gaps" need no code — appending to the CSV + re-running fitcurve is the designed loop, enforced by T8.
- **Compile-boundary caveat:** Task 6 removes `hasteDpsModSamples`, which `weights.go` references — the Step 3 note requires bridging `weights.go` in the same commit (or squashing T6+T7). Executors must not commit a non-compiling tree.
- **Type consistency:** `fit.Reading{Stat,Raw,Effect,Era}`, `QuadParams{A,B}`/`LogParams{A,B}` with `Eval`, `model.HasteDpsModA/B` exported — names match across T2-T8.
