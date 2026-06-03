package catalog

import (
	"encoding/csv"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
)

var fixedCols = []string{
	"id", "name", "slot", "tier", "itemlevel", "armor_type", "classes",
	"weapon_min_dmg", "weapon_max_dmg", "delay", "damage_rating", "gamelink",
}

func slotNames(it census.Item) string {
	names := make([]string, 0, len(it.Slots))
	for _, s := range it.Slots {
		names = append(names, s.Name)
	}
	return strings.Join(names, "|")
}

func classNames(it census.Item) string {
	names := make([]string, 0, len(it.TypeInfo.Classes))
	for k := range it.TypeInfo.Classes {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, "|")
}

func statKeyUnion(items []census.Item) []string {
	set := map[string]struct{}{}
	for _, it := range items {
		for k := range it.Modifiers {
			set[k] = struct{}{}
		}
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func floatStr(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// atof parses a float from our own CSV data; malformed cells default to 0.
func atof(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64) //nolint:errcheck
	return v
}

// WriteCSV emits items in wide format: fixed columns + the union of all stat keys.
func WriteCSV(w io.Writer, items []census.Item) error {
	statCols := statKeyUnion(items)
	cw := csv.NewWriter(w)
	if err := cw.Write(append(append([]string{}, fixedCols...), statCols...)); err != nil {
		return err
	}
	for _, it := range items {
		row := []string{
			strconv.FormatInt(it.ID, 10),
			it.DisplayName,
			slotNames(it),
			it.Tier,
			strconv.Itoa(it.ItemLevel),
			ArmorType(it.TypeInfo.SkillType),
			classNames(it),
			floatStr(it.TypeInfo.MinBaseDamage),
			floatStr(it.TypeInfo.MaxBaseDamage),
			floatStr(it.TypeInfo.Delay),
			floatStr(it.TypeInfo.DamageRating),
			it.GameLink,
		}
		for _, k := range statCols {
			if m, ok := it.Modifiers[k]; ok {
				row = append(row, floatStr(m.Value))
			} else {
				row = append(row, "")
			}
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// ReadCSV reconstructs items from a WriteCSV stream (cache load-back).
func ReadCSV(r io.Reader) ([]census.Item, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	rows, err := cr.ReadAll()
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	header := rows[0]
	idx := map[string]int{}
	for i, h := range header {
		idx[h] = i
	}
	statCols := header[len(fixedCols):]
	var items []census.Item
	for _, row := range rows[1:] {
		id, _ := strconv.ParseInt(row[idx["id"]], 10, 64) //nolint:errcheck
		lvl, _ := strconv.Atoi(row[idx["itemlevel"]])     //nolint:errcheck
		it := census.Item{
			ID:          id,
			DisplayName: row[idx["name"]],
			Tier:        row[idx["tier"]],
			ItemLevel:   lvl,
			GameLink:    row[idx["gamelink"]],
			TypeInfo: census.TypeInfo{
				MinBaseDamage: atof(row[idx["weapon_min_dmg"]]),
				MaxBaseDamage: atof(row[idx["weapon_max_dmg"]]),
				Delay:         atof(row[idx["delay"]]),
				DamageRating:  atof(row[idx["damage_rating"]]),
				Classes:       map[string]census.ClassReq{},
			},
			Modifiers: map[string]census.Modifier{},
		}
		for _, name := range strings.Split(row[idx["slot"]], "|") {
			if name != "" {
				it.Slots = append(it.Slots, census.Slot{Name: name})
			}
		}
		for _, c := range strings.Split(row[idx["classes"]], "|") {
			if c != "" {
				it.TypeInfo.Classes[c] = census.ClassReq{}
			}
		}
		for _, k := range statCols {
			if cell := row[idx[k]]; cell != "" {
				it.Modifiers[k] = census.Modifier{Value: atof(cell)}
			}
		}
		items = append(items, it)
	}
	return items, nil
}
