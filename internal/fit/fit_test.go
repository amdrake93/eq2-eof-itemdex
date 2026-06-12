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
	rs := syntheticQuad(0.8, 0.001)
	q := FitQuad(rs)
	require.InDelta(t, 0.8, q.A, 1e-9)
	require.InDelta(t, 0.001, q.B, 1e-9)
	require.InDelta(t, 0.0, RMS(q, rs), 1e-9)
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
	require.InDelta(t, h.A, d.A, 0.01, "haste/dpsmod fits diverge — shared curve in doubt")
	require.InDelta(t, h.B, d.B, 5e-5, "haste/dpsmod fits diverge — shared curve in doubt")
}

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
