# Upgrade-Report Redesign + EQ2U Links + Committed Reports Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign the `bis --loadout` upgrade report into a per-slot-instance "spreadsheet" table with clickable EQ2U item links, total-DPS-relative percentages, and no tier tags; add the same EQ2U links to `bis-report.md`; and commit the per-character generated reports so GitHub renders them.

**Architecture:** The report's row unit becomes a physical **slot instance** (two-capacity `Ear`/`Finger`/`Wrist` slots emit two rows). Each row's upgrade ΔDPS is computed by a new `Set` method that swaps one specific worn item for a candidate while holding the slot's other instance fixed. A shared `EQ2ULink` helper turns item names into `https://u.eq2wire.com/item/<id>` markdown links. The `--top` flag gains a per-mode default. Spec: `docs/SPEC.md` §7 ("Tiered upgrade report") and §8.

**Tech Stack:** Go (stdlib + `testify/require`), packages `internal/bis`, `internal/store`, `internal/model`, `cmd/bis`.

---

## File Structure

- `internal/bis/render.go` — add exported `EQ2ULink`; update `writeScored` to link the item name and drop the in-game gamelink suffix.
- `internal/bis/set.go` — add per-instance helpers: `slotDPS`, `ReplaceInstanceDelta`, `EquippedInstanceValue`.
- `internal/bis/loadoutset.go` — change `UpgradeOption` (drop `Tier`, add `ID`) and `SlotUpgrade` (add `EquippedName`/`EquippedID`/`EquippedValue`); rewrite `RankSlotUpgrades` for per-instance rows; **remove** the now-unused `UpgradeDelta`.
- `cmd/bis/main.go` — rewrite `renderLoadoutReport` for the 4-column linked table; add `upgradeCell` helper; give `--top` a per-mode default.
- Tests: `internal/bis/render_test.go`, `internal/bis/set_test.go`, `internal/bis/loadoutset_test.go`, new `cmd/bis/main_test.go`.
- `.gitignore` — remove the three `/characters/*/` report lines.
- `characters/biffels/` — regenerate + commit `loadout.toml`, `upgrade-report.md`, `bis-report.md`.

**Key facts for the implementer:**
- `capacityOf(slot)` already exists in `internal/bis/build.go` (`Ear`/`Finger`/`Wrist`/`Charm` = 2, else 1). Reuse it — do NOT add a new capacity map. (`Charm` is not optimizable, so it never reaches the upgrade report.)
- `Set.Equipped` is `map[string][]store.ScorableItem`; items are appended in import order by `SetFromLoadout`. `store.ScorableItem` has `ID int`, `Name string`, `Stats model.StatBlock`, and `IsWeapon() bool` (`WeaponDelay > 0`).
- `offHandSlot = "Secondary"` (const in `set.go`). The off-hand weapon is derived from `Set.Equipped["Secondary"]`.
- `model.TotalDPSDual(sb StatBlock, main, off Weapon, cas, classAutoMult, fightLen) float64`.
- `renderLoadoutReport` already receives `current` (= `set.DPS()`); use it for the `%`.

---

## Task 1: `EQ2ULink` helper + link item names in `bis-report.md`

**Files:**
- Modify: `internal/bis/render.go`
- Test: `internal/bis/render_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/bis/render_test.go`:

```go
func TestEQ2ULink(t *testing.T) {
	require.Equal(t,
		"[Cloak of Flames](https://u.eq2wire.com/item/264598753)",
		EQ2ULink("Cloak of Flames", 264598753))

	// No catalog id -> plain text, no link.
	require.Equal(t, "Empty", EQ2ULink("Empty", 0))
	require.Equal(t, "Mystery", EQ2ULink("Mystery", -1))
}
```

