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
	msg := "model curve constants are stale — run `go run ./cmd/fitcurve`, update internal/model/curve.go, and re-pin the dataset-snapshot tests in internal/fit (readings counts, exact fit params) and internal/model (floored effect values)"
	require.InDelta(t, model.HasteDpsModA, q.A, 1e-6, msg)
	require.InDelta(t, model.HasteDpsModB, q.B, 1e-8, msg)
}
