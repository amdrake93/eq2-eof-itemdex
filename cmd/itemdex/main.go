package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/amdrake93/eq2-eof-itemdex/internal/bis"
	"github.com/amdrake93/eq2-eof-itemdex/internal/catalog"
	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/amdrake93/eq2-eof-itemdex/internal/charconfig"
	"github.com/amdrake93/eq2-eof-itemdex/internal/loadout"
	"github.com/amdrake93/eq2-eof-itemdex/internal/source"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "import" {
		runImport(os.Args[2:])
		return
	}

	var (
		dir      = flag.String("out", "data", "directory for CSV catalog (also the cache)")
		refresh  = flag.Bool("refresh", false, "force a fresh Census pull (rewrites CSVs)")
		sid      = flag.String("sid", "s:example", "Census service ID")
		pageSize = flag.Int("page", 1000, "items per Census request")
		effects  = flag.Bool("effects", false, "backfill effect_list from cache and write item-effects.csv, item-procs.csv, effect-audit.md")
	)
	flag.Parse()

	c := census.New(*sid)

	if *effects {
		runEffectsBackfill(c, *dir)
		return
	}

	fromCache := !*refresh && source.CacheExists(*dir)
	items, err := source.Load(context.Background(), c, *dir, *refresh, *pageSize)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	srcName := "Census"
	if fromCache {
		srcName = "cache"
	}
	fmt.Printf("loaded %d EoF items from %s -> %s/\n", len(items), srcName, *dir)
}

// runEffectsBackfill resumably fetches effect_list for every cataloged item across
// rate-limited sessions. It processes a stable, ascending-sorted ID list from a
// saved offset, seeds its accumulators from the existing CSVs, fetches in batches
// of 50, and after the loop (whether it completed or a quota error stopped it)
// rewrites all four effect artifacts plus the progress offset. It exits non-zero
// when more IDs remain (resume needed) and zero when the backfill is complete.
func runEffectsBackfill(c *census.Client, dir string) {
	cached, err := source.LoadCache(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error loading cache:", err)
		os.Exit(1)
	}

	sortedIDs := make([]int64, 0, len(cached))
	for _, it := range cached {
		sortedIDs = append(sortedIDs, it.ID)
	}
	sort.Slice(sortedIDs, func(i, j int) bool { return sortedIDs[i] < sortedIDs[j] })

	offset := source.ReadEffectProgress(dir)
	if offset >= len(sortedIDs) {
		slog.Info("effects backfill already complete", "processed", len(sortedIDs), "total", len(sortedIDs))
		os.Exit(0)
	}

	acc, err := source.SeedEffectAccumulator(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error seeding effect accumulator:", err)
		os.Exit(1)
	}

	const batchSize = 50
	processed := offset
	for processed < len(sortedIDs) {
		end := processed + batchSize
		if end > len(sortedIDs) {
			end = len(sortedIDs)
		}
		batch := sortedIDs[processed:end]

		ids := make([]string, len(batch))
		for j, id := range batch {
			ids[j] = strconv.FormatInt(id, 10)
		}
		query := fmt.Sprintf("id=%s&c:show=id,effect_list&c:limit=%d",
			strings.Join(ids, ","), len(batch))

		body, err := c.Get(context.Background(), "get", "item", query)
		if err != nil {
			slog.Warn("census fetch stopped (quota?); persisting progress and exiting for resume",
				"processed", processed, "total", len(sortedIDs), "err", err)
			break
		}
		batchItems, err := census.DecodeItems(body)
		if err != nil {
			slog.Warn("census decode stopped; persisting progress and exiting for resume",
				"processed", processed, "total", len(sortedIDs), "err", err)
			break
		}

		acc = source.MergeEffects(acc, batchItems)
		processed = end
	}

	if err := source.WriteEffectAccumulator(acc, dir); err != nil {
		fmt.Fprintln(os.Stderr, "error writing effect artifacts:", err)
		os.Exit(1)
	}
	if err := source.WriteEffectProgress(dir, processed); err != nil {
		fmt.Fprintln(os.Stderr, "error writing effect progress:", err)
		os.Exit(1)
	}

	slog.Info("processed items", "processed", processed, "total", len(sortedIDs))

	if processed < len(sortedIDs) {
		os.Exit(1)
	}
}


