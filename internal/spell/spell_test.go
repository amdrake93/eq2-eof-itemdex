package spell

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const sampleSpells = `{"spell_list":[{
  "name":"Assassinate","name_lower":"assassinate","level":50,"tier_name":"Expert","type":"arts","beneficial":0,
  "cast_secs_hundredths":50,"recast_secs":300.0,
  "classes":{"assassin":{"displayname":"Assassin","id":40,"level":50}},
  "effect_list":[{"description":"Inflicts 3,011 - 5,018 melee damage on target"},{"description":"You must be sneaking to use this ability."}]
}],"returned":1}`

func TestDecodeSpells(t *testing.T) {
	sp, err := DecodeSpells([]byte(sampleSpells))
	require.NoError(t, err)
	require.Len(t, sp, 1)
	require.Equal(t, "Assassinate", sp[0].Name)
	require.Equal(t, "Expert", sp[0].TierName)
	require.Equal(t, "arts", sp[0].Type)
	require.Equal(t, 300.0, sp[0].RecastSecs)
	require.Len(t, sp[0].Effects, 2)
	require.Contains(t, sp[0].Classes, "assassin")
}

func TestDecodeSpellsError(t *testing.T) {
	_, err := DecodeSpells([]byte(`{"error":"Missing Service ID"}`))
	require.Error(t, err)
}

func TestDecodeSpellsParsesIndentationAndDuration(t *testing.T) {
	body := []byte(`{"spell_list":[{
	  "name":"Gushing Wound","level":2,"tier_name":"Expert","type":"arts","beneficial":0,
	  "recast_secs":30.0,"cast_secs_hundredths":50,
	  "duration":{"max_sec_tenths":240,"min_sec_tenths":240,"does_not_expire":0},
	  "classes":{"assassin":{"id":40,"level":2}},
	  "effect_list":[
	    {"description":"Applies Untreated Bleeding on termination.","indentation":0},
	    {"description":"Inflicts 6 - 10 piercing damage on target.","indentation":1},
	    {"description":"Inflicts 0 - 1 melee damage on target","indentation":0}
	  ]}],"returned":1}`)
	spells, err := DecodeSpells(body)
	require.NoError(t, err)
	require.Len(t, spells, 1)
	require.Equal(t, 240, spells[0].Duration.MaxSecTenths)
	require.Equal(t, 1, spells[0].Effects[1].Indentation)
	require.Equal(t, 0, spells[0].Effects[2].Indentation)
}
