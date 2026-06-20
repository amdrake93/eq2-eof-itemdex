package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/amdrake93/eq2-eof-itemdex/internal/source"
)

func main() {
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
