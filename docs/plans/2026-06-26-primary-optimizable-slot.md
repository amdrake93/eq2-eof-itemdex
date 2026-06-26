# Primary (Main-Hand) as a Normal Optimizable Slot — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Task 3 is a single tightly-coupled refactor (the tree does not compile until all of its sub-steps are done) — execute it as one unit and commit once at its end.

**Goal:** Make the main-hand (`Primary`) a normal optimizable weapon slot — derived from `Equipped["Primary"]` exactly like the off-hand is from `Equipped["Secondary"]` — with weapon eligibility driven by class config, removing the hardcoded fixed Soulfire main.

**Architecture:** Delete the fixed `Set.Main` field; derive the main weapon via `mainWeapon()` (mirror of `offWeapon()`). `model.ItemDelta` gains a `newMain` override to match its `newOff`. `SlotCandidates` builds both weapon pools from class config (`dual_wield`, `weapon_wield_styles`). The coordinate-ascent optimizer fills the two weapon slots first with a no-duplicate constraint. Both reports then show/optimize Primary as an ordinary slot. Spec: `docs/SPEC.md` §6 (class TOML), §7 (Set representation / Slot candidates / Imported loadout / Coordinate-ascent optimizer), §16.4.

**Tech Stack:** Go (stdlib + `testify/require`, `BurntSushi/toml`), packages `internal/model`, `internal/charconfig`, `internal/bis`, `internal/store`, `cmd/bis`.

---

## File Structure

- `internal/model/itemdelta.go` — `ItemDelta` gains a `newMain *Weapon` param (rename `main`→`restMain`).
- `internal/charconfig/charconfig.go` — `ClassData` gains `DualWield bool`, `WeaponWieldStyles []string`; `LoadClass` validates them.
- `classes/assassin.toml` — add `dual_wield`, `weapon_wield_styles`.
- `internal/bis/set.go` — remove `Set.Main`; add `weaponFrom`, `mainWeapon()`, `restMain()`; update `NewSet`, `DPS`, `slotDPS`, `CandidateDelta`.
- `internal/bis/candidates.go` — `SlotCandidates` gains a weapon-config param; builds Primary + Secondary pools from it; no Soulfire exclusion.
- `internal/bis/loadoutset.go` — `optimizableCatalogSlots` adds `Primary`; `SetFromLoadout` drops the `set.Main` install and derives optimizability from `OptimizableSlot`.
- `internal/bis/build.go` — `BuildSet` optimizes weapon slots first with a cross-slot no-duplicate constraint; `pickBest` gains a `forbidden` set; remove the `mainHandSlot` exclusion.
- `internal/bis/report.go` — `ConvergedWeights` uses `mainWeapon()`; `BuildSlotReports` no longer skips Primary.
- `internal/bis/render.go` — update the main-hand assumptions sentence.
- `internal/store/store.go` — `LoadLoadout` drops the Soulfire query; `Loadout` drops `Main`/`MainName`.
- `cmd/bis/main.go` — remove `withFixedPrimary`, `findByName(lo.MainName)`, `profile.Add(mainItem.Stats)`, the `MainName` summary; thread weapon config into every `SlotCandidates` call.
- Tests across `internal/model`, `internal/bis`, `internal/store`.

**Key facts:**
- `mainHandSlot = "Primary"` const lives in `internal/bis/build.go`; `offHandSlot = "Secondary"` in `set.go`. Both usable package-wide.
- Weapons are catalogued under slot `Primary` with `WieldStyle` `"One-Handed"` (188) / `"Two-Handed"` (3); both Soulfires are `Primary`/`One-Handed`.
- The importer already sets `SlotEntry.Optimizable` via `bis.OptimizableSlot` (`internal/loadout/resolve.go:49`), so it is purely slot-based; `SetFromLoadout` can use `OptimizableSlot(e.CatalogSlot)` directly and ignore the (possibly stale) file flag.
- The committed `characters/biffels/loadout.toml` has the worn main-hand (Soulfire Sabre) with weapon fields + stats under a `Primary` slot entry; no re-import is needed.

