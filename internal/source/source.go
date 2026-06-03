package source

import (
	"context"
	"os"
	"path/filepath"

	"github.com/amdrake93/eq2-eof-itemdex/internal/catalog"
	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/amdrake93/eq2-eof-itemdex/internal/extract"
)

// categoryFiles are the per-category catalog CSVs (NOT maxlife.csv, which is a
// cross-cut subset and would duplicate items if loaded).
var categoryFiles = []string{"weapons.csv", "armor.csv", "jewelry-charms.csv", "other.csv"}

// CacheExists reports whether at least one category CSV is present in dir.
func CacheExists(dir string) bool {
	for _, name := range categoryFiles {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

// LoadCache reconstructs the full item set by reading every category CSV in dir.
func LoadCache(dir string) ([]census.Item, error) {
	var all []census.Item
	for _, name := range categoryFiles {
		path := filepath.Join(dir, name)
		f, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		items, err := catalog.ReadCSV(f)
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

// FreshPull queries Census for the full EoF item set and writes the category +
// max-life CSVs into dir, returning the items.
func FreshPull(ctx context.Context, c *census.Client, dir string, pageSize int) ([]census.Item, error) {
	items, err := extract.AllEoF(ctx, c, pageSize)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	for cat, group := range catalog.SplitByCategory(items) {
		if err := writeFile(filepath.Join(dir, cat+".csv"), group); err != nil {
			return nil, err
		}
	}
	if err := writeFile(filepath.Join(dir, "maxlife.csv"), catalog.WithMaxLife(items)); err != nil {
		return nil, err
	}
	return items, nil
}

func writeFile(path string, items []census.Item) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	return catalog.WriteCSV(f, items)
}

// Load returns items from the CSV cache when present and refresh is false;
// otherwise it does a fresh Census pull (which rewrites the cache).
func Load(ctx context.Context, c *census.Client, dir string, refresh bool, pageSize int) ([]census.Item, error) {
	if !refresh && CacheExists(dir) {
		return LoadCache(dir)
	}
	return FreshPull(ctx, c, dir, pageSize)
}
