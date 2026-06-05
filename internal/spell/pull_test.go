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
	  {"name":"Assassinate","level":70,"tier_name":"Expert","type":"arts","beneficial":0,"recast_secs":300.0,"cast_secs_hundredths":50,
	   "classes":{"assassin":{"id":40,"level":70}},
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

func sp(name string, beneficial int, recast float64, cast int, effects ...string) Spell {
	effs := make([]Effect, len(effects))
	for i, e := range effects {
		effs[i] = Effect{Description: e}
	}
	return Spell{Name: name, Beneficial: beneficial, RecastSecs: recast, CastSecsHundredths: cast, Effects: effs}
}

func withLevel(s Spell, lvl int) Spell { s.Level = lvl; return s }

func TestFilterCombatArts(t *testing.T) {
	spells := []Spell{
		withLevel(sp("Eviscerate V", 0, 60, 50, "Inflicts 1133 - 1889 melee damage on target", "Must be flanking or behind"), 66),
		withLevel(sp("Whirling Blades IV", 1, 0, 50, "Increases Slashing of caster by 40.9", "Inflicts 252 - 421 melee damage on target"), 59),
		withLevel(sp("Spine Shot IV", 0, 60, 150, "Inflicts 830 - 1383 ranged damage on target", "If weapon equipped in Ranged"), 57),
		withLevel(sp("Caltrops", 0, 20, 50, "Decreases Speed of target"), 7),
		withLevel(sp("Assassinate II", 0, 300, 50, "Inflicts 7754 - 12924 melee damage on target", "You must be sneaking to use this ability."), 70),
		withLevel(sp("Pierce", 0, 10, 50, "Inflicts 47 - 79 melee damage on target"), 15),
	}

	arts := FilterCombatArts(spells)

	names := map[string]bool{}
	for _, a := range arts {
		names[a.Name] = true
	}
	require.True(t, names["Eviscerate V"], "L66 melee damaging art kept")
	require.True(t, names["Assassinate II"], "L70 sneaking art kept")
	require.False(t, names["Whirling Blades IV"], "beneficial buff dropped")
	require.False(t, names["Spine Shot IV"], "ranged art dropped")
	require.False(t, names["Caltrops"], "non-damaging art dropped")
	require.False(t, names["Pierce"], "low-level (<57) art dropped by the level floor")
	require.Len(t, arts, 2)
}
