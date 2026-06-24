package census

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
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

func TestFetchCharacterBuildsQuery(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"character_list":[{"displayname":"Biffels (Wuoshi)","type":{"class":"Assassin","level":70},"equipmentslot_list":[]}],"returned":1}`))
	}))
	defer srv.Close()

	c := New("s:example")
	c.BaseURL = srv.URL
	c.Limiter = rate.NewLimiter(rate.Inf, 1)

	ch, err := FetchCharacter(context.Background(), c, "Biffels", 618)
	require.NoError(t, err)
	require.Equal(t, "Biffels (Wuoshi)", ch.DisplayName)
	require.Contains(t, gotQuery, "name.first_lower=biffels")
	require.Contains(t, gotQuery, "locationdata.worldid=618")
	require.Contains(t, gotQuery, "c%3Ashow=")
}

func TestFetchItemsByIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Contains(t, r.URL.RawQuery, "id=101%2C102")
		_, _ = w.Write([]byte(`{"item_list":[{"id":101,"displayname":"A"},{"id":102,"displayname":"B"}]}`))
	}))
	defer srv.Close()

	c := New("s:example")
	c.BaseURL = srv.URL
	c.Limiter = rate.NewLimiter(rate.Inf, 1)

	items, err := FetchItemsByIDs(context.Background(), c, []int64{101, 102})
	require.NoError(t, err)
	require.Len(t, items, 2)
}

func TestFetchItemsByIDsEmpty(t *testing.T) {
	c := New("s:example")
	c.BaseURL = "http://invalid.invalid"
	c.Limiter = rate.NewLimiter(rate.Inf, 1)

	items, err := FetchItemsByIDs(context.Background(), c, nil)
	require.NoError(t, err)
	require.Nil(t, items)
}
