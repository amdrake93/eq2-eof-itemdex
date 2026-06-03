package census

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const sampleItemList = `{"item_list":[{
  "id":4202408049,"displayname":"Soulfire Hammer","tier":"FABLED","itemlevel":70,
  "gamelink":"040000...","slot_list":[{"id":0,"name":"Primary"}],
  "typeinfo":{"name":"weapon","skilltype":"crushing","delay":4.0,"damagerating":75.9,
    "minbasedamage":76,"maxbasedamage":228,
    "classes":{"assassin":{"displayname":"Assassin","id":15,"level":70}}},
  "modifiers":{"strength":{"displayname":"str","type":"attribute","value":39}},
  "_extended":{"discovered":{"timestamp":1183075200,
    "world_list":[{"timestamp":1183075200,"id":108,"charid":1},{"timestamp":1686700800,"id":614,"charid":2}]}}
}],"returned":1}`

func TestDecodeItems(t *testing.T) {
	items, err := DecodeItems([]byte(sampleItemList))
	require.NoError(t, err)
	require.Len(t, items, 1)

	it := items[0]
	require.Equal(t, "Soulfire Hammer", it.DisplayName)
	require.Equal(t, 70, it.ItemLevel)
	require.Equal(t, "Primary", it.Slots[0].Name)
	require.Contains(t, it.TypeInfo.Classes, "assassin")
	require.Equal(t, float64(39), it.Modifiers["strength"].Value)

	var has614 bool
	for _, w := range it.Extended.Discovered.Worlds {
		if w.ID == 614 {
			has614 = true
		}
	}
	require.True(t, has614, "missing world 614 entry")
}

func TestDecodeItemsError(t *testing.T) {
	_, err := DecodeItems([]byte(`{"errorCode":"SERVER_ERROR"}`))
	require.Error(t, err, "expected error on SERVER_ERROR payload")
}
