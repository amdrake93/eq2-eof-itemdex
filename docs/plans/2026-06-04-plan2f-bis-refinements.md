# Plan 2f — BiS Refinements Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refine the working Plan 2e BiS tool: fix the off-hand candidate pool, add a curated accessibility-exclusion mechanism, replace the two-baseline output with three accessibility tiers (pre-raid / raid / best-of-best), merge each slot's alternatives into one priority list, and add a per-slot progression summary.

**Architecture:** Builds on the implemented Plan 2e (`internal/model`, `internal/store`, `internal/bis`, `cmd/bis` — on branch `plan2e-scoring-report`, not yet merged). These tasks are additive/modifying; reuse the existing `BuildSet`, `ConvergedWeights`, `CandidateDelta`, `SlotCandidatesScored`, and `Render` building blocks rather than rewriting them. Execute on the same branch.

**Tech Stack:** Go 1.26, `modernc.org/sqlite`, `stretchr/testify`.

**Validated decisions (from review with the domain expert):**
- Assassins legitimately wear cloth/leather/**chain** (plate correctly excluded already) — **class parsing needs NO change**.
- Off-hand can be **any one-handed weapon**; the bug is that 1H weapons are Census-slotted "Primary" and never reach the Secondary pool.
- **Inaccessible gear** = `(MYTHICAL and not Soulfire)` [avatar] ∪ `name contains "Hunter's"` [the three Hunter's sets] ∪ a hand-curated list. Hunter's is excluded from **all** tiers; avatar is excluded from pre-raid & raid but **included** in best-of-best. The list is curated/extensible (the expert adds items as spotted).
- Buff context per tier: **pre-raid → SOLO baseline**, **raid & best-of-best → RAID baseline**.

---

## File Structure

| File | Change |
|---|---|
| `internal/bis/exclusions.go` *(new)* | `IsHunters`, `IsAvatar`, `Curated` predicates + the curated name list. |
| `internal/store/store.go` *(modify)* | add `WieldStyle` to `ScorableItem` + `LoadScorableItems` query. |
| `internal/bis/candidates.go` *(new)* | `SlotCandidates(items, keep)` — group by slot with a keep filter, and override the off-hand slot with *all* 1H weapons. |
| `internal/bis/report.go` *(modify)* | add `Ranked []ScoredItem` to `SlotReport`; populate it in `BuildSlotReports`. |
| `internal/bis/render.go` *(modify)* | render the merged `Ranked` list (rarity-tagged) instead of per-tier blocks; append a per-slot progression summary. |
| `cmd/bis/main.go` *(modify)* | three-tier loop (pre-raid/raid/best) using `SlotCandidates` + per-tier baselines; progression; scores; keep `--lock` within the raid tier; drop dead `SlotReport` fields. |

---

### Task 1: Accessibility predicates (`internal/bis/exclusions.go`)

**Files:**
- Create: `internal/bis/exclusions.go`
- Test: `internal/bis/exclusions_test.go`

- [ ] **Step 1: Write the failing test**

```go
package bis

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
	"github.com/stretchr/testify/require"
)

func TestAccessibilityPredicates(t *testing.T) {
	avatar := store.ScorableItem{Name: "Fragment of the Chime", Tier: "MYTHICAL"}
	soulfire := store.ScorableItem{Name: "Soulfire Sabre", Tier: "MYTHICAL"}
	hunters := store.ScorableItem{Name: "Sky Hunter's Stiletto", Tier: "LEGENDARY"}
	fabled := store.ScorableItem{Name: "Bee Sting", Tier: "FABLED"}

	require.True(t, IsAvatar(avatar))
	require.False(t, IsAvatar(soulfire), "Soulfire mythicals are accessible")
	require.False(t, IsAvatar(fabled))

	require.True(t, IsHunters(hunters))
	require.False(t, IsHunters(fabled))

	require.False(t, Curated(fabled)) // empty curated list by default
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bis/ -run TestAccessibilityPredicates -v`
Expected: FAIL — `undefined: IsAvatar` / `IsHunters` / `Curated`.

- [ ] **Step 3: Write minimal implementation**

```go
package bis

import (
	"strings"

	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
)

// curatedExclusions is a hand-maintained set of item names that are discoverable
// on Varsoon but inaccessible to us, beyond the avatar/Hunter's rules below.
// Append item names here as they are identified.
var curatedExclusions = map[string]bool{}

// IsAvatar reports whether an item is contested-avatar gear: a Mythical that is
// not a Soulfire weapon. (All EoF mythicals are avatar drops except Soulfire.)
func IsAvatar(it store.ScorableItem) bool {
	return it.Tier == "MYTHICAL" && !strings.HasPrefix(it.Name, "Soulfire")
}

// IsHunters reports whether an item is from the inaccessible Hunter's sets
// (Cutthroat/Desert/Sky Hunter's ...).
func IsHunters(it store.ScorableItem) bool {
	return strings.Contains(it.Name, "Hunter's")
}

// Curated reports whether an item is on the hand-maintained exclusion list.
func Curated(it store.ScorableItem) bool {
	return curatedExclusions[it.Name]
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bis/ -run TestAccessibilityPredicates -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
go test ./internal/bis/ && make lint
git add internal/bis/exclusions.go internal/bis/exclusions_test.go
git commit -m "Add accessibility predicates: avatar / Hunter's / curated exclusions"
```

---

### Task 2: `WieldStyle` on `ScorableItem`

**Files:**
- Modify: `internal/store/store.go`
- Test: `internal/store/scorable_test.go` (extend the existing `TestLoadScorableItems`)

**Context:** `SlotCandidates` (Task 3) must identify one-handed weapons for the off-hand pool, but `ScorableItem` doesn't carry `wieldstyle`. The `items` table already has a `wieldstyle` column.

- [ ] **Step 1: Add a failing assertion to the existing test**

In `internal/store/scorable_test.go`, inside `TestLoadScorableItems`, after the existing `dirk := byID[11]` assertions, add:

```go
	require.Equal(t, "One-Handed", dirk.WieldStyle)
	require.Equal(t, "", chest.WieldStyle) // armor has no wield style
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestLoadScorableItems -v`
Expected: FAIL — `dirk.WieldStyle undefined` (field missing).

- [ ] **Step 3: Implement**

In `internal/store/store.go`, add the field to `ScorableItem` (after `Tier`):

```go
	WieldStyle  string
```

In `LoadScorableItems`, change the `SELECT` and `Scan` to include `wieldstyle`:

```go
	rows, err := d.db.Query(
		`SELECT id, name, slot, tier, wieldstyle, gamelink, weapon_min_dmg, weapon_max_dmg, delay
		 FROM items WHERE classes LIKE '%assassin%'`)
```
and
```go
		if err := rows.Scan(&it.ID, &it.Name, &it.Slot, &it.Tier, &it.WieldStyle, &it.GameLink, &mn, &mx, &delay); err != nil {
			return nil, err
		}
```

(The Task-5 seed rows in `scorable_test.go` already set `wieldstyle` — the dirk is `'One-Handed'`, the chest `''` — so the new assertions pass without changing the seed.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestLoadScorableItems -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
go test ./internal/store/ && make lint
git add internal/store/store.go internal/store/scorable_test.go
git commit -m "Add WieldStyle to ScorableItem"
```

---

### Task 3: `SlotCandidates` — keep-filter + off-hand override

**Files:**
- Create: `internal/bis/candidates.go`
- Test: `internal/bis/candidates_test.go`

**Context:** This assembles the per-slot candidate map for a tier. It (a) drops items the tier excludes via a `keep` predicate, and (b) **overrides the off-hand (`Secondary`) slot with every one-handed weapon** — fixing the bug where 1H weapons Census-slotted "Primary" never reached the off-hand pool.

- [ ] **Step 1: Write the failing test**

```go
package bis

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
	"github.com/stretchr/testify/require"
)

func TestSlotCandidatesOffHandAndFilter(t *testing.T) {
	items := []store.ScorableItem{
		{ID: 1, Name: "Fabled Rapier", Slot: "Primary", Tier: "FABLED", WieldStyle: "One-Handed", WeaponAvg: 144, WeaponDelay: 4},
		{ID: 2, Name: "Sky Hunter's Stiletto", Slot: "Secondary", Tier: "LEGENDARY", WieldStyle: "One-Handed", WeaponAvg: 242, WeaponDelay: 6},
		{ID: 3, Name: "Fabled Chest", Slot: "Chest", Tier: "FABLED", Stats: model.StatBlock{Potency: 30}},
		{ID: 4, Name: "Great Axe", Slot: "Primary", Tier: "FABLED", WieldStyle: "Two-Handed", WeaponAvg: 300, WeaponDelay: 6},
	}
	keep := func(it store.ScorableItem) bool { return !IsHunters(it) }

	bySlot := SlotCandidates(items, keep)

	// Off-hand pool = all 1H weapons that pass keep. Fabled Rapier (census Primary,
	// 1H) is now an off-hand candidate; the Hunter's stiletto is filtered out; the
	// two-handed axe is not a 1H weapon so it's excluded.
	secIDs := map[int]bool{}
	for _, c := range bySlot["Secondary"] {
		secIDs[c.ID] = true
	}
	require.True(t, secIDs[1], "fabled 1H weapon must be an off-hand candidate")
	require.False(t, secIDs[2], "Hunter's stiletto filtered by keep")
	require.False(t, secIDs[4], "two-handed weapon is not an off-hand candidate")

	// Non-weapon slots still grouped normally (and keep-filtered).
	require.Len(t, bySlot["Chest"], 1)
	require.Equal(t, 3, bySlot["Chest"][0].ID)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bis/ -run TestSlotCandidates -v`
Expected: FAIL — `undefined: SlotCandidates`.

- [ ] **Step 3: Write minimal implementation**

```go
package bis

import "github.com/amdrake93/eq2-eof-itemdex/internal/store"

// SlotCandidates groups items by their census slot, dropping any that fail keep,
// then overrides the off-hand slot with every one-handed weapon that passes keep
// (1H weapons are dual-wieldable but census-slotted "Primary", so they would
// otherwise never reach the off-hand pool). The main-hand (Primary) bucket is
// left as-is; BuildSet ignores it (the main is fixed).
func SlotCandidates(items []store.ScorableItem, keep func(store.ScorableItem) bool) map[string][]store.ScorableItem {
	bySlot := map[string][]store.ScorableItem{}
	var oneHanders []store.ScorableItem
	for _, it := range items {
		if !keep(it) {
			continue
		}
		bySlot[it.Slot] = append(bySlot[it.Slot], it)
		if it.WieldStyle == "One-Handed" {
			oneHanders = append(oneHanders, it)
		}
	}
	bySlot[offHandSlot] = oneHanders
	return bySlot
}
```

(`offHandSlot` is the existing const `"Secondary"` in `internal/bis/set.go`.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bis/ -run TestSlotCandidates -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
go test ./internal/bis/ && make lint
git add internal/bis/candidates.go internal/bis/candidates_test.go
git commit -m "Add SlotCandidates: keep-filter + off-hand pool of all 1H weapons"
```

---

### Task 4: Merged priority list on `SlotReport`

**Files:**
- Modify: `internal/bis/report.go`
- Test: `internal/bis/report_test.go` (extend `TestBuildSlotReports`)

**Context:** Add a single `Ranked []ScoredItem` (top-N by in-context ΔDPS across *all* of the slot's candidates, regardless of rarity tier). This is additive — the existing `Mythical/Fabled/Legendary` fields remain for now (removed in Task 7) so the repo stays green.

- [ ] **Step 1: Extend the test**

In `internal/bis/report_test.go`, inside `TestBuildSlotReports`, after the existing assertions, add:

```go
	// merged priority list: top-3 across ALL candidates by Delta (the flurry-20
	// item first), regardless of rarity tier.
	require.Len(t, r.Ranked, 3)
	require.Equal(t, 6, r.Ranked[0].Item.ID) // mythical flurry-50 has the highest delta
	require.GreaterOrEqual(t, r.Ranked[0].Delta, r.Ranked[1].Delta)
	require.GreaterOrEqual(t, r.Ranked[1].Delta, r.Ranked[2].Delta)
```

(Recall the test's `Ear` candidates: ids 1-4 FABLED flurry {5,20,12,1}, id 5 LEGENDARY flurry 8, id 6 MYTHICAL flurry 50. Highest flurry → highest Delta, so id 6 ranks first across the merged list.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bis/ -run TestBuildSlotReports -v`
Expected: FAIL — `r.Ranked undefined`.

- [ ] **Step 3: Implement**

In `internal/bis/report.go`, add `Ranked` to the struct:

```go
// SlotReport is one slot's converged pick plus ranked alternatives.
type SlotReport struct {
	Slot      string
	Chosen    []store.ScorableItem
	Ranked    []ScoredItem // top-N across all candidates by in-context ΔDPS
	Mythical  []ScoredItem
	Fabled    []ScoredItem
	Legendary []ScoredItem
}
```

Add a `topN` helper (sorted input → first n):

```go
func topN(scored []ScoredItem, n int) []ScoredItem {
	if n >= 0 && len(scored) > n {
		return scored[:n]
	}
	return scored
}
```

In `BuildSlotReports`, set `Ranked` from the already-sorted `scored`:

```go
		reports = append(reports, SlotReport{
			Slot:      slot,
			Chosen:    set.Equipped[slot],
			Ranked:    topN(scored, n),
			Mythical:  topByTier(scored, "MYTHICAL", -1),
			Fabled:    topByTier(scored, "FABLED", n),
			Legendary: topByTier(scored, "LEGENDARY", n),
		})
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bis/ -run TestBuildSlotReports -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
go test ./internal/bis/ && make lint
git add internal/bis/report.go internal/bis/report_test.go
git commit -m "Add merged Ranked priority list to SlotReport"
```

---

### Task 5: Render the merged list with rarity tags

**Files:**
- Modify: `internal/bis/render.go`
- Test: `internal/bis/render_test.go` (extend `TestRender`)

**Context:** Replace the three per-tier blocks (`Mythical`/`Fabled`/`Legendary`) per slot with the single merged `Ranked` list, each line tagged with its rarity tier and an `avatar` marker where applicable.

- [ ] **Step 1: Extend the test**

Replace `TestRender` in `internal/bis/render_test.go` with:

```go
func TestRender(t *testing.T) {
	weights := map[string]float64{"reuse": 16.67, "potency": 7.19}
	reports := []BaselineReport{{
		Name:    "RAID",
		Weights: weights,
		Reports: []SlotReport{{
			Slot:   "Chest",
			Chosen: []store.ScorableItem{{ID: 2, Name: "Fabled Chest", Tier: "FABLED", GameLink: "LINK2"}},
			Ranked: []ScoredItem{
				{Item: store.ScorableItem{ID: 2, Name: "Fabled Chest", Tier: "FABLED", GameLink: "LINK2"},
					Delta: 41.2, Terms: []model.ScoreTerm{{Stat: "potency", ItemValue: 35, Weight: 7.19, Contribution: 251.65}}},
				{Item: store.ScorableItem{ID: 9, Name: "Avatar Robe", Tier: "MYTHICAL"}, Delta: 60.0},
			},
		}},
	}}

	out := Render(reports)

	require.Contains(t, out, "## RAID")
	require.Contains(t, out, "### Chest")
	require.Contains(t, out, "BiS: **Fabled Chest**")
	require.Contains(t, out, "Fabled Chest")
	require.Contains(t, out, "+41.2 DPS")
	require.Contains(t, out, "[FABLED]")          // rarity tag on the line
	require.Contains(t, out, "[MYTHICAL · avatar]") // avatar marker
	require.Contains(t, out, "potency 35 × 7.19")
	require.Contains(t, out, "reuse")              // weight table
	require.Contains(t, out, "Assumptions")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bis/ -run TestRender -v`
Expected: FAIL — output lacks `[FABLED]` / `[MYTHICAL · avatar]` tags.

- [ ] **Step 3: Implement**

In `internal/bis/render.go`, add a tag helper and rewrite `writeScored` to include it; replace the three `writeTier` calls in `Render` with one merged list.

Add:
```go
func rarityTag(it store.ScorableItem) string {
	if IsAvatar(it) {
		return it.Tier + " · avatar"
	}
	return it.Tier
}
```

Change `writeScored` to print the tag:
```go
func writeScored(b *strings.Builder, s ScoredItem) {
	fmt.Fprintf(b, "- **%s** [%s] — +%.1f DPS", s.Item.Name, rarityTag(s.Item), s.Delta)
	if s.Item.GameLink != "" {
		fmt.Fprintf(b, " ([item](%s))", s.Item.GameLink)
	}
	b.WriteString("\n")
	for i, term := range s.Terms {
		if i >= 4 {
			break
		}
		fmt.Fprintf(b, "  - %s %.0f × %.2f = %.1f\n", term.Stat, term.ItemValue, term.Weight, term.Contribution)
	}
}
```

In `Render`, replace the per-tier block section:
```go
		for _, sr := range r.Reports {
			fmt.Fprintf(&b, "### %s\n\n", sr.Slot)
			if len(sr.Chosen) > 0 {
				names := make([]string, 0, len(sr.Chosen))
				for _, c := range sr.Chosen {
					names = append(names, c.Name)
				}
				fmt.Fprintf(&b, "BiS: **%s**\n\n", strings.Join(names, "**, **"))
			}
			for _, s := range sr.Ranked {
				writeScored(&b, s)
			}
			b.WriteString("\n")
		}
```

Add the `store` import to `render.go` (for `rarityTag`'s parameter):
```go
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
```

`writeTier` becomes unused — delete it. (`topByTier` in report.go is still referenced by the soon-to-be-removed fields; leave it until Task 7.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bis/ -run TestRender -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
go test ./internal/bis/ && go build ./... && make lint
git add internal/bis/render.go internal/bis/render_test.go
git commit -m "Render merged per-slot priority list with rarity/avatar tags"
```

---

### Task 6: Per-slot progression summary

**Files:**
- Modify: `internal/bis/render.go`
- Test: `internal/bis/render_test.go`

**Context:** After the per-tier reports, emit a progression section: for each slot, one line per tier showing that tier's #1 pick (`Ranked[0]`), so the reader sees the upgrade path pre-raid → raid → best.

- [ ] **Step 1: Write the failing test**

Add to `internal/bis/render_test.go`:

```go
func TestRenderProgression(t *testing.T) {
	mk := func(name, tier string, d float64) SlotReport {
		return SlotReport{
			Slot:   "Chest",
			Ranked: []ScoredItem{{Item: store.ScorableItem{Name: name, Tier: tier}, Delta: d}},
		}
	}
	reports := []BaselineReport{
		{Name: "PRE-RAID", Reports: []SlotReport{mk("Dungeon Robe", "LEGENDARY", 30)}},
		{Name: "RAID", Reports: []SlotReport{mk("Fabled Chest", "FABLED", 50)}},
		{Name: "BEST-OF-BEST", Reports: []SlotReport{mk("Avatar Robe", "MYTHICAL", 70)}},
	}
	out := Render(reports)

	require.Contains(t, out, "## Progression")
	// per-slot progression line order: pre-raid then raid then best
	chest := out[strings.Index(out, "## Progression"):]
	require.Contains(t, chest, "### Chest")
	pre := strings.Index(chest, "Dungeon Robe")
	raid := strings.Index(chest, "Fabled Chest")
	best := strings.Index(chest, "Avatar Robe")
	require.True(t, pre >= 0 && raid > pre && best > raid, "progression order pre-raid → raid → best")
}
```

(`strings` is already imported in `render_test.go` via the package's other tests; if not, add it.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bis/ -run TestRenderProgression -v`
Expected: FAIL — no `## Progression` section.

- [ ] **Step 3: Implement**

In `internal/bis/render.go`, add a progression writer and call it from `Render` (before `writeAssumptions`):

```go
func writeProgression(b *strings.Builder, reports []BaselineReport) {
	// collect, per slot, each report's #1 pick (preserving report order)
	type pick struct {
		tier string
		item ScoredItem
	}
	bySlot := map[string][]pick{}
	var slotOrder []string
	for _, r := range reports {
		for _, sr := range r.Reports {
			if len(sr.Ranked) == 0 {
				continue
			}
			if _, seen := bySlot[sr.Slot]; !seen {
				slotOrder = append(slotOrder, sr.Slot)
			}
			bySlot[sr.Slot] = append(bySlot[sr.Slot], pick{tier: r.Name, item: sr.Ranked[0]})
		}
	}
	sort.Strings(slotOrder)

	b.WriteString("---\n\n## Progression (per slot)\n\n")
	b.WriteString("_Top pick per accessibility tier. ΔDPS is in each tier's own buff context, so values are not directly comparable across tiers._\n\n")
	for _, slot := range slotOrder {
		fmt.Fprintf(b, "### %s\n", slot)
		for _, p := range bySlot[slot] {
			fmt.Fprintf(b, "- %-12s **%s** [%s] (+%.1f)\n", p.tier, p.item.Item.Name, rarityTag(p.item.Item), p.item.Delta)
		}
		b.WriteString("\n")
	}
}
```

In `Render`, insert the call just before `writeAssumptions(&b)`:
```go
	writeProgression(&b, reports)
	writeAssumptions(&b)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bis/ -run "TestRender|TestRenderProgression" -v`
Expected: PASS (both).

- [ ] **Step 5: Commit**

```bash
go test ./internal/bis/ && make lint
git add internal/bis/render.go internal/bis/render_test.go
git commit -m "Add per-slot progression summary to the report"
```

---

### Task 7: `cmd/bis` three-tier wiring

**Files:**
- Modify: `cmd/bis/main.go`
- Modify: `internal/bis/report.go` (remove dead fields)
- Test: manual run

**Context:** Replace the SOLO/RAID baseline loop with the three accessibility tiers, each using its own candidate filter (`SlotCandidates` + tier `keep`) and baseline. Then drop the now-unused `Mythical/Fabled/Legendary` fields + `topByTier`.

- [ ] **Step 1: Rewrite the tier loop in `cmd/bis/main.go`**

Replace `groupBySlot` usage and the `baselines` loop. New `main.go` body (keep `parseLocks`, `lockedItems`, and imports; `groupBySlot` is no longer needed — delete it):

```go
func scoreRows(reports []bis.SlotReport, tierName string) []store.ScoreRow {
	var rows []store.ScoreRow
	for _, r := range reports {
		for _, s := range r.Ranked {
			rows = append(rows, store.ScoreRow{ItemID: s.Item.ID, Baseline: tierName, DPSScore: s.Delta, Slot: r.Slot})
		}
	}
	return rows
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
```

- [ ] **Step 2: Remove the dead `SlotReport` fields**

In `internal/bis/report.go`: remove `Mythical`, `Fabled`, `Legendary` from the `SlotReport` struct; remove the `topByTier` function; and in `BuildSlotReports` drop the three `topByTier(...)` lines, leaving:

```go
		reports = append(reports, SlotReport{
			Slot:   slot,
			Chosen: set.Equipped[slot],
			Ranked: topN(scored, n),
		})
```

- [ ] **Step 3: Build, vet, run**

Run:
```bash
go build ./... && go vet ./...
go run ./cmd/builddb     # ensure bis.db has the scores table + fresh data
go run ./cmd/bis --db bis.db --out bis-report.md
```
Expected: prints the loadout line + `wrote bis-report.md and N score rows`. (`go test ./...` must also pass — the old `report_test.go` assertions on Fabled/Legendary were already replaced in Task 4/5; confirm none reference the removed fields. If any test still reads `.Fabled`/`.Legendary`/`.Mythical`, update it to `.Ranked`.)

- [ ] **Step 4: Sanity-check the output**

Run: `sed -n '1,60p' bis-report.md` then `grep -n "^## " bis-report.md`
Expected: `## PRE-RAID`, `## RAID`, `## BEST-OF-BEST`, and `## Progression` sections. Spot-check: pre-raid slots show only LEGENDARY/TREASURED items; no `Hunter's` items anywhere; avatar (`[MYTHICAL · avatar]`) appears only under BEST-OF-BEST; the off-hand (Secondary) now shows fabled 1H weapons, not just stilettos.

- [ ] **Step 5: Full suite + lint + commit**

```bash
go test ./... && make lint
git add cmd/bis/main.go internal/bis/report.go
git commit -m "Wire cmd/bis to three accessibility tiers + progression"
```

---

## Self-Review

**1. Spec coverage:**
- Off-hand fix → Task 3 (`SlotCandidates` overrides Secondary with all 1H weapons), enabled by Task 2 (`WieldStyle`). ✔
- Curated extensible exclusions (avatar + Hunter's + hand list) → Task 1. ✔
- Three accessibility tiers with per-tier baselines + filters → Task 7 (composing Task 1 predicates). Pre-raid = SOLO + {legendary,treasured}−avatar−hunters; Raid = RAID + (all − avatar − hunters); Best = RAID + (all − hunters). ✔
- Hunter's excluded everywhere incl. best-of-best; avatar only in best-of-best → Task 7 keep funcs. ✔
- Merged priority top-N per slot → Task 4 (`Ranked`) + Task 5 (render with rarity/avatar tags). ✔
- Progression summary → Task 6. ✔
- `--lock` retained (raid tier) → Task 7. ✔
- Class parsing unchanged → not touched. ✔

**2. Placeholder scan:** No TBD/TODO; every code step is complete; tests have real assertions.

**3. Type consistency:** `store.ScorableItem.WieldStyle` (Task 2) used by `SlotCandidates` (Task 3). `bis.IsAvatar/IsHunters/Curated` (Task 1) used by `rarityTag` (Task 5) and the tier keeps (Task 7). `SlotReport.Ranked []ScoredItem` (Task 4) consumed by render (Task 5/6) and `scoreRows` (Task 7); `Mythical/Fabled/Legendary` removed in Task 7 after all consumers moved to `Ranked`. `topN` (Task 4) vs the removed `topByTier` (Task 7). `offHandSlot` const reused from Plan 2e. Tier `keep`/`baseline` shape consistent in Task 7.

**Green-at-each-commit note:** Task 4 is additive (keeps old fields); Task 5 switches render to `Ranked`; Task 7 removes the dead fields only after Task 7-step-1 moves `scoreRows` to `Ranked`. So every commit compiles and tests pass.

**Sequencing caveat (for the executor):** Task 5 deletes `writeTier`; Task 7 deletes `topByTier` and the dead fields. If `make lint` flags `topByTier` as unused after Task 5 (it is still referenced by `BuildSlotReports` until Task 7), that's expected — it stays referenced until Task 7, so no lint break in between.
