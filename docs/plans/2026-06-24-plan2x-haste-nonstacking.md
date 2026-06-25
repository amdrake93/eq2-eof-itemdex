# Non-stacking "Haste" Item Effect Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the named "Haste" item effect max-wins (highest single equipped value, not the sum) so the optimizer and gear-import report stop double-counting two haste-effect items.

**Architecture:** Add `StatBlock.HasteEffect`, MAX-combined in `StatBlock.Add` (every other field still sums). Effect-sourced haste (`item_stats.source='effect'`, stat `attackspeed`) routes into `HasteEffect`; modifier-sourced haste stays in `Haste`. Effective haste = `Haste + HasteEffect`, used by `effDelay` and the `DeriveWeights` haste-curve bracket. Routing happens at every `StatBlock` construction point that knows the source: `store.LoadScorableItems` (the `item_stats.source` column) and the import path (`loadout.ItemStatBlock` + an effect-stats lookup). Scope: **only** the Haste effect is non-stacking; all other effect grants still sum.

**Tech Stack:** Go 1.26 (module `github.com/amdrake93/eq2-eof-itemdex`), testify/require, modernc SQLite (`:memory:` in tests). Build `go build ./...`; test `go test ./...`. Branch `character-gear-import-spec`. Authoritative spec: `docs/SPEC.md` §11 "Non-stacking 'Haste' item effect (max-wins)" + §16.1.

---

## File Structure

| File | Change |
|---|---|
| `internal/model/stats.go` | add `HasteEffect` field; `Add` maxes it; add `EffectiveHaste()` |
| `internal/model/dps.go` | `effDelay` uses `EffectiveHaste()` |
| `internal/model/weights.go` | `curveStatMarginal` positions/evaluates the haste bracket at effective haste |
| `internal/store/store.go` | `LoadScorableItems` selects `source`, routes effect-attackspeed → `HasteEffect` |
| `internal/loadout/stats.go` | replace `ItemStatGrants`/`MergeEffectStats` with `ItemStatBlock` (routed) |
| `internal/loadout/resolve.go` | `Resolve` takes an effect-stats lookup, builds via `ItemStatBlock` |
| `cmd/itemdex/main.go` | `runImport` builds the effect-stats lookup from `item-effects.csv` |

Tasks 1–4 are independent of the import path. Task 5 → Task 6 → Task 7 are sequential (import path). Run sequentially under subagent-driven-development (shared working tree).

---

## Task 1: `StatBlock.HasteEffect` field + MAX in `Add`

**Files:**
- Modify: `internal/model/stats.go`
- Test: `internal/model/stats_test.go`

- [ ] **Step 1: Write the failing test** (append to `internal/model/stats_test.go`)

```go
func TestAddMaxesHasteEffect(t *testing.T) {
	a := StatBlock{Haste: 10, HasteEffect: 25}
	b := StatBlock{Haste: 5, HasteEffect: 21}
	sum := a.Add(b)
	require.InDelta(t, 15, sum.Haste, 1e-9)        // stackable haste sums
	require.InDelta(t, 25, sum.HasteEffect, 1e-9)  // item-effect haste maxes (25 > 21)
	require.InDelta(t, 35, sum.EffectiveHaste(), 1e-9)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run TestAddMaxesHasteEffect -v`
Expected: FAIL — `HasteEffect`/`EffectiveHaste` undefined.

- [ ] **Step 3: Write minimal implementation**

In the `StatBlock` struct (after `CritBonus`), add:
```go
	HasteEffect   float64 // non-stacking named "Haste" item effect — max-wins, NOT summed (spec §11)
```

In `func (s StatBlock) Add(o StatBlock) StatBlock`, add to the returned literal (Go 1.21+ builtin `max`):
```go
		HasteEffect:   max(s.HasteEffect, o.HasteEffect),
```

