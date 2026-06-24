package source

import (
	"os"
	"path/filepath"

	"github.com/amdrake93/eq2-eof-itemdex/internal/catalog"
	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
)

// AppendItems adds catalog items absent from the existing CSVs in dir, returning
// the count newly added. New items are categorized with the same SplitByCategory
// logic FreshPull uses (non-gear "other" items are dropped), and each affected
// category CSV is rewritten with its existing rows plus the new ones, deduped by
// id. The maxlife.csv cross-cut and effect artifacts are left to a later builddb
// pass — this only grows the per-category catalog so a fresh import can resolve.
func AppendItems(dir string, items []census.Item) (added int, err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}

	existing, err := LoadCache(dir)
	if err != nil {
		return 0, err
	}

	known := make(map[int64]bool, len(existing))
	for _, it := range existing {
		known[it.ID] = true
	}

	var fresh []census.Item
	for _, it := range items {
		if known[it.ID] {
			continue
		}
		known[it.ID] = true
		fresh = append(fresh, it)
	}
	if len(fresh) == 0 {
		return 0, nil
	}

	combined := append(append([]census.Item{}, existing...), fresh...)
	groups := catalog.SplitByCategory(combined)
	delete(groups, "other")

	newByCategory := catalog.SplitByCategory(fresh)
	delete(newByCategory, "other")
	added = 0
	for _, group := range newByCategory {
		added += len(group)
	}

	for cat := range newByCategory {
		if err := writeFile(filepath.Join(dir, cat+".csv"), groups[cat]); err != nil {
			return 0, err
		}
	}
	return added, nil
}
