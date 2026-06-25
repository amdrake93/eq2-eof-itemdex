# Tiered Actionable Upgrade Report Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `bis --loadout`'s "what to upgrade next" a tiered, actionable list — the biggest upgrades from your current set in three accessibility buckets (Get-now / Raid / Best-of-best), each slot showing a best pick + one alternative.

**Architecture:** Extract the three accessibility-tier candidate filters into reusable `bis` functions (shared by the from-scratch tier runs and the new report — DRY). Add a testable `bis.RankSlotUpgrades` that, per optimizable slot, finds the best + 2nd-best positive `Set.UpgradeDelta` and ranks slots by the **primary** candidate's ΔDPS. Rewire `runLoadoutReport`/`renderLoadoutReport` (cmd/bis) to build three buckets via the filters and emit three sections. All buckets sim against the one imported (raid) set, so +ΔDPS is comparable.

**Tech Stack:** Go 1.26 (module `github.com/amdrake93/eq2-eof-itemdex`), testify/require. Build `go build ./...`; test `go test ./...`. Branch `character-gear-import-spec`. Spec: `docs/SPEC.md` §7 "Tiered upgrade report".

---

## File Structure

| File | Change |
|---|---|
| `internal/bis/filters.go` | **Create** — `PreRaidFilter`/`RaidFilter`/`BestFilter` (composed; reuse existing `IsAvatar`/`IsHunters`/`Curated`) |
| `internal/bis/loadoutset.go` | Add `UpgradeOption`/`SlotUpgrade` types + `RankSlotUpgrades` |
| `internal/bis/filters_test.go`, `internal/bis/loadoutset_test.go` | Tests |
| `cmd/bis/main.go` | Rewire `runLoadoutReport` (3 buckets) + `renderLoadoutReport` (3 sections, best+alt); DRY the tier loop + lock path to use the new filters; remove the `upgrade` struct |

Task 1 (bis) is independent and testable. Task 2 (cmd) depends on Task 1. Task 3 verifies end-to-end. Run sequentially.

---

## Task 1: `bis` tier filters + `RankSlotUpgrades`

**Files:**
- Create: `internal/bis/filters.go`, `internal/bis/filters_test.go`
- Modify: `internal/bis/loadoutset.go`
- Test: `internal/bis/loadoutset_test.go`

- [ ] **Step 1: Write the failing filter test** (`internal/bis/filters_test.go`)

```go
package bis

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
)

func TestTierFilters(t *testing.T) {
	leg := store.ScorableItem{Tier: "LEGENDARY", Name: "Leg Cloak"}
	treas := store.ScorableItem{Tier: "TREASURED", Name: "Treasured Ring"}
	fabled := store.ScorableItem{Tier: "FABLED", Name: "Fabled Blade"}
	avatar := store.ScorableItem{Tier: "MYTHICAL", Name: "Avatar Helm"} // MYTHICAL, not Soulfire => avatar
	hunters := store.ScorableItem{Tier: "FABLED", Name: "Hunter's Cloak"}

	require.True(t, PreRaidFilter(leg))
	require.True(t, PreRaidFilter(treas))
	require.False(t, PreRaidFilter(fabled)) // fabled is not pre-raid

	require.True(t, RaidFilter(fabled))
	require.True(t, RaidFilter(leg))     // nested: pre-raid items are also raid-eligible
	require.False(t, RaidFilter(avatar)) // avatar excluded from raid

	require.True(t, BestFilter(avatar)) // best-of-best includes avatar

	require.False(t, BestFilter(hunters)) // Hunter's always excluded
	require.False(t, RaidFilter(hunters))
	require.False(t, PreRaidFilter(hunters))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bis/ -run TestTierFilters -v`
Expected: FAIL — `PreRaidFilter`/`RaidFilter`/`BestFilter` undefined.

- [ ] **Step 3: Create `internal/bis/filters.go`**

