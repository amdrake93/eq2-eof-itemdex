package catalog

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
)

func TestSplitByCategory(t *testing.T) {
	items := []census.Item{
		{ID: 1, Slots: []census.Slot{{Name: "Primary"}}},
		{ID: 2, Slots: []census.Slot{{Name: "Head"}}},
		{ID: 3, Slots: []census.Slot{{Name: "Ring"}}},
	}
	groups := SplitByCategory(items)
	if len(groups["weapons"]) != 1 || len(groups["armor"]) != 1 || len(groups["jewelry-charms"]) != 1 {
		t.Fatalf("bad split: %+v", groups)
	}
}

func TestWithMaxLife(t *testing.T) {
	items := []census.Item{
		{ID: 1, Modifiers: map[string]census.Modifier{"maxhpperc": {Value: 5}}},
		{ID: 2, Modifiers: map[string]census.Modifier{"strength": {Value: 5}}},
		{ID: 3, Modifiers: map[string]census.Modifier{"health": {Value: 100}}},
	}
	got := WithMaxLife(items)
	if len(got) != 2 {
		t.Fatalf("expected 2 max-life items, got %d", len(got))
	}
}
