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
	require.Equal(t, 18.0, HasteDpsModEffect(24))
	require.Equal(t, 21.0, HasteDpsModEffect(28.1))
	require.Equal(t, 35.0, HasteDpsModEffect(48.3))
	require.Equal(t, 48.0, HasteDpsModEffect(67.5))
	require.Equal(t, 125.0, HasteDpsModEffect(200))
	require.Equal(t, 125.0, HasteDpsModEffect(300))  // hard cap
	require.Equal(t, 125.0, HasteDpsModEffect(5000)) // hard cap
	require.Equal(t, 66.0, HasteDpsModEffect(100))   // interp 66.88, floor 66
	require.Equal(t, 36.0, HasteDpsModEffect(50))    // interp 36.15, floor 36
}

func TestCurveBracketMultiAttack(t *testing.T) {
	lo, hi := curveBracket(multiAttackSamples, 34.2)
	require.Equal(t, 30.0, lo)
	require.Equal(t, 40.0, hi)
}

func TestCurveBracketHasteDpsMod(t *testing.T) {
	lo, hi := curveBracket(hasteDpsModSamples, 0)
	require.Equal(t, 0.0, lo)
	require.Equal(t, 24.0, hi)

	lo, hi = curveBracket(hasteDpsModSamples, 24)
	require.Equal(t, 24.0, lo)
	require.Equal(t, 28.1, hi)

	lo, hi = curveBracket(hasteDpsModSamples, 50)
	require.Equal(t, 48.3, lo)
	require.Equal(t, 67.5, hi)

	lo, hi = curveBracket(hasteDpsModSamples, 200)
	require.Equal(t, 200.0, lo)
	require.Equal(t, 200.0, hi)

	lo, hi = curveBracket(hasteDpsModSamples, 300)
	require.Equal(t, 200.0, lo)
	require.Equal(t, 200.0, hi)
}
