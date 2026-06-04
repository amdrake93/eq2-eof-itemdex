package store

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteScores(t *testing.T) {
	d, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { require.NoError(t, d.Close()) }()
	require.NoError(t, d.Init())

	rows := []ScoreRow{
		{ItemID: 1, Baseline: "solo", DPSScore: 100.5, Slot: "Chest"},
		{ItemID: 1, Baseline: "raid", DPSScore: 220.0, Slot: "Chest"},
		{ItemID: 2, Baseline: "solo", DPSScore: 80.0, Slot: "Head"},
	}
	require.NoError(t, d.WriteScores(rows))

	var n int
	require.NoError(t, d.SQL().QueryRow(`SELECT COUNT(*) FROM scores`).Scan(&n))
	require.Equal(t, 3, n)

	var score float64
	require.NoError(t, d.SQL().QueryRow(
		`SELECT dps_score FROM scores WHERE item_id=1 AND baseline='raid'`).Scan(&score))
	require.InDelta(t, 220.0, score, 1e-9)

	require.NoError(t, d.WriteScores(rows)) // idempotent (composite PK)
	require.NoError(t, d.SQL().QueryRow(`SELECT COUNT(*) FROM scores`).Scan(&n))
	require.Equal(t, 3, n)
}
