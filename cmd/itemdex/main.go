package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
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
		cached, err := source.LoadCache(*dir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error loading cache:", err)
			os.Exit(1)
		}

		const batchSize = 50
		var results []census.Item
		for i := 0; i < len(cached); i += batchSize {
			end := i + batchSize
			if end > len(cached) {
				end = len(cached)
			}
			batch := cached[i:end]

			ids := make([]string, len(batch))
			for j, it := range batch {
				ids[j] = strconv.FormatInt(it.ID, 10)
			}
			query := fmt.Sprintf("id=%s&c:show=id,effect_list&c:limit=%d",
				strings.Join(ids, ","), len(batch))

			body, err := c.Get(context.Background(), "get", "item", query)
			if err != nil {
				slog.Error("census fetch failed", "fetched_so_far", len(results), "err", err)
				os.Exit(1)
			}
			batch_items, err := census.DecodeItems(body)
			if err != nil {
				slog.Error("census decode failed", "fetched_so_far", len(results), "err", err)
				os.Exit(1)
			}
			results = append(results, batch_items...)
		}

		withEffects := 0
		for _, it := range results {
			if len(it.EffectList) > 0 {
				withEffects++
			}
		}
		slog.Info("effect backfill complete", "items_fetched", len(results), "items_with_effects", withEffects)

		if err := source.WriteEffectArtifacts(results, *dir); err != nil {
			fmt.Fprintln(os.Stderr, "error writing effect artifacts:", err)
			os.Exit(1)
		}
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
