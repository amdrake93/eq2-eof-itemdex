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
