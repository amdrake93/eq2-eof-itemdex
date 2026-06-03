package classify

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/stretchr/testify/assert"
)

func itemWithVarsoon(unix float64) census.Item {
	return census.Item{Extended: census.Extended{Discovered: census.Discovered{
		Worlds: []census.WorldDiscovery{
			{ID: 104, Timestamp: 1},
			{ID: 614, Timestamp: unix},
		}}}}
}

func TestIsEoF(t *testing.T) {
	cases := []struct {
		name string
		unix float64
		want bool
	}{
		{"KoS Qeynos Claymore 2022-12-13", 1670889600, false},
		{"EoF White Oak Acorn 2023-04-11", 1681171200, true},
		{"EoF Soulfire 2023-06-14", 1686700800, true},
		{"RoK Tuft 2023-08-08 (ceiling, exclusive)", 1691452800, false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, IsEoF(itemWithVarsoon(c.unix)))
		})
	}
}

func TestIsEoFNoVarsoonEntry(t *testing.T) {
	it := census.Item{Extended: census.Extended{Discovered: census.Discovered{
		Worlds: []census.WorldDiscovery{{ID: 108, Timestamp: 1183075200}}}}}
	assert.False(t, IsEoF(it), "item never on Varsoon must not classify as EoF")
}