Add the helper (below `Add`):
```go
// EffectiveHaste is the total haste used by the model: stackable haste (AA +
// modifier-block, in Haste) plus the non-stacking "Haste" item effect (HasteEffect,
// already resolved to the max across a set by Add). See spec §11.
func (s StatBlock) EffectiveHaste() float64 { return s.Haste + s.HasteEffect }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/model/ -run TestAddMaxesHasteEffect -v` → PASS. Then `go build ./...`.

- [ ] **Step 5: Commit**

```bash
git add internal/model/stats.go internal/model/stats_test.go
git commit -m "feat(model): StatBlock.HasteEffect (max-combined in Add) + EffectiveHaste"
```

---

## Task 2: `effDelay` uses effective haste

**Files:**
- Modify: `internal/model/dps.go`
- Test: `internal/model/dps_test.go`

- [ ] **Step 1: Write the failing test** (append to `internal/model/dps_test.go`)

```go
func TestEffectiveHasteDrivesSwingRate(t *testing.T) {
	w := Weapon{AvgDamage: 100, MinDamage: 100, MaxDamage: 100, DelaySecs: 4}
	// haste from the stat vs the item effect must drive swing rate identically.
	viaStat := AutoDPS(StatBlock{Haste: 25}, w)
	viaEffect := AutoDPS(StatBlock{HasteEffect: 25}, w)
	require.InDelta(t, viaStat, viaEffect, 1e-9)
	// and a second non-stacking effect does NOT add (handled by Add maxing) — here
	// just assert the effect contributes at all vs none:
	require.Greater(t, viaEffect, AutoDPS(StatBlock{}, w))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run TestEffectiveHasteDrivesSwingRate -v`
Expected: FAIL — `viaEffect` equals the no-haste DPS (HasteEffect not yet consumed), so `viaStat != viaEffect`.

- [ ] **Step 3: Write minimal implementation**

