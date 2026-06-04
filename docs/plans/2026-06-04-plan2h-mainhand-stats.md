# Plan 2h — Pin Soulfire Sabre + Count Main-Hand Stats Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Pin the fixed main-hand to **Soulfire Sabre** (its +6.3 Multi-Attack beats the Gladius's dead +2.5 block) and fold the main-hand's full stat line into every tier's baseline, so the Soulfire's +98 ability-mod / potency / crit / MA actually count instead of being silently dropped.

**Architecture:** Two changes on branch `plan2e-scoring-report` (not yet merged): (1) `store.LoadLoadout` prefers Soulfire Sabre for the main-hand; (2) `cmd/bis` adds the main-hand item's `Stats` to each tier's baseline `StatBlock` before `BuildSet`. The main-hand weapon damage was already counted; this adds its gear stats. Locked items already fold in (they're equipped in the set), so only the main-hand needed fixing.

**Tech Stack:** Go 1.26, `modernc.org/sqlite`, `stretchr/testify`.

**Why it matters:** The Soulfire is a big stat stick (`all` 98 = ability-mod, basemodifier 3.7 = potency, critchance 2.4, plus the Sabre's doubleattackchance 6.3 = MA). Excluding it understated the assassin's baseline and made the Sabre-vs-Gladius choice purely cosmetic. Folding it in raises baseline ability-mod toward saturation, so ability-mod-heavy items lose a little edge — the picks/weights will shift and should be re-validated.

---

### Task 1: Pin the main-hand to Soulfire Sabre

**Files:**
- Modify: `internal/store/store.go` (the main-hand query in `LoadLoadout`)
- Test: `internal/store/loadout_test.go`

**Context:** `LoadLoadout`'s main query is currently `WHERE name LIKE 'Soulfire%' AND classes LIKE '%assassin%' ORDER BY weapon_max_dmg DESC LIMIT 1`. All Soulfire 1H variants tie on weapon damage, so the tie-break is arbitrary (it happened to return the Gladius). Change it to prefer the Sabre, falling back to the best Soulfire if no Sabre exists.

- [ ] **Step 1: Write the failing test.** In `internal/store/loadout_test.go`, in `seedLoadout`, add a Soulfire Sabre row alongside the existing Gladius (id 1). Add after the id-1 INSERT:
```go
	exec(`INSERT INTO items (id,name,slot,tier,itemlevel,armor_type,skill,wieldstyle,classes,gamelink,weapon_min_dmg,weapon_max_dmg,delay,damage_rating)
	      VALUES (3,'Soulfire Sabre','Primary','MYTHICAL',70,'','piercing','One-Handed','assassin','',80,239,4.0,79.7)`)
```
And in `TestLoadLoadout`, change the main assertion from `"Soulfire Gladius"` to:
```go
	require.Equal(t, "Soulfire Sabre", lo.MainName)
```
(Leave the `Main.AvgDamage`/`DelaySecs` assertions — the Sabre shares the same 80–239 / 4.0, so `AvgDamage` is `(80+239)/2 = 159.5`; UPDATE that assertion to `require.InDelta(t, 159.5, lo.Main.AvgDamage, 1e-9)` and keep `4.0` for delay. The existing id-1 Gladius row's mn/mx must also be 80/239 for the tie to be real — if the seed's Gladius uses different numbers, set both rows to 80/239 so the test isolates the *name* pin, not a damage difference.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestLoadLoadout -v`
Expected: FAIL — `lo.MainName` is "Soulfire Gladius" (current query tie-break), not "Soulfire Sabre".

- [ ] **Step 3: Implement.** In `internal/store/store.go`, change the main-hand query in `LoadLoadout` to prefer the Sabre:
```go
	main, mainName, err := d.loadWeapon(
		`SELECT name, weapon_min_dmg, weapon_max_dmg, delay FROM items
		 WHERE name LIKE 'Soulfire%' AND classes LIKE '%assassin%'
		 ORDER BY (name = 'Soulfire Sabre') DESC, weapon_max_dmg DESC LIMIT 1`)
```
(The `(name = 'Soulfire Sabre')` term is 1 for the Sabre and 0 otherwise; `DESC` floats the Sabre to the top, with the old `weapon_max_dmg DESC` as the fallback ordering.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestLoadLoadout -v`
Expected: PASS — `lo.MainName == "Soulfire Sabre"`.

- [ ] **Step 5: Commit**

```bash
go test ./internal/store/ && make lint
git add internal/store/store.go internal/store/loadout_test.go
git commit -m "Pin main-hand to Soulfire Sabre (its MA beats the Gladius's block)"
```

---

### Task 2: Fold the main-hand's stats into the baseline

**Files:**
- Modify: `cmd/bis/main.go`
- Modify: `internal/bis/render.go` (assumptions note)
- Test: manual run + the existing suite

**Context:** `cmd/bis` already does `mainItem, haveMain := findByName(items, lo.MainName)` (for the fixed-Primary display). The main-hand's `Stats` (a `model.StatBlock`) must be added to each tier's baseline before `BuildSet`. `model.StatBlock` has `Add(o StatBlock) StatBlock`. The render assumptions block currently states the main-hand's stats are NOT folded in — update it.

- [ ] **Step 1: Fold main-hand stats into each tier's baseline.** In `cmd/bis/main.go`, in the three-tier loop, compute the effective baseline. Change the loop body's first lines:
```go
	for _, t := range tiers {
		profile := t.baseline
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
```
And in the `--lock` block, fold it into the raid baseline the same way:
```go
	if len(lockIDs) > 0 {
		locked := lockedItems(items, lockIDs)
		profile := baseline.Raid
		if haveMain {
			profile = profile.Add(mainItem.Stats)
		}
		bySlot := bis.SlotCandidates(items, func(it store.ScorableItem) bool { return !bis.IsAvatar(it) && notExcluded(it) })
		set := bis.BuildSet(profile, lo, bySlot, locked, maxBuildPasses)
		weights := bis.ConvergedWeights(set)
		slotReports := withFixedPrimary(bis.BuildSlotReports(set, bySlot, weights, *topN), mainItem, haveMain)
		reports = append(reports, bis.BaselineReport{
			Name: fmt.Sprintf("RAID (locked: %s)", *lock), Weights: weights, Reports: slotReports,
		})
	}
```
(`mainItem.Stats` is `model.StatBlock`; `t.baseline`/`baseline.Raid` are `model.StatBlock`; `.Add` returns the sum. `model` is already imported in main.go.)

- [ ] **Step 2: Update the render assumptions note.** In `internal/bis/render.go`, in `writeAssumptions`, replace the main-hand line:
```go
	b.WriteString("- Main-hand is fixed (Soulfire Sabre); its weapon damage AND full stat line are included in the baseline.\n")
```
(Replaces the old "its own gear stats are not folded into the baseline" line.)

- [ ] **Step 3: Build, vet, test, lint**

Run:
```bash
go build ./... && go vet ./...
go test ./...
make lint
```
Expected: all pass (no unit test asserts the old baseline; the existing render test doesn't check the assumptions wording precisely — if it does, update it).

- [ ] **Step 4: Run against the real db + sanity-check the shift.**

Run:
```bash
go run ./cmd/builddb
go run ./cmd/bis --db bis.db --out bis-report.md
```
Then compare the converged weights and a few picks to the prior run. Expected qualitative shifts (report, don't assert): baseline ability-mod is now ~+98 higher, so the **ability-mod weight drops** (closer to saturation) and ability-mod-heavy off-hands (Wrath's +71) lose a little edge vs auto-stat weapons (Clearcutter's haste/flurry). Confirm: Primary shows **Soulfire Sabre _(fixed)_** in every tier; the report still has all three tiers + progression; no Hunter's; avatar only in best-of-best. Capture the RAID converged weight table and the Secondary pick to show the user.

- [ ] **Step 5: Commit**

```bash
git add cmd/bis/main.go internal/bis/render.go
git commit -m "Fold the fixed main-hand's stats into the baseline"
```

---

## Self-Review

**1. Spec coverage:**
- Pin main-hand to Soulfire Sabre → Task 1 (query prefers Sabre, falls back to best Soulfire). ✔
- Count the main-hand's stats → Task 2 (baseline `+= mainItem.Stats` for all three tiers + lock). ✔
- Accurate report note → Task 2 Step 2. ✔
- Locked items already fold in (equipped in the set) — no change needed; noted in Architecture. ✔

**2. Placeholder scan:** No TBD/TODO; code steps are complete; Task 2 validation is qualitative-by-design (the point is to re-derive and eyeball the shift), not a brittle numeric assertion.

**3. Type consistency:** `mainItem` is `store.ScorableItem` (from the existing `findByName`); `mainItem.Stats` is `model.StatBlock`; `t.baseline` / `baseline.Raid` are `model.StatBlock`; `StatBlock.Add(StatBlock) StatBlock` exists and returns the sum. `haveMain` guards the fold (no main item → baseline unchanged, no panic). `LoadLoadout` still returns `Main`/`MainName`/`Arts`; the Sabre shares the Gladius's weapon damage so `lo.Main` (avg/delay) is unchanged in magnitude.

**Note:** This shifts the converged weights and some picks (more baseline ability-mod). That's the intended correction — the user re-validates the new output. Do NOT "fix" the model to restore the old numbers.
