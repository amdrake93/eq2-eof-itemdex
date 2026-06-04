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

func groupBySlot(items []store.ScorableItem) map[string][]store.ScorableItem {
	m := map[string][]store.ScorableItem{}
	for _, it := range items {
		m[it.Slot] = append(m[it.Slot], it)
	}
	return m
}

func scoreRows(reports []bis.SlotReport, baselineName string) []store.ScoreRow {
	var rows []store.ScoreRow
	add := func(items []bis.ScoredItem, slot string) {
		for _, s := range items {
			rows = append(rows, store.ScoreRow{ItemID: s.Item.ID, Baseline: baselineName, DPSScore: s.Delta, Slot: slot})
		}
	}
	for _, r := range reports {
		add(r.Mythical, r.Slot)
		add(r.Fabled, r.Slot)
		add(r.Legendary, r.Slot)
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
	topN := flag.Int("top", 3, "alternatives per tier per slot")
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
	bySlot := groupBySlot(items)
	fmt.Printf("loadout: %s + %s; %d combat arts; %d assassin items\n",
		lo.MainName, lo.OffName, len(lo.Arts), len(items))

	baselines := []struct {
		name string
		sb   model.StatBlock
	}{{"SOLO", baseline.Solo}, {"RAID", baseline.Raid}}

	var reports []bis.BaselineReport
	var allRows []store.ScoreRow
	for _, b := range baselines {
		set := bis.BuildSet(b.sb, lo, bySlot, nil, maxBuildPasses)
		weights := bis.ConvergedWeights(set)
		slotReports := bis.BuildSlotReports(set, bySlot, weights, *topN)
		allRows = append(allRows, scoreRows(slotReports, strings.ToLower(b.name))...)
		reports = append(reports, bis.BaselineReport{Name: b.name, Weights: weights, Reports: slotReports})
	}

	if len(lockIDs) > 0 {
		locked := lockedItems(items, lockIDs)
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
