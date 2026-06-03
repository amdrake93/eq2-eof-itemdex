package catalog

import (
	"bytes"
	"strings"
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
)

func TestWriteCSVUnionColumns(t *testing.T) {
	items := []census.Item{
		{ID: 1, DisplayName: "Sword", Tier: "FABLED", ItemLevel: 70, GameLink: "lnkA",
			Slots:     []census.Slot{{Name: "Primary"}},
			TypeInfo:  census.TypeInfo{Name: "weapon", SkillType: "slashing", MinBaseDamage: 10, MaxBaseDamage: 20, Delay: 3, DamageRating: 50, Classes: map[string]census.ClassReq{"assassin": {}}},
			Modifiers: map[string]census.Modifier{"strength": {Value: 40}},
		},
		{ID: 2, DisplayName: "Cap", Tier: "LEGENDARY", ItemLevel: 68, GameLink: "lnkB",
			Slots:     []census.Slot{{Name: "Head"}},
			TypeInfo:  census.TypeInfo{Name: "armor", SkillType: "heavyarmor", Classes: map[string]census.ClassReq{"guardian": {}}},
			Modifiers: map[string]census.Modifier{"stamina": {Value: 55}, "maxhpperc": {Value: 6}},
		},
	}

	var buf bytes.Buffer
	if err := WriteCSV(&buf, items); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}
	out := buf.String()
	header := strings.SplitN(out, "\n", 2)[0]
	for _, col := range []string{"name", "slot", "tier", "itemlevel", "armor_type", "classes", "gamelink", "id"} {
		if !strings.Contains(header, col) {
			t.Errorf("header missing %q: %s", col, header)
		}
	}
	for _, col := range []string{"strength", "stamina", "maxhpperc"} {
		if !strings.Contains(header, col) {
			t.Errorf("header missing stat %q: %s", col, header)
		}
	}
	if !strings.Contains(out, "Plate") {
		t.Errorf("expected Plate armor_type in output:\n%s", out)
	}
}

func TestRoundTrip(t *testing.T) {
	items := []census.Item{
		{ID: 1, DisplayName: "Sword", Tier: "FABLED", ItemLevel: 70,
			Slots:     []census.Slot{{Name: "Primary"}},
			TypeInfo:  census.TypeInfo{Name: "weapon", SkillType: "slashing", MinBaseDamage: 10, MaxBaseDamage: 20, Delay: 3, DamageRating: 50, Classes: map[string]census.ClassReq{"assassin": {}}},
			Modifiers: map[string]census.Modifier{"strength": {Value: 40}},
		},
	}
	var buf bytes.Buffer
	if err := WriteCSV(&buf, items); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}
	got, err := ReadCSV(&buf)
	if err != nil {
		t.Fatalf("ReadCSV: %v", err)
	}
	if len(got) != 1 || got[0].DisplayName != "Sword" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got[0].Modifiers["strength"].Value != 40 {
		t.Fatalf("lost modifier: %+v", got[0].Modifiers)
	}
	if _, ok := got[0].TypeInfo.Classes["assassin"]; !ok {
		t.Fatalf("lost class eligibility: %+v", got[0].TypeInfo.Classes)
	}
}

// TestRoundTripBlankFill exercises the union-column blank-fill path: two items
// with disjoint stats must round-trip so each item keeps only its own stat —
// a blank cell must produce an absent modifier, not a zero-valued entry.
func TestRoundTripBlankFill(t *testing.T) {
	items := []census.Item{
		{ID: 1, DisplayName: "Sword",
			TypeInfo:  census.TypeInfo{Classes: map[string]census.ClassReq{}},
			Modifiers: map[string]census.Modifier{"strength": {Value: 40}},
		},
		{ID: 2, DisplayName: "Cap",
			TypeInfo:  census.TypeInfo{Classes: map[string]census.ClassReq{}},
			Modifiers: map[string]census.Modifier{"stamina": {Value: 55}},
		},
	}
	var buf bytes.Buffer
	if err := WriteCSV(&buf, items); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}
	got, err := ReadCSV(&buf)
	if err != nil {
		t.Fatalf("ReadCSV: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 items, got %d", len(got))
	}
	if got[0].Modifiers["strength"].Value != 40 {
		t.Errorf("item 0 lost strength: %+v", got[0].Modifiers)
	}
	if _, ok := got[0].Modifiers["stamina"]; ok {
		t.Errorf("item 0 should not have stamina (blank cell): %+v", got[0].Modifiers)
	}
	if got[1].Modifiers["stamina"].Value != 55 {
		t.Errorf("item 1 lost stamina: %+v", got[1].Modifiers)
	}
	if _, ok := got[1].Modifiers["strength"]; ok {
		t.Errorf("item 1 should not have strength (blank cell): %+v", got[1].Modifiers)
	}
}
