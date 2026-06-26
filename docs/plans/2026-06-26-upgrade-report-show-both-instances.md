# Always Show Both Worn Instances for Multi-Capacity Slots Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** In the `bis --loadout` upgrade report, multi-capacity slots (`Ear`/`Finger`/`Wrist`) always emit a row for every worn instance — even when that instance has no positive upgrade in a bucket — so both rings/ears/wrists are always visible; the no-upgrade row shows `—`. Single-capacity slots keep the existing omit-when-no-upgrade behavior.

**Architecture:** One conditional change in `RankSlotUpgrades` (don't `continue` past a worn multi-cap instance with zero candidates — emit it with a zero-value `Best` sentinel) and one rendering branch in `renderLoadoutReport`/`upgradeCell` usage (render `—` when `Best.Name == ""`). Spec: `docs/SPEC.md` §7 "Tiered upgrade report" (Ordering & coverage / Columns bullets), already amended.

**Tech Stack:** Go, packages `internal/bis`, `cmd/bis`.

---

## File Structure

- `internal/bis/loadoutset.go` — `RankSlotUpgrades`: replace the unconditional `if len(cands) == 0 { continue }` with logic that, for multi-capacity slots (`capacityOf(slot) > 1`), still appends the row (zero-value `Best`, nil `Alt`); single-cap slots still `continue`.
- `cmd/bis/main.go` — `renderLoadoutReport`: when a row's `Best.Name == ""`, render the Best cell as `—` and leave Alternative blank (skip `upgradeCell`).
- Tests: `internal/bis/loadoutset_test.go`, `cmd/bis/main_test.go`.

**Key facts:**
- A "no upgrade" `SlotUpgrade` is signalled by a zero-value `Best` (`UpgradeOption{}` → `Name == ""`, `Delta == 0`). It sorts last because `Best.Delta == 0`.
- `UpgradeOption` is `{ ID int; Name string; Delta float64 }`; `SlotUpgrade` is `{ Slot, EquippedName string; EquippedID int; EquippedValue float64; Best UpgradeOption; Alt *UpgradeOption }`.
- `capacityOf(slot)` (in `build.go`) returns 2 for `Ear`/`Finger`/`Wrist`/`Charm`, else 1. Charm is not optimizable so never reaches here.
- The em dash to use is `—` (U+2014).

---

## Task 1: Multi-cap slots always emit a row per worn instance

**Files:**
- Modify: `internal/bis/loadoutset.go`
- Test: `internal/bis/loadoutset_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/bis/loadoutset_test.go`:

```go
func TestRankSlotUpgradesMultiCapAlwaysShowsBothInstances(t *testing.T) {
	lo := store.Loadout{Main: model.Weapon{AvgDamage: 160, MinDamage: 100, MaxDamage: 220, DelaySecs: 4}}
	set := NewSet(model.StatBlock{}, lo, 1.0, 600)
	// A very strong ring and a weak ring; the only candidate beats ONLY the weak one.
	set.Equipped["Finger"] = []store.ScorableItem{
		{ID: 10, Name: "StrongRing", Slot: "Finger", Stats: model.StatBlock{MultiAttack: 100}},
		{ID: 11, Name: "WeakRing", Slot: "Finger", Stats: model.StatBlock{MultiAttack: 5}},
	}
	optimizable := map[string]bool{"Finger": true}
	bySlot := map[string][]store.ScorableItem{
		"Finger": {{ID: 20, Name: "MidRing", Slot: "Finger", Stats: model.StatBlock{MultiAttack: 50}}},
	}

	got := RankSlotUpgrades(set, bySlot, optimizable, 0)

	// Both worn instances appear, even though StrongRing has no positive upgrade.
	require.Len(t, got, 2)

	byEquipped := map[string]SlotUpgrade{}
	for _, su := range got {
		byEquipped[su.EquippedName] = su
	}
	require.Contains(t, byEquipped, "WeakRing")
	require.Contains(t, byEquipped, "StrongRing")

	// WeakRing has a real upgrade; StrongRing has the zero-value "no upgrade" sentinel.
	require.Equal(t, "MidRing", byEquipped["WeakRing"].Best.Name)
	require.Equal(t, "", byEquipped["StrongRing"].Best.Name)
	require.Nil(t, byEquipped["StrongRing"].Alt)

	// No-upgrade rows sort last (Best.Delta 0).
	require.Equal(t, "StrongRing", got[1].EquippedName)
}

func TestRankSlotUpgradesSingleCapStillOmitsNoUpgrade(t *testing.T) {
	lo := store.Loadout{Main: model.Weapon{AvgDamage: 160, MinDamage: 100, MaxDamage: 220, DelaySecs: 4}}
	set := NewSet(model.StatBlock{}, lo, 1.0, 600)
	// Single-cap Head with a strong worn item the candidate cannot beat.
	set.Equipped["Head"] = []store.ScorableItem{
		{ID: 10, Name: "StrongHead", Slot: "Head", Stats: model.StatBlock{MultiAttack: 100}},
	}
	optimizable := map[string]bool{"Head": true}
	bySlot := map[string][]store.ScorableItem{
		"Head": {{ID: 20, Name: "WeakHead", Slot: "Head", Stats: model.StatBlock{MultiAttack: 5}}},
	}

	// No positive upgrade for a single-cap slot -> the slot is omitted entirely.
	require.Empty(t, RankSlotUpgrades(set, bySlot, optimizable, 0))
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/bis/ -run 'RankSlotUpgrades'`
Expected: `TestRankSlotUpgradesMultiCapAlwaysShowsBothInstances` FAILS (currently the StrongRing instance is omitted, so `got` has length 1). `TestRankSlotUpgradesSingleCapStillOmitsNoUpgrade` should PASS already (it documents preserved behavior).

- [ ] **Step 3: Implement the coverage change**

In `internal/bis/loadoutset.go`, inside `RankSlotUpgrades`, replace this block:

```go
			if len(cands) == 0 {
				continue
			}
```

with:

```go
			if len(cands) == 0 {
				// Single-capacity slots stay actionable: omit when there's no upgrade.
				// Multi-capacity slots always show every worn instance (both rings/
				// ears/wrists), even with no upgrade — Best stays the zero-value
				// "no upgrade" sentinel and sorts last.
				if capacityOf(slot) <= 1 || idx >= len(worn) {
					continue
				}
				out = append(out, su)
				continue
			}
```

(The `idx >= len(worn)` guard means an empty position with no candidate is still skipped — only WORN instances are force-shown.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/bis/`
Expected: PASS (whole package).

- [ ] **Step 5: Commit**

```bash
git add internal/bis/loadoutset.go internal/bis/loadoutset_test.go
git commit -m "feat(bis): multi-cap slots always show every worn instance"
```

---

## Task 2: Render `—` for no-upgrade rows

**Files:**
- Modify: `cmd/bis/main.go`
- Test: `cmd/bis/main_test.go`

- [ ] **Step 1: Write the failing test**

Append to `cmd/bis/main_test.go`:

```go
func TestRenderLoadoutReportNoUpgradeRow(t *testing.T) {
	f := loadout.File{CharacterName: "Biffels", LastUpdate: 123}
	reports := []bucketReport{{
		Title: "Get now — pre-raid accessible",
		Upgrades: []bis.SlotUpgrade{
			{
				Slot:          "Finger",
				EquippedName:  "StrongRing",
				EquippedID:    10,
				EquippedValue: 300,
				// zero-value Best => no upgrade in this bucket
			},
		},
	}}

	md := renderLoadoutReport(f, 30000, 30000, reports)

	// Worn item still linked; best cell is an em dash; alternative cell blank.
	require.Contains(t, md, "[StrongRing](https://u.eq2wire.com/item/10) (300) | — |  |")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/bis/ -run TestRenderLoadoutReportNoUpgradeRow`
Expected: FAIL — current code calls `upgradeCell(u.Best, current)` on an empty `Best`, producing a broken `**[](https://u.eq2wire.com/item/0) +0 (+0.0%)**` cell, not `—`.

- [ ] **Step 3: Implement the `—` branch**

In `cmd/bis/main.go`, in `renderLoadoutReport`'s row loop, replace these lines:

```go
			wearing := fmt.Sprintf("%s (%.0f)", bis.EQ2ULink(u.EquippedName, u.EquippedID), u.EquippedValue)
			best := fmt.Sprintf("**%s**", upgradeCell(u.Best, current))
			alt := ""
			if u.Alt != nil {
				alt = upgradeCell(*u.Alt, current)
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", u.Slot, wearing, best, alt)
```

with:

```go
			wearing := fmt.Sprintf("%s (%.0f)", bis.EQ2ULink(u.EquippedName, u.EquippedID), u.EquippedValue)
			best := "—"
			alt := ""
			if u.Best.Name != "" {
				best = fmt.Sprintf("**%s**", upgradeCell(u.Best, current))
				if u.Alt != nil {
					alt = upgradeCell(*u.Alt, current)
				}
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", u.Slot, wearing, best, alt)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/bis/`
Expected: PASS (including the existing `TestRenderLoadoutReportLinkedTable`).

- [ ] **Step 5: Commit**

```bash
git add cmd/bis/main.go cmd/bis/main_test.go
git commit -m "feat(bis): render em dash for no-upgrade instance rows"
```

---

## Task 3: Deterministic ordering (so committed reports diff cleanly)

**Problem:** Ranking sorts compare only ΔDPS and the code ranges over slot maps, so tied rows (e.g. multiple `+10` upgrades, all the `—`/0 rows) reshuffle every rebuild — noisy diffs on committed reports.

**Files:**
- Modify: `internal/bis/loadoutset.go` (candidate sort + cross-instance sort), `internal/bis/report.go` (`SlotCandidatesScored`), `internal/bis/render.go` (weight table sort).
- Test: `internal/bis/loadoutset_test.go`, `internal/bis/report_test.go`.

- [ ] **Step 1: Failing test for the upgrade report**

Append to `internal/bis/loadoutset_test.go`:

```go
func TestRankSlotUpgradesDeterministicTieBreak(t *testing.T) {
	lo := store.Loadout{Main: model.Weapon{AvgDamage: 160, MinDamage: 100, MaxDamage: 220, DelaySecs: 4}}
	set := NewSet(model.StatBlock{}, lo, 1.0, 600)
	set.Equipped["Finger"] = []store.ScorableItem{{ID: 1, Name: "Worn", Slot: "Finger"}}
	optimizable := map[string]bool{"Finger": true}
	// Two candidates with identical stats => identical ΔDPS (a tie).
	bySlot := map[string][]store.ScorableItem{
		"Finger": {
			{ID: 77, Name: "RingHi", Slot: "Finger", Stats: model.StatBlock{MultiAttack: 20}},
			{ID: 33, Name: "RingLo", Slot: "Finger", Stats: model.StatBlock{MultiAttack: 20}},
		},
	}

	got := RankSlotUpgrades(set, bySlot, optimizable, 0)
	// Tie broken by candidate id: lower id is Best.
	require.Equal(t, 33, got[0].Best.ID)
	require.Equal(t, 77, got[0].Alt.ID)

	// Whole result is stable across repeated runs.
	for i := 0; i < 5; i++ {
		require.Equal(t, got, RankSlotUpgrades(set, bySlot, optimizable, 0))
	}
}
```

- [ ] **Step 2: Run it — expect flaky/fail**

Run: `go test ./internal/bis/ -run TestRankSlotUpgradesDeterministicTieBreak -count=10`
Expected: FAIL on at least one of the 10 runs (the tie currently resolves by unstable sort, so `Best.ID` is sometimes 77).

- [ ] **Step 3: Add tiebreakers in `loadoutset.go`**

Replace the candidate sort:

```go
			sort.Slice(cands, func(i, j int) bool { return cands[i].Delta > cands[j].Delta })
```

with:

```go
			sort.Slice(cands, func(i, j int) bool {
				if cands[i].Delta != cands[j].Delta {
					return cands[i].Delta > cands[j].Delta
				}
				return cands[i].ID < cands[j].ID
			})
```

Replace the cross-instance sort:

```go
	sort.Slice(out, func(i, j int) bool { return out[i].Best.Delta > out[j].Best.Delta })
```

with:

```go
	sort.Slice(out, func(i, j int) bool {
		if out[i].Best.Delta != out[j].Best.Delta {
			return out[i].Best.Delta > out[j].Best.Delta
		}
		if out[i].Slot != out[j].Slot {
			return out[i].Slot < out[j].Slot
		}
		if out[i].EquippedID != out[j].EquippedID {
			return out[i].EquippedID < out[j].EquippedID
		}
		if out[i].EquippedName != out[j].EquippedName {
			return out[i].EquippedName < out[j].EquippedName
		}
		return out[i].Best.ID < out[j].Best.ID
	})
```

- [ ] **Step 4: Failing test for the from-scratch report**

Append to `internal/bis/report_test.go`:

```go
func TestSlotCandidatesScoredDeterministicTieBreak(t *testing.T) {
	lo := store.Loadout{Main: model.Weapon{AvgDamage: 160, DelaySecs: 4}}
	set := NewSet(model.StatBlock{}, lo, 1.0, 600)
	weights := map[string]float64{}
	// Identical stats => identical ΔDPS tie.
	cands := []store.ScorableItem{
		{ID: 77, Name: "Hi", Slot: "Chest", Stats: model.StatBlock{Flurry: 10}},
		{ID: 33, Name: "Lo", Slot: "Chest", Stats: model.StatBlock{Flurry: 10}},
	}

	for i := 0; i < 5; i++ {
		got := SlotCandidatesScored(set, "Chest", cands, weights)
		require.Equal(t, 33, got[0].Item.ID) // lower id first on tie
		require.Equal(t, 77, got[1].Item.ID)
	}
}
```

(If `report_test.go` lacks imports for `store`/`model`/`require`, add them.)

- [ ] **Step 5: Add the tiebreaker in `report.go`**

Replace:

```go
	sort.Slice(out, func(i, j int) bool { return out[i].Delta > out[j].Delta })
```

with:

```go
	sort.Slice(out, func(i, j int) bool {
		if out[i].Delta != out[j].Delta {
			return out[i].Delta > out[j].Delta
		}
		return out[i].Item.ID < out[j].Item.ID
	})
```

- [ ] **Step 6: Tiebreak the weight table in `render.go`** (for fully stable `bis-report.md`)

Replace:

```go
	sort.Slice(keys, func(i, j int) bool { return weights[keys[i]] > weights[keys[j]] })
```

with:

```go
	sort.Slice(keys, func(i, j int) bool {
		if weights[keys[i]] != weights[keys[j]] {
			return weights[keys[i]] > weights[keys[j]]
		}
		return keys[i] < keys[j]
	})
```

- [ ] **Step 7: Run all tests (with repeats to catch residual flakiness)**

Run: `go test ./internal/bis/ -count=5`
Expected: PASS on all runs.

- [ ] **Step 8: Commit**

```bash
git add internal/bis/loadoutset.go internal/bis/loadoutset_test.go internal/bis/report.go internal/bis/report_test.go internal/bis/render.go
git commit -m "fix(bis): deterministic ranking tiebreakers so committed reports diff cleanly"
```

## Final verification

- [ ] `go build ./... && go vet ./... && go test ./...` — all green.
- [ ] Regenerate the loadout report and confirm both worn instances now appear in every bucket for `Finger`/`Ear`/`Wrist`:
  ```bash
  go run ./cmd/bis --loadout characters/biffels/loadout.toml
  ```
  Open `characters/biffels/upgrade-report.md` and confirm the "Get now" bucket now shows two `Finger`, two `Ear`, and two `Wrist` rows (the stronger half showing `—` where it has no pre-raid upgrade).
- [ ] Commit the regenerated report:
  ```bash
  git add characters/biffels/upgrade-report.md
  git commit -m "chore: regenerate upgrade-report with both worn instances shown"
  ```
