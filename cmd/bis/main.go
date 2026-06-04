package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/amdrake93/eq2-eof-itemdex/internal/baseline"
	"github.com/amdrake93/eq2-eof-itemdex/internal/bis"
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

func main() {
	dbPath := flag.String("db", "bis.db", "scored SQLite db (built by builddb)")
	out := flag.String("out", "bis-report.md", "report output path")
	lock := flag.String("lock", "", "comma-separated item IDs to lock (raid re-model)")
	topN := flag.Int("top", 3, "alternatives per slot")
	flag.Parse()

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
	items, err := db.LoadScorableItems()
	if err != nil {
		fmt.Fprintln(os.Stderr, "load items:", err)
		os.Exit(1)
	}
	fmt.Printf("loadout: %s + %s; %d combat arts; %d assassin items\n",
		lo.MainName, lo.OffName, len(lo.Arts), len(items))

	notExcluded := func(it store.ScorableItem) bool { return !bis.IsHunters(it) && !bis.Curated(it) }
	tiers := []struct {
		name     string
		baseline model.StatBlock
		keep     func(store.ScorableItem) bool
	}{
		{"PRE-RAID", baseline.Solo, func(it store.ScorableItem) bool {
			return (it.Tier == "LEGENDARY" || it.Tier == "TREASURED") && !bis.IsAvatar(it) && notExcluded(it)
		}},
		{"RAID", baseline.Raid, func(it store.ScorableItem) bool {
			return !bis.IsAvatar(it) && notExcluded(it)
		}},
		{"BEST-OF-BEST", baseline.Raid, func(it store.ScorableItem) bool {
			return notExcluded(it)
		}},
	}

	var reports []bis.BaselineReport
	var allRows []store.ScoreRow
	for _, t := range tiers {
		bySlot := bis.SlotCandidates(items, t.keep)
		set := bis.BuildSet(t.baseline, lo, bySlot, nil, maxBuildPasses)
		weights := bis.ConvergedWeights(set)
		slotReports := bis.BuildSlotReports(set, bySlot, weights, *topN)
		allRows = append(allRows, scoreRows(slotReports, strings.ToLower(t.name))...)
		reports = append(reports, bis.BaselineReport{Name: t.name, Weights: weights, Reports: slotReports})
	}

	if len(lockIDs) > 0 {
		locked := lockedItems(items, lockIDs)
		bySlot := bis.SlotCandidates(items, func(it store.ScorableItem) bool { return !bis.IsAvatar(it) && notExcluded(it) })
		set := bis.BuildSet(baseline.Raid, lo, bySlot, locked, maxBuildPasses)
		weights := bis.ConvergedWeights(set)
		slotReports := bis.BuildSlotReports(set, bySlot, weights, *topN)
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
