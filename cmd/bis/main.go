package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/amdrake93/eq2-eof-itemdex/internal/bis"
	"github.com/amdrake93/eq2-eof-itemdex/internal/charconfig"
	"github.com/amdrake93/eq2-eof-itemdex/internal/constants"
	"github.com/amdrake93/eq2-eof-itemdex/internal/loadout"
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
)

const maxBuildPasses = 12

func parseLocks(s string) ([]int, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var ids []int
	for _, p := range strings.Split(s, ",") {
		id, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return nil, fmt.Errorf("bad --lock id %q: %w", p, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func scoreRows(reports []bis.SlotReport, tierName string) []store.ScoreRow {
	var rows []store.ScoreRow
	for _, r := range reports {
		for _, s := range r.Ranked {
			rows = append(rows, store.ScoreRow{ItemID: s.Item.ID, Baseline: tierName, DPSScore: s.Delta, Slot: r.Slot})
		}
	}
	return rows
}

func lockedItems(items []store.ScorableItem, ids []int) map[string][]store.ScorableItem {
	want := map[int]bool{}
	for _, id := range ids {
		want[id] = true
	}
	m := map[string][]store.ScorableItem{}
	for _, it := range items {
		if want[it.ID] {
			m[it.Slot] = append(m[it.Slot], it)
		}
	}
	return m
}

func findByName(items []store.ScorableItem, name string) (store.ScorableItem, bool) {
	for _, it := range items {
		if it.Name == name {
			return it, true
		}
	}
	return store.ScorableItem{}, false
}

// withFixedPrimary prepends the fixed main-hand as a Primary slot report so each
// list is complete, showing the given Soulfire (not an optimized pick).
func withFixedPrimary(reports []bis.SlotReport, main store.ScorableItem, ok bool) []bis.SlotReport {
	if !ok {
		return reports
	}
	primary := bis.SlotReport{Slot: "Primary", Chosen: []store.ScorableItem{main}}
	return append([]bis.SlotReport{primary}, reports...)
}

type bucketReport struct {
	Title    string
	Upgrades []bis.SlotUpgrade
}

func runLoadoutReport(classData charconfig.ClassData, lo store.Loadout,
	profile model.StatBlock, items []store.ScorableItem, loadoutPath, out string, topN int, fight float64) {

	f, err := loadout.Read(loadoutPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bis: read loadout:", err)
		os.Exit(1)
	}
	set, optimizable := bis.SetFromLoadout(f, profile, lo, classData.AutoAttackMultiplier, fight)
	current := set.DPS()

	buckets := []struct {
		title string
		keep  func(store.ScorableItem) bool
	}{
		{"Get now — pre-raid accessible", bis.PreRaidFilter},
		{"Raid look-out", bis.RaidFilter},
		{"Best-of-best (aspirational)", bis.BestFilter},
	}
	var reports []bucketReport
	for _, bk := range buckets {
		bySlot := bis.SlotCandidates(items, bk.keep)
		reports = append(reports, bucketReport{Title: bk.title, Upgrades: bis.RankSlotUpgrades(set, bySlot, optimizable, topN)})
	}

	// Seeded optimization: re-optimize optimizable slots from the raid pool with the
	// imported set's fixed slots (charm/ranged/ammo/event) locked.
	raidBySlot := bis.SlotCandidates(items, bis.RaidFilter)
	locked := map[string][]store.ScorableItem{}
	for slot, eq := range set.Equipped {
		if !optimizable[slot] {
			locked[slot] = eq
		}
	}
	seeded := bis.BuildSet(profile, lo, raidBySlot, locked, maxBuildPasses, classData.AutoAttackMultiplier, fight)

	md := renderLoadoutReport(f, current, seeded.DPS(), reports)
	if err := os.WriteFile(out, []byte(md), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "bis: write report:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (current set %.0f DPS, %d unresolved imports)\n", out, current, len(f.Unresolved))
	if len(f.Unresolved) > 0 {
		fmt.Printf("unresolved (stats not counted): %v\n", f.Unresolved)
	}
}

func renderLoadoutReport(f loadout.File, current, seededDPS float64, reports []bucketReport) string {
	equippedBySlot := map[string]string{}
	for _, s := range f.Slots {
		if _, seen := equippedBySlot[s.CatalogSlot]; !seen {
			equippedBySlot[s.CatalogSlot] = s.Name
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Loadout report: %s\n\n", f.CharacterName)
	fmt.Fprintf(&b, "_last_update: %.0f_\n\n", f.LastUpdate)
	fmt.Fprintf(&b, "**Current set DPS:** %.0f\n\n", current)
	fmt.Fprintf(&b, "## Biggest upgrades from your current set\n\n")

	for _, r := range reports {
		fmt.Fprintf(&b, "### %s\n\n", r.Title)
		if len(r.Upgrades) == 0 {
			fmt.Fprintf(&b, "_no upgrades available in this bucket_\n\n")
			continue
		}
		fmt.Fprintf(&b, "| Slot | currently equipped | upgrade | +ΔDPS |\n")
		fmt.Fprintf(&b, "|------|--------------------|---------|------:|\n")
		for _, u := range r.Upgrades {
			cur := equippedBySlot[u.Slot]
			if cur == "" {
				cur = "(empty)"
			}
			fmt.Fprintf(&b, "| %s | %s | %s [%s] | +%.0f |\n", u.Slot, cur, u.Best.Name, u.Best.Tier, u.Best.Delta)
			if u.Alt != nil {
				fmt.Fprintf(&b, "| | | ↳ alt: %s [%s] | +%.0f |\n", u.Alt.Name, u.Alt.Tier, u.Alt.Delta)
			}
		}
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "## Seeded optimization\n\n")
	fmt.Fprintf(&b, "Optimizing from the imported set (fixed slots locked): **%.0f DPS** (%+.0f over current).\n\n",
		seededDPS, seededDPS-current)

	if len(f.Unresolved) > 0 {
		fmt.Fprintf(&b, "> Unresolved imports (stats NOT counted in current-set DPS): %s\n",
			strings.Join(f.Unresolved, ", "))
	}
	return b.String()
}

func main() {
	dbPath := flag.String("db", "bis.db", "scored SQLite db (built by builddb)")
	out := flag.String("out", "bis-report.md", "report output path")
	lock := flag.String("lock", "", "comma-separated item IDs to lock (raid re-model)")
	topN := flag.Int("top", 3, "alternatives per slot")
	character := flag.String("character", "characters/alex.toml", "character config (TOML)")
	fight := flag.Float64("fight", constants.FightDurationSecs, "target fight length in seconds (smoothed)")
	loadoutPath := flag.String("loadout", "", "imported loadout file to sim from (itemdex import output)")
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

	lockIDs, err := parseLocks(*lock)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	db, err := store.Open(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open db:", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	lo, err := db.LoadLoadout()
	if err != nil {
		fmt.Fprintln(os.Stderr, "load loadout:", err)
		os.Exit(1)
	}
	lo.Arts, err = charconfig.ApplyArtMods(lo.Arts, cfg.ArtMods)
	if err != nil {
		fmt.Fprintln(os.Stderr, "apply art mods:", err)
		os.Exit(1)
	}

	solo, err := cfg.ContextBlock("solo")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	raid, err := cfg.ContextBlock("raid")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	items, err := db.LoadScorableItems()
	if err != nil {
		fmt.Fprintln(os.Stderr, "load items:", err)
		os.Exit(1)
	}
	mainItem, haveMain := findByName(items, lo.MainName)
	fmt.Printf("loadout: %s (main-hand, fixed); %d combat arts; %d assassin items\n",
		lo.MainName, len(lo.Arts), len(items))

	if *loadoutPath != "" {
		// Loadout is simmed in the RAID context: an imported set represents the
		// player's real, raid-buffed-capable gear, so the raid baseline is correct.
		runLoadoutReport(classData, lo, raid, items, *loadoutPath, *out, *topN, *fight)
		return
	}

	tiers := []struct {
		name    string
		profile model.StatBlock
		keep    func(store.ScorableItem) bool
	}{
		{"PRE-RAID", solo, bis.PreRaidFilter},
		{"RAID", raid, bis.RaidFilter},
		{"BEST-OF-BEST", raid, bis.BestFilter},
	}

	var reports []bis.BaselineReport
	var allRows []store.ScoreRow
	for _, t := range tiers {
		profile := t.profile
		if haveMain {
			profile = profile.Add(mainItem.Stats)
		}
		bySlot := bis.SlotCandidates(items, t.keep)
		set := bis.BuildSet(profile, lo, bySlot, nil, maxBuildPasses, classData.AutoAttackMultiplier, *fight)
		weights := bis.ConvergedWeights(set)
		slotReports := withFixedPrimary(bis.BuildSlotReports(set, bySlot, weights, *topN), mainItem, haveMain)
		allRows = append(allRows, scoreRows(slotReports, strings.ToLower(t.name))...)
		reports = append(reports, bis.BaselineReport{Name: t.name, Weights: weights, Reports: slotReports})
	}

	if len(lockIDs) > 0 {
		locked := lockedItems(items, lockIDs)
		bySlot := bis.SlotCandidates(items, bis.RaidFilter)
		profile := raid
		if haveMain {
			profile = profile.Add(mainItem.Stats)
		}
		set := bis.BuildSet(profile, lo, bySlot, locked, maxBuildPasses, classData.AutoAttackMultiplier, *fight)
		weights := bis.ConvergedWeights(set)
		slotReports := withFixedPrimary(bis.BuildSlotReports(set, bySlot, weights, *topN), mainItem, haveMain)
		reports = append(reports, bis.BaselineReport{
			Name: fmt.Sprintf("RAID (locked: %s)", *lock), Weights: weights, Reports: slotReports,
		})
	}

	if err := db.WriteScores(allRows); err != nil {
		fmt.Fprintln(os.Stderr, "write scores:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, []byte(bis.Render(reports, *fight)), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write report:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s and %d score rows to %s\n", *out, len(allRows), *dbPath)
}
