package source

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/catalog"
	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/stretchr/testify/require"
)

func TestLoadFromCacheWhenPresent(t *testing.T) {
	dir := t.TempDir()
	items := []census.Item{
		{
			ID:          1,
			DisplayName: "Sword",
			Slots:       []census.Slot{{Name: "Primary"}},
			TypeInfo:    census.TypeInfo{Classes: map[string]census.ClassReq{}},
			Modifiers:   map[string]census.Modifier{},
		},
	}
	f, err := os.Create(filepath.Join(dir, "weapons.csv"))
	require.NoError(t, err)
	require.NoError(t, catalog.WriteCSV(f, items))
	require.NoError(t, f.Close())

	got, err := LoadCache(dir)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "Sword", got[0].DisplayName)
}

func TestCacheExists(t *testing.T) {
	dir := t.TempDir()
	if CacheExists(dir) {
		t.Fatal("empty dir should not be a cache")
	}
	f, err := os.Create(filepath.Join(dir, "weapons.csv"))
	require.NoError(t, err)
	require.NoError(t, f.Close())
	if !CacheExists(dir) {
		t.Fatal("dir with weapons.csv should be a cache")
	}
}
