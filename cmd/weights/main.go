package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/amdrake93/eq2-eof-itemdex/internal/baseline"
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	_ "modernc.org/sqlite"
)

func loadCAs(dbPath string) ([]spell.CombatArt, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			fmt.Fprintln(os.Stderr, "close db:", cerr)
		}
	}()
	rows, err := db.Query(`SELECT name, min_dmg, max_dmg, recast_secs FROM combat_arts`)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			fmt.Fprintln(os.Stderr, "close rows:", cerr)
		}
	}()
	var cas []spell.CombatArt
	for rows.Next() {
		var ca spell.CombatArt
		if err := rows.Scan(&ca.Name, &ca.MinDamage, &ca.MaxDamage, &ca.RecastSecs); err != nil {
			return nil, err
		}
		cas = append(cas, ca)
	}
	return cas, rows.Err()
}

func main() {
	dbPath := flag.String("db", "bis.db", "sqlite db from builddb")
	flag.Parse()

	cas, err := loadCAs(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load combat arts:", err)
		os.Exit(1)
	}
	cas = spell.HighestRanks(cas)

	// Reference weapon: a generic 1H (avg 100, delay 4.0) so weights are comparable.
	ref := model.Weapon{AvgDamage: 100, DelaySecs: 4.0}

	for _, b := range []struct {
		name string
		sb   model.StatBlock
	}{{"SOLO", baseline.Solo}, {"RAID", baseline.Raid}} {
		fmt.Printf("\n== %s baseline weights (marginal DPS per +1 stat; ref 1H weapon, %d combat arts) ==\n", b.name, len(cas))
		dps := func(sb model.StatBlock) float64 { return model.TotalDPS(sb, ref, cas) }
		ws := model.DeriveWeights(b.sb, dps)
		keys := make([]string, 0, len(ws))
		for k := range ws {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return ws[keys[i]] > ws[keys[j]] })
		for _, k := range keys {
			fmt.Printf("  %-12s %.4f\n", k, ws[k])
		}
	}
}
