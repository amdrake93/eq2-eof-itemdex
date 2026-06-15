package store

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
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

func TestCombatArtsRoundTripComponents(t *testing.T) {
	db, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	require.NoError(t, db.Init())

	art := spell.CombatArt{
		Name: "Gushing Wound", Level: 66, RecastSecs: 30, DurationSecs: 24,
		Components: []spell.Component{
			{Kind: spell.Termination, DamageType: "piercing", MinDamage: 6, MaxDamage: 10, TriggeredSpell: "Untreated Bleeding"},
			{Kind: spell.DirectHit, DamageType: "melee", MinDamage: 0, MaxDamage: 1},
			{Kind: spell.DoT, DamageType: "piercing", MinDamage: 1, MaxDamage: 2, IntervalSecs: 4, HasInstant: true},
		},
	}
	require.NoError(t, db.LoadCombatArts([]spell.CombatArt{art}))

	got, err := db.CombatArts()
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, 24.0, got[0].DurationSecs)
	require.Len(t, got[0].Components, 3)
	require.Equal(t, spell.Termination, got[0].Components[0].Kind)
	require.Equal(t, "Untreated Bleeding", got[0].Components[0].TriggeredSpell)
	require.Equal(t, spell.DoT, got[0].Components[2].Kind)
	require.True(t, got[0].Components[2].HasInstant)
	require.Equal(t, 4.0, got[0].Components[2].IntervalSecs)
}