In `internal/model/dps.go`, `effDelay`:
```go
func effDelay(sb StatBlock, w Weapon) float64 {
	h := HasteDpsModEffect(sb.EffectiveHaste())
	return w.DelaySecs / (1 + h/100)
}
```
(Change `sb.Haste` → `sb.EffectiveHaste()`.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/model/ -run TestEffectiveHasteDrivesSwingRate -v` → PASS. Then `go test ./internal/model/` (full package — confirm no regression).

- [ ] **Step 5: Commit**

```bash
git add internal/model/dps.go internal/model/dps_test.go
git commit -m "feat(model): effDelay uses EffectiveHaste"
```

---

## Task 3: `DeriveWeights` haste bracket at effective haste

**Files:**
- Modify: `internal/model/weights.go`
- Test: `internal/model/weights_test.go`

Context: `curveStatMarginal` (weights.go) computes the haste weight as a slope between two stat values bracketing the current position on the fitted curve. Both the position and the two DPS evaluations must use **effective** haste (`Haste + HasteEffect`). `getStat`/`setStat` are intentionally left unchanged (they are shared with `score.go`); the effective-haste logic is contained in the haste branch of `curveStatMarginal`.

- [ ] **Step 1: Write the failing test** (append to `internal/model/weights_test.go`)

```go
func TestHasteWeightUsesEffectiveHaste(t *testing.T) {
	dps := func(sb StatBlock) float64 {
		// monotonic-in-effective-haste stand-in: weight should depend on Haste+HasteEffect
		return AutoDPS(sb, Weapon{AvgDamage: 100, MinDamage: 100, MaxDamage: 100, DelaySecs: 4})
	}
	// Same effective haste (60) reached two ways must yield the same haste weight.
	wA := DeriveWeights(StatBlock{Haste: 60}, dps)["haste"]
	wB := DeriveWeights(StatBlock{Haste: 35, HasteEffect: 25}, dps)["haste"]
	require.InDelta(t, wA, wB, 1e-6)
	// And it must differ from evaluating as if HasteEffect were ignored (Haste=35).
	wIgnored := DeriveWeights(StatBlock{Haste: 35}, dps)["haste"]
	require.NotInDelta(t, wB, wIgnored, 1e-6)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run TestHasteWeightUsesEffectiveHaste -v`
Expected: FAIL — `wB` currently equals `wIgnored` (the bracket positions at `Haste`=35, ignoring HasteEffect), so `wA != wB`.

- [ ] **Step 3: Write minimal implementation**

In `curveStatMarginal`, replace the position line and the return line. Current:
```go
	v := getStat(base, stat)

	var lo, hi float64
	switch stat {
	...
	}

	if hi <= lo {
		return 0
	}
	return (dps(setStat(base, stat, hi)) - dps(setStat(base, stat, lo))) / (hi - lo)
```
New:
```go
	v := getStat(base, stat)
	if stat == "haste" {
		v = base.EffectiveHaste() // curve position uses stackable + item-effect haste (spec §11)
	}

	var lo, hi float64
	switch stat {
	...
	}

	if hi <= lo {
		return 0
	}
	// For haste, lo/hi are EFFECTIVE-haste stat values; evaluate DPS at those totals
	// by adjusting only the stackable Haste field (HasteEffect is fixed in base).
	loSet, hiSet := lo, hi
	if stat == "haste" {
		loSet, hiSet = lo-base.HasteEffect, hi-base.HasteEffect
	}
	return (dps(setStat(base, stat, hiSet)) - dps(setStat(base, stat, loSet))) / (hi - lo)
```
(Leave the `switch` body — multiattack/haste/dpsmod/mainstat bracket computation — unchanged.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/model/ -run TestHasteWeightUsesEffectiveHaste -v` → PASS. Then `go test ./internal/model/` (full package).

- [ ] **Step 5: Commit**

```bash
git add internal/model/weights.go internal/model/weights_test.go
git commit -m "feat(model): haste weight bracket evaluated at effective haste"
```

---

## Task 4: `LoadScorableItems` routes effect-haste → `HasteEffect`

**Files:**
- Modify: `internal/store/store.go`
- Test: `internal/store/store_test.go`

Context: the catalog/optimizer path. `item_stats` has a `source` column (`'modifier'` | `'effect'`). The current `LoadScorableItems` query selects only `stat, value` and sums everything into `Mods` → `AddModifiers` (so effect-haste lands in `Haste`). Add `source`; route `source='effect'` + `stat='attackspeed'` into `Stats.HasteEffect` (max) instead.

- [ ] **Step 1: Write the failing test** (append to `internal/store/store_test.go`; follow the existing `Open(":memory:")` + `Init()` pattern)

```go
func TestLoadScorableItemsRoutesEffectHaste(t *testing.T) {
	db, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	require.NoError(t, db.Init())

	// One assassin item; haste from BOTH a modifier row and an effect row.
	_, err = db.SQL().Exec(`INSERT INTO items (id, name, slot, classes, weapon_min_dmg, weapon_max_dmg, delay)
		VALUES (1, 'Test Cloak', 'Cloak', 'assassin', 0, 0, 0)`)
	require.NoError(t, err)
	_, err = db.SQL().Exec(`INSERT INTO item_stats (item_id, stat, value, source) VALUES
		(1, 'attackspeed', 7, 'modifier'),
		(1, 'attackspeed', 25, 'effect'),
		(1, 'critchance', 2, 'effect')`)
	require.NoError(t, err)

	items, err := db.LoadScorableItems()
	require.NoError(t, err)
	require.Len(t, items, 1)
	it := items[0]
	require.InDelta(t, 7, it.Stats.Haste, 1e-9)         // modifier-source haste only
	require.InDelta(t, 25, it.Stats.HasteEffect, 1e-9)  // effect-source haste routed to HasteEffect
	require.InDelta(t, 2, it.Stats.CritChance, 1e-9)    // non-haste effect stat still folded normally
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestLoadScorableItemsRoutesEffectHaste -v`
Expected: FAIL — `it.Stats.Haste` is 32 (7+25 summed) and `HasteEffect` is 0.

- [ ] **Step 3: Write minimal implementation**

In `LoadScorableItems`, change the per-item stats query + scan loop:
```go
		sr, err := d.db.Query(`SELECT stat, value, source FROM item_stats WHERE item_id = ?`, items[i].ID)
		if err != nil {
			return nil, err
		}
		for sr.Next() {
			var stat, source string
			var val float64
			if err := sr.Scan(&stat, &val, &source); err != nil {
				_ = sr.Close()
				return nil, err
			}
			if stat == "attackspeed" && source == "effect" {
				if val > items[i].Stats.HasteEffect {
					items[i].Stats.HasteEffect = val
				}
				continue
			}
			items[i].Mods[stat] += val
		}
		if err := sr.Err(); err != nil {
			_ = sr.Close()
			return nil, err
		}
		_ = sr.Close()
		items[i].Stats.AddModifiers(items[i].Mods)
```
(`Stats.HasteEffect` is set before `AddModifiers`, which does not touch it.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestLoadScorableItemsRoutesEffectHaste -v` → PASS. Then `go test ./internal/store/`.

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go
git commit -m "feat(store): route effect-source haste to StatBlock.HasteEffect"
```

---

## Task 5: `loadout.ItemStatBlock` (routed StatBlock for the import path)

**Files:**
- Modify: `internal/loadout/stats.go`
- Test: `internal/loadout/stats_test.go`

Context: the import path builds a `StatBlock` from a census item's `modifiers` block plus its effect grants. Effect grants come from two sources depending on whether the item is freshly fetched (carries `effect_list`) or cataloged (its grants live in `item-effects.csv`, passed in as `extraEffectStats`). Either way, effect-source `attackspeed` must route to `HasteEffect`. This replaces `ItemStatGrants` (which returned a flat map, losing source) and `MergeEffectStats` (which folded effect stats into `Modifiers`, conflating source).

- [ ] **Step 1: Write the failing test** (replace the body of `internal/loadout/stats_test.go` — drop the old `TestItemStatGrants*`/`TestMergeEffectStats` tests for the removed functions; keep package + imports)

```go
package loadout

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
)

func modItem(mods map[string]float64) census.Item {
	m := map[string]census.Modifier{}
	for k, v := range mods {
		m[k] = census.Modifier{Value: v}
	}
	return census.Item{Modifiers: m}
}

func TestItemStatBlockRoutesEffectHaste(t *testing.T) {
	// modifier-block haste 7 + an effect-list haste 25 (via EffectList) + a cataloged
	// effect crit 2 (via extraEffectStats, as if from item-effects.csv).
	it := modItem(map[string]float64{"attackspeed": 7})
	it.EffectList = []census.Effect{
		{Description: "When Equipped:", Indentation: 0},
		{Description: "Increases Haste of caster by 25.0.", Indentation: 1},
	}
	sb := ItemStatBlock(it, map[string]float64{"critchance": 2})
	require.InDelta(t, 7, sb.Haste, 1e-9)         // modifier-block haste stays in Haste
	require.InDelta(t, 25, sb.HasteEffect, 1e-9)  // effect-list haste → HasteEffect
	require.InDelta(t, 2, sb.CritChance, 1e-9)    // extra (cataloged) effect stat folds normally
}

func TestItemStatBlockCatalogedHaste(t *testing.T) {
	// cached item: empty EffectList, effect-haste supplied via extraEffectStats.
	it := modItem(nil)
	sb := ItemStatBlock(it, map[string]float64{"attackspeed": 25})
	require.InDelta(t, 0, sb.Haste, 1e-9)
	require.InDelta(t, 25, sb.HasteEffect, 1e-9)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/loadout/ -run TestItemStatBlock -v`
Expected: FAIL — `ItemStatBlock` undefined.

- [ ] **Step 3: Write minimal implementation**

Replace the entire contents of `internal/loadout/stats.go` with:
```go
package loadout

import (
	"github.com/amdrake93/eq2-eof-itemdex/internal/catalog"
	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
)

// ItemStatBlock builds an item's StatBlock from its modifiers block plus its effect
// grants, routing the non-stacking "Haste" item effect (effect-source attackspeed)
// into HasteEffect rather than the additive Haste field (spec §11). Effect grants
// come from the item's effect_list (freshly-fetched items) and/or extraEffectStats
// (cataloged items, whose grants are persisted in item-effects.csv). All other
// effect stats fold normally. census attackspeed key == haste.
func ItemStatBlock(it census.Item, extraEffectStats map[string]float64) model.StatBlock {
	var sb model.StatBlock

	mods := map[string]float64{}
	for k, m := range it.Modifiers {
		mods[k] += m.Value
	}
	sb.AddModifiers(mods)

	effects := map[string]float64{}
	parsed, _, _ := catalog.ParseEffects(it.EffectList)
	for k, v := range parsed {
		effects[k] += v
	}
	for k, v := range extraEffectStats {
		effects[k] += v
	}
	for k, v := range effects {
		if k == "attackspeed" {
			if v > sb.HasteEffect {
				sb.HasteEffect = v
			}
			continue
		}
		sb.AddModifiers(map[string]float64{k: v})
	}
	return sb
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/loadout/ -run TestItemStatBlock -v` → PASS.
Note: this REMOVES `ItemStatGrants` and `MergeEffectStats`. `go build ./...` will now fail at their call sites (`resolve.go`, `cmd/itemdex/main.go`) — that is expected and fixed in Task 6. Do not run the full build green here; just confirm the loadout package's new test passes (`go test ./internal/loadout/ -run TestItemStatBlock`).

- [ ] **Step 5: Commit**

```bash
git add internal/loadout/stats.go internal/loadout/stats_test.go
git commit -m "feat(loadout): ItemStatBlock routes effect-haste to HasteEffect (replaces ItemStatGrants/MergeEffectStats)"
```

---

## Task 6: `Resolve` + `runImport` use the routed builder

**Files:**
- Modify: `internal/loadout/resolve.go`
- Modify: `cmd/itemdex/main.go`
- Test: `internal/loadout/resolve_test.go`

Context: `Resolve` currently does `mods := ItemStatGrants(it); sb.AddModifiers(mods)` (Task 5 removed `ItemStatGrants`). Change it to call `ItemStatBlock(it, effectStatsLookup(it.Item.ID))`. Add an `effectStatsLookup func(int64) map[string]float64` parameter so the command can supply cataloged items' `item-effects.csv` grants; freshly-fetched items return nil and rely on their `effect_list`.

- [ ] **Step 1: Write the failing test** (update `internal/loadout/resolve_test.go`)

Update every `Resolve(...)` call in this file to pass an effect-stats lookup as the new 3rd argument (before `optimizable`). Add this test:
```go
func TestResolveRoutesEffectHasteToHasteEffect(t *testing.T) {
	ch := census.Character{EquipmentSlots: []census.EquipmentSlot{
		{Name: "cloak", Item: census.EquippedItem{ID: 1}},
	}}
	catalog := func(id int64) (census.Item, bool) {
		it := census.Item{ID: 1, DisplayName: "Cloak of Flames", Slots: []census.Slot{{Name: "Cloak"}}}
		return it, true
	}
	// cataloged item: effect-haste supplied via the effect-stats lookup (item-effects.csv).
	effects := func(id int64) map[string]float64 { return map[string]float64{"attackspeed": 25} }
	optimizable := func(string) bool { return true }

	f, miss := Resolve(ch, catalog, effects, optimizable)
	require.Empty(t, miss)
	require.Len(t, f.Slots, 1)
	require.InDelta(t, 0, f.Slots[0].Stats.Haste, 1e-9)
	require.InDelta(t, 25, f.Slots[0].Stats.HasteEffect, 1e-9)
}
```
For the existing `TestResolveItemStats` / `TestResolveOffHandWeaponGetsSecondarySlot` / `TestResolveItemEffectStatsCounted` / `TestResolveReportsMissing`: add the new arg as `func(int64) map[string]float64 { return nil }` (these exercise modifier-block and effect_list paths, not cataloged effect-stats). In `TestResolveItemEffectStatsCounted`, the item's haste comes via `effect_list`, so the expectation moves from `Stats.Haste == 25` to **`Stats.HasteEffect == 25`** (effect-haste now routes to HasteEffect).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/loadout/ -run TestResolve -v`
Expected: FAIL — `Resolve` signature mismatch (old 3-arg calls) / `TestResolveItemEffectStatsCounted` haste assertion.

- [ ] **Step 3: Write minimal implementation**

In `internal/loadout/resolve.go`, change the signature and the item-stat construction:
```go
func Resolve(
	ch census.Character,
	catalogLookup func(int64) (census.Item, bool),
	effectStatsLookup func(int64) map[string]float64,
	optimizable func(catalogSlot string) bool,
) (f File, missItems []int64) {
```
Inside the loop, replace:
```go
		mods := ItemStatGrants(it)
		sb := model.StatBlock{}
		sb.AddModifiers(mods)
```
(or whatever the current `ItemStatGrants`-based block is) with:
```go
		sb := ItemStatBlock(it, effectStatsLookup(slot.Item.ID))
```
Keep the rest of the loop (the `slot.Name == "secondary"` off-hand fix, weapon fields, `Optimizable`, `SlotEntry` build) unchanged. Remove the now-unused `model` import if it is no longer referenced.

In `cmd/itemdex/main.go` `runImport`:
- Build the effect-stats index from `item-effects.csv` (the file is already opened there for the old `MergeEffectStats` path). Replace the `MergeEffectStats` block with:
```go
	effectByID := map[int64]map[string]float64{}
	if ef, err := os.Open(filepath.Join(*dir, "item-effects.csv")); err == nil {
		effectStats, err := catalog.ReadEffectStatsCSV(ef)
		_ = ef.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error reading item-effects.csv:", err)
			os.Exit(1)
		}
		for _, es := range effectStats {
			id := int64(es.ItemID)
			if effectByID[id] == nil {
				effectByID[id] = map[string]float64{}
			}
			effectByID[id][es.Stat] += es.Value
		}
	} else if !os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "error opening item-effects.csv:", err)
		os.Exit(1)
	}
	effectStatsLookup := func(id int64) map[string]float64 { return effectByID[id] }
```
- Remove the `cachedItems = loadout.MergeEffectStats(...)` line (cached items keep their pure `modifiers` block; effect grants now flow through `effectStatsLookup`). Build `catIndex` from the plain `cachedItems`.
- Update BOTH `loadout.Resolve(...)` calls to pass `effectStatsLookup` as the new 3rd argument:
```go
	_, missItems := loadout.Resolve(ch, catLookup, effectStatsLookup, bis.OptimizableSlot)
	...
	f, missItems2 := loadout.Resolve(ch, catLookup, effectStatsLookup, bis.OptimizableSlot)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/loadout/ -run TestResolve -v` → PASS. Then `go build ./...` (now green again) and full `go test ./...`.

- [ ] **Step 5: Commit**

```bash
git add internal/loadout/resolve.go cmd/itemdex/main.go internal/loadout/resolve_test.go
git commit -m "feat(loadout): Resolve/runImport route effect-haste via ItemStatBlock + effect-stats lookup"
```

---

## Task 7: End-to-end regression — two effect-haste items don't double-count

**Files:**
- Test: `internal/bis/loadoutset_test.go`

- [ ] **Step 1: Write the failing test** (append to `internal/bis/loadoutset_test.go`)

```go
func TestTwoEffectHasteItemsDoNotStack(t *testing.T) {
	lo := store.Loadout{Main: model.Weapon{AvgDamage: 160, MinDamage: 100, MaxDamage: 220, DelaySecs: 4}}
	set := NewSet(model.StatBlock{}, lo, 1.0, 600)
	set.Equipped["Cloak"] = []store.ScorableItem{
		{ID: 1, Name: "Cloak of Flames", Slot: "Cloak", Stats: model.StatBlock{HasteEffect: 25}},
	}
	withCloak := set.DPS()

	// A second effect-haste item (21) is REDUNDANT — its haste shouldn't add (max-wins),
	// so its CandidateDelta for the Hands slot is ~0 (it has no other stats).
	redundant := store.ScorableItem{ID: 2, Name: "Lesser Haste Gloves", Slot: "Hands", Stats: model.StatBlock{HasteEffect: 21}}
	require.InDelta(t, 0, set.CandidateDelta("Hands", redundant), 1e-6)

	// A BIGGER effect-haste item (35) DOES help — it raises the max from 25 to 35.
	bigger := store.ScorableItem{ID: 3, Name: "Greater Haste Gloves", Slot: "Hands", Stats: model.StatBlock{HasteEffect: 35}}
	require.Greater(t, set.CandidateDelta("Hands", bigger), 0.0)

	_ = withCloak
}
```

- [ ] **Step 2: Run test to verify it fails or passes**

Run: `go test ./internal/bis/ -run TestTwoEffectHasteItemsDoNotStack -v`
Expected: PASS (Tasks 1–6 already implement the behavior). If it FAILS, the set-aggregation path isn't routing through `Add`'s max — investigate `restBase`/`CandidateDelta` before proceeding (do not edit the test to pass).

- [ ] **Step 3: Full suite + build**

Run: `go test ./...` (all pass) and `go build ./...` (clean) and `go vet ./...`.

- [ ] **Step 4: Commit**

```bash
git add internal/bis/loadoutset_test.go
git commit -m "test(bis): two effect-haste items max-win (no double-count)"
```

---

## Self-Review

**Spec coverage (§11 + §16.1):**
- `StatBlock.HasteEffect` field + MAX in `Add` → Task 1 ✓
- Effective haste = `Haste + HasteEffect` for `effDelay` → Task 2 ✓; for `DeriveWeights` bracket → Task 3 ✓
- Source routing — `store.LoadScorableItems` → Task 4 ✓; import (`ItemStatBlock` + lookup, replacing `ItemStatGrants`/`MergeEffectStats`) → Tasks 5–6 ✓
- Loadout file carries `HasteEffect` automatically (it's a `StatBlock` field) → no task needed; covered by Task 6's `TestResolveRoutesEffectHasteToHasteEffect` (the resolved `SlotEntry.Stats.HasteEffect`) ✓
- Scope: only `attackspeed`+`effect` routed; all other effect stats still fold via `AddModifiers` → enforced in Tasks 4 & 5 ✓
- Emergent optimizer behavior (redundant 2nd haste item ΔDPS ≈ 0; bigger one helps) → Task 7 ✓

**Placeholder scan:** none — every step has concrete code and exact commands.

**Type consistency:** `HasteEffect` / `EffectiveHaste()` (T1) used in T2 (`effDelay`), T3 (`curveStatMarginal`), T4 (`store`), T5 (`ItemStatBlock`), T7. `ItemStatBlock(it census.Item, extraEffectStats map[string]float64) model.StatBlock` (T5) called in T6. `Resolve(ch, catalogLookup, effectStatsLookup, optimizable)` new signature (T6) matches both call sites in `cmd/itemdex/main.go`. `getStat`/`setStat` deliberately unchanged (shared with `score.go`); effective-haste contained in `curveStatMarginal`.

**Cross-task build note:** Task 5 removes `ItemStatGrants`/`MergeEffectStats`, breaking the build until Task 6 rewires the call sites — flagged in Task 5 Step 4. Run Tasks 5 and 6 back-to-back.
