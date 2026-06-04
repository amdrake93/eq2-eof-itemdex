package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCombatModEffect(t *testing.T) {
	require.Equal(t, 0.0, CombatModEffect(0))
	require.Equal(t, 6.0, CombatModEffect(5)) // interp (0,0)-(10,12) = 6
	require.Equal(t, 12.0, CombatModEffect(10))
	require.Equal(t, 37.0, CombatModEffect(34.2)) // interp 33+0.42*10 = 37.2, floor 37
	require.Equal(t, 43.0, CombatModEffect(40))
	require.Equal(t, 91.0, CombatModEffect(100))
	require.Equal(t, 97.0, CombatModEffect(110))
	require.Equal(t, 102.0, CombatModEffect(120))
	require.Equal(t, 125.0, CombatModEffect(200))
	require.Equal(t, 200.0, CombatModEffect(3400))
	require.Equal(t, 200.0, CombatModEffect(5000))
}

func TestCombatModBracket(t *testing.T) {
	lo, hi := combatModBracket(34.2)
	require.Equal(t, 30.0, lo)
	require.Equal(t, 40.0, hi)

	lo, hi = combatModBracket(0)
	require.Equal(t, 0.0, lo)
	require.Equal(t, 10.0, hi)

	lo, hi = combatModBracket(5)
	require.Equal(t, 0.0, lo)
	require.Equal(t, 10.0, hi)

	lo, hi = combatModBracket(200)
	require.Equal(t, 200.0, lo)
	require.Equal(t, 300.0, hi)

	lo, hi = combatModBracket(3400)
	require.Equal(t, 3400.0, lo)
	require.Equal(t, 3400.0, hi)

	lo, hi = combatModBracket(5000)
	require.Equal(t, 3400.0, lo)
	require.Equal(t, 3400.0, hi)
}
