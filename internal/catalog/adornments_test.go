package catalog

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdornmentCSVRoundTrip(t *testing.T) {
	rows := []Adornment{
		{ID: 111, Name: "Deadly Adornment", Stats: map[string]float64{"critchance": 2}},
		{ID: 222, Name: "Adornment of Haste", Stats: map[string]float64{"attackspeed": 3, "flurry": 1}},
	}
	var buf bytes.Buffer
	require.NoError(t, WriteAdornmentsCSV(&buf, rows))

	got, err := ReadAdornmentsCSV(&buf)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, int64(111), got[0].ID)
	require.Equal(t, "Deadly Adornment", got[0].Name)
	require.InDelta(t, 2, got[0].Stats["critchance"], 1e-9)
	require.InDelta(t, 3, got[1].Stats["attackspeed"], 1e-9)
}