```go
package bis

import "github.com/amdrake93/eq2-eof-itemdex/internal/store"

// The accessibility-tier candidate filters, shared by the from-scratch tier runs
// (cmd/bis) and the imported-loadout tiered upgrade report (§6, §7). They compose
// as nested supersets: PreRaid ⊂ Raid ⊂ Best.

// BestFilter keeps everything except Hunter's sets and curated exclusions.
func BestFilter(it store.ScorableItem) bool { return !IsHunters(it) && !Curated(it) }

// RaidFilter additionally excludes avatar (MYTHICAL non-Soulfire) gear.
func RaidFilter(it store.ScorableItem) bool { return !IsAvatar(it) && BestFilter(it) }

// PreRaidFilter keeps only LEGENDARY/TREASURED items within the raid-eligible set.
func PreRaidFilter(it store.ScorableItem) bool {
	return (it.Tier == "LEGENDARY" || it.Tier == "TREASURED") && RaidFilter(it)
}
```

- [ ] **Step 4: Run the filter test to verify it passes**

Run: `go test ./internal/bis/ -run TestTierFilters -v`
Expected: PASS. (`go build ./...` still green — nothing references these yet.)

- [ ] **Step 5: Write the failing `RankSlotUpgrades` test** (append to `internal/bis/loadoutset_test.go`)

```go
func TestRankSlotUpgrades(t *testing.T) {
	lo := store.Loadout{Main: model.Weapon{AvgDamage: 160, MinDamage: 100, MaxDamage: 220, DelaySecs: 4}}
	set := NewSet(model.StatBlock{}, lo, 1.0, 600)
	set.Equipped["Head"] = []store.ScorableItem{{ID: 1, Slot: "Head"}}
	set.Equipped["Hands"] = []store.ScorableItem{{ID: 2, Slot: "Hands"}}
	optimizable := map[string]bool{"Head": true, "Hands": true}
	bySlot := map[string][]store.ScorableItem{
		"Head": {
			{Name: "BigHead", Tier: "FABLED", Slot: "Head", Stats: model.StatBlock{MultiAttack: 40}},
			{Name: "MidHead", Tier: "LEGENDARY", Slot: "Head", Stats: model.StatBlock{MultiAttack: 20}},
		},
		"Hands": {
			{Name: "SmallHands", Tier: "LEGENDARY", Slot: "Hands", Stats: model.StatBlock{MultiAttack: 5}},
		},
	}

	got := RankSlotUpgrades(set, bySlot, optimizable, 0)
	require.Len(t, got, 2)
	// Head ranks first (larger primary ΔDPS) and carries an alternative.
	require.Equal(t, "Head", got[0].Slot)
	require.Equal(t, "BigHead", got[0].Best.Name)
	require.Equal(t, "FABLED", got[0].Best.Tier)
	require.NotNil(t, got[0].Alt)
	require.Equal(t, "MidHead", got[0].Alt.Name)
	require.Greater(t, got[0].Best.Delta, got[0].Alt.Delta) // alt never outranks best
	// Hands second, single candidate => no alternative.
	require.Equal(t, "Hands", got[1].Slot)
	require.Nil(t, got[1].Alt)

	// topN limits by PRIMARY ΔDPS.
	top1 := RankSlotUpgrades(set, bySlot, optimizable, 1)
	require.Len(t, top1, 1)
	require.Equal(t, "Head", top1[0].Slot)
}
```

- [ ] **Step 6: Run it to verify it fails**

Run: `go test ./internal/bis/ -run TestRankSlotUpgrades -v`
Expected: FAIL — `RankSlotUpgrades` / `SlotUpgrade` / `UpgradeOption` undefined.

- [ ] **Step 7: Add the types + function to `internal/bis/loadoutset.go`**