---

## Task 1: `ItemDelta` gains a `newMain` override

**Files:**
- Modify: `internal/model/itemdelta.go`
- Update callers: `internal/bis/set.go` (2 calls in `CandidateDelta`), `internal/model/itemdelta_test.go`
- Test: `internal/model/itemdelta_test.go`

- [ ] **Step 1: Update the failing test (new newMain case)**

In `internal/model/itemdelta_test.go`, the existing calls pass `newOff` as the 6th arg. Update every `ItemDelta(...)` call to insert a `newMain` arg (nil) **before** the `newOff` arg, and add a main-weapon test. First update existing calls: change each `..., nil, 1.0, 600)` to `..., nil, nil, 1.0, 600)` and the off-hand case `..., &w, 1.0, 600)` to `..., nil, &w, 1.0, 600)`. Then append:

```go
func TestItemDeltaMainHandWeapon(t *testing.T) {
	base := StatBlock{}
	restMain := Weapon{AvgDamage: 100, MinDamage: 60, MaxDamage: 140, DelaySecs: 4}
	bigMain := Weapon{AvgDamage: 200, MinDamage: 120, MaxDamage: 280, DelaySecs: 4}
	// Swapping in a stronger main-hand (via newMain) raises DPS.
	d := ItemDelta(base, restMain, Weapon{}, nil, StatBlock{}, &bigMain, nil, 1.0, 600)
	require.Greater(t, d, 0.0)
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/model/ -run TestItemDelta`
Expected: FAIL — compile error (call has too many args / `newMain` not a param) until Step 3.

- [ ] **Step 3: Add the param**

Replace the whole `ItemDelta` func in `internal/model/itemdelta.go` with (note the doc + the renamed `restMain` param + new `newMain`):

```go
// ItemDelta is the ΔDPS of equipping an item into an otherwise-fixed set.
// restBase is the set's StatBlock with the target slot empty; restMain/restOff are
// the main-hand / off-hand weapons with the target slot empty (zero Weapon when the
// target IS that weapon slot, or none is equipped). itemStats is the candidate's
// stats; for a weapon candidate pass its weapon as newMain (main-hand slot) or
// newOff (off-hand slot), else nil.
//
// It diffs full TotalDPSDual evaluations at the set's live stat totals, so a
// stat already at its cap in restBase contributes ~0 and the multiplicative
// auto cluster (haste·MA·crit·flurry·dps-mod) makes a stat worth more as the
// set accumulates its partners.
func ItemDelta(restBase StatBlock, restMain, restOff Weapon, arts []spell.CombatArt, itemStats StatBlock, newMain, newOff *Weapon, classAutoMult, fightLen float64) float64 {
	before := TotalDPSDual(restBase, restMain, restOff, arts, classAutoMult, fightLen)
	main := restMain
	if newMain != nil {
		main = *newMain
	}
	off := restOff
	if newOff != nil {
		off = *newOff
	}
	after := TotalDPSDual(restBase.Add(itemStats), main, off, arts, classAutoMult, fightLen)
	return after - before
}
```

