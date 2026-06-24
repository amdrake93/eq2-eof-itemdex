package census

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeCharacterEquipment(t *testing.T) {
	body := []byte(`{"character_list":[{
		"displayname":"Biffels (Wuoshi)",
		"type":{"class":"Assassin","level":70},
		"last_update":1782258823.5,
		"equipmentslot_list":[
			{"name":"cloak","item":{"id":264598753,"adornment_list":[{"color":"white"},{"id":111,"color":"orange"}]}},
			{"name":"food","item":{"id":461060541,"adornment_list":[]}},
			{"name":"mount_armor","item":{"adornment_list":[]}}
		]
	}],"returned":1}`)

	ch, err := DecodeCharacter(body)
	require.NoError(t, err)
	require.Equal(t, "Biffels (Wuoshi)", ch.DisplayName)
	require.Equal(t, "Assassin", ch.Type.Class)
	require.Equal(t, 70, ch.Type.Level)
	require.InDelta(t, 1782258823.5, ch.LastUpdate, 1e-3)
	require.Len(t, ch.EquipmentSlots, 3)

	cloak := ch.EquipmentSlots[0]
	require.Equal(t, "cloak", cloak.Name)
	require.Equal(t, int64(264598753), cloak.Item.ID)
	require.Equal(t, []int64{111}, cloak.Item.FilledAdornmentIDs())
}

func TestDecodeCharacterErrorEnvelope(t *testing.T) {
	_, err := DecodeCharacter([]byte(`{"errorCode":"SERVER_ERROR"}`))
	require.Error(t, err)
}

func TestDecodeCharacterNotFound(t *testing.T) {
	_, err := DecodeCharacter([]byte(`{"character_list":[],"returned":0}`))
	require.ErrorIs(t, err, ErrCharacterNotFound)
}
