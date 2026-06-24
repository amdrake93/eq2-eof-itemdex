package source

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/stretchr/testify/require"
)

func TestAppendItems_AddsOnlyNew(t *testing.T) {
	dir := t.TempDir()

	existing := census.Item{
		ID:          1,
		DisplayName: "Sword of Testing",
		Slots:       []census.Slot{{Name: "primary"}},
		Modifiers:   map[string]census.Modifier{"strength": {Value: 10}},
	}
	require.NoError(t, writeFile(dir+"/weapons.csv", []census.Item{existing}))

	newItem := census.Item{
		ID:          2,
		DisplayName: "Ring of Testing",
		Slots:       []census.Slot{{Name: "ring"}},
		Modifiers:   map[string]census.Modifier{"agility": {Value: 5}},
	}

	added, err := AppendItems(dir, []census.Item{existing, newItem})
	require.NoError(t, err)
	require.Equal(t, 1, added)

	loaded, err := LoadCache(dir)
	require.NoError(t, err)

	byID := map[int64]census.Item{}
	for _, it := range loaded {
		byID[it.ID] = it
	}
	require.Contains(t, byID, int64(1))
	require.Contains(t, byID, int64(2))
	require.Equal(t, "Ring of Testing", string(byID[2].DisplayName))
}

func TestAppendItems_NoNew(t *testing.T) {
	dir := t.TempDir()
	it := census.Item{ID: 1, Slots: []census.Slot{{Name: "primary"}}}
	require.NoError(t, writeFile(dir+"/weapons.csv", []census.Item{it}))

	added, err := AppendItems(dir, []census.Item{it})
	require.NoError(t, err)
	require.Equal(t, 0, added)
}

func TestAppendItems_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	newItem := census.Item{ID: 7, Slots: []census.Slot{{Name: "ring"}}}

	added, err := AppendItems(dir, []census.Item{newItem})
	require.NoError(t, err)
	require.Equal(t, 1, added)

	loaded, err := LoadCache(dir)
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	require.Equal(t, int64(7), loaded[0].ID)
}
