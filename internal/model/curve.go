package model

import "math"

// combatModSamples is the shared haste/multi-attack stat→effect% curve, anchored
// at (0,0). Effect % = min(double,100) + triple (e.g. 120 → 102).
var combatModSamples = []struct{ stat, effect float64 }{
	{0, 0}, {10, 12}, {20, 22}, {30, 33}, {40, 43}, {50, 52}, {60, 61},
	{70, 69}, {80, 77}, {90, 84}, {100, 91}, {110, 97}, {120, 102},
	{130, 107}, {140, 111}, {150, 115}, {160, 118}, {170, 121}, {180, 123},
	{190, 124}, {200, 125}, {300, 135}, {500, 145}, {700, 155}, {900, 165},
	{1200, 175}, {3400, 200},
}

// combatModInterp is the continuous (un-floored) effect: piecewise-linear between
// samples, anchored at (0,0), clamped to the final sample above the top.
func combatModInterp(stat float64) float64 {
	if stat <= 0 {
		return 0
	}
	last := combatModSamples[len(combatModSamples)-1]
	if stat >= last.stat {
		return last.effect
	}
	for i := 1; i < len(combatModSamples); i++ {
		hi := combatModSamples[i]
		if stat <= hi.stat {
			lo := combatModSamples[i-1]
			frac := (stat - lo.stat) / (hi.stat - lo.stat)
			return lo.effect + frac*(hi.effect-lo.effect)
		}
	}
	return last.effect
}

// CombatModEffect is the in-game effect %: the continuous curve floored to a
// whole percent (the game evaluates a formula and floors per integer %).
func CombatModEffect(stat float64) float64 {
	return math.Floor(combatModInterp(stat))
}

// combatModBracket returns the sample stat values bracketing v (lo ≤ v < hi).
// Below the first positive sample → (0, 10). At/above the top sample → (top, top).
// Used for the sample-to-sample weight slope; at sample points the floored effect
// equals the table value, so the slope has no flooring noise.
func combatModBracket(v float64) (lo, hi float64) {
	top := combatModSamples[len(combatModSamples)-1].stat
	if v >= top {
		return top, top
	}
	for i := 1; i < len(combatModSamples); i++ {
		if v < combatModSamples[i].stat {
			return combatModSamples[i-1].stat, combatModSamples[i].stat
		}
	}
	return top, top
}
