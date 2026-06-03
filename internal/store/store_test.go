package store

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/stretchr/testify/require"
)

func TestLoadGear(t *testing.T) {
	db, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	require.NoError(t, db.Init())

	items := []census.Item{
		{
			ID: 1, DisplayName: "Dirk", Tier: "FABLED", ItemLevel: 70,
			Slots:     []census.Slot{{Name: "Primary"}},
			TypeInfo:  census.TypeInfo{Skill: "piercing", WieldStyle: "One-Handed", Classes: map[string]census.ClassReq{"assassin": {}}},
			Modifiers: map[string]census.Modifier{"strength": {Value: 32}, "critchance": {Value: 1.2}},
		},
	}
	require.NoError(t, db.LoadGear(items))

	var name, skill string
	require.NoError(t, db.SQL().QueryRow(`SELECT name, skill FROM items WHERE id=1`).Scan(&name, &skill))
	require.Equal(t, "Dirk", name)
	require.Equal(t, "piercing", skill)

	var n int
	require.NoError(t, db.SQL().QueryRow(`SELECT COUNT(*) FROM item_stats WHERE item_id=1`).Scan(&n))
	require.Equal(t, 2, n)
}