If `internal/bis/render_test.go` does not already import `testify/require`, add it to the import block.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bis/ -run TestEQ2ULink`
Expected: FAIL — `undefined: EQ2ULink`.

- [ ] **Step 3: Implement `EQ2ULink` and update `writeScored`**

In `internal/bis/render.go`, add the helper (place it just above `writeScored`):

```go
// EQ2ULink renders an item name as a markdown link to its EQ2U page. Items
// without a catalog id (id <= 0) render as plain text.
func EQ2ULink(name string, id int) string {
	if id <= 0 {
		return name
	}
	return fmt.Sprintf("[%s](https://u.eq2wire.com/item/%d)", name, id)
}
```

Replace the body of `writeScored` so the name links to EQ2U and the in-game gamelink suffix is gone:

```go
func writeScored(b *strings.Builder, s ScoredItem) {
	fmt.Fprintf(b, "- **%s** [%s] — +%.1f DPS\n", EQ2ULink(s.Item.Name, s.Item.ID), rarityTag(s.Item), s.Delta)
	for i, term := range s.Terms {
		if i >= 4 {
			break
		}
		fmt.Fprintf(b, "  - %s %.0f × %.2f = %.1f\n", term.Stat, term.ItemValue, term.Weight, term.Contribution)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/bis/ -run 'TestEQ2ULink|Render'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/bis/render.go internal/bis/render_test.go
git commit -m "feat(bis): EQ2U item links; drop dead in-game gamelink in bis-report"
```

---

## Task 2: Per-instance `Set` delta helpers

**Files:**
- Modify: `internal/bis/set.go`
- Test: `internal/bis/set_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/bis/set_test.go`:

```go
func TestReplaceInstanceDeltaHoldsOtherInstanceFixed(t *testing.T) {
	set := NewSet(model.StatBlock{}, testLoadout(), 1.0, 600)
	strong := store.ScorableItem{ID: 1, Slot: "Finger", Stats: model.StatBlock{MultiAttack: 40}}
	weak := store.ScorableItem{ID: 2, Slot: "Finger", Stats: model.StatBlock{MultiAttack: 5}}
	set.Equipped["Finger"] = []store.ScorableItem{strong, weak}

	cand := store.ScorableItem{ID: 3, Slot: "Finger", Stats: model.StatBlock{MultiAttack: 30}}

	// Replacing the WEAK ring (idx 1) with a better one is a positive gain.
	upWeak := set.ReplaceInstanceDelta("Finger", 1, cand)
	require.Greater(t, upWeak, 0.0)

	// Replacing the STRONG ring (idx 0) with the same candidate is a smaller gain
	// (or a loss) — the two instances are evaluated independently.
	upStrong := set.ReplaceInstanceDelta("Finger", 0, cand)
	require.Greater(t, upWeak, upStrong)

	// Filling an empty position (idx -1) ADDS the candidate alongside both rings.
	add := set.ReplaceInstanceDelta("Finger", -1, cand)
	require.Greater(t, add, 0.0)
}

func TestEquippedInstanceValueIsMarginalContribution(t *testing.T) {
	set := NewSet(model.StatBlock{}, testLoadout(), 1.0, 600)
	ring := store.ScorableItem{ID: 1, Slot: "Finger", Stats: model.StatBlock{MultiAttack: 40}}
	set.Equipped["Finger"] = []store.ScorableItem{ring}

	// The worn ring contributes positive DPS vs the slot without it.
	require.Greater(t, set.EquippedInstanceValue("Finger", 0), 0.0)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bis/ -run 'ReplaceInstanceDelta|EquippedInstanceValue'`
Expected: FAIL — `set.ReplaceInstanceDelta undefined` / `set.EquippedInstanceValue undefined`.

- [ ] **Step 3: Implement the helpers**

Append to `internal/bis/set.go`:

```go
// slotDPS computes full-set DPS with `slot`'s equipped items REPLACED by `items`,
// every other slot held fixed. For the off-hand slot the off-hand weapon is
// re-derived from `items`; otherwise the current off-hand is kept.
func (s *Set) slotDPS(slot string, items []store.ScorableItem) float64 {
	rb := s.restBase(slot)
	for _, it := range items {
		rb = rb.Add(it.Stats)
	}
	off := s.offWeapon()
	if slot == offHandSlot {
		off = model.Weapon{}
		for _, it := range items {
			if it.IsWeapon() {
				off = model.Weapon{AvgDamage: it.WeaponAvg, MinDamage: it.WeaponMin, MaxDamage: it.WeaponMax, DelaySecs: it.WeaponDelay}
				break
			}
		}
	}
	return model.TotalDPSDual(rb, s.Main, off, s.Arts, s.AutoMult, s.FightLen)
}

// ReplaceInstanceDelta is the ΔDPS of swapping the worn item at index `idx` in
// `slot` for candidate `c`, holding every other equipped item — including the
// slot's other instance(s) — fixed. idx == -1 fills an empty position (appends c,
// replacing nothing).
func (s *Set) ReplaceInstanceDelta(slot string, idx int, c store.ScorableItem) float64 {
	cur := s.Equipped[slot]
	swapped := make([]store.ScorableItem, 0, len(cur)+1)
	for i, it := range cur {
		if i == idx {
			continue
		}
		swapped = append(swapped, it)
	}
	swapped = append(swapped, c)
	return s.slotDPS(slot, swapped) - s.DPS()
}

// EquippedInstanceValue is the marginal slot-DPS contribution of the worn item at
// index `idx` in `slot`: current DPS minus DPS with that item removed (others fixed).
func (s *Set) EquippedInstanceValue(slot string, idx int) float64 {
	cur := s.Equipped[slot]
	without := make([]store.ScorableItem, 0, len(cur))
	for i, it := range cur {
		if i == idx {
			continue
		}
		without = append(without, it)
	}
	return s.DPS() - s.slotDPS(slot, without)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/bis/ -run 'ReplaceInstanceDelta|EquippedInstanceValue|Set'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/bis/set.go internal/bis/set_test.go
git commit -m "feat(bis): per-instance replace-delta and equipped-contribution helpers"
```

---

## Task 3: Per-instance `RankSlotUpgrades` + type changes

**Files:**
- Modify: `internal/bis/loadoutset.go`
- Test: `internal/bis/loadoutset_test.go`

- [ ] **Step 1: Update the failing tests**

In `internal/bis/loadoutset_test.go`:

1. **Delete** `TestUpgradeDeltaIsSwapGainNotStandalone` entirely (it tested the removed `UpgradeDelta`).

2. **Replace** `TestRankSlotUpgrades` with this per-instance version:

```go
func TestRankSlotUpgrades(t *testing.T) {
	lo := store.Loadout{Main: model.Weapon{AvgDamage: 160, MinDamage: 100, MaxDamage: 220, DelaySecs: 4}}
	set := NewSet(model.StatBlock{}, lo, 1.0, 600)
	// Finger is a two-capacity slot: one strong ring, one weak ring.
	set.Equipped["Finger"] = []store.ScorableItem{
		{ID: 10, Name: "StrongRing", Slot: "Finger", Stats: model.StatBlock{MultiAttack: 40}},
		{ID: 11, Name: "WeakRing", Slot: "Finger", Stats: model.StatBlock{MultiAttack: 5}},
	}
	optimizable := map[string]bool{"Finger": true}
	// One candidate strong enough to beat BOTH worn rings, so both instances yield
	// a positive upgrade row.
	bySlot := map[string][]store.ScorableItem{
		"Finger": {
			{ID: 20, Name: "BigRing", Tier: "FABLED", Slot: "Finger", Stats: model.StatBlock{MultiAttack: 50}},
		},
	}

	got := RankSlotUpgrades(set, bySlot, optimizable, 0)

	// Two-capacity slot -> two instance rows, both labelled "Finger".
	require.Len(t, got, 2)
	require.Equal(t, "Finger", got[0].Slot)
	require.Equal(t, "Finger", got[1].Slot)

	// Rows are ranked by best delta: replacing the WEAK ring ranks first.
	require.Equal(t, "WeakRing", got[0].EquippedName)
	require.Equal(t, 11, got[0].EquippedID)
	require.Greater(t, got[0].EquippedValue, 0.0)
	require.Equal(t, "BigRing", got[0].Best.Name)
	require.Equal(t, 20, got[0].Best.ID)
	require.Greater(t, got[0].Best.Delta, got[1].Best.Delta)

	require.Equal(t, "StrongRing", got[1].EquippedName)
}

func TestRankSlotUpgradesEmptyPositionRow(t *testing.T) {
	lo := store.Loadout{Main: model.Weapon{AvgDamage: 160, MinDamage: 100, MaxDamage: 220, DelaySecs: 4}}
	set := NewSet(model.StatBlock{}, lo, 1.0, 600)
	// Only ONE ring worn in a two-capacity slot: the second position is empty.
	set.Equipped["Finger"] = []store.ScorableItem{
		{ID: 10, Name: "OnlyRing", Slot: "Finger", Stats: model.StatBlock{MultiAttack: 40}},
	}
	optimizable := map[string]bool{"Finger": true}
	// NewRing beats OnlyRing, so the worn-item row is also a positive upgrade —
	// giving two rows total: the OnlyRing row and the Empty row.
	bySlot := map[string][]store.ScorableItem{
		"Finger": {{ID: 20, Name: "NewRing", Slot: "Finger", Stats: model.StatBlock{MultiAttack: 50}}},
	}

	got := RankSlotUpgrades(set, bySlot, optimizable, 0)
	require.Len(t, got, 2)

	var foundEmpty bool
	for _, su := range got {
		if su.EquippedName == "Empty" {
			foundEmpty = true
			require.Equal(t, 0, su.EquippedID)
			require.Equal(t, 0.0, su.EquippedValue)
		}
	}
	require.True(t, foundEmpty, "an unfilled position should produce an Empty row")
}

func TestRankSlotUpgradesTopNCapsRows(t *testing.T) {
	lo := store.Loadout{Main: model.Weapon{AvgDamage: 160, MinDamage: 100, MaxDamage: 220, DelaySecs: 4}}
	set := NewSet(model.StatBlock{}, lo, 1.0, 600)
	set.Equipped["Finger"] = []store.ScorableItem{
		{ID: 10, Name: "WeakRing", Slot: "Finger", Stats: model.StatBlock{MultiAttack: 5}},
		{ID: 11, Name: "WeakRing2", Slot: "Finger", Stats: model.StatBlock{MultiAttack: 5}},
	}
	optimizable := map[string]bool{"Finger": true}
	bySlot := map[string][]store.ScorableItem{
		"Finger": {{ID: 20, Name: "BigRing", Slot: "Finger", Stats: model.StatBlock{MultiAttack: 40}}},
	}

	require.Len(t, RankSlotUpgrades(set, bySlot, optimizable, 1), 1)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/bis/ -run 'RankSlotUpgrades'`
Expected: FAIL — compile errors (`SlotUpgrade` has no field `EquippedName`, `UpgradeOption` has no field `ID`, etc.).

- [ ] **Step 3: Rewrite the types, `RankSlotUpgrades`, and remove `UpgradeDelta`**

In `internal/bis/loadoutset.go`:

(a) **Delete** the `UpgradeDelta` method entirely (the whole doc comment + func, currently lines ~12–31). It is no longer referenced.

(b) Replace the `UpgradeOption` and `SlotUpgrade` definitions with:

```go
// UpgradeOption is one candidate upgrade for a slot instance: its catalog id (for
// the EQ2U link), name, and in-context ΔDPS over the worn item it would replace.
type UpgradeOption struct {
	ID    int
	Name  string
	Delta float64
}

// SlotUpgrade is one physical slot instance: the worn item (name/id/slot-DPS
// value, or "Empty" with id 0 and value 0), its best upgrade, and an optional
// second-best alternative.
type SlotUpgrade struct {
	Slot          string
	EquippedName  string
	EquippedID    int
	EquippedValue float64
	Best          UpgradeOption
	Alt           *UpgradeOption // nil when the instance has only one positive upgrade
}
```

(c) Replace `RankSlotUpgrades` with the per-instance version:

```go
// RankSlotUpgrades returns one row per physical slot instance across the
// optimizable slots. Two-capacity slots (capacityOf) emit a row per worn item;
// any unfilled position becomes a synthetic "Empty" row (value 0). Each row's
// candidates are ranked by ReplaceInstanceDelta — the gain of swapping that one
// worn item (or filling the Empty) while the slot's other instance is held fixed
// — keeping the best plus an optional second-best alternative. Rows are ranked
// across all instances by the best candidate's ΔDPS and limited to topN rows
// (topN <= 0 = no limit). Instances with no positive upgrade are omitted.
func RankSlotUpgrades(set *Set, bySlot map[string][]store.ScorableItem, optimizable map[string]bool, topN int) []SlotUpgrade {
	var out []SlotUpgrade
	for slot := range optimizable {
		worn := set.Equipped[slot]
		for idx := 0; idx < capacityOf(slot); idx++ {
			su := SlotUpgrade{Slot: slot}
			replaceIdx := idx
			if idx < len(worn) {
				su.EquippedName = worn[idx].Name
				su.EquippedID = worn[idx].ID
				su.EquippedValue = set.EquippedInstanceValue(slot, idx)
			} else {
				su.EquippedName = "Empty"
				replaceIdx = -1
			}

			var cands []UpgradeOption
			for _, c := range bySlot[slot] {
				if d := set.ReplaceInstanceDelta(slot, replaceIdx, c); d > 0 {
					cands = append(cands, UpgradeOption{ID: c.ID, Name: c.Name, Delta: d})
				}
			}
			if len(cands) == 0 {
				continue
			}
			sort.Slice(cands, func(i, j int) bool { return cands[i].Delta > cands[j].Delta })
			su.Best = cands[0]
			if len(cands) > 1 {
				alt := cands[1]
				su.Alt = &alt
			}
			out = append(out, su)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Best.Delta > out[j].Best.Delta })
	if topN > 0 && len(out) > topN {
		out = out[:topN]
	}
	return out
}
```

(d) Remove the now-unused `"math"` import from `internal/bis/loadoutset.go` (it was only used by `UpgradeDelta`). Keep `"sort"`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/bis/`
Expected: PASS (whole package, to catch the removed import / method).

- [ ] **Step 5: Commit**

```bash
git add internal/bis/loadoutset.go internal/bis/loadoutset_test.go
git commit -m "feat(bis): per-instance upgrade rows; drop UpgradeDelta and tier tag"
```

---

## Task 4: Rewrite `renderLoadoutReport` for the linked 4-column table

**Files:**
- Modify: `cmd/bis/main.go`
- Test: `cmd/bis/main_test.go` (new)

- [ ] **Step 1: Write the failing test**

Create `cmd/bis/main_test.go`:

```go
package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/amdrake93/eq2-eof-itemdex/internal/bis"
	"github.com/amdrake93/eq2-eof-itemdex/internal/loadout"
)

func TestRenderLoadoutReportLinkedTable(t *testing.T) {
	f := loadout.File{CharacterName: "Biffels", LastUpdate: 123}
	reports := []bucketReport{{
		Title: "Get now — pre-raid accessible",
		Upgrades: []bis.SlotUpgrade{
			{
				Slot:          "Finger",
				EquippedName:  "WeakRing",
				EquippedID:    11,
				EquippedValue: 120,
				Best:          bis.UpgradeOption{ID: 20, Name: "BigRing", Delta: 300},
				Alt:           &bis.UpgradeOption{ID: 21, Name: "MidRing", Delta: 150},
			},
			{
				Slot:         "Finger",
				EquippedName: "Empty",
				Best:         bis.UpgradeOption{ID: 22, Name: "AnyRing", Delta: 400},
			},
		},
	}}

	md := renderLoadoutReport(f, 30000, 31000, reports)

	require.Contains(t, md, "| Slot | Wearing | Best upgrade | Alternative |")
	// Worn item is linked and shows its slot-DPS value.
	require.Contains(t, md, "[WeakRing](https://u.eq2wire.com/item/11) (120)")
	// Best is bold-linked with +ΔDPS and total-relative %.
	require.Contains(t, md, "**[BigRing](https://u.eq2wire.com/item/20) +300 (+1.0%)**")
	// Alternative cell present.
	require.Contains(t, md, "[MidRing](https://u.eq2wire.com/item/21) +150 (+0.5%)")
	// Empty row: plain text, no link, value 0, blank alternative cell.
	require.Contains(t, md, "| Finger | Empty (0) |")
	// No tier tags anywhere.
	require.NotContains(t, md, "[FABLED]")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/bis/ -run TestRenderLoadoutReportLinkedTable`
Expected: FAIL — current `renderLoadoutReport` emits the old columns / tier tags.

- [ ] **Step 3: Rewrite `renderLoadoutReport` and add `upgradeCell`**

In `cmd/bis/main.go`, replace the whole `renderLoadoutReport` function with:

```go
func renderLoadoutReport(f loadout.File, current, seededDPS float64, reports []bucketReport) string {
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
		fmt.Fprintf(&b, "| Slot | Wearing | Best upgrade | Alternative |\n")
		fmt.Fprintf(&b, "|------|---------|--------------|-------------|\n")
		for _, u := range r.Upgrades {
			wearing := fmt.Sprintf("%s (%.0f)", bis.EQ2ULink(u.EquippedName, u.EquippedID), u.EquippedValue)
			best := fmt.Sprintf("**%s**", upgradeCell(u.Best, current))
			alt := ""
			if u.Alt != nil {
				alt = upgradeCell(*u.Alt, current)
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", u.Slot, wearing, best, alt)
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

// upgradeCell renders one upgrade as "[name](url) +ΔDPS (+pct%)" where pct is the
// gain as a share of total current-set DPS.
func upgradeCell(o bis.UpgradeOption, setDPS float64) string {
	pct := 0.0
	if setDPS > 0 {
		pct = o.Delta / setDPS * 100
	}
	return fmt.Sprintf("%s +%.0f (+%.1f%%)", bis.EQ2ULink(o.Name, o.ID), o.Delta, pct)
}
```

This removes the old `equippedBySlot` map (the worn item now comes from `SlotUpgrade`). The `f loadout.File` parameter is still used for the header and unresolved list.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/bis/ -run TestRenderLoadoutReportLinkedTable`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/bis/main.go cmd/bis/main_test.go
git commit -m "feat(bis): linked 4-column per-instance upgrade table"
```

---

## Task 5: Per-mode `--top` default (loadout = all, from-scratch = 3)

**Files:**
- Modify: `cmd/bis/main.go`

- [ ] **Step 1: Change the flag to a sentinel default**

In `main()`, change the `--top` flag declaration from:

```go
	topN := flag.Int("top", 3, "alternatives per slot")
```

to:

```go
	topN := flag.Int("top", -1, "loadout mode: max upgrade rows per bucket (default all); from-scratch mode: alternatives per slot (default 3)")
```

- [ ] **Step 2: Resolve the default per mode**

In the `if *loadoutPath != "" {` branch, resolve to "all" (0) when unset, just before the `runLoadoutReport` call:

```go
		loadoutTop := *topN
		if loadoutTop < 0 {
			loadoutTop = 0 // all instance rows
		}
		runLoadoutReport(classData, lo, raid, items, *loadoutPath, reportPath, loadoutTop, *fight)
		return
```

For the from-scratch path, add right after the `tiers := []struct{...}{...}` block (before the `for _, t := range tiers` loop):

```go
	reportTop := *topN
	if reportTop < 0 {
		reportTop = 3 // default alternatives per slot
	}
```

Then replace both `bis.BuildSlotReports(set, bySlot, weights, *topN)` calls (in the tiers loop and the `lockIDs` block) with `bis.BuildSlotReports(set, bySlot, weights, reportTop)`.

- [ ] **Step 3: Verify the build + tests**

Run: `go build ./... && go test ./cmd/bis/ ./internal/bis/`
Expected: build succeeds; tests PASS.

- [ ] **Step 4: Verify behavior end-to-end (manual)**

Run: `go run ./cmd/bis --loadout characters/biffels/loadout.toml --out /tmp/upg.md`
Expected: stdout reports it wrote the file; `/tmp/upg.md` shows the new 4-column linked table with **all** instance rows (more than 3) per bucket.

Run: `go run ./cmd/bis --loadout characters/biffels/loadout.toml --out /tmp/upg-top2.md --top 2`
Expected: each bucket capped at 2 rows.

- [ ] **Step 5: Commit**

```bash
git add cmd/bis/main.go
git commit -m "feat(bis): per-mode --top default (loadout=all, from-scratch=3)"
```

---

## Task 6: Un-ignore and commit per-character reports

**Files:**
- Modify: `.gitignore`
- Add: `characters/biffels/loadout.toml`, `characters/biffels/upgrade-report.md`, `characters/biffels/bis-report.md`

- [ ] **Step 1: Remove the report-ignore lines from `.gitignore`**

Delete these three lines from `.gitignore`:

```
/characters/*/loadout.toml
/characters/*/upgrade-report.md
/characters/*/bis-report.md
```

Also remove the now-inaccurate comment line directly above them if it only describes these (currently `# Generated per-character outputs — local only (config.toml is committed)`).

- [ ] **Step 2: Regenerate the reports with the new code**

Run (writes into `characters/biffels/`):

```bash
go run ./cmd/bis                                            # bis-report.md
go run ./cmd/bis --loadout characters/biffels/loadout.toml  # upgrade-report.md
```

Expected: both commands print a "wrote …" line.

- [ ] **Step 3: Eyeball the rendered output**

Open `characters/biffels/upgrade-report.md` and confirm: 4-column table, item names are `[Name](https://u.eq2wire.com/item/<id>)` links, percentages present, two `Finger`/`Ear`/`Wrist` rows where applicable, any `Empty (0)` rows, no `[FABLED]`-style tags. Open `characters/biffels/bis-report.md` and confirm item names link to EQ2U with no leftover `\aITEM…\/a` text.

- [ ] **Step 4: Verify which files git now sees**

Run: `git status --porcelain characters/`
Expected: `.gitignore` plus the three `characters/biffels/*.{toml,md}` files are now tracked/untracked (not ignored).

- [ ] **Step 5: Commit**

```bash
git add .gitignore characters/biffels/loadout.toml characters/biffels/upgrade-report.md characters/biffels/bis-report.md
git commit -m "feat: commit per-character reports so GitHub renders EQ2U-linked tables"
```

---

## Final verification

- [ ] Run the full suite: `go test ./...` — expected: all PASS.
- [ ] Run `go vet ./...` — expected: clean.
- [ ] Confirm `git grep -n "GameLink" internal/bis/render.go` returns nothing (the dead gamelink suffix is gone from `writeScored`).
- [ ] Confirm `git grep -n "UpgradeDelta" internal/` returns nothing (method fully removed).
