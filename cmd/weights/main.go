package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/amdrake93/eq2-eof-itemdex/internal/charconfig"
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	_ "modernc.org/sqlite"
)

// TODO: consolidate with store.LoadLoadout — same query, different load path.
func loadCAs(db *sql.DB) ([]spell.CombatArt, error) {
	rows, err := db.Query(`SELECT name, min_dmg, max_dmg, recast_secs, cast_secs_hundredths FROM combat_arts`)
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
		if err := rows.Scan(&ca.Name, &ca.MinDamage, &ca.MaxDamage, &ca.RecastSecs, &ca.CastSecsHundredths); err != nil {
			return nil, err
		}
		cas = append(cas, ca)
	}
	return cas, rows.Err()
}

func loadWeapon(db *sql.DB, query string, args ...any) (model.Weapon, string, error) {
	var name string
	var mn, mx, delay float64
	if err := db.QueryRow(query, args...).Scan(&name, &mn, &mx, &delay); err != nil {
		return model.Weapon{}, "", err
	}
	return model.Weapon{AvgDamage: (mn + mx) / 2, DelaySecs: delay}, name, nil
}

func main() {
	dbPath := flag.String("db", "bis.db", "sqlite db from builddb")
	character := flag.String("character", "characters/alex.toml", "character config (TOML)")
	flag.Parse()

	cfg, err := charconfig.Load(*character)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load character:", err)
		os.Exit(1)
	}

	classData, err := charconfig.LoadClass("classes", cfg.Character.Class)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load class data:", err)
		os.Exit(1)
	}

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open db:", err)
		os.Exit(1)
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			fmt.Fprintln(os.Stderr, "close db:", cerr)
		}
	}()

	cas, err := loadCAs(db)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load combat arts:", err)
		os.Exit(1)
	}
	cas = spell.HighestRanks(cas)
	cas, err = charconfig.ApplyArtMods(cas, cfg.ArtMods)
	if err != nil {
		fmt.Fprintln(os.Stderr, "apply art mods:", err)
		os.Exit(1)
	}

	mainWeapon, mainName, err := loadWeapon(db,
		`SELECT name, weapon_min_dmg, weapon_max_dmg, delay FROM items
		 WHERE name LIKE 'Soulfire%' AND wieldstyle='One-Handed' AND classes LIKE '%assassin%'
		 ORDER BY weapon_max_dmg DESC LIMIT 1`)
	if err != nil {
		// Fallback: relax filters if no exact Soulfire 1H assassin row.
		mainWeapon, mainName, err = loadWeapon(db,
			`SELECT name, weapon_min_dmg, weapon_max_dmg, delay FROM items
			 WHERE name LIKE 'Soulfire%' AND classes LIKE '%assassin%'
			 ORDER BY weapon_max_dmg DESC LIMIT 1`)
		if err != nil {
			fmt.Fprintln(os.Stderr, "load main weapon:", err)
			os.Exit(1)
		}
	}

	offWeapon, offName, err := loadWeapon(db,
		`SELECT name, weapon_min_dmg, weapon_max_dmg, delay FROM items
		 WHERE tier='FABLED' AND wieldstyle='One-Handed' AND classes LIKE '%assassin%'
		   AND skill IN ('piercing','slashing') AND delay BETWEEN 3.5 AND 4.5
		 ORDER BY weapon_max_dmg DESC LIMIT 1`)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load off weapon:", err)
		os.Exit(1)
	}

	fmt.Printf("main-hand: %s (avg %.0f / %.1fs)   off-hand: %s (avg %.0f / %.1fs)\n",
		mainName, mainWeapon.AvgDamage, mainWeapon.DelaySecs, offName, offWeapon.AvgDamage, offWeapon.DelaySecs)

	names := make([]string, 0, len(cfg.Contexts))
	for n := range cfg.Contexts {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		block, err := cfg.ContextBlock(name)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("\n== %s context weights (marginal DPS per +1 stat; dual-wield, %d combat arts) ==\n", strings.ToUpper(name), len(cas))
		dps := func(sb model.StatBlock) float64 {
			return model.TotalDPSDual(sb, mainWeapon, offWeapon, cas, classData.AutoAttackMultiplier)
		}
		ws := model.DeriveWeights(block, dps)
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
