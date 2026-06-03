package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/amdrake93/eq2-eof-itemdex/internal/source"
)

func main() {
	var (
		dir      = flag.String("out", "data", "directory for CSV catalog (also the cache)")
		refresh  = flag.Bool("refresh", false, "force a fresh Census pull (rewrites CSVs)")
		sid      = flag.String("sid", "s:example", "Census service ID")
		pageSize = flag.Int("page", 1000, "items per Census request")
	)
	flag.Parse()

	c := census.New(*sid)
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
