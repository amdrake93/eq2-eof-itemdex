package store

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/catalog"
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

func TestLoadItemEffectsCoexistsWithModifier(t *testing.T) {
	db, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	require.NoError(t, db.Init())

	require.NoError(t, db.LoadGear([]census.Item{{
		ID: 1, DisplayName: "Cloak", Tier: "LEGENDARY",
		Slots:     []census.Slot{{Name: "Cloak"}},
		TypeInfo:  census.TypeInfo{Classes: map[string]census.ClassReq{"assassin": {}}},
		Modifiers: map[string]census.Modifier{"attackspeed": {Value: 10}},
	}}))
	require.NoError(t, db.LoadItemEffects([]catalog.EffectStat{{ItemID: 1, Stat: "attackspeed", Value: 25}}))

	var n int
	require.NoError(t, db.SQL().QueryRow(`SELECT COUNT(*) FROM item_stats WHERE item_id=1 AND stat='attackspeed'`).Scan(&n))
	require.Equal(t, 2, n) // modifier + effect rows coexist

	items, err := db.LoadScorableItems()
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.InDelta(t, 10.0, items[0].Stats.Haste, 1e-9)        // modifier-source only
	require.InDelta(t, 25.0, items[0].Stats.HasteEffect, 1e-9)  // effect-source routed to HasteEffect
}

func TestLoadItemProcs(t *testing.T) {
	db, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	require.NoError(t, db.Init())

	require.NoError(t, db.LoadItemProcs([]catalog.ItemProc{
		{ItemID: 1, Trigger: "on melee hit", PerMinute: 3.5, DmgType: "poison", MinDmg: 100, MaxDmg: 200, Raw: "raw text"},
	}))

	var trigger, dmgType, raw string
	var perMinute, minDmg, maxDmg float64
	require.NoError(t, db.SQL().QueryRow(
		`SELECT trigger, per_minute, dmg_type, min_dmg, max_dmg, raw FROM item_procs WHERE item_id=1`).
		Scan(&trigger, &perMinute, &dmgType, &minDmg, &maxDmg, &raw))
	require.Equal(t, "on melee hit", trigger)
	require.InDelta(t, 3.5, perMinute, 1e-9)
	require.Equal(t, "poison", dmgType)
	require.InDelta(t, 100.0, minDmg, 1e-9)
	require.InDelta(t, 200.0, maxDmg, 1e-9)
	require.Equal(t, "raw text", raw)
}

func TestLoadScorableItemsRoutesEffectHaste(t *testing.T) {
	db, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	require.NoError(t, db.Init())

	_, err = db.SQL().Exec(`INSERT INTO items (id, name, slot, tier, wieldstyle, gamelink, classes, weapon_min_dmg, weapon_max_dmg, delay)
		VALUES (1, 'Test Cloak', 'Cloak', '', '', '', 'assassin', 0, 0, 0)`)
	require.NoError(t, err)
	_, err = db.SQL().Exec(`INSERT INTO item_stats (item_id, stat, value, source) VALUES
		(1, 'attackspeed', 7, 'modifier'),
		(1, 'attackspeed', 25, 'effect'),
		(1, 'critchance', 2, 'effect')`)
	require.NoError(t, err)

	items, err := db.LoadScorableItems()
	require.NoError(t, err)
	require.Len(t, items, 1)
	it := items[0]
	require.InDelta(t, 7, it.Stats.Haste, 1e-9)        // modifier-source haste only
	require.InDelta(t, 25, it.Stats.HasteEffect, 1e-9) // effect-source haste routed to HasteEffect
	require.InDelta(t, 2, it.Stats.CritChance, 1e-9)   // non-haste effect stat still folds normally
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
