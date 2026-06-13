package fit

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/stretchr/testify/require"
)

// The main-stat sample table in internal/model must mirror the committed
// readings byte-for-byte: append a reading to the CSV without updating the
// table (or vice versa) and this fails.
func TestMainStatSamplesMatchReadings(t *testing.T) {
	rs, err := LoadReadings("../../data/mainstat-readings.csv")
	require.NoError(t, err)
	require.NotEmpty(t, rs)

	msg := "main-stat table is stale — sync internal/model/curve.go mainStatSamples with data/mainstat-readings.csv"
	for _, r := range rs {
		require.Equal(t, r.Stat, "agi", "unexpected stat in mainstat readings")
		require.InDelta(t, r.Effect, model.MainStatEffect(r.Raw), 1e-9, msg)
	}
}
