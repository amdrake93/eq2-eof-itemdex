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
	      VALUES (1,'Soulfire Gladius','Primary','MYTHICAL',70,'','slashing','One-Handed','assassin','',80,239,4.0,80)`)
	exec(`INSERT INTO items (id,name,slot,tier,itemlevel,armor_type,skill,wieldstyle,classes,gamelink,weapon_min_dmg,weapon_max_dmg,delay,damage_rating)
	      VALUES (3,'Soulfire Sabre','Primary','MYTHICAL',70,'','piercing','One-Handed','assassin','',80,239,4.0,79.7)`)
	exec(`INSERT INTO items (id,name,slot,tier,itemlevel,armor_type,skill,wieldstyle,classes,gamelink,weapon_min_dmg,weapon_max_dmg,delay,damage_rating)
	      VALUES (2,'Enchanted Grove Scimitar','Secondary','FABLED',70,'','piercing','One-Handed','assassin','',118,198,4.4,75)`)
	exec(`INSERT INTO combat_arts (name,level,min_dmg,max_dmg,recast_secs,cast_secs_hundredths)
	      VALUES ('Assassinate II',70,7000,12000,300,200)`) // 2.0s cast
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
	require.Len(t, lo.Arts, 1)
	require.Equal(t, "Assassinate II", lo.Arts[0].Name)
	require.Equal(t, 200, lo.Arts[0].CastSecsHundredths)
}
