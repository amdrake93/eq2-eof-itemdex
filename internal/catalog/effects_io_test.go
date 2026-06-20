package catalog

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEffectStatsCSVRoundTrip(t *testing.T) {
	in := []EffectStat{{ItemID: 1, Stat: "attackspeed", Value: 25}, {ItemID: 2, Stat: "basemodifier", Value: -3}}
	var b bytes.Buffer
	require.NoError(t, WriteEffectStatsCSV(&b, in))
	out, err := ReadEffectStatsCSV(&b)
	require.NoError(t, err)
	require.Equal(t, in, out)
}

func TestProcsCSVRoundTrip(t *testing.T) {
	in := []ItemProc{
		{ItemID: 2, Trigger: "On a spell cast", PerMinute: 1.8, Raw: "On a spell cast | Decrease reuse 10%"},
		{ItemID: 3, Trigger: "On a successful melee attack", PerMinute: 3.0, DmgType: "heat", MinDmg: 1200, MaxDmg: 2000, Raw: "x"},
	}
	var b bytes.Buffer
	require.NoError(t, WriteProcsCSV(&b, in))
	out, err := ReadProcsCSV(&b)
	require.NoError(t, err)
	require.Equal(t, in, out)
}

func TestAuditCSVRoundTrip(t *testing.T) {
	in := map[int][]AuditLine{
		1: {{Description: "Increases Haste of caster by 25.0.", Kind: "stat", Detail: "attackspeed"}},
		2: {
			{Description: "On a spell cast this spell may cast Foo.", Kind: "proc", Detail: "On a spell cast"},
			{Description: "Increases Bogus of caster by 7.", Kind: "skip", Detail: "unknown stat: bogus"},
		},
	}
	var b bytes.Buffer
	require.NoError(t, WriteAuditCSV(&b, in))
	out, err := ReadAuditCSV(&b)
	require.NoError(t, err)
	require.Equal(t, in, out)
}

func TestWriteAuditReport(t *testing.T) {
	var b bytes.Buffer
	rep := map[int][]AuditLine{
		1: {{Description: "Increases Haste of caster by 25.0.", Kind: "stat", Detail: "attackspeed"}},
		2: {{Description: "Increases Bogus of caster by 7.", Kind: "skip", Detail: "unknown stat: bogus"}},
	}
	require.NoError(t, WriteAuditReport(&b, rep))
	s := b.String()
	require.Contains(t, s, "attackspeed")
	require.Contains(t, s, "unknown stat: bogus")
}
