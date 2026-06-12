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