// runImport fetches the configured character's live equipped loadout from Census,
// backfills any items/adornments missing from the local catalog, and writes
// characters/<name>-loadout.toml. It is thin wiring over loadout.Resolve plus the
// source/catalog backfill helpers; all non-trivial logic lives in those packages.
func runImport(argv []string) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	character := fs.String("character", "characters/alex.toml", "character config TOML")
	dir := fs.String("out", "data", "catalog directory (cache)")
	sid := fs.String("sid", "s:example", "Census service ID")
	_ = fs.Parse(argv)

	cfg, err := charconfig.Load(*character)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error loading character config:", err)
		os.Exit(1)
	}
	if cfg.Character.CensusName == "" || cfg.Character.World == 0 {
		fmt.Fprintln(os.Stderr, "error: character config needs census_name and world for gear import")
		os.Exit(1)
	}

	ctx := context.Background()
	c := census.New(*sid)

	ch, err := census.FetchCharacter(ctx, c, cfg.Character.CensusName, cfg.Character.World)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error fetching character:", err)
		os.Exit(1)
	}

	cachedItems, err := source.LoadCache(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error loading catalog cache:", err)
		os.Exit(1)
	}

	if ef, err := os.Open(filepath.Join(*dir, "item-effects.csv")); err == nil {
		effectStats, err := catalog.ReadEffectStatsCSV(ef)
		ef.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error reading item-effects.csv:", err)
			os.Exit(1)
		}
		cachedItems = loadout.MergeEffectStats(cachedItems, effectStats)
	} else if !os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "error opening item-effects.csv:", err)
		os.Exit(1)
	}

	catIndex := make(map[int64]census.Item, len(cachedItems))
	for _, it := range cachedItems {
		catIndex[it.ID] = it
	}

	adornIndex, err := loadAdornmentIndex(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error loading adornments:", err)
		os.Exit(1)
	}

	catLookup := func(id int64) (census.Item, bool) {
		it, ok := catIndex[id]
		return it, ok
	}
	adornLookup := func(id int64) (map[string]float64, bool) {
		s, ok := adornIndex[id]
		return s, ok
	}

	_, missItems, missAdorns := loadout.Resolve(ch, catLookup, adornLookup, bis.OptimizableSlot)

	addedItems, addedAdorns := 0, 0

	if len(missItems) > 0 {
		fetched, err := census.FetchItemsByIDs(ctx, c, missItems)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error fetching missing items:", err)
			os.Exit(1)
		}
		for _, it := range fetched {
			catIndex[it.ID] = it
		}
		addedItems, err = source.AppendItems(*dir, fetched)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error appending items to catalog:", err)
			os.Exit(1)
		}
	}

	if len(missAdorns) > 0 {
		fetched, err := census.FetchItemsByIDs(ctx, c, missAdorns)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error fetching missing adornments:", err)
			os.Exit(1)
		}
		var newAdorns []catalog.Adornment
		for _, it := range fetched {
			stats := loadout.ItemStatGrants(it)
			adornIndex[it.ID] = stats
			newAdorns = append(newAdorns, catalog.Adornment{
				ID:    it.ID,
				Name:  string(it.DisplayName),
				Stats: stats,
			})
		}
		if err := mergeAdornmentsCSV(*dir, newAdorns, &addedAdorns); err != nil {
			fmt.Fprintln(os.Stderr, "error writing adornments:", err)
			os.Exit(1)
		}
	}

	f, missItems2, missAdorns2 := loadout.Resolve(ch, catLookup, adornLookup, bis.OptimizableSlot)
	f.MarkUnresolved("item", missItems2)
	f.MarkUnresolved("adornment", missAdorns2)

	outPath := filepath.Join("characters", strings.ToLower(cfg.Character.CensusName)+"-loadout.toml")
	if err := loadout.Write(outPath, f); err != nil {
		fmt.Fprintln(os.Stderr, "error writing loadout:", err)
		os.Exit(1)
	}

	unresolved := len(missItems2) + len(missAdorns2)
	fmt.Printf("wrote %s (%d slots; %d unresolved)\n", outPath, len(f.Slots), unresolved)
	if addedItems+addedAdorns > 0 {
		fmt.Printf("added %d items + %d adornments to %s/ — run builddb before bis\n", addedItems, addedAdorns, *dir)
	}
}

// loadAdornmentIndex reads <dir>/adornments.csv into an id -> stat-grant map,
// returning an empty (non-nil) index when the file is absent.
func loadAdornmentIndex(dir string) (map[int64]map[string]float64, error) {
	index := map[int64]map[string]float64{}
	path := filepath.Join(dir, "adornments.csv")
	fh, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return index, nil
		}
		return nil, err
	}
	defer fh.Close()

	rows, err := catalog.ReadAdornmentsCSV(fh)
	if err != nil {
		return nil, err
	}
	for _, a := range rows {
		index[a.ID] = a.Stats
	}
	return index, nil
}

// mergeAdornmentsCSV merges newAdorns into <dir>/adornments.csv by id, recording
// the count newly added.
func mergeAdornmentsCSV(dir string, newAdorns []catalog.Adornment, added *int) error {
	path := filepath.Join(dir, "adornments.csv")

	var existing []catalog.Adornment
	if fh, err := os.Open(path); err == nil {
		existing, err = catalog.ReadAdornmentsCSV(fh)
		closeErr := fh.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	merged, n := catalog.MergeAdornments(existing, newAdorns)
	*added = n

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	return catalog.WriteAdornmentsCSV(out, merged)
}
