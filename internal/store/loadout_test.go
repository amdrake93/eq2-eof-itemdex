package store

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func seedLoadout(t *testing.T, d *DB) {
	t.Helper()
	exec := func(q string, a ...any) {
		_, err := d.SQL().Exec(q, a...)
		require.NoError(t, err)
	}
	exec(`INSERT INTO items (id,name,slot,tier,itemlevel,armor_type,skill,wieldstyle,classes,gamelink,weapon_min_dmg,weapon_max_dmg,delay,damage_rating)
	      VALUES (1,'Soulfire Gladius','Primary','MYTHICAL',70,'','slashing','One-Handed','assassin','',120,200,4.0,80)`)
	exec(`INSERT INTO items (id,name,slot,tier,itemlevel,armor_type,skill,wieldstyle,classes,gamelink,weapon_min_dmg,weapon_max_dmg,delay,damage_rating)
	      VALUES (2,'Enchanted Grove Scimitar','Secondary','FABLED',70,'','piercing','One-Handed','assassin','',118,198,4.4,75)`)
	exec(`INSERT INTO combat_arts (name,level,min_dmg,max_dmg,recast_secs,cast_secs_hundredths)
	      VALUES ('Assassinate II',70,7000,12000,300,50)`)
	exec(`INSERT INTO combat_arts (name,level,min_dmg,max_dmg,recast_secs,cast_secs_hundredths)
	      VALUES ('Assassinate I',60,5000,9000,300,50)`)
}

func TestLoadLoadout(t *testing.T) {
	d, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { require.NoError(t, d.Close()) }()
	require.NoError(t, d.Init())
	seedLoadout(t, d)

	lo, err := d.LoadLoadout()
	require.NoError(t, err)
	require.Equal(t, "Soulfire Gladius", lo.MainName)
	require.InDelta(t, 160.0, lo.Main.AvgDamage, 1e-9)
	require.InDelta(t, 4.0, lo.Main.DelaySecs, 1e-9)
	require.Equal(t, "Enchanted Grove Scimitar", lo.OffName)
	require.InDelta(t, 158.0, lo.Off.AvgDamage, 1e-9)
	require.Len(t, lo.Arts, 1)
	require.Equal(t, "Assassinate II", lo.Arts[0].Name)
}
