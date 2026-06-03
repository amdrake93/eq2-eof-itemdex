package store

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	"github.com/stretchr/testify/require"
)

func TestLoadCombatArts(t *testing.T) {
	db, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	require.NoError(t, db.Init())

	arts := []spell.CombatArt{
		{Name: "Assassinate", Level: 50, MinDamage: 3011, MaxDamage: 5018, RecastSecs: 300, CastSecsHundredths: 50},
	}
	require.NoError(t, db.LoadCombatArts(arts))

	var max float64
	require.NoError(t, db.SQL().QueryRow(`SELECT max_dmg FROM combat_arts WHERE name='Assassinate'`).Scan(&max))
	require.Equal(t, 5018.0, max)
}
