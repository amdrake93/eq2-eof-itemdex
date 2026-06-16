package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/amdrake93/eq2-eof-itemdex/internal/source"
	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
)

func main() {
	var (
		dataDir = flag.String("data", "data", "catalog CSV directory")
		dbPath  = flag.String("db", "bis.db", "output SQLite DB path")
		sid     = flag.String("sid", "s:example", "Census service ID")
	)
	flag.Parse()

	gear, err := source.LoadCache(*dataDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load gear:", err)
		os.Exit(1)
	}
	c := census.New(*sid)
	arts, err := spell.AssassinCombatArts(context.Background(), c)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pull combat arts:", err)
		os.Exit(1)
	}
	arts = append(arts, spell.ManualArts()...) // low-level-learned arts the census pull misses (§9)

	_ = os.Remove(*dbPath)
	db, err := store.Open(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open db:", err)
		os.Exit(1)
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			fmt.Fprintln(os.Stderr, "close db:", cerr)
		}
	}()
	if err := db.Init(); err != nil {
		fmt.Fprintln(os.Stderr, "init db:", err)
		os.Exit(1)
	}
	if err := db.LoadGear(gear); err != nil {
		fmt.Fprintln(os.Stderr, "load gear -> db:", err)
		os.Exit(1)
	}
	if err := db.LoadCombatArts(arts); err != nil {
		fmt.Fprintln(os.Stderr, "load CAs -> db:", err)
		os.Exit(1)
	}
	fmt.Printf("built %s: %d gear items, %d combat arts\n", *dbPath, len(gear), len(arts))
}
