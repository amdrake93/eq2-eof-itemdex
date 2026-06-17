package model

import (
	"math"

	"github.com/amdrake93/eq2-eof-itemdex/internal/constants"
)

type curvePoint struct{ stat, effect float64 }

// multiAttackSamples: MA stat → effect% (min(double,100)+triple), anchored (0,0).
// Gentle curve; no hard cap (runs to 3400→200%, the >100% part is triple-attack).
var multiAttackSamples = []curvePoint{
	{0, 0}, {10, 12}, {20, 22}, {30, 33}, {40, 43}, {50, 52}, {60, 61},
	{70, 69}, {80, 77}, {90, 84}, {100, 91}, {110, 97}, {120, 102},
	{130, 107}, {140, 111}, {150, 115}, {160, 118}, {170, 121}, {180, 123},
	{190, 124}, {200, 125}, {300, 135}, {500, 145}, {700, 155}, {900, 165},
	{1200, 175}, {3400, 200},
}

// Fitted haste/dps-mod curve (2026-06 refit): quadratic f(s) = A·s − B·s²,
// joint floor-aware fit over the 20 readings in data/curve-readings.csv
// (RMS 0.29 vs 1.93 for the logarithmic alternative; haste-only and dpsmod-only
// fits agree within flooring noise → shared curve confirmed). Peak at
// A/2B ≈ 314, just past the 300 hard cap; f(300) ≈ 125.56 → shows as 125%.
// Re-derive after appending readings: go run ./cmd/fitcurve, record the new
// constants here, and re-pin the dataset-snapshot tests in internal/fit and
// internal/model (internal/fit's TestFittedConstantsMatchReadings enforces the
// constants half of the loop; the snapshot pins fail loudly on their own).
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

// curveInterp: continuous piecewise-linear interpolation between samples, anchored
// at (0,0), clamped to the final sample's effect above the top stat.
func curveInterp(s []curvePoint, stat float64) float64 {
	if stat <= 0 {
		return 0
	}
	last := s[len(s)-1]
	if stat >= last.stat {
		return last.effect
	}
	for i := 1; i < len(s); i++ {
		hi := s[i]
		if stat <= hi.stat {
			lo := s[i-1]
			frac := (stat - lo.stat) / (hi.stat - lo.stat)
			return lo.effect + frac*(hi.effect-lo.effect)
		}
	}
	return last.effect
}

// curveEffect: in-game effect %, floored to a whole percent.
func curveEffect(s []curvePoint, stat float64) float64 {
	return math.Floor(curveInterp(s, stat))
}

// curveBracket: sample stat values bracketing v (lo ≤ v < hi); at/above top → (top, top).
func curveBracket(s []curvePoint, v float64) (lo, hi float64) {
	top := s[len(s)-1].stat
	if v >= top {
		return top, top
	}
	for i := 1; i < len(s); i++ {
		if v < s[i].stat {
			return s[i-1].stat, s[i].stat
		}
	}
	return top, top
}

func MultiAttackEffect(stat float64) float64 { return curveEffect(multiAttackSamples, stat) }

// mainStatSamples: AGI → CA-damage % (live readings, data/mainstat-readings.csv;
// the sync test in internal/fit pins this table to the CSV). UNFLOORED — AGI
// tooltips display two decimals ("Agility increases your damage by 64.06%").
// The curve flattens below ~600 (73→6.08 sits under the high-range trend), so
// no equation is assumed — interpolation only.
//
// Three regimes (the old "hard cap 1100" was WRONG — measured 2026-06-16):
//   - climb to 1100 → 65% (decelerating into the cap; slope ~0.008 near 1100),
//   - DEADZONE ~1100–1200 flat at 65% (1109 read 65%; {1200,65} marks the plateau
//     end — observed, no measured point inside the gap),
//   - SECOND REGIME >~1200: climbs again at ~0.027–0.031 %/AGI (1294→69.45 …
//     1661→79.54), ~3–4× the cap-approach slope.
//
// Clamps above the top sample (1661); raid AGI reaches ~1800, so a >1661 reading
// is still needed to anchor that range (else it under-reads there).
var mainStatSamples = []curvePoint{
	{0, 0}, {73, 6.08}, {156, 15.01}, {625, 51.74}, {664, 53.74}, {695, 55.22},
	{738, 57.10}, {780, 58.74}, {819, 60.10}, {859, 61.33}, {899, 62.39},
	{941, 63.32}, {983, 64.06}, {1100, 65}, {1200, 65},
	{1294, 69.45}, {1327, 70.47}, {1393, 72.43}, {1661, 79.54},
}

// MainStatEffect is the CA-damage % for a main-stat value: interpolated,
// unfloored, clamped at the top sample (1661 — see mainStatSamples regimes).
func MainStatEffect(stat float64) float64 { return curveInterp(mainStatSamples, stat) }
