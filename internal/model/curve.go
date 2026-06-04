package model

import "math"

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

// hasteDpsModSamples: the steeper curve shared by haste and dps-mod (live Varsoon
// readings), anchored (0,0), hard cap 200→125%.
var hasteDpsModSamples = []curvePoint{
	{0, 0}, {24, 18}, {28.1, 21}, {48.3, 35}, {67.5, 48}, {200, 125},
}

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
func HasteDpsModEffect(stat float64) float64 { return curveEffect(hasteDpsModSamples, stat) }