Add `"sort"` to the import block, then append:
```go
// UpgradeOption is one candidate upgrade for a slot: its name, item tier (for
// display), and in-context ΔDPS over the equipped item (Set.UpgradeDelta).
type UpgradeOption struct {
	Name  string
	Tier  string
	Delta float64
}

// SlotUpgrade is a slot's best upgrade plus an optional second-best alternative.
type SlotUpgrade struct {
	Slot string
	Best UpgradeOption
	Alt  *UpgradeOption // nil when the slot has only one positive upgrade in the pool
}

// RankSlotUpgrades returns, per optimizable slot, the best and second-best positive
// UpgradeDelta candidate from bySlot, ranked across slots by the BEST (primary)
// candidate's ΔDPS and limited to topN slots (topN <= 0 = no limit). The alternative
// is informational and never affects ordering. Slots with no positive upgrade are
// omitted.
func RankSlotUpgrades(set *Set, bySlot map[string][]store.ScorableItem, optimizable map[string]bool, topN int) []SlotUpgrade {
	var out []SlotUpgrade
	for slot := range optimizable {
		var cands []UpgradeOption
		for _, c := range bySlot[slot] {
			if d := set.UpgradeDelta(slot, c); d > 0 {
				cands = append(cands, UpgradeOption{Name: c.Name, Tier: c.Tier, Delta: d})
			}
		}
		if len(cands) == 0 {
			continue
		}
		sort.Slice(cands, func(i, j int) bool { return cands[i].Delta > cands[j].Delta })
		su := SlotUpgrade{Slot: slot, Best: cands[0]}
		if len(cands) > 1 {
			alt := cands[1]
			su.Alt = &alt
		}
		out = append(out, su)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Best.Delta > out[j].Best.Delta })
	if topN > 0 && len(out) > topN {
		out = out[:topN]
	}
	return out
}
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test ./internal/bis/ -run "TestTierFilters|TestRankSlotUpgrades" -v` → PASS. Then full `go test ./internal/bis/` and `go build ./...`.

- [ ] **Step 9: Commit**

```bash
git add internal/bis/filters.go internal/bis/filters_test.go internal/bis/loadoutset.go internal/bis/loadoutset_test.go
git commit -m "feat(bis): tier filters + RankSlotUpgrades (best + alternative, ranked by primary)"
```

---

## Task 2: Rewire `runLoadoutReport` + `renderLoadoutReport` into three buckets

**Files:**
- Modify: `cmd/bis/main.go`

There are no `cmd` tests; verify by `go build`/`go vet` and a manual run. Reuse the Task-1 `bis` functions.

- [ ] **Step 1: DRY the from-scratch tier loop + lock path to the shared filters**

In `cmd/bis/main.go`, replace the inline tier keep predicates. The tier slice becomes:
```go
	tiers := []struct {
		name    string
		profile model.StatBlock
		keep    func(store.ScorableItem) bool
	}{
		{"PRE-RAID", solo, bis.PreRaidFilter},
		{"RAID", raid, bis.RaidFilter},
		{"BEST-OF-BEST", raid, bis.BestFilter},
	}
```
Then find the `--lock` re-model path: replace its `bis.SlotCandidates(items, func(it store.ScorableItem) bool { return !bis.IsAvatar(it) && notExcluded(it) })` with `bis.SlotCandidates(items, bis.RaidFilter)`. After both edits, the `notExcluded := func(it store.ScorableItem) bool { return !bis.IsHunters(it) && !bis.Curated(it) }` local is unreferenced — delete it. (Run `go build ./...` after to confirm no other references remain; if one does, replace `notExcluded(it)` → `bis.BestFilter(it)` and `!bis.IsAvatar(it) && notExcluded(it)` → `bis.RaidFilter(it)`.)

- [ ] **Step 2: Remove the old `upgrade` struct and add a bucket type**

Delete:
```go
type upgrade struct {
	Slot, Best string
	Delta      float64
}
```
Add (near the other report types):
```go
type bucketReport struct {
	Title    string
	Upgrades []bis.SlotUpgrade
}
```

- [ ] **Step 3: Rewrite `runLoadoutReport`**

Replace the body from the `bySlot := ...` line through the `md := renderLoadoutReport(...)` line with:
```go
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

	// Seeded optimization: re-optimize the optimizable slots from the raid pool with
	// fixed slots (charm/ranged/ammo/event) locked from the imported set.
	raidBySlot := bis.SlotCandidates(items, bis.RaidFilter)
	locked := map[string][]store.ScorableItem{}
	for slot, eq := range set.Equipped {
		if !optimizable[slot] {
			locked[slot] = eq
		}
	}
	seeded := bis.BuildSet(profile, lo, raidBySlot, locked, maxBuildPasses, classData.AutoAttackMultiplier, fight)

	md := renderLoadoutReport(f, current, seeded.DPS(), reports)
```
(Keep the surrounding `loadout.Read`, the `os.WriteFile`, and the stdout prints unchanged.)

