package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMultiAttackEffect(t *testing.T) {
	require.Equal(t, 0.0, MultiAttackEffect(0))
	require.Equal(t, 6.0, MultiAttackEffect(5))     // interp (0,0)-(10,12) = 6
	require.Equal(t, 37.0, MultiAttackEffect(34.2)) // interp 33+0.42*10 = 37.2, floor 37
	require.Equal(t, 43.0, MultiAttackEffect(40))
	require.Equal(t, 91.0, MultiAttackEffect(100))
	require.Equal(t, 102.0, MultiAttackEffect(120))
	require.Equal(t, 125.0, MultiAttackEffect(200))
	require.Equal(t, 200.0, MultiAttackEffect(3400))
	require.Equal(t, 200.0, MultiAttackEffect(5000))
}

func TestHasteDpsModEffect(t *testing.T) {
	require.Equal(t, 0.0, HasteDpsModEffect(0))
	require.Equal(t, 0.0, HasteDpsModEffect(-5))
	// Committed readings reproduced exactly:
	require.Equal(t, 18.0, HasteDpsModEffect(24))
	require.Equal(t, 21.0, HasteDpsModEffect(28.1))
	require.Equal(t, 35.0, HasteDpsModEffect(48.3))
	require.Equal(t, 48.0, HasteDpsModEffect(67.5))
	require.Equal(t, 124.0, HasteDpsModEffect(281)) // the reading that disproved (200→125)
	// Fitted values between/beyond readings:
	require.Equal(t, 67.0, HasteDpsModEffect(100))   // f=67.31 (was 66 on the old piecewise)
	require.Equal(t, 109.0, HasteDpsModEffect(200))  // mid-curve now — NOT 125
	require.Equal(t, 125.0, HasteDpsModEffect(300))  // hard cap: f(300)=125.56
	require.Equal(t, 125.0, HasteDpsModEffect(5000)) // overcap clamps to f(300)
}

func TestCurveBracketMultiAttack(t *testing.T) {
	lo, hi := curveBracket(multiAttackSamples, 34.2)
	require.Equal(t, 30.0, lo)
	require.Equal(t, 40.0, hi)
}

func TestMainStatEffect(t *testing.T) {
	require.Equal(t, 0.0, MainStatEffect(0))
	// Committed readings reproduced exactly (unfloored — AGI tooltips show 2dp):
	require.InDelta(t, 6.08, MainStatEffect(73), 1e-9)
	require.InDelta(t, 15.01, MainStatEffect(156), 1e-9)
	require.InDelta(t, 51.74, MainStatEffect(625), 1e-9)
	require.InDelta(t, 64.06, MainStatEffect(983), 1e-9)
	// Interpolated between samples: (738,57.10)-(780,58.74) midpoint
	require.InDelta(t, 57.92, MainStatEffect(759), 1e-9)
	// Hard cap 1100 → 65, overcap clamps:
	require.InDelta(t, 65.0, MainStatEffect(1100), 1e-9)
	require.InDelta(t, 65.0, MainStatEffect(5000), 1e-9)
}
