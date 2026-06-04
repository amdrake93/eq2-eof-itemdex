# Plan 2g — BiS Fix-Up Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix three issues a final holistic review found in the implemented BiS tool: the off-hand pool wrongly includes Soulfire (recommending an impossible second Soulfire), the fixed Primary slot is ranked with meaningless ΔDPS, and `Loadout.Off` is dead code.

**Architecture:** Small, targeted changes to the existing `internal/bis` + `internal/store` + `cmd/bis` code on branch `plan2e-scoring-report` (not yet merged). The off-hand is chosen from the Secondary candidate pool (all non-Soulfire 1H weapons); the Primary slot is shown as the already-given Soulfire main, not optimized.

**Tech Stack:** Go 1.26, `modernc.org/sqlite`, `stretchr/testify`.

**Domain facts:** The player gets exactly ONE Soulfire (the fixed main-hand from the questline), so the off-hand must be a non-Soulfire 1H weapon. The main-hand is a given, not a slot to optimize — but it should still appear in each list so the list is complete.

---

## File Structure

| File | Change |
|---|---|
| `internal/bis/candidates.go` *(modify)* | exclude Soulfire from the off-hand (Secondary) pool. |
| `internal/bis/report.go` *(modify)* | `BuildSlotReports` skips the main-hand slot. |
| `internal/bis/render.go` *(modify)* | a Chosen-but-no-Ranked slot renders as `BiS: **x** _(fixed)_`. |
| `internal/store/store.go` *(modify)* | drop dead `Loadout.Off`/`OffName` + the off-hand query. |
| `internal/store/loadout_test.go` *(modify)* | drop Off/OffName assertions. |
| `cmd/bis/main.go` *(modify)* | inject a fixed Primary report (the main item) into each tier; fix startup line. |

---

### Task 1: Off-hand pool excludes Soulfire

**Files:**
- Modify: `internal/bis/candidates.go`
- Test: `internal/bis/candidates_test.go`

- [ ] **Step 1: Add a failing assertion.** In `internal/bis/candidates_test.go`, inside `TestSlotCandidatesOffHandAndFilter`, add a Soulfire 1H weapon to the `items` slice (as a new element) and assert it's excluded from the off-hand pool. Add this item to the literal:

```go
		{ID: 5, Name: "Soulfire Gladius", Slot: "Primary", Tier: "MYTHICAL", WieldStyle: "One-Handed", WeaponAvg: 160, WeaponDelay: 4},
```
and after the existing `secIDs` assertions add:
```go
	require.False(t, secIDs[5], "the single Soulfire is the fixed main-hand, not an off-hand option")
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bis/ -run TestSlotCandidates -v`
Expected: FAIL — `secIDs[5]` is currently true (Soulfire is a 1H weapon, so it's in the off-hand pool).

- [ ] **Step 3: Implement.** In `internal/bis/candidates.go`, add the `strings` import and exclude Soulfire when collecting one-handers:

```go
package bis

import (
	"strings"

	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
)

// SlotCandidates groups items by their census slot, dropping any that fail keep,
// then overrides the off-hand slot with every one-handed weapon that passes keep
// — except Soulfire weapons, since the single Soulfire is the fixed main-hand and
// the player never owns a second one. The main-hand (Primary) bucket is left as-is;
// BuildSet ignores it (the main is fixed).
func SlotCandidates(items []store.ScorableItem, keep func(store.ScorableItem) bool) map[string][]store.ScorableItem {
	bySlot := map[string][]store.ScorableItem{}
	var oneHanders []store.ScorableItem
	for _, it := range items {
		if !keep(it) {
			continue
		}
		bySlot[it.Slot] = append(bySlot[it.Slot], it)
		if it.WieldStyle == "One-Handed" && !strings.HasPrefix(it.Name, "Soulfire") {
			oneHanders = append(oneHanders, it)
		}
	}
	bySlot[offHandSlot] = oneHanders
	return bySlot
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bis/ -run TestSlotCandidates -v`
Expected: PASS — Soulfire (id 5) excluded; the fabled 1H rapier (id 1) still included.

- [ ] **Step 5: Commit**

```bash
go test ./internal/bis/ && make lint
git add internal/bis/candidates.go internal/bis/candidates_test.go
git commit -m "Exclude Soulfire from the off-hand pool (single Soulfire is the fixed main)"
```

---

### Task 2: Primary not ranked; fixed-slot render

**Files:**
- Modify: `internal/bis/report.go`
- Modify: `internal/bis/render.go`
- Test: `internal/bis/report_test.go`, `internal/bis/render_test.go`

- [ ] **Step 1: Failing test — BuildSlotReports skips Primary.** Add to `internal/bis/report_test.go`:

```go
func TestBuildSlotReportsSkipsPrimary(t *testing.T) {
	lo := testLoadout()
	bySlot := map[string][]store.ScorableItem{
		"Primary": {{ID: 1, Slot: "Primary", Tier: "FABLED", WieldStyle: "One-Handed", WeaponAvg: 160, WeaponDelay: 4}},
		"Chest":   {{ID: 2, Slot: "Chest", Tier: "FABLED", Stats: model.StatBlock{Potency: 30}}},
	}
	set := BuildSet(model.StatBlock{}, lo, bySlot, nil, 12)
	reports := BuildSlotReports(set, bySlot, ConvergedWeights(set), 3)

	for _, r := range reports {
		require.NotEqual(t, "Primary", r.Slot, "main-hand slot is fixed; it should not be ranked")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/bis/ -run TestBuildSlotReportsSkipsPrimary -v`
Expected: FAIL — a `Primary` report is currently produced.

- [ ] **Step 3: Implement skip in `BuildSlotReports`.** In `internal/bis/report.go`, in the slot loop, skip the main-hand slot. The loop becomes:

```go
	reports := make([]SlotReport, 0, len(slots))
	for _, slot := range slots {
		if slot == mainHandSlot {
			continue
		}
		scored := SlotCandidatesScored(set, slot, bySlot[slot], weights)
		reports = append(reports, SlotReport{
			Slot:   slot,
			Chosen: set.Equipped[slot],
			Ranked: topN(scored, n),
		})
	}
```
(`mainHandSlot` is the existing const `"Primary"` in `build.go`, same package.)

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/bis/ -run TestBuildSlotReportsSkipsPrimary -v`
Expected: PASS.

- [ ] **Step 5: Failing test — fixed-slot render.** Add to `internal/bis/render_test.go`:

```go
func TestRenderFixedSlot(t *testing.T) {
	reports := []BaselineReport{{
		Name:    "RAID",
		Weights: map[string]float64{"potency": 7.0},
		Reports: []SlotReport{{
			Slot:   "Primary",
			Chosen: []store.ScorableItem{{Name: "Soulfire Gladius", Tier: "MYTHICAL"}},
			Ranked: nil,
		}},
	}}
	out := Render(reports)
	require.Contains(t, out, "### Primary")
	require.Contains(t, out, "BiS: **Soulfire Gladius** _(fixed)_")
}
```

- [ ] **Step 6: Run to verify it fails**

Run: `go test ./internal/bis/ -run TestRenderFixedSlot -v`
Expected: FAIL — output has `BiS: **Soulfire Gladius**` without the `_(fixed)_` marker.

- [ ] **Step 7: Implement the fixed marker in `render.go`.** In `Render`, change the per-slot `BiS:` block so a Chosen-but-no-Ranked slot is marked fixed:

```go
			if len(sr.Chosen) > 0 {
				names := make([]string, 0, len(sr.Chosen))
				for _, c := range sr.Chosen {
					names = append(names, c.Name)
				}
				line := fmt.Sprintf("BiS: **%s**", strings.Join(names, "**, **"))
				if len(sr.Ranked) == 0 {
					line += " _(fixed)_"
				}
				fmt.Fprintf(&b, "%s\n\n", line)
			}
			for _, s := range sr.Ranked {
				writeScored(&b, s)
			}
			b.WriteString("\n")
```

- [ ] **Step 8: Run to verify it passes**

Run: `go test ./internal/bis/ -run "TestRenderFixedSlot|TestRender" -v`
Expected: PASS (both — the existing `TestRender` still passes; its Chest report has Ranked items so no `_(fixed)_`).

- [ ] **Step 9: Commit**

```bash
go test ./internal/bis/ && go build ./... && make lint
git add internal/bis/report.go internal/bis/render.go internal/bis/report_test.go internal/bis/render_test.go
git commit -m "Skip main-hand slot in reports; render fixed slots as (fixed)"
```

---

### Task 3: Inject fixed Primary; drop dead Loadout.Off

**Files:**
- Modify: `internal/store/store.go`
- Modify: `internal/store/loadout_test.go`
- Modify: `cmd/bis/main.go`
- Test: `internal/store/loadout_test.go` + manual run

- [ ] **Step 1: Update the loadout test for the slimmer struct.** In `internal/store/loadout_test.go`, in `TestLoadLoadout`, remove the two off-hand assertions:
```go
	require.Equal(t, "Enchanted Grove Scimitar", lo.OffName)
	require.InDelta(t, 158.0, lo.Off.AvgDamage, 1e-9)
```
(Keep the Main/MainName/Arts assertions. The seed's id-2 row can stay; it's just no longer asserted.)

- [ ] **Step 2: Run to verify it fails (compile error once the struct loses the fields — but first it still compiles).** Run `go test ./internal/store/ -run TestLoadLoadout -v` now (still PASS). The real failure appears after Step 3 removes the fields; this step just stages the test edit. Proceed to Step 3.

- [ ] **Step 3: Slim `Loadout` + `LoadLoadout` in `internal/store/store.go`.** Change the struct to drop `Off`/`OffName`:

```go
// Loadout is the fixed main-hand + collapsed combat arts the model scores against.
// The off-hand is chosen from the Secondary candidate pool, not fixed here.
type Loadout struct {
	Main     model.Weapon
	MainName string
	Arts     []spell.CombatArt
}
```

In `LoadLoadout`, remove the off-hand query block and return the slimmer struct:

```go
func (d *DB) LoadLoadout() (Loadout, error) {
	main, mainName, err := d.loadWeapon(
		`SELECT name, weapon_min_dmg, weapon_max_dmg, delay FROM items
		 WHERE name LIKE 'Soulfire%' AND classes LIKE '%assassin%'
		 ORDER BY weapon_max_dmg DESC LIMIT 1`)
	if err != nil {
		return Loadout{}, err
	}
	rows, err := d.db.Query(`SELECT name, min_dmg, max_dmg, recast_secs FROM combat_arts`)
	if err != nil {
		return Loadout{}, err
	}
	defer func() { _ = rows.Close() }()
	var arts []spell.CombatArt
	for rows.Next() {
		var a spell.CombatArt
		if err := rows.Scan(&a.Name, &a.MinDamage, &a.MaxDamage, &a.RecastSecs); err != nil {
			return Loadout{}, err
		}
		arts = append(arts, a)
	}
	if err := rows.Err(); err != nil {
		return Loadout{}, err
	}
	return Loadout{Main: main, MainName: mainName, Arts: spell.HighestRanks(arts)}, nil
}
```
(`loadWeapon` stays — still used for the main-hand.)

- [ ] **Step 4: Verify store tests pass**

Run: `go test ./internal/store/ -v`
Expected: PASS. (If anything else in the repo references `lo.Off`/`lo.OffName`, the build will fail — Step 5 removes the last reference in `cmd/bis`.)

- [ ] **Step 5: Wire the fixed Primary + fix startup line in `cmd/bis/main.go`.** Add two helpers above `main`:

```go
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
```

Change the startup `Printf` (it currently uses `lo.OffName`) to:
```go
	mainItem, haveMain := findByName(items, lo.MainName)
	fmt.Printf("loadout: %s (main-hand, fixed); %d combat arts; %d assassin items\n",
		lo.MainName, len(lo.Arts), len(items))
```

In the three-tier loop, prepend the fixed Primary to each tier's reports before appending the BaselineReport:
```go
	for _, t := range tiers {
		bySlot := bis.SlotCandidates(items, t.keep)
		set := bis.BuildSet(t.baseline, lo, bySlot, nil, maxBuildPasses)
		weights := bis.ConvergedWeights(set)
		slotReports := withFixedPrimary(bis.BuildSlotReports(set, bySlot, weights, *topN), mainItem, haveMain)
		allRows = append(allRows, scoreRows(slotReports, strings.ToLower(t.name))...)
		reports = append(reports, bis.BaselineReport{Name: t.name, Weights: weights, Reports: slotReports})
	}
```

And in the `--lock` block, wrap its `slotReports` the same way:
```go
		slotReports := withFixedPrimary(bis.BuildSlotReports(set, bySlot, weights, *topN), mainItem, haveMain)
```

(`scoreRows` over the fixed Primary report contributes nothing since its `Ranked` is empty — fine.)

- [ ] **Step 6: Build, vet, test, lint**

Run:
```bash
go build ./... && go vet ./...
go test ./...
make lint
```
Expected: all pass; 0 lint issues. (Confirm no remaining `lo.Off`/`lo.OffName` references: `grep -rn "\.Off\b\|OffName" internal/ cmd/` → none.)

- [ ] **Step 7: Run against the real db + sanity-check**

Run:
```bash
go run ./cmd/builddb         # ensure bis.db has the scores table + fresh data
go run ./cmd/bis --db bis.db --out bis-report.md
```
Then verify:
```bash
grep -n "^## " bis-report.md                 # PRE-RAID, RAID, BEST-OF-BEST, Progression, Assumptions
grep -c "Hunter's" bis-report.md             # expect 0
awk '/^### Primary/{p=1} p&&/_\(fixed\)_/{print; p=0}' bis-report.md   # Primary shows Soulfire (fixed)
awk '/^### Secondary/{s=1} s&&/^- /{print; if(++n>=2) s=0}' bis-report.md  # off-hand picks: NOT Soulfire
```
Sanity: every tier lists `### Primary` → `BiS: **Soulfire Gladius** _(fixed)_` with no alternatives; the `### Secondary` BiS is a non-Soulfire 1H weapon; lists are complete; no Hunter's; avatar only under BEST-OF-BEST.

- [ ] **Step 8: Commit**

```bash
git add internal/store/store.go internal/store/loadout_test.go cmd/bis/main.go
git commit -m "Inject fixed Primary into each list; drop dead Loadout.Off"
```

---

### Task 4 (OPTIONAL): Minor cleanups

The holistic review flagged these non-blocking nits. Do them only if quick; skip if risky. Each its own commit.

- [ ] **Unexport `SlotCandidatesScored`** → `slotCandidatesScored` in `report.go` (only `BuildSlotReports` calls it; grep to confirm no external use first). Run `go test ./... && make lint`, commit.
- [ ] **Co-locate slot consts** — move `offHandSlot` (set.go) and `mainHandSlot` (build.go) next to `slotCapacity` in one place (e.g. top of `build.go`). Pure move; run tests, commit.
- [ ] **Dedupe the raid keep predicate** in `cmd/bis/main.go` — the `!bis.IsAvatar(it) && notExcluded(it)` appears in the RAID tier and the `--lock` block; extract a named local `raidKeep := func(...){...}` and reuse. Run tests, commit.

(Leave `ScorableItem.Mods` as-is — it's a struct field used during load; converting to a local is more churn than it's worth.)

---

## Self-Review

**1. Spec coverage:**
- Off-hand excludes Soulfire → Task 1 (`SlotCandidates` skips `Soulfire`-prefixed in the 1H pool). ✔
- Primary not ranked → Task 2a (`BuildSlotReports` skips `mainHandSlot`). ✔
- Fixed-slot render `_(fixed)_` → Task 2b. ✔
- Fixed Primary injected so lists are complete → Task 3 (`withFixedPrimary` prepended per tier + lock). ✔
- Dead `Loadout.Off`/`OffName` removed + startup line fixed → Task 3. ✔
- Minor cleanups → Task 4 (optional). ✔

**2. Placeholder scan:** No TBD/TODO; every code step is complete; tests have real assertions. (Task 3 Step 2 is a deliberate "edit-then-fail-after-next-step" note, not a placeholder — the compile failure is realized in Step 3/4.)

**3. Type consistency:** `Loadout` loses `Off`/`OffName` (Task 3) — the only consumer was `cmd/bis`'s startup line (Task 3 Step 5) and `loadout_test.go` (Task 3 Step 1); both updated. `withFixedPrimary`/`findByName` signatures consistent. `SlotReport{Slot, Chosen, Ranked}` used consistently. `mainHandSlot` const reused in Task 2. `bis.BuildSet` still takes `lo store.Loadout` and uses only `lo.Main`/`lo.Arts` — unaffected by dropping `Off`.

**Green-at-each-commit:** Task 1 and Task 2 are self-contained (each compiles + passes). Task 3 removes `Loadout.Off` and its only references in the same task (loadout_test Step 1, cmd/bis Step 5) before building in Step 6 — so the repo compiles by the end of Task 3. Within Task 3 the build is briefly red between Step 3 and Step 5; that's expected and resolved before the Step 6 verification/commit.
