package spell

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

func TestAssassinCombatArts(t *testing.T) {
	page := `{"spell_list":[
	  {"name":"Assassinate","level":50,"tier_name":"Expert","type":"arts","beneficial":0,"recast_secs":300.0,"cast_secs_hundredths":50,
	   "classes":{"assassin":{"id":40,"level":50}},
	   "effect_list":[{"description":"Inflicts 3,011 - 5,018 melee damage on target"}]},
	  {"name":"Honed Reflexes","level":40,"tier_name":"Expert","type":"arts","beneficial":1,"recast_secs":0.0,"cast_secs_hundredths":0,
	   "classes":{"assassin":{"id":40,"level":40}},
	   "effect_list":[{"description":"Increases Haste of caster by 30.6."}]}
	],"returned":2}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, page)
	}))
	defer srv.Close()

	c := census.New("s:example")
	c.BaseURL = srv.URL
	c.Limiter = rate.NewLimiter(rate.Inf, 1)

	cas, err := AssassinCombatArts(context.Background(), c)
	require.NoError(t, err)
	require.Len(t, cas, 1) // only the damaging one
	require.Equal(t, "Assassinate", cas[0].Name)
	require.Equal(t, 5018.0, cas[0].MaxDamage)
	require.Equal(t, 300.0, cas[0].RecastSecs)
}
