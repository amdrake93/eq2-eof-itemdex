package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/amdrake93/eq2-eof-itemdex/internal/catalog"
	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/amdrake93/eq2-eof-itemdex/internal/extract"
)

// categoryFiles are the per-category catalog CSVs (NOT maxlife.csv, which is a
// cross-cut subset and would duplicate items if loaded). Non-gear items (slot
// category "other") are intentionally excluded from the catalog and the model's
// input — see FreshPull.
var categoryFiles = []string{"weapons.csv", "armor.csv", "jewelry-charms.csv"}

// offsetFile stores the next Census page offset for incremental pulls.
const offsetFile = ".census_next_offset"

func readOffset(dir string) int {
	b, err := os.ReadFile(filepath.Join(dir, offsetFile))
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0
	}
	return n
}

func writeOffset(dir string, offset int) error {
	return os.WriteFile(filepath.Join(dir, offsetFile), []byte(fmt.Sprintf("%d\n", offset)), 0o644)
}

func clearOffset(dir string) {
	_ = os.Remove(filepath.Join(dir, offsetFile))
}

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
//
// If a previous --refresh was interrupted by the s:example quota, FreshPull
// resumes from the saved offset, merges the new items with the existing cache,
// and writes the combined result. Re-run --refresh until the offset file
// disappears (signalling a complete pull).
func FreshPull(ctx context.Context, c *census.Client, dir string, pageSize int) ([]census.Item, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	// Load any items already written from a previous partial pull.
	var prior []census.Item
	nextOffset := readOffset(dir)
	if nextOffset > 0 && CacheExists(dir) {
		var loadErr error
		prior, loadErr = LoadCache(dir)
		if loadErr != nil {
			return nil, fmt.Errorf("loading prior partial cache: %w", loadErr)
		}
		slog.Info("resuming incremental pull", "prior_items", len(prior), "next_offset", nextOffset)
	}

	newItems, err := extract.AllEoFFrom(ctx, c, pageSize, nextOffset)

	pulledAll := err == nil
	var partial *extract.PartialError
	if errors.As(err, &partial) {
		if len(partial.Items) == 0 {
			// Quota hit immediately — nothing new; don't clobber existing CSVs.
			slog.Warn("census quota hit immediately; no new items — retry --refresh when quota resets")
			return prior, nil
		}
		slog.Warn("census quota reached; writing partial results — re-run --refresh to collect more",
			"new_items", len(partial.Items), "next_offset", partial.NextOffset)
		if writeErr := writeOffset(dir, partial.NextOffset); writeErr != nil {
			slog.Warn("failed to write resume offset", "err", writeErr)
		}
		newItems = partial.Items
		err = nil
	}
	if err != nil {
		return nil, err
	}

	// Full pull complete (no quota interruption): remove the offset marker.
	if pulledAll && nextOffset > 0 {
		clearOffset(dir)
		slog.Info("incremental pull complete — full catalog assembled")
	}

	allItems := append(prior, newItems...)

	// Drop non-gear (slot category "other"): the catalog and the model only
	// care about equippable gear.
	groups := catalog.SplitByCategory(allItems)
	delete(groups, "other")

	var gear []census.Item
	for cat, group := range groups {
		if err := writeFile(filepath.Join(dir, cat+".csv"), group); err != nil {
			return nil, err
		}
		gear = append(gear, group...)
	}
	if err := writeFile(filepath.Join(dir, "maxlife.csv"), catalog.WithMaxLife(gear)); err != nil {
		return nil, err
	}
	if err := WriteEffectArtifacts(gear, dir); err != nil {
		return nil, err
	}
	return gear, nil
}

// WriteEffectArtifacts parses each item's effect_list and writes the three effect
// catalog files into dir: item-effects.csv (static stats, source "effect"),
// item-procs.csv (cataloged procs), and effect-audit.md (the human-review report).
func WriteEffectArtifacts(items []census.Item, dir string) error {
	var stats []catalog.EffectStat
	var procs []catalog.ItemProc
	audit := map[int][]catalog.AuditLine{}
	for _, it := range items {
		if len(it.EffectList) == 0 {
			continue
		}
		s, ps, a := catalog.ParseEffects(it.EffectList)
		for k, v := range s {
			stats = append(stats, catalog.EffectStat{ItemID: int(it.ID), Stat: k, Value: v})
		}
		for _, p := range ps {
			procs = append(procs, catalog.ItemProc{
				ItemID:    int(it.ID),
				Trigger:   p.Trigger,
				PerMinute: p.PerMinute,
				DmgType:   p.DmgType,
				MinDmg:    p.MinDmg,
				MaxDmg:    p.MaxDmg,
				Raw:       p.Raw,
			})
		}
		if len(a) > 0 {
			audit[int(it.ID)] = a
		}
	}

	sort.Slice(stats, func(i, j int) bool {
		if stats[i].ItemID != stats[j].ItemID {
			return stats[i].ItemID < stats[j].ItemID
		}
		return stats[i].Stat < stats[j].Stat
	})
	sort.Slice(procs, func(i, j int) bool {
		if procs[i].ItemID != procs[j].ItemID {
			return procs[i].ItemID < procs[j].ItemID
		}
		return procs[i].Trigger < procs[j].Trigger
	})

	if err := writeEffectFile(filepath.Join(dir, "item-effects.csv"), func(w io.Writer) error {
		return catalog.WriteEffectStatsCSV(w, stats)
	}); err != nil {
		return err
	}
	if err := writeEffectFile(filepath.Join(dir, "item-procs.csv"), func(w io.Writer) error {
		return catalog.WriteProcsCSV(w, procs)
	}); err != nil {
		return err
	}
	return writeEffectFile(filepath.Join(dir, "effect-audit.md"), func(w io.Writer) error {
		return catalog.WriteAuditReport(w, audit)
	})
}

func writeEffectFile(path string, fn func(io.Writer) error) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	return fn(f)
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