Update the two callers in `internal/bis/set.go` `CandidateDelta` (they will be rewritten fully in Task 3, but to keep the tree green NOW, just insert a `nil` newMain arg): change
`model.ItemDelta(rb, s.Main, ro, s.Arts, c.Stats, &w, s.AutoMult, s.FightLen)` → `model.ItemDelta(rb, s.Main, ro, s.Arts, c.Stats, nil, &w, s.AutoMult, s.FightLen)`
and
`model.ItemDelta(rb, s.Main, ro, s.Arts, c.Stats, nil, s.AutoMult, s.FightLen)` → `model.ItemDelta(rb, s.Main, ro, s.Arts, c.Stats, nil, nil, s.AutoMult, s.FightLen)`.

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/model/ ./internal/bis/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/model/itemdelta.go internal/model/itemdelta_test.go internal/bis/set.go
git commit -m "feat(model): ItemDelta gains newMain override mirroring newOff"
```

---

## Task 2: Class weapon config (`dual_wield`, `weapon_wield_styles`)

**Files:**
- Modify: `internal/charconfig/charconfig.go`, `classes/assassin.toml`
- Test: `internal/charconfig/charconfig_test.go` (or wherever `LoadClass` is tested)

- [ ] **Step 1: Write the failing test**

Find the existing `LoadClass` test (search `LoadClass` in `internal/charconfig/*_test.go`). Add:

```go
func TestLoadClassWeaponConfig(t *testing.T) {
	cd, err := LoadClass("../../classes", "assassin")
	require.NoError(t, err)
	require.True(t, cd.DualWield)
	require.Equal(t, []string{"One-Handed"}, cd.WeaponWieldStyles)
}
```

(If the test package import path for `classes` differs, match the existing `LoadClass` test's path.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/charconfig/ -run TestLoadClassWeaponConfig`
Expected: FAIL — `cd.DualWield`/`cd.WeaponWieldStyles` undefined.

- [ ] **Step 3: Add the fields + validation + TOML**

In `internal/charconfig/charconfig.go`, change `ClassData`:

```go
type ClassData struct {
	AutoAttackMultiplier float64  `toml:"auto_attack_multiplier"`
	DualWield            bool     `toml:"dual_wield"`
	WeaponWieldStyles    []string `toml:"weapon_wield_styles"`
}
```

In `LoadClass`, after the `AutoAttackMultiplier` check, add:

```go
	if len(cd.WeaponWieldStyles) == 0 {
		return ClassData{}, fmt.Errorf("%s: weapon_wield_styles must be non-empty", path)
	}
```

In `classes/assassin.toml`, append:

```toml
dual_wield = true
weapon_wield_styles = ["One-Handed"]
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/charconfig/`
Expected: PASS (the strict-undecoded-keys check now accepts the two new keys).

- [ ] **Step 5: Commit**

```bash
git add internal/charconfig/charconfig.go classes/assassin.toml internal/charconfig/charconfig_test.go
git commit -m "feat(charconfig): class weapon config (dual_wield, weapon_wield_styles)"
```

---

## Task 3: Core refactor — Primary as a derived, optimizable slot

**This whole task is one atomic change — the tree does not compile until every sub-step is done. Work through 3.1–3.7, then run the full suite and commit once.**

**Files:** `internal/bis/set.go`, `internal/bis/candidates.go`, `internal/bis/loadoutset.go`, `internal/bis/build.go`, `internal/bis/report.go`, `internal/bis/render.go`, `internal/store/store.go`, `cmd/bis/main.go`, and tests in `internal/bis`, `internal/store`.

### 3.1 — `set.go`: derive the main-hand

- [ ] Remove the `Main model.Weapon` field from the `Set` struct.
- [ ] In `NewSet`, drop `Main: lo.Main` (keep `Arts: lo.Arts`):

```go
func NewSet(profile model.StatBlock, lo store.Loadout, autoMult, fightLen float64) *Set {
	return &Set{Profile: profile, Arts: lo.Arts, AutoMult: autoMult, FightLen: fightLen, Equipped: map[string][]store.ScorableItem{}}
}
```

- [ ] Add a shared weapon extractor and the main-hand derivations; rewrite `offWeapon` to use it. Place near `offWeapon`:

```go
// weaponFrom returns the first equippable weapon in items (zero Weapon if none).
func weaponFrom(items []store.ScorableItem) model.Weapon {
	for _, it := range items {
		if it.IsWeapon() {
			return model.Weapon{AvgDamage: it.WeaponAvg, MinDamage: it.WeaponMin, MaxDamage: it.WeaponMax, DelaySecs: it.WeaponDelay}
		}
	}
	return model.Weapon{}
}

// mainWeapon is the equipped main-hand weapon (zero Weapon if none), derived from
// Equipped["Primary"] — the mirror of offWeapon()/Equipped["Secondary"].
func (s *Set) mainWeapon() model.Weapon { return weaponFrom(s.Equipped[mainHandSlot]) }

// offWeapon is the equipped off-hand weapon (zero Weapon if none).
func (s *Set) offWeapon() model.Weapon { return weaponFrom(s.Equipped[offHandSlot]) }

// restMain is the main-hand weapon excluding a slot (zero if the slot IS the main-hand).
func (s *Set) restMain(exclude string) model.Weapon {
	if exclude == mainHandSlot {
		return model.Weapon{}
	}
	return s.mainWeapon()
}
```

- [ ] `DPS()` uses the derived main:

```go
func (s *Set) DPS() float64 {
	return model.TotalDPSDual(s.restBase(""), s.mainWeapon(), s.offWeapon(), s.Arts, s.AutoMult, s.FightLen)
}
```

- [ ] `slotDPS` derives both weapons from `items` for the matching weapon slot:

```go
func (s *Set) slotDPS(slot string, items []store.ScorableItem) float64 {
	rb := s.restBase(slot)
	for _, it := range items {
		rb = rb.Add(it.Stats)
	}
	main := s.mainWeapon()
	if slot == mainHandSlot {
		main = weaponFrom(items)
	}
	off := s.offWeapon()
	if slot == offHandSlot {
		off = weaponFrom(items)
	}
	return model.TotalDPSDual(rb, main, off, s.Arts, s.AutoMult, s.FightLen)
}
```

- [ ] `CandidateDelta` handles main-hand, off-hand, and other slots via the new `newMain`/`newOff`:

```go
func (s *Set) CandidateDelta(slot string, c store.ScorableItem) float64 {
	rb := s.restBase(slot)
	var newMain, newOff *model.Weapon
	if c.IsWeapon() {
		w := model.Weapon{AvgDamage: c.WeaponAvg, MinDamage: c.WeaponMin, MaxDamage: c.WeaponMax, DelaySecs: c.WeaponDelay}
		switch slot {
		case mainHandSlot:
			newMain = &w
		case offHandSlot:
			newOff = &w
		}
	}
	return model.ItemDelta(rb, s.restMain(slot), s.restOff(slot), s.Arts, c.Stats, newMain, newOff, s.AutoMult, s.FightLen)
}
```

(`ReplaceInstanceDelta`/`EquippedInstanceValue` are unchanged — they call `slotDPS`, which now handles the main-hand.)

### 3.2 — `report.go`: weights use the derived main

- [ ] In `ConvergedWeights` (`internal/bis/report.go:~30`), change `set.Main` → `set.mainWeapon()` in the `TotalDPSDual` call.

### 3.3 — `loadoutset.go`: Primary optimizable + drop the `set.Main` install

- [ ] Add `"Primary"` to `optimizableCatalogSlots`:

```go
var optimizableCatalogSlots = map[string]bool{
	"Primary": true, "Secondary": true, "Head": true, "Chest": true, "Shoulders": true,
	"Forearms": true, "Hands": true, "Legs": true, "Feet": true,
	"Finger": true, "Ear": true, "Wrist": true, "Neck": true,
	"Cloak": true, "Waist": true,
}
```

- [ ] In `SetFromLoadout`, delete the `if e.CatalogSlot == "Primary" && it.IsWeapon() { set.Main = ... }` block (the imported Primary is already appended to `Equipped["Primary"]`). Change the optimizability decision from the file flag to the authoritative slot rule: replace `if e.Optimizable {` with `if OptimizableSlot(e.CatalogSlot) {`.

### 3.4 — `candidates.go`: weapon pools from class config

- [ ] Rewrite `SlotCandidates` to take the class weapon config and build both weapon pools (no Soulfire exclusion). New signature + body:

```go
// SlotCandidates groups items by census slot (dropping any that fail keep), then
// overrides the weapon slots from the class weapon config: the main-hand (Primary)
// pool is every weapon whose WieldStyle is allowed, and — when dualWield — the
// off-hand (Secondary) pool is the same one-handed set. The single-physical-weapon
// reality is enforced by the optimizer's no-duplicate rule, not by pool exclusion.
func SlotCandidates(items []store.ScorableItem, keep func(store.ScorableItem) bool, wieldStyles []string, dualWield bool) map[string][]store.ScorableItem {
	allowed := map[string]bool{}
	for _, ws := range wieldStyles {
		allowed[ws] = true
	}
	bySlot := map[string][]store.ScorableItem{}
	var weapons []store.ScorableItem
	for _, it := range items {
		if !keep(it) {
			continue
		}
		if it.Slot != mainHandSlot {
			bySlot[it.Slot] = append(bySlot[it.Slot], it)
		}
		if allowed[it.WieldStyle] {
			weapons = append(weapons, it)
		}
	}
	bySlot[mainHandSlot] = weapons
	if dualWield {
		bySlot[offHandSlot] = weapons
	} else {
		delete(bySlot, offHandSlot)
	}
	return bySlot
}
```

(`strings` import may become unused in `candidates.go` once the `HasPrefix(... "Soulfire")` check is gone — remove the import if so.)

### 3.5 — `build.go`: optimize weapon slots first, no-duplicate constraint

- [ ] `pickBest` gains a `forbidden map[int]bool` of ids it may not use (the other weapon slot's current pick). Change its signature and the skip check:

```go
func pickBest(set *Set, slot string, cands []store.ScorableItem, forbidden map[int]bool) []store.ScorableItem {
	orig := set.Equipped[slot]
	defer func() { set.Equipped[slot] = orig }()

	capN := capacityOf(slot)
	chosen := []store.ScorableItem{}
	used := map[int]bool{}
	for len(chosen) < capN {
		bestIdx, bestDPS := -1, math.Inf(-1)
		for i, c := range cands {
			if used[c.ID] || forbidden[c.ID] {
				continue
			}
			set.Equipped[slot] = append(append([]store.ScorableItem{}, chosen...), c)
			if d := set.DPS(); d > bestDPS {
				bestDPS, bestIdx = d, i
			}
		}
		if bestIdx < 0 {
			break
		}
		chosen = append(chosen, cands[bestIdx])
		used[cands[bestIdx].ID] = true
	}
	return chosen
}
```

- [ ] In `BuildSet`, optimize the two weapon slots first each pass, and pass the cross-slot `forbidden` set for weapon slots. Replace the slot-collection + pass loop:

```go
	armor := make([]string, 0, len(bySlot))
	for slot := range bySlot {
		if slot == mainHandSlot || slot == offHandSlot || lockedSlot[slot] {
			continue
		}
		armor = append(armor, slot)
	}
	sort.Strings(armor)

	// Weapon slots first (so a main-hand is present when armor is evaluated), then armor.
	order := []string{}
	for _, w := range []string{mainHandSlot, offHandSlot} {
		if _, ok := bySlot[w]; ok && !lockedSlot[w] {
			order = append(order, w)
		}
	}
	order = append(order, armor...)

	for pass := 0; pass < maxPasses; pass++ {
		changed := false
		for _, slot := range order {
			var forbidden map[int]bool
			if slot == mainHandSlot || slot == offHandSlot {
				forbidden = weaponForbid(set, slot)
			}
			best := pickBest(set, slot, bySlot[slot], forbidden)
			if !sameItems(best, set.Equipped[slot]) {
				set.Equipped[slot] = best
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return set
```

- [ ] Add the helper (the other weapon slot's current ids are off-limits):

```go
// weaponForbid returns the item ids equipped in the OTHER weapon slot, which the
// given weapon slot may not reuse (the player owns one of each physical weapon).
func weaponForbid(set *Set, slot string) map[int]bool {
	other := offHandSlot
	if slot == offHandSlot {
		other = mainHandSlot
	}
	forbid := map[int]bool{}
	for _, it := range set.Equipped[other] {
		forbid[it.ID] = true
	}
	return forbid
}
```

(The numbered SPEC step says "Primary then Secondary first"; `order` does exactly that. The `mainHandSlot` exclusion that previously sat in the slot loop is gone.)

### 3.6 — `report.go`: stop skipping Primary

- [ ] In `BuildSlotReports`, remove the `if slot == mainHandSlot { continue }` line so Primary is ranked like any slot.

### 3.7 — `store.go`, `render.go`, `cmd/bis`: remove the Soulfire hardcoding

- [ ] `internal/store/store.go`: in `LoadLoadout`, drop the Soulfire `loadWeapon` query and the `Main`/`MainName` return values. New body:

```go
func (d *DB) LoadLoadout() (Loadout, error) {
	arts, err := d.CombatArts()
	if err != nil {
		return Loadout{}, err
	}
	return Loadout{Arts: spell.HighestRanks(arts)}, nil
}
```

Remove `Main`/`MainName` from the `Loadout` struct. If `loadWeapon` is now unused, delete it (check with `grep -n loadWeapon internal/store`).

- [ ] `internal/bis/render.go`: replace the main-hand assumptions sentence (line ~140, "Main-hand is fixed (Soulfire Sabre); ...") with one that states the current model, e.g.:

```go
	b.WriteString("_Main-hand and off-hand are optimized weapon slots; their weapon damage and full stat lines are included like any other slot._\n\n")
```

(Match the surrounding `b.WriteString(...)`/`Fprintf` style at that line.)

- [ ] `cmd/bis/main.go`:
  - Delete the `withFixedPrimary` function (lines ~69–77) and `findByName` if it becomes unused (`grep -n findByName cmd/bis`).
  - In the loadout summary print (~line 244–250) drop `lo.MainName` (e.g. print only `len(lo.Arts)` arts + item count). Remove the `mainItem, haveMain := findByName(...)` line.
  - In the from-scratch tier loop, remove `if haveMain { profile = profile.Add(mainItem.Stats) }`, and change `withFixedPrimary(bis.BuildSlotReports(set, bySlot, weights, reportTop), mainItem, haveMain)` → `bis.BuildSlotReports(set, bySlot, weights, reportTop)`. Same in the `lockIDs` block.
  - Thread weapon config into **every** `SlotCandidates` call (in this file and in `runLoadoutReport`): `bis.SlotCandidates(items, keep)` → `bis.SlotCandidates(items, keep, classData.WeaponWieldStyles, classData.DualWield)`. (`runLoadoutReport` already receives `classData`.)

### 3.8 — Tests

- [ ] `internal/bis/set_test.go`: the `testLoadout()` helper and inline `Loadout{Main: ...}` constructors no longer set the main via `Loadout.Main`. For each `Set` that needs a main-hand weapon, seed it explicitly into `Equipped["Primary"]`. Update `testLoadout()` to just return arts/empty, and in tests that assert auto-attack DPS, add a Primary weapon. Concretely, update `TestSetDPSAndCandidateDelta` and `TestSetAppliesClassAutoMult` to set:

```go
	set.Equipped["Primary"] = []store.ScorableItem{{ID: 999, Slot: "Primary", WeaponAvg: 160, WeaponDelay: 4}}
```

before asserting DPS. Match the OLD `Loadout{Main: {AvgDamage:160, DelaySecs:4}}` numbers exactly — `WeaponAvg: 160, WeaponDelay: 4` only, **no** `WeaponMin`/`WeaponMax` (leaving them 0 reproduces the old zero min/max so the crit range-shift floor is unchanged). `TestSetDPSAndCandidateDelta` asserted `40.0` (160/4 single-wield, crit chance 0) — this holds with the seed above.
- [ ] `internal/bis/report_test.go`: `TestBuildSlotReportsSkipsPrimary` now asserts the opposite — Primary IS present. Rename to `TestBuildSlotReportsIncludesPrimary` and assert a `Primary` slot report exists when the candidate pool has a Primary weapon. Update `TestBuildSlotReports` if it relied on Primary absence.
- [ ] `internal/bis/render_test.go`: `TestRenderFixedSlot` asserted the "(fixed)"/Soulfire main line — update it to the new assumptions sentence (or remove the fixed-main assertion).
- [ ] `internal/bis/build_test.go`, `buildset_test.go`: update `pickBest(...)` calls to pass a `nil` (or `map[int]bool{}`) `forbidden` arg; update `SlotCandidates(...)` calls to the new signature (`..., []string{"One-Handed"}, true`). Where these tests build weapon-bearing sets, ensure expectations still hold (the converged set now also fills Primary).
- [ ] `internal/store/loadout_test.go`: `TestLoadLoadout` asserted `lo.Main`/`lo.MainName` — update to assert only the arts are loaded (drop the weapon assertions).
- [ ] Any remaining `Loadout{Main:` / `.Main` / `SlotCandidates(` / `pickBest(` compile errors: fix mechanically per the new signatures.

### 3.9 — Build, test, commit

- [ ] Run: `go build ./... && go vet ./...` → both clean.
- [ ] Run: `go test ./... -count=1` → all pass. (If a from-scratch test now picks a different main-hand by tier, confirm the new pick is correct per tier filters before adjusting the assertion.)
- [ ] Commit:

```bash
git add -A
git commit -m "feat(bis): main-hand is a normal optimizable slot (derived from Equipped[Primary])"
```

---

## Task 4: Regenerate reports, verify, commit

**Files:** `characters/biffels/upgrade-report.md`, `characters/biffels/bis-report.md`

- [ ] **Step 1: Regenerate both reports**

```bash
go run ./cmd/bis
go run ./cmd/bis --loadout characters/biffels/loadout.toml
```

Expected: both write successfully.

- [ ] **Step 2: Verify Primary now appears and is optimized**

- `characters/biffels/upgrade-report.md`: confirm a `Primary` row exists in the buckets, `Wearing` = the worn `Soulfire Sabre` (linked), with main-hand upgrade candidates (or `—` if Soulfire is best).
- `characters/biffels/bis-report.md`: confirm a `Primary` slot section now appears as a ranked slot (not a fixed prepended entry).

Run: `grep -c "| Primary |" characters/biffels/upgrade-report.md` (expect ≥ 1) and `grep -n "^## Primary\|Primary" characters/biffels/bis-report.md | head`.

- [ ] **Step 3: Verify reproducibility (determinism still holds)**

```bash
go run ./cmd/bis >/dev/null 2>&1; cp characters/biffels/bis-report.md /tmp/p1.md
go run ./cmd/bis >/dev/null 2>&1; diff -q /tmp/p1.md characters/biffels/bis-report.md && echo "reproducible"
```

Expected: `reproducible` (no diff). If it differs, the weapon-pool ordering needs the same id-sort tiebreak as the other pools — confirm `SlotCandidates` weapon pool preserves catalog (`ORDER BY id`) order and `pickBest` ties resolve by slice order.

- [ ] **Step 4: Commit**

```bash
git add characters/biffels/upgrade-report.md characters/biffels/bis-report.md
git commit -m "chore: regenerate reports with Primary as an optimized slot"
```

---

## Final verification

- [ ] `go build ./... && go vet ./... && go test ./... -count=2` — all green twice.
- [ ] `git grep -n "Set.Main\|\.Main\b" internal/ cmd/ | grep -v "lo\.\|Loadout\|MainStat\|MainName"` — no remaining references to the removed `Set.Main` field.
- [ ] `git grep -n "withFixedPrimary\|LIKE 'Soulfire" .` — returns nothing.
- [ ] Confirm `docs/SPEC.md` §6/§7 match the implementation (config field names, `mainWeapon()`, no-duplicate rule, weapon-slots-first).
