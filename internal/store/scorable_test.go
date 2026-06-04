package store

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadScorableItems(t *testing.T) {
	d, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { require.NoError(t, d.Close()) }()
	require.NoError(t, d.Init())
	exec := func(q string, a ...any) {
		_, err := d.SQL().Exec(q, a...)
		require.NoError(t, err)
	}
	exec(`INSERT INTO items (id,name,slot,tier,itemlevel,armor_type,skill,wieldstyle,classes,gamelink,weapon_min_dmg,weapon_max_dmg,delay,damage_rating)
	      VALUES (10,'Fabled Chest','Chest','FABLED',70,'Leather','','','assassin|ranger','link10',0,0,0,0)`)
	exec(`INSERT INTO item_stats (item_id,stat,value) VALUES (10,'basemodifier',35)`)
	exec(`INSERT INTO item_stats (item_id,stat,value) VALUES (10,'critchance',22)`)
	exec(`INSERT INTO items (id,name,slot,tier,itemlevel,armor_type,skill,wieldstyle,classes,gamelink,weapon_min_dmg,weapon_max_dmg,delay,damage_rating)
	      VALUES (11,'Fabled Dirk','Secondary','FABLED',70,'','piercing','One-Handed','assassin','link11',118,198,4.4,75)`)
	exec(`INSERT INTO item_stats (item_id,stat,value) VALUES (11,'flurry',5)`)
	exec(`INSERT INTO items (id,name,slot,tier,itemlevel,armor_type,skill,wieldstyle,classes,gamelink,weapon_min_dmg,weapon_max_dmg,delay,damage_rating)
	      VALUES (12,'Wizard Hat','Head','FABLED',70,'Cloth','','','wizard','link12',0,0,0,0)`)

	items, err := d.LoadScorableItems()
	require.NoError(t, err)
	require.Len(t, items, 2) // wizard item excluded

	byID := map[int]ScorableItem{}
	for _, it := range items {
		byID[it.ID] = it
	}
	chest := byID[10]
	require.Equal(t, "Chest", chest.Slot)
	require.Equal(t, "FABLED", chest.Tier)
	require.Equal(t, "link10", chest.GameLink)
	require.InDelta(t, 35.0, chest.Stats.Potency, 1e-9)
	require.InDelta(t, 22.0, chest.Stats.CritChance, 1e-9)
	require.False(t, chest.IsWeapon())

	dirk := byID[11]
	require.True(t, dirk.IsWeapon())
	require.InDelta(t, 158.0, dirk.WeaponAvg, 1e-9)
	require.InDelta(t, 4.4, dirk.WeaponDelay, 1e-9)
	require.InDelta(t, 5.0, dirk.Stats.Flurry, 1e-9)
}