- [ ] **Step 4: Rewrite `renderLoadoutReport`**

Replace the whole function with:
```go
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
```

- [ ] **Step 5: Build, vet, full test**

Run: `go build ./...` (green), `go vet ./...` (clean), `go test ./...` (all pass — the from-scratch tier behavior is unchanged because `bis.{PreRaid,Raid,Best}Filter` are byte-equivalent to the old inline predicates; if any tier/score test fails, the filters diverged — fix the filter, do not edit the test).

- [ ] **Step 6: Commit**

```bash
git add cmd/bis/main.go
git commit -m "feat(bis): tiered upgrade report — get-now/raid/best-of-best buckets, best+alt per slot"
```

---

## Task 3: End-to-end verification on the real loadout

**Files:** none (verification only).

- [ ] **Step 1: Regenerate and inspect the report**

Run (the loadout file `characters/biffels-loadout.toml` already exists from prior import; `bis.db` already has the source column):
```bash
go run ./cmd/bis --loadout characters/biffels-loadout.toml --out loadout-report.md
cat loadout-report.md
```
Expected: a report with **three sections** — "Get now — pre-raid accessible", "Raid look-out", "Best-of-best (aspirational)" — each a table of up to `--top` (default 3) slots, ranked by the primary upgrade's ΔDPS, with `↳ alt:` rows where a second upgrade exists. The current-set DPS line and seeded-optimization total remain. Confirm: (a) no haste-effect double-counters reappear as top picks (plan 2x still holds), (b) a slot's `alt` row never shows a larger +ΔDPS than its primary row, (c) the "Get now" bucket contains only LEGENDARY/TREASURED items, the "Best-of-best" bucket may contain MYTHICAL.

- [ ] **Step 2: Full suite + build (final gate)**

Run: `go test ./...` (all pass), `go build ./...`, `go vet ./...`.

- [ ] **Step 3: Commit (if the loadout-report.md is gitignored, nothing to commit here)**

The report output is gitignored (plan 2w). No commit needed; this task is verification only. If any code tweak was required to satisfy Step 1's expectations, commit it with `git commit -m "fix(bis): <what>"`.

---

## Self-Review

**Spec coverage (§7 Tiered upgrade report):**
- Three buckets reuse nested tier filters → Task 1 (`PreRaidFilter`/`RaidFilter`/`BestFilter`) + Task 2 (bucket loop) ✓
- One (raid) context, comparable +ΔDPS → Task 2 sims all buckets against the single `set` (built with the `raid` profile passed to `runLoadoutReport`) ✓
- Per slot best + alternative; rank by primary; top-N → Task 1 `RankSlotUpgrades` ✓
- Alternative shown, never affects ordering → `RankSlotUpgrades` sorts by `Best.Delta`; render prints `Alt` as a sub-row ✓
- Current-set DPS + seeded total retained → Task 2 `renderLoadoutReport` ✓
- Keeper signal (pre-raid item topping Raid bucket) → emerges naturally from nested `RaidFilter` including pre-raid items ✓

**Placeholder scan:** none — every step has concrete code/commands.

**Type consistency:** `UpgradeOption{Name,Tier,Delta}` / `SlotUpgrade{Slot,Best,Alt}` (Task 1) consumed by `bucketReport.Upgrades []bis.SlotUpgrade` and `renderLoadoutReport` (Task 2). `RankSlotUpgrades(set *Set, bySlot map[string][]store.ScorableItem, optimizable map[string]bool, topN int)` signature matches the Task-2 call. `bis.PreRaidFilter`/`RaidFilter`/`BestFilter` (Task 1) used in Task 2's tier loop, lock path, and bucket loop. Old `type upgrade struct` removed in Task 2 (it was only used by the rewritten functions).

**DRY note:** Task 2 deletes the `notExcluded` local and the inline tier predicates, routing both the from-scratch tier loop and the lock path through the shared `bis` filters — single source of truth for the accessibility tiers.
