package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/amdrake93/eq2-eof-itemdex/internal/bis"
	"github.com/amdrake93/eq2-eof-itemdex/internal/charconfig"
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

func main() {
	dbPath := flag.String("db", "bis.db", "scored SQLite db (built by builddb)")
	out := flag.String("out", "bis-report.md", "report output path")
	lock := flag.String("lock", "", "comma-separated item IDs to lock (raid re-model)")
	topN := flag.Int("top", 3, "alternatives per slot")
	character := flag.String("character", "characters/alex.toml", "character config (TOML)")
	flag.Parse()

	cfg, err := charconfig.Load(*character)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load character:", err)
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

	notExcluded := func(it store.ScorableItem) bool { return !bis.IsHunters(it) && !bis.Curated(it) }
	tiers := []struct {
		name    string
		profile model.StatBlock
		keep    func(store.ScorableItem) bool
	}{
		{"PRE-RAID", solo, func(it store.ScorableItem) bool {
			return (it.Tier == "LEGENDARY" || it.Tier == "TREASURED") && !bis.IsAvatar(it) && notExcluded(it)
		}},
		{"RAID", raid, func(it store.ScorableItem) bool {
			return !bis.IsAvatar(it) && notExcluded(it)
		}},
		{"BEST-OF-BEST", raid, func(it store.ScorableItem) bool {
			return notExcluded(it)
		}},
	}

	var reports []bis.BaselineReport
	var allRows []store.ScoreRow
	for _, t := range tiers {
		profile := t.profile
		if haveMain {
			profile = profile.Add(mainItem.Stats)
		}
		bySlot := bis.SlotCandidates(items, t.keep)
		set := bis.BuildSet(profile, lo, bySlot, nil, maxBuildPasses)
		weights := bis.ConvergedWeights(set)
		slotReports := withFixedPrimary(bis.BuildSlotReports(set, bySlot, weights, *topN), mainItem, haveMain)
		allRows = append(allRows, scoreRows(slotReports, strings.ToLower(t.name))...)
		reports = append(reports, bis.BaselineReport{Name: t.name, Weights: weights, Reports: slotReports})
	}

	if len(lockIDs) > 0 {
		locked := lockedItems(items, lockIDs)
		bySlot := bis.SlotCandidates(items, func(it store.ScorableItem) bool { return !bis.IsAvatar(it) && notExcluded(it) })
		profile := raid
		if haveMain {
			profile = profile.Add(mainItem.Stats)
		}
		set := bis.BuildSet(profile, lo, bySlot, locked, maxBuildPasses)
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
	if err := os.WriteFile(*out, []byte(bis.Render(reports)), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write report:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s and %d score rows to %s\n", *out, len(allRows), *dbPath)
}
