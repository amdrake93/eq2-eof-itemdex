# Plan 2e — Converging BiS Set-Builder & Report Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Assassin's best-in-slot gear *set* by iterating to convergence — picking each slot's item by its real in-context ΔDPS at the evolving set baseline — then emit an explainable per-slot report (converged pick + top-3 Fabled / top-3 Legendary alternatives, with breakdowns) plus a `scores` table in `bis.db`.

**Architecture:** The model layer (`internal/model`: DPS, weights, the haste/MA/dps-mod curves with caps, `TotalDPSDual`, `AutoDPS`, `DeriveWeights`) is complete and merged on `main`. This plan adds: (1) `model.ItemDelta` — the exact ΔDPS of equipping an item into a set state, weapon-aware; (2) `model.ScoreItem` — a linear `Σ weight×stat` breakdown used only for the report's explainable "why"; (3) `scores` table + loaders in `internal/store`; (4) `internal/bis` — a `Set` whose DPS is recomputed from the full set every time, a coordinate-ascent `BuildSet` that fills each slot with the DPS-maximizing pick and repeats passes until convergence, per-slot in-context ranking, the locked-set re-model, and the markdown renderer; (5) `cmd/bis`.

**Why a converging builder, not per-item greedy:** the auto-attack stats — haste, multi-attack, crit, flurry, dps-mod — combine **multiplicatively** (`AutoDPS = swings(haste) × MA × crit × flurry × dpsMod`), so each one is worth *more* as the set accumulates its partners (raising haste makes flurry worth more, etc.). Meanwhile haste and dps-mod **hard-cap at 200** (the curve is flat past it). Scoring items in isolation against one fixed baseline therefore builds broken sets: it overstacks a capped stat (e.g. 400 haste, 200 wasted) because each item "looks good" alone, and it undervalues partner stats because the baseline starts at haste 0 / flurry 0. Recomputing DPS from the live set at every step fixes both for free: a stat already at its cap in the set contributes ~0 to the next pick, and a partner stat's value rises as the set fills in. This is spec §3's "iterate to convergence so saturation/caps self-correct."

**Tech Stack:** Go 1.26, `modernc.org/sqlite` (pure-Go), `stretchr/testify`.

**Spec:** `docs/design-plan2.md` §3 (scoring + iterate-to-convergence), §3.1 (curves/caps), §6 (scores table), §7 (report + breakdowns), §8 (locked-set re-model), §9 (validation).

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/model/itemdelta.go` *(new)* | `ItemDelta` — exact ΔDPS of adding an item (stats + optional off-hand weapon) to a set state. Pure. |
| `internal/model/score.go` *(new)* | `ScoreItem` + `ScoreTerm` — linear `Σ weight×stat` breakdown for the report's "why". Pure. |
| `internal/store/store.go` *(modify)* | add `scores` table; `ScoreRow`+`WriteScores`; `Loadout`+`LoadLoadout`; `ScorableItem`+`LoadScorableItems`. |
| `internal/bis/set.go` *(new)* | `Set` (profile + fixed main/arts + equipped-per-slot); full-set `DPS`, `restBase`/`offWeapon`/`restOff`, `CandidateDelta`. |
| `internal/bis/build.go` *(new)* | `slotCapacity`/`offHandSlot`; `pickBest` (greedy within slot, capacity-aware); `BuildSet` (coordinate ascent + convergence + locked slots). |
| `internal/bis/report.go` *(new)* | `ScoredItem`/`SlotReport`; `ConvergedWeights`; `SlotCandidatesScored`; `BuildSlotReports`. |
| `internal/bis/render.go` *(new)* | `Render` — markdown: per baseline, weight table, per-slot converged pick + top-N alternatives with breakdowns, assumptions block. |
| `cmd/bis/main.go` *(new)* | orchestrate: open `bis.db` → load loadout + items → per baseline build set, score, write `scores`, collect reports → render `--out`; `--lock` path. |

**Notes:**
- `cmd/weights` stays as a debug tool; do not delete it.
- Known simplification (consistent with `cmd/weights`): the fixed main-hand (Soulfire) contributes its weapon damage/delay but **not** its own gear stats to the baseline — `LoadLoadout` only reads avg/delay. Acceptable for relative ranking; noted in the report assumptions.

---

### Task 1: `model.ItemDelta` — exact ΔDPS of equipping an item

**Files:**
- Create: `internal/model/itemdelta.go`
- Test: `internal/model/itemdelta_test.go`

**Why:** This is the honest item-value primitive. It diffs two full `TotalDPSDual` evaluations at the set's live stat totals, so every nonlinearity is respected — a stat already at its cap contributes ~0, and the multiplicative auto cluster makes a stat worth more as its partners accumulate.

- [ ] **Step 1: Write the failing test**

```go
package model

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	"github.com/stretchr/testify/require"
)

func TestItemDeltaInteractionMultiplicative(t *testing.T) {
	main := Weapon{AvgDamage: 160, DelaySecs: 4}
	off := Weapon{AvgDamage: 158, DelaySecs: 4.4}
	var arts []spell.CombatArt
	flurry := StatBlock{Flurry: 10}

	// flurry is worth MORE when the set already has dps-mod (auto-attack is bigger),
	// because flurry multiplies auto-attack.
	low := ItemDelta(StatBlock{}, main, off, arts, flurry, nil)
	high := ItemDelta(StatBlock{DPSMod: 200}, main, off, arts, flurry, nil)
	require.Greater(t, high, low)
}

func TestItemDeltaCappedStatZero(t *testing.T) {
	main := Weapon{AvgDamage: 160, DelaySecs: 4}
	off := Weapon{AvgDamage: 158, DelaySecs: 4.4}
	var arts []spell.CombatArt
	// haste already at the 200 cap → more haste does nothing (curve flat past cap)
	d := ItemDelta(StatBlock{Haste: 200}, main, off, arts, StatBlock{Haste: 50}, nil)
	require.InDelta(t, 0.0, d, 1e-9)
}

func TestItemDeltaOffHandWeapon(t *testing.T) {
	main := Weapon{AvgDamage: 160, DelaySecs: 4}
	var arts []spell.CombatArt
	// equipping an off-hand weapon into an empty off-hand adds its auto-attack
	w := Weapon{AvgDamage: 150, DelaySecs: 4}
	d := ItemDelta(StatBlock{}, main, Weapon{}, arts, StatBlock{}, &w)
	require.Greater(t, d, 0.0)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run TestItemDelta -v`
Expected: FAIL — `undefined: ItemDelta`.

- [ ] **Step 3: Write minimal implementation**

```go
package model

import "github.com/amdrake93/eq2-eof-itemdex/internal/spell"

// ItemDelta is the ΔDPS of equipping an item into an otherwise-fixed set.
// restBase is the set's StatBlock with the target slot empty; restOff is the
// off-hand weapon when that slot is empty (zero Weapon if the target IS the
// off-hand slot, or no off-hand is equipped). itemStats is the candidate's
// stats; for an off-hand weapon candidate pass its weapon as newOff (else nil).
//
// It diffs full TotalDPSDual evaluations at the set's live stat totals, so a
// stat already at its cap in restBase contributes ~0 and the multiplicative
// auto cluster (haste·MA·crit·flurry·dps-mod) makes a stat worth more as the
// set accumulates its partners.
func ItemDelta(restBase StatBlock, main, restOff Weapon, arts []spell.CombatArt, itemStats StatBlock, newOff *Weapon) float64 {
	before := TotalDPSDual(restBase, main, restOff, arts)
	off := restOff
	if newOff != nil {
		off = *newOff
	}
	after := TotalDPSDual(restBase.Add(itemStats), main, off, arts)
	return after - before
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/model/ -run TestItemDelta -v`
Expected: PASS (all three).

- [ ] **Step 5: Commit**

```bash
git add internal/model/itemdelta.go internal/model/itemdelta_test.go
git commit -m "Add ItemDelta: exact cap/interaction-aware item ΔDPS"
```

---

### Task 2: `model.ScoreItem` — linear breakdown (for the report's "why")

**Files:**
- Create: `internal/model/score.go`
- Test: `internal/model/score_test.go`

**Why:** The set-builder ranks by `ItemDelta` (Task 1). `ScoreItem` is *only* for the explainable breakdown shown next to each ranked item — it attributes a `weight × stat` line per stat so a reader can see which stats drove an item, judged at the converged-baseline weights.

- [ ] **Step 1: Write the failing test**

```go
package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScoreItem(t *testing.T) {
	weights := map[string]float64{
		"reuse": 16.0, "potency": 7.0, "critchance": 2.0,
	}
	item := StatBlock{Potency: 35, CritChance: 22, Reuse: 4}

	total, terms := ScoreItem(weights, item)

	// 35*7 + 22*2 + 4*16 = 245 + 44 + 64 = 353
	require.InDelta(t, 353.0, total, 1e-9)
	require.Len(t, terms, 3)
	require.Equal(t, "potency", terms[0].Stat) // sorted by contribution desc
	require.InDelta(t, 245.0, terms[0].Contribution, 1e-9)
	require.InDelta(t, 35.0, terms[0].ItemValue, 1e-9)
	require.InDelta(t, 7.0, terms[0].Weight, 1e-9)
	require.Equal(t, "reuse", terms[1].Stat)
	require.Equal(t, "critchance", terms[2].Stat)
}

func TestScoreItemEmpty(t *testing.T) {
	total, terms := ScoreItem(map[string]float64{"potency": 7}, StatBlock{})
	require.InDelta(t, 0.0, total, 1e-9)
	require.Empty(t, terms)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run TestScoreItem -v`
Expected: FAIL — `undefined: ScoreItem` / `ScoreTerm`.

- [ ] **Step 3: Write minimal implementation**

```go
package model

import "sort"

// ScoreTerm is one stat's linear contribution: itemValue × weight.
type ScoreTerm struct {
	Stat         string
	ItemValue    float64
	Weight       float64
	Contribution float64
}

// ScoreItem returns Σ(weight × itemStat) over WeightStats and the per-stat
// breakdown (nonzero stats only) sorted by contribution descending. Used for
// the report's explainable breakdown, not for set selection (that uses ItemDelta).
func ScoreItem(weights map[string]float64, item StatBlock) (total float64, terms []ScoreTerm) {
	for _, s := range WeightStats {
		v := getStat(item, s)
		if v == 0 {
			continue
		}
		w := weights[s]
		c := w * v
		terms = append(terms, ScoreTerm{Stat: s, ItemValue: v, Weight: w, Contribution: c})
		total += c
	}
	sort.Slice(terms, func(i, j int) bool { return terms[i].Contribution > terms[j].Contribution })
	return total, terms
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/model/ -run TestScoreItem -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/model/score.go internal/model/score_test.go
git commit -m "Add ScoreItem: linear stat breakdown for report explainability"
```

---

### Task 3: `scores` table + `WriteScores`

**Files:**
- Modify: `internal/store/store.go` (schema const + new method)
- Test: `internal/store/scores_test.go`

- [ ] **Step 1: Write the failing test**

```go
package store

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteScores(t *testing.T) {
	d, err := Open(":memory:")
	require.NoError(t, err)
	defer d.Close()
	require.NoError(t, d.Init())

	rows := []ScoreRow{
		{ItemID: 1, Baseline: "solo", DPSScore: 100.5, Slot: "Chest"},
		{ItemID: 1, Baseline: "raid", DPSScore: 220.0, Slot: "Chest"},
		{ItemID: 2, Baseline: "solo", DPSScore: 80.0, Slot: "Head"},
	}
	require.NoError(t, d.WriteScores(rows))

	var n int
	require.NoError(t, d.SQL().QueryRow(`SELECT COUNT(*) FROM scores`).Scan(&n))
	require.Equal(t, 3, n)

	var score float64
	require.NoError(t, d.SQL().QueryRow(
		`SELECT dps_score FROM scores WHERE item_id=1 AND baseline='raid'`).Scan(&score))
	require.InDelta(t, 220.0, score, 1e-9)

	require.NoError(t, d.WriteScores(rows)) // idempotent (composite PK)
	require.NoError(t, d.SQL().QueryRow(`SELECT COUNT(*) FROM scores`).Scan(&n))
	require.Equal(t, 3, n)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestWriteScores -v`
Expected: FAIL — `undefined: ScoreRow` / `WriteScores`, `no such table: scores`.

- [ ] **Step 3: Write minimal implementation**

In `internal/store/store.go`, append to the `schema` const (after the `combat_arts` table, inside the backticks):

```sql
CREATE TABLE IF NOT EXISTS scores (
  item_id INTEGER, baseline TEXT, dps_score REAL, slot TEXT,
  PRIMARY KEY (item_id, baseline)
);
```

Then add:

```go
// ScoreRow is one item's in-context ΔDPS under one baseline.
type ScoreRow struct {
	ItemID   int
	Baseline string
	DPSScore float64
	Slot     string
}

// WriteScores upserts score rows in a single transaction.
func (d *DB) WriteScores(rows []ScoreRow) (err error) {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	for _, r := range rows {
		if _, err = tx.Exec(
			`INSERT OR REPLACE INTO scores (item_id, baseline, dps_score, slot) VALUES (?, ?, ?, ?)`,
			r.ItemID, r.Baseline, r.DPSScore, r.Slot,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestWriteScores -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/scores_test.go
git commit -m "Add scores table and WriteScores"
```

---

### Task 4: `store.LoadLoadout` — weapons + collapsed combat arts

**Files:**
- Modify: `internal/store/store.go` (add `model` import, `Loadout`, `LoadLoadout`)
- Test: `internal/store/loadout_test.go`

- [ ] **Step 1: Write the failing test**

```go
package store

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func seedLoadout(t *testing.T, d *DB) {
	t.Helper()
	exec := func(q string, a ...any) {
		_, err := d.SQL().Exec(q, a...)
		require.NoError(t, err)
	}
	exec(`INSERT INTO items (id,name,slot,tier,itemlevel,armor_type,skill,wieldstyle,classes,gamelink,weapon_min_dmg,weapon_max_dmg,delay,damage_rating)
	      VALUES (1,'Soulfire Gladius','Primary','MYTHICAL',70,'','slashing','One-Handed','assassin','',120,200,4.0,80)`)
	exec(`INSERT INTO items (id,name,slot,tier,itemlevel,armor_type,skill,wieldstyle,classes,gamelink,weapon_min_dmg,weapon_max_dmg,delay,damage_rating)
	      VALUES (2,'Enchanted Grove Scimitar','Secondary','FABLED',70,'','piercing','One-Handed','assassin','',118,198,4.4,75)`)
	exec(`INSERT INTO combat_arts (name,level,min_dmg,max_dmg,recast_secs,cast_secs_hundredths)
	      VALUES ('Assassinate II',70,7000,12000,300,50)`)
	exec(`INSERT INTO combat_arts (name,level,min_dmg,max_dmg,recast_secs,cast_secs_hundredths)
	      VALUES ('Assassinate I',60,5000,9000,300,50)`)
}

func TestLoadLoadout(t *testing.T) {
	d, err := Open(":memory:")
	require.NoError(t, err)
	defer d.Close()
	require.NoError(t, d.Init())
	seedLoadout(t, d)

	lo, err := d.LoadLoadout()
	require.NoError(t, err)
	require.Equal(t, "Soulfire Gladius", lo.MainName)
	require.InDelta(t, 160.0, lo.Main.AvgDamage, 1e-9)
	require.InDelta(t, 4.0, lo.Main.DelaySecs, 1e-9)
	require.Equal(t, "Enchanted Grove Scimitar", lo.OffName)
	require.InDelta(t, 158.0, lo.Off.AvgDamage, 1e-9)
	require.Len(t, lo.Arts, 1) // HighestRanks collapses the two Assassinate ranks
	require.Equal(t, "Assassinate II", lo.Arts[0].Name)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestLoadLoadout -v`
Expected: FAIL — `undefined: Loadout` / `LoadLoadout`.

- [ ] **Step 3: Write minimal implementation**

Add `"github.com/amdrake93/eq2-eof-itemdex/internal/model"` to the `internal/store/store.go` import block, then add:

```go
// Loadout is the fixed dual-wield setup + collapsed combat arts the model scores against.
type Loadout struct {
	Main, Off         model.Weapon
	MainName, OffName string
	Arts              []spell.CombatArt
}

func (d *DB) loadWeapon(query string, args ...any) (model.Weapon, string, error) {
	var name string
	var mn, mx, delay float64
	if err := d.db.QueryRow(query, args...).Scan(&name, &mn, &mx, &delay); err != nil {
		return model.Weapon{}, "", err
	}
	return model.Weapon{AvgDamage: (mn + mx) / 2, DelaySecs: delay}, name, nil
}

// LoadLoadout reads the Soulfire main-hand, the best Fabled 1H off-hand, and the
// Assassin combat arts collapsed to highest rank.
func (d *DB) LoadLoadout() (Loadout, error) {
	main, mainName, err := d.loadWeapon(
		`SELECT name, weapon_min_dmg, weapon_max_dmg, delay FROM items
		 WHERE name LIKE 'Soulfire%' AND classes LIKE '%assassin%'
		 ORDER BY weapon_max_dmg DESC LIMIT 1`)
	if err != nil {
		return Loadout{}, err
	}
	off, offName, err := d.loadWeapon(
		`SELECT name, weapon_min_dmg, weapon_max_dmg, delay FROM items
		 WHERE tier='FABLED' AND wieldstyle='One-Handed' AND classes LIKE '%assassin%'
		   AND skill IN ('piercing','slashing') AND delay BETWEEN 3.5 AND 4.5
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
	return Loadout{Main: main, Off: off, MainName: mainName, OffName: offName, Arts: spell.HighestRanks(arts)}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestLoadLoadout -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/loadout_test.go
git commit -m "Add LoadLoadout: weapons + collapsed combat arts"
```

---

### Task 5: `store.ScorableItem` + `LoadScorableItems`

**Files:**
- Modify: `internal/store/store.go`
- Test: `internal/store/scorable_test.go`

- [ ] **Step 1: Write the failing test**

```go
package store

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadScorableItems(t *testing.T) {
	d, err := Open(":memory:")
	require.NoError(t, err)
	defer d.Close()
	require.NoError(t, d.Init())
	exec := func(q string, a ...any) {
		_, err := d.SQL().Exec(q, a...)
		require.NoError(t, err)
	}
	exec(`INSERT INTO items (id,name,slot,tier,itemlevel,armor_type,skill,wieldstyle,classes,gamelink,weapon_min_dmg,weapon_max_dmg,delay,damage_rating)
	      VALUES (10,'Fabled Chest','Chest','FABLED',70,'Leather','','','assassin|ranger','link10',0,0,0,0)`)
	exec(`INSERT INTO item_stats (item_id,stat,value) VALUES (10,'basemodifier',35)`)
	exec(`INSERT INTO item_stats (item_id,stat,value) VALUES (10,'critchance',22)`)
	exec(`INSERT INTO items (id,name,slot,tier,itemlevel,armor_type,skill,wieldstyle,classes,gamelink,weapon_min_dmg,weapon_max_dmg,delay,damage_rating)
	      VALUES (11,'Fabled Dirk','Secondary','FABLED',70,'','piercing','One-Handed','assassin','link11',118,198,4.4,75)`)
	exec(`INSERT INTO item_stats (item_id,stat,value) VALUES (11,'flurry',5)`)
	exec(`INSERT INTO items (id,name,slot,tier,itemlevel,armor_type,skill,wieldstyle,classes,gamelink,weapon_min_dmg,weapon_max_dmg,delay,damage_rating)
	      VALUES (12,'Wizard Hat','Head','FABLED',70,'Cloth','','','wizard','link12',0,0,0,0)`)

	items, err := d.LoadScorableItems()
	require.NoError(t, err)
	require.Len(t, items, 2) // wizard item excluded

	byID := map[int]ScorableItem{}
	for _, it := range items {
		byID[it.ID] = it
	}
	chest := byID[10]
	require.Equal(t, "Chest", chest.Slot)
	require.Equal(t, "FABLED", chest.Tier)
	require.Equal(t, "link10", chest.GameLink)
	require.InDelta(t, 35.0, chest.Stats.Potency, 1e-9)
	require.InDelta(t, 22.0, chest.Stats.CritChance, 1e-9)
	require.False(t, chest.IsWeapon())

	dirk := byID[11]
	require.True(t, dirk.IsWeapon())
	require.InDelta(t, 158.0, dirk.WeaponAvg, 1e-9)
	require.InDelta(t, 4.4, dirk.WeaponDelay, 1e-9)
	require.InDelta(t, 5.0, dirk.Stats.Flurry, 1e-9)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestLoadScorableItems -v`
Expected: FAIL — `undefined: ScorableItem` / `LoadScorableItems` / `IsWeapon`.

- [ ] **Step 3: Write minimal implementation**

```go
// ScorableItem is one Assassin-usable item with its DPS-relevant stats resolved
// into a StatBlock plus weapon damage fields (0 for non-weapons).
type ScorableItem struct {
	ID          int
	Name        string
	Slot        string
	Tier        string
	GameLink    string
	WeaponAvg   float64
	WeaponDelay float64
	Stats       model.StatBlock
	Mods        map[string]float64
}

// IsWeapon reports whether the item swings as a weapon (has an attack delay).
func (it ScorableItem) IsWeapon() bool { return it.WeaponDelay > 0 }

// LoadScorableItems loads every Assassin-usable item with its modifier stats.
func (d *DB) LoadScorableItems() ([]ScorableItem, error) {
	rows, err := d.db.Query(
		`SELECT id, name, slot, tier, gamelink, weapon_min_dmg, weapon_max_dmg, delay
		 FROM items WHERE classes LIKE '%assassin%'`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var items []ScorableItem
	for rows.Next() {
		var it ScorableItem
		var mn, mx, delay float64
		if err := rows.Scan(&it.ID, &it.Name, &it.Slot, &it.Tier, &it.GameLink, &mn, &mx, &delay); err != nil {
			return nil, err
		}
		if delay > 0 {
			it.WeaponAvg = (mn + mx) / 2
			it.WeaponDelay = delay
		}
		it.Mods = map[string]float64{}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range items {
		sr, err := d.db.Query(`SELECT stat, value FROM item_stats WHERE item_id = ?`, items[i].ID)
		if err != nil {
			return nil, err
		}
		for sr.Next() {
			var stat string
			var val float64
			if err := sr.Scan(&stat, &val); err != nil {
				_ = sr.Close()
				return nil, err
			}
			items[i].Mods[stat] = val
		}
		if err := sr.Err(); err != nil {
			_ = sr.Close()
			return nil, err
		}
		_ = sr.Close()
		items[i].Stats.AddModifiers(items[i].Mods)
	}
	return items, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestLoadScorableItems -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/scorable_test.go
git commit -m "Add ScorableItem and LoadScorableItems"
```

---

### Task 6: `bis.Set` — full-set DPS + slot context helpers

**Files:**
- Create: `internal/bis/set.go`
- Test: `internal/bis/set_test.go`

**Why:** The set's DPS is always recomputed from the full set (profile baseline + every equipped item's stats + the off-hand weapon), so caps and interactions are exact at every step. `CandidateDelta` gives a candidate's in-context ΔDPS for a slot by diffing against the rest of the set.

- [ ] **Step 1: Write the failing test**

```go
package bis

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
	"github.com/stretchr/testify/require"
)

func testLoadout() store.Loadout {
	return store.Loadout{
		Main: model.Weapon{AvgDamage: 160, DelaySecs: 4},
		Off:  model.Weapon{},
	}
}

func TestSetDPSAndCandidateDelta(t *testing.T) {
	set := NewSet(model.StatBlock{}, testLoadout())

	// empty set DPS = main-hand auto only (no off-hand, no arts, no stats)
	require.InDelta(t, 40.0, set.DPS(), 1e-6) // 160/4 swings * 1.0 factors

	// a chest with potency raises CADPS? no arts here, so potency does nothing;
	// use flurry which multiplies auto-attack
	chest := store.ScorableItem{ID: 1, Slot: "Chest", Stats: model.StatBlock{Flurry: 10}}
	d := set.CandidateDelta("Chest", chest)
	require.Greater(t, d, 0.0)

	// an off-hand weapon adds its own auto-attack
	off := store.ScorableItem{ID: 2, Slot: "Secondary", WeaponAvg: 150, WeaponDelay: 4, Stats: model.StatBlock{}}
	require.True(t, off.IsWeapon())
	require.Greater(t, set.CandidateDelta("Secondary", off), 0.0)
}

func TestSetRestBaseExcludesSlot(t *testing.T) {
	set := NewSet(model.StatBlock{}, testLoadout())
	set.Equipped["Head"] = []store.ScorableItem{{ID: 1, Slot: "Head", Stats: model.StatBlock{Potency: 10}}}
	set.Equipped["Chest"] = []store.ScorableItem{{ID: 2, Slot: "Chest", Stats: model.StatBlock{Potency: 25}}}

	require.InDelta(t, 35.0, set.restBase("").Potency, 1e-9)      // both
	require.InDelta(t, 25.0, set.restBase("Head").Potency, 1e-9)  // Head excluded
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bis/ -run TestSet -v`
Expected: FAIL — package/symbols undefined.

- [ ] **Step 3: Write minimal implementation**

```go
// Package bis builds the Assassin's best-in-slot gear set by iterating to
// convergence and renders the explainable report.
package bis

import (
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
)

// offHandSlot is the census slot name for the off-hand weapon.
const offHandSlot = "Secondary"

// Set is a candidate gear set: a profile baseline + fixed main-hand/arts + the
// items chosen per slot. DPS is always recomputed from the full set so caps and
// interactions are exact.
type Set struct {
	Profile  model.StatBlock
	Main     model.Weapon
	Arts     []spell.CombatArt
	Equipped map[string][]store.ScorableItem
}

// NewSet returns an empty set seeded with the profile baseline and loadout.
func NewSet(profile model.StatBlock, lo store.Loadout) *Set {
	return &Set{Profile: profile, Main: lo.Main, Arts: lo.Arts, Equipped: map[string][]store.ScorableItem{}}
}

// restBase is the set's StatBlock with one slot's items excluded (exclude=""
// includes everything).
func (s *Set) restBase(exclude string) model.StatBlock {
	b := s.Profile
	for slot, items := range s.Equipped {
		if slot == exclude {
			continue
		}
		for _, it := range items {
			b = b.Add(it.Stats)
		}
	}
	return b
}

// offWeapon is the equipped off-hand weapon (zero Weapon if none).
func (s *Set) offWeapon() model.Weapon {
	for _, it := range s.Equipped[offHandSlot] {
		if it.IsWeapon() {
			return model.Weapon{AvgDamage: it.WeaponAvg, DelaySecs: it.WeaponDelay}
		}
	}
	return model.Weapon{}
}

// restOff is the off-hand weapon excluding a slot (zero if the slot IS the off-hand).
func (s *Set) restOff(exclude string) model.Weapon {
	if exclude == offHandSlot {
		return model.Weapon{}
	}
	return s.offWeapon()
}

// DPS is the full set's modeled TotalDPS.
func (s *Set) DPS() float64 {
	return model.TotalDPSDual(s.restBase(""), s.Main, s.offWeapon(), s.Arts)
}

// CandidateDelta is the in-context ΔDPS of putting a candidate in a slot, given
// the rest of the (otherwise-fixed) set with that slot emptied.
func (s *Set) CandidateDelta(slot string, c store.ScorableItem) float64 {
	rb := s.restBase(slot)
	ro := s.restOff(slot)
	if slot == offHandSlot && c.IsWeapon() {
		w := model.Weapon{AvgDamage: c.WeaponAvg, DelaySecs: c.WeaponDelay}
		return model.ItemDelta(rb, s.Main, ro, s.Arts, c.Stats, &w)
	}
	return model.ItemDelta(rb, s.Main, ro, s.Arts, c.Stats, nil)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bis/ -run TestSet -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/bis/set.go internal/bis/set_test.go
git commit -m "Add bis.Set: full-set DPS and slot-context deltas"
```

---

### Task 7: `bis.pickBest` — greedy capacity-aware slot fill

**Files:**
- Create: `internal/bis/build.go`
- Test: `internal/bis/build_test.go`

**Why:** A slot may hold more than one item (Ear/Finger/Wrist/Charm = 2). `pickBest` greedily adds the candidate that maximizes the *full set* DPS, in the context of those already chosen for that slot — so a second item that would just overstack a near-capped stat loses to one that adds a fresh multiplier.

- [ ] **Step 1: Write the failing test**

```go
package bis

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
	"github.com/stretchr/testify/require"
)

func TestCapacityOf(t *testing.T) {
	require.Equal(t, 2, capacityOf("Ear"))
	require.Equal(t, 2, capacityOf("Finger"))
	require.Equal(t, 1, capacityOf("Chest"))
}

func TestPickBestRespectsCapacityAndContext(t *testing.T) {
	set := NewSet(model.StatBlock{}, testLoadout())
	// Ear holds 2. One flurry item and one haste item beat two of the same,
	// because the auto cluster is multiplicative (mixing partners > stacking one).
	cands := []store.ScorableItem{
		{ID: 1, Slot: "Ear", Tier: "FABLED", Stats: model.StatBlock{Flurry: 20}},
		{ID: 2, Slot: "Ear", Tier: "FABLED", Stats: model.StatBlock{Haste: 100}},
		{ID: 3, Slot: "Ear", Tier: "FABLED", Stats: model.StatBlock{Flurry: 20}},
	}
	got := pickBest(set, "Ear", cands)
	require.Len(t, got, 2)
	ids := map[int]bool{got[0].ID: true, got[1].ID: true}
	require.True(t, ids[2], "should include the haste item (a fresh multiplier)")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bis/ -run "TestCapacityOf|TestPickBest" -v`
Expected: FAIL — `undefined: capacityOf` / `pickBest`.

- [ ] **Step 3: Write minimal implementation**

```go
package bis

import (
	"math"

	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
)

// slotCapacity is how many items a census slot equips; unlisted slots hold 1.
var slotCapacity = map[string]int{
	"Ear": 2, "Finger": 2, "Wrist": 2, "Charm": 2,
}

func capacityOf(slot string) int {
	if n, ok := slotCapacity[slot]; ok {
		return n
	}
	return 1
}

// pickBest greedily chooses up to capacityOf(slot) distinct candidates that
// maximize the full set DPS, each addition evaluated in the context of those
// already chosen (so within-slot caps/interactions are respected). It restores
// the slot's original contents before returning.
func pickBest(set *Set, slot string, cands []store.ScorableItem) []store.ScorableItem {
	orig := set.Equipped[slot]
	defer func() { set.Equipped[slot] = orig }()

	capN := capacityOf(slot)
	chosen := []store.ScorableItem{}
	used := map[int]bool{}
	for len(chosen) < capN {
		bestIdx, bestDPS := -1, math.Inf(-1)
		for i, c := range cands {
			if used[c.ID] {
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

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bis/ -run "TestCapacityOf|TestPickBest" -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/bis/build.go internal/bis/build_test.go
git commit -m "Add pickBest: greedy capacity-aware slot fill"
```

---

### Task 8: `bis.BuildSet` — coordinate-ascent to convergence

**Files:**
- Modify: `internal/bis/build.go`
- Test: `internal/bis/buildset_test.go`

**Why:** This is the core. Fill every optimizable slot with its DPS-maximizing pick at the current set state, then repeat passes until no slot changes. Because every evaluation uses the live set totals, a capped stat stops being chosen once the SET hits the cap (no 400-haste sets) and a partner stat gets picked once its companions are present. `Primary` is the fixed Soulfire (not a slot here); locked slots are pinned.

- [ ] **Step 1: Write the failing test**

```go
package bis

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
	"github.com/stretchr/testify/require"
)

func TestBuildSetStopsStackingPastCap(t *testing.T) {
	lo := testLoadout()
	haste := func(id int, slot string) store.ScorableItem {
		return store.ScorableItem{ID: id, Slot: slot, Tier: "FABLED", Stats: model.StatBlock{Haste: 150}}
	}
	flurry := func(id int, slot string) store.ScorableItem {
		return store.ScorableItem{ID: id, Slot: slot, Tier: "FABLED", Stats: model.StatBlock{Flurry: 30}}
	}
	bySlot := map[string][]store.ScorableItem{
		"Head":  {haste(1, "Head"), flurry(2, "Head")},
		"Chest": {haste(3, "Chest"), flurry(4, "Chest")},
	}
	set := BuildSet(model.StatBlock{}, lo, bySlot, nil, 12)

	// One slot toward the haste cap, the other switches to flurry — a SECOND
	// 150-haste would push 300 (capped at 200, mostly wasted), so flurry wins.
	picks := []int{set.Equipped["Head"][0].ID, set.Equipped["Chest"][0].ID}
	hasteCount := 0
	for _, id := range picks {
		if id == 1 || id == 3 {
			hasteCount++
		}
	}
	require.Equal(t, 1, hasteCount, "exactly one haste item; the cap stops the second")
}

func TestBuildSetRespectsLocked(t *testing.T) {
	lo := testLoadout()
	bySlot := map[string][]store.ScorableItem{
		"Head": {{ID: 1, Slot: "Head", Tier: "FABLED", Stats: model.StatBlock{Potency: 5}}},
	}
	locked := map[string][]store.ScorableItem{
		"Chest": {{ID: 99, Slot: "Chest", Tier: "FABLED", Stats: model.StatBlock{Flurry: 50}}},
	}
	set := BuildSet(model.StatBlock{}, lo, bySlot, locked, 12)
	require.Equal(t, 99, set.Equipped["Chest"][0].ID) // locked stays
	require.Equal(t, 1, set.Equipped["Head"][0].ID)   // optimized around it
}

func TestBuildSetConverges(t *testing.T) {
	lo := testLoadout()
	bySlot := map[string][]store.ScorableItem{
		"Head":  {{ID: 1, Slot: "Head", Tier: "FABLED", Stats: model.StatBlock{Flurry: 10}}},
		"Chest": {{ID: 2, Slot: "Chest", Tier: "FABLED", Stats: model.StatBlock{Haste: 50}}},
	}
	a := BuildSet(model.StatBlock{}, lo, bySlot, nil, 12)
	b := BuildSet(model.StatBlock{}, lo, bySlot, nil, 12)
	// deterministic / idempotent
	require.Equal(t, a.Equipped["Head"][0].ID, b.Equipped["Head"][0].ID)
	require.Equal(t, 1, a.Equipped["Head"][0].ID)
	require.Equal(t, 2, a.Equipped["Chest"][0].ID)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bis/ -run TestBuildSet -v`
Expected: FAIL — `undefined: BuildSet`.

- [ ] **Step 3: Write minimal implementation**

Append to `internal/bis/build.go` (add `"sort"` to its imports):

```go
// mainHandSlot is the fixed main-hand slot (Soulfire); it is not optimized.
const mainHandSlot = "Primary"

func sameItems(a, b []store.ScorableItem) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID {
			return false
		}
	}
	return true
}

// BuildSet runs coordinate ascent: each pass fills every optimizable slot with
// the DPS-maximizing pick at the current set state; passes repeat until no slot
// changes (converged) or maxPasses is hit. Locked slots are pre-filled and never
// re-optimized; the main-hand slot is fixed to the loadout and excluded.
func BuildSet(profile model.StatBlock, lo store.Loadout, bySlot, locked map[string][]store.ScorableItem, maxPasses int) *Set {
	set := NewSet(profile, lo)
	lockedSlot := map[string]bool{}
	for slot, items := range locked {
		set.Equipped[slot] = items
		lockedSlot[slot] = true
	}
	slots := make([]string, 0, len(bySlot))
	for slot := range bySlot {
		if slot == mainHandSlot || lockedSlot[slot] {
			continue
		}
		slots = append(slots, slot)
	}
	sort.Strings(slots)

	for pass := 0; pass < maxPasses; pass++ {
		changed := false
		for _, slot := range slots {
			best := pickBest(set, slot, bySlot[slot])
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
}
```

Add `"github.com/amdrake93/eq2-eof-itemdex/internal/model"` to `build.go` imports (used by the signature).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bis/ -run TestBuildSet -v`
Expected: PASS (all three).

- [ ] **Step 5: Commit**

```bash
git add internal/bis/build.go internal/bis/buildset_test.go
git commit -m "Add BuildSet: coordinate-ascent converging set-builder"
```

---

### Task 9: `bis` report layer — converged weights + per-slot in-context ranking

**Files:**
- Create: `internal/bis/report.go`
- Test: `internal/bis/report_test.go`

**Why:** Once the set converges, derive the weights at its full baseline (for explainable breakdowns), then rank each slot's candidates by their in-context ΔDPS (Task 6's `CandidateDelta`) so every alternative is judged in the set it would actually join. Top-3 Fabled + top-3 Legendary + all Mythical per slot (spec §7).

- [ ] **Step 1: Write the failing test**

```go
package bis

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
	"github.com/stretchr/testify/require"
)

func TestBuildSlotReports(t *testing.T) {
	lo := testLoadout()
	mk := func(id int, tier string, flurry float64) store.ScorableItem {
		return store.ScorableItem{ID: id, Name: "i", Slot: "Ear", Tier: tier, Stats: model.StatBlock{Flurry: flurry}}
	}
	bySlot := map[string][]store.ScorableItem{
		"Ear": {mk(1, "FABLED", 5), mk(2, "FABLED", 20), mk(3, "FABLED", 12), mk(4, "FABLED", 1),
			mk(5, "LEGENDARY", 8), mk(6, "MYTHICAL", 50)},
	}
	set := BuildSet(model.StatBlock{}, lo, bySlot, nil, 12)
	weights := ConvergedWeights(set)
	reports := BuildSlotReports(set, bySlot, weights, 3)

	require.Len(t, reports, 1)
	r := reports[0]
	require.Equal(t, "Ear", r.Slot)
	require.Len(t, r.Chosen, 2) // Ear capacity 2

	// Fabled alternatives: top 3 by in-context ΔDPS, highest flurry first
	require.Len(t, r.Fabled, 3)
	require.Equal(t, 2, r.Fabled[0].Item.ID) // flurry 20 is the strongest
	require.Greater(t, r.Fabled[0].Delta, r.Fabled[1].Delta)
	require.NotEmpty(t, r.Fabled[0].Terms) // breakdown attached
	require.Len(t, r.Legendary, 1)
	require.Len(t, r.Mythical, 1)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bis/ -run TestBuildSlotReports -v`
Expected: FAIL — undefined symbols.

- [ ] **Step 3: Write minimal implementation**

```go
package bis

import (
	"sort"

	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
)

// ScoredItem is one candidate with its in-context ΔDPS and explainable breakdown.
type ScoredItem struct {
	Item  store.ScorableItem
	Delta float64
	Terms []model.ScoreTerm
}

// SlotReport is one slot's converged pick plus ranked alternatives by tier.
type SlotReport struct {
	Slot      string
	Chosen    []store.ScorableItem
	Mythical  []ScoredItem
	Fabled    []ScoredItem
	Legendary []ScoredItem
}

// ConvergedWeights derives the stat weights at the converged set's full baseline,
// against the converged off-hand — the weights used for explainable breakdowns.
func ConvergedWeights(set *Set) map[string]float64 {
	base := set.restBase("")
	off := set.offWeapon()
	dps := func(sb model.StatBlock) float64 {
		return model.TotalDPSDual(sb, set.Main, off, set.Arts)
	}
	return model.DeriveWeights(base, dps)
}

// SlotCandidatesScored ranks a slot's candidates by in-context ΔDPS (against the
// converged set with that slot emptied), attaching the weight×stat breakdown.
func SlotCandidatesScored(set *Set, slot string, cands []store.ScorableItem, weights map[string]float64) []ScoredItem {
	out := make([]ScoredItem, 0, len(cands))
	for _, c := range cands {
		_, terms := model.ScoreItem(weights, c.Stats)
		out = append(out, ScoredItem{Item: c, Delta: set.CandidateDelta(slot, c), Terms: terms})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Delta > out[j].Delta })
	return out
}

func topByTier(scored []ScoredItem, tier string, n int) []ScoredItem {
	var f []ScoredItem
	for _, s := range scored {
		if s.Item.Tier == tier {
			f = append(f, s)
		}
	}
	if n >= 0 && len(f) > n {
		f = f[:n]
	}
	return f
}

// BuildSlotReports produces one SlotReport per slot (sorted), each with the
// converged pick and top-n Fabled/Legendary + all Mythical alternatives.
func BuildSlotReports(set *Set, bySlot map[string][]store.ScorableItem, weights map[string]float64, n int) []SlotReport {
	slots := make([]string, 0, len(bySlot))
	for slot := range bySlot {
		slots = append(slots, slot)
	}
	sort.Strings(slots)

	reports := make([]SlotReport, 0, len(slots))
	for _, slot := range slots {
		scored := SlotCandidatesScored(set, slot, bySlot[slot], weights)
		reports = append(reports, SlotReport{
			Slot:      slot,
			Chosen:    set.Equipped[slot],
			Mythical:  topByTier(scored, "MYTHICAL", -1),
			Fabled:    topByTier(scored, "FABLED", n),
			Legendary: topByTier(scored, "LEGENDARY", n),
		})
	}
	return reports
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bis/ -run TestBuildSlotReports -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/bis/report.go internal/bis/report_test.go
git commit -m "Add report layer: converged weights and per-slot in-context ranking"
```

---

### Task 10: `bis.Render` — markdown report

**Files:**
- Create: `internal/bis/render.go`
- Test: `internal/bis/render_test.go`

- [ ] **Step 1: Write the failing test**

```go
package bis

import (
	"strings"
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
	"github.com/stretchr/testify/require"
)

func TestRender(t *testing.T) {
	weights := map[string]float64{"reuse": 16.67, "potency": 7.19}
	reports := []SlotReport{{
		Slot:   "Chest",
		Chosen: []store.ScorableItem{{ID: 2, Name: "Fabled Chest", Tier: "FABLED", GameLink: "LINK2"}},
		Fabled: []ScoredItem{{
			Item:  store.ScorableItem{ID: 2, Name: "Fabled Chest", Tier: "FABLED", GameLink: "LINK2"},
			Delta: 41.2,
			Terms: []model.ScoreTerm{{Stat: "potency", ItemValue: 35, Weight: 7.19, Contribution: 251.65}},
		}},
	}}

	out := Render([]BaselineReport{{Name: "RAID", Weights: weights, Reports: reports}})

	require.Contains(t, out, "## RAID")
	require.Contains(t, out, "### Chest")
	require.Contains(t, out, "BiS: **Fabled Chest**")
	require.Contains(t, out, "Fabled Chest")
	require.Contains(t, out, "+41.2 DPS") // in-context ΔDPS
	require.Contains(t, out, "LINK2")
	require.Contains(t, out, "potency 35 × 7.19")
	require.Contains(t, out, "reuse")       // weight table
	require.Contains(t, out, "Assumptions") // constants block
	require.Contains(t, out, "in-context ΔDPS")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bis/ -run TestRender -v`
Expected: FAIL — `undefined: Render` / `BaselineReport`.

- [ ] **Step 3: Write minimal implementation**

```go
package bis

import (
	"fmt"
	"sort"
	"strings"

	"github.com/amdrake93/eq2-eof-itemdex/internal/constants"
)

// BaselineReport is one baseline's converged weights + per-slot reports.
type BaselineReport struct {
	Name    string
	Weights map[string]float64
	Reports []SlotReport
}

func writeWeightTable(b *strings.Builder, weights map[string]float64) {
	keys := make([]string, 0, len(weights))
	for k := range weights {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return weights[keys[i]] > weights[keys[j]] })
	b.WriteString("| stat | weight |\n|---|---:|\n")
	for _, k := range keys {
		fmt.Fprintf(b, "| %s | %.4f |\n", k, weights[k])
	}
	b.WriteString("\n")
}

func writeScored(b *strings.Builder, s ScoredItem) {
	fmt.Fprintf(b, "- **%s** — +%.1f DPS", s.Item.Name, s.Delta)
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

func writeTier(b *strings.Builder, label string, items []ScoredItem) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "_%s_\n\n", label)
	for _, s := range items {
		writeScored(b, s)
	}
	b.WriteString("\n")
}

// Render produces the full markdown BiS report across all baselines.
func Render(reports []BaselineReport) string {
	var b strings.Builder
	b.WriteString("# Assassin EoF Best-in-Slot\n\n")
	b.WriteString("_Per-item numbers are in-context ΔDPS at the converged set; the `stat × weight` lines are the explainable breakdown at the converged-baseline weights._\n\n")
	for _, r := range reports {
		fmt.Fprintf(&b, "## %s\n\n", r.Name)
		b.WriteString("Converged stat weights (marginal DPS per +1 stat):\n\n")
		writeWeightTable(&b, r.Weights)
		for _, sr := range r.Reports {
			fmt.Fprintf(&b, "### %s\n\n", sr.Slot)
			if len(sr.Chosen) > 0 {
				names := make([]string, 0, len(sr.Chosen))
				for _, c := range sr.Chosen {
					names = append(names, c.Name)
				}
				fmt.Fprintf(&b, "BiS: **%s**\n\n", strings.Join(names, "**, **"))
			}
			writeTier(&b, "Mythical (ceiling)", sr.Mythical)
			writeTier(&b, "Fabled", sr.Fabled)
			writeTier(&b, "Legendary", sr.Legendary)
		}
	}
	writeAssumptions(&b)
	return b.String()
}

func writeAssumptions(b *strings.Builder) {
	b.WriteString("---\n\n## Assumptions & Constants\n\n")
	fmt.Fprintf(b, "- crit ×%.2f; flurry ×%.1f; ability-mod cap = %.0f%% of potency-adjusted CA base\n",
		constants.CritMultiplier, constants.FlurryMultiplier, constants.AbilityModCapFrac*100)
	fmt.Fprintf(b, "- haste & dps-mod: shared diminishing curve, hard cap %.0f stat → 125%%\n", constants.HasteStatCap)
	fmt.Fprintf(b, "- reuse halves recast at %.0f%%; CA cast+recovery = %.2fs; fight = %.0fs\n",
		constants.ReuseHalvesAt, constants.CACastTimeSecs+constants.CARecoverySecs, constants.FightDurationSecs)
	b.WriteString("- Set built by coordinate-ascent to convergence (caps/interactions resolved at the live set baseline).\n")
	b.WriteString("- Main-hand is fixed (Soulfire); its own gear stats are not folded into the baseline. See docs/design-plan2.md §3.1.\n")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bis/ -run TestRender -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/bis/render.go internal/bis/render_test.go
git commit -m "Add Render: markdown BiS report (converged set + in-context alternatives)"
```

---

### Task 11: `cmd/bis` — orchestrator

**Files:**
- Create: `cmd/bis/main.go`
- Test: manual (CLI integration; logic unit-tested in Tasks 1–10)

**Context:** Operates on an existing `bis.db` (built by `cmd/builddb`). For each baseline it builds the converged set, ranks per slot, writes the `scores` table (in-context ΔDPS), and renders the report. `--lock` (comma-separated item IDs) adds a locked-set raid re-model section.

- [ ] **Step 1: Write the implementation**

```go
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

// scoreRows turns every slot report's ranked candidates into score rows.
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

// lockedItems pulls the full ScorableItem for each locked id, grouped by slot.
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
```

- [ ] **Step 2: Build and run against the real db**

Run:
```bash
go build ./... && go vet ./...
go run ./cmd/bis --db bis.db --out bis-report.md
```
Expected: prints the loadout line + `wrote bis-report.md and N score rows`. (If `bis.db` is missing: `go run ./cmd/builddb` first.)

- [ ] **Step 3: Sanity-check the output**

Run: `sed -n '1,80p' bis-report.md`
Expected: `## SOLO` with a weight table, then `### <slot>` blocks each showing `BiS: **<item>**` plus Fabled/Legendary alternatives with `+X DPS` and `stat N × W = C` breakdown lines; the Soulfire weapon under Mythical in its slot. Eyeball that no slot stacks a capped stat absurdly (the whole point of the rebuild).

- [ ] **Step 4: Full suite + lint**

Run:
```bash
go test ./...
golangci-lint run ./...
```
Expected: all pass; 0 lint issues.

- [ ] **Step 5: Commit**

```bash
git add cmd/bis/main.go
git commit -m "Add cmd/bis: build converged set, write scores, render report"
```

---

## Self-Review

**1. Spec coverage:**
- §3 scoring + **iterate to convergence** → Task 1 (`ItemDelta`), Task 8 (`BuildSet` coordinate ascent). ✔
- §3.1 caps/curves honored → `ItemDelta` diffs full `TotalDPSDual`, so the curve caps and multiplicative interactions apply automatically (Task 1 tests assert cap→0 and partner-amplification). ✔
- §6 `scores (item_id, baseline, dps_score, slot)` → Task 3; `items`/`item_stats` already exist. ✔
- §7 top-3 Fabled + top-3 Legendary per slot, Mythical ceiling, per-item breakdown, weight table, assumptions, `bis.db` artifact → Tasks 9, 10, 11. ✔
- §8 locked-items re-model → Task 8 (`locked` arg) + `--lock` in Task 11. ✔
- §9 validation: in-context ΔDPS + breakdown is the eyeball artifact; loop is human-driven. ✔

**2. Placeholder scan:** No TBD/TODO; every code step is complete; every test asserts real values. ✔

**3. Type consistency:** `model.ItemDelta(restBase, main, restOff, arts, itemStats, *newOff)` used identically in Tasks 1 and 6. `store.ScorableItem` (`ID/Name/Slot/Tier/GameLink/WeaponAvg/WeaponDelay/Stats/Mods` + `IsWeapon`) consistent in Tasks 5–9, 11. `store.Loadout` (`Main/Off/MainName/OffName/Arts`) consistent in Tasks 4, 6, 11. `bis.Set` (`Profile/Main/Arts/Equipped`, methods `restBase/offWeapon/restOff/DPS/CandidateDelta`) consistent in Tasks 6–9. `bis.ScoredItem` (`Item/Delta/Terms`), `SlotReport` (`Slot/Chosen/Mythical/Fabled/Legendary`), `BaselineReport` (`Name/Weights/Reports`) consistent in Tasks 9–11. `slotCapacity`/`offHandSlot`/`mainHandSlot` consistent in Tasks 6–8. `store.ScoreRow` consistent in Tasks 3, 11. ✔

**Design notes (intentional):**
- **Greedy coordinate ascent is a heuristic, not a global optimum.** Slots interact only through the shared baseline (caps + multiplicative scaling), and convergence passes re-optimize each slot against the others, so it lands at a strong local optimum. Full combinatorial optimization is intractable and unnecessary. `BuildSet` is deterministic (slots sorted, candidates in load order) → reproducible reports.
- **Main-hand stats not folded in** (Soulfire contributes weapon damage/delay only) — consistent with `cmd/weights`; noted in the report. Folding them in later is a localized `LoadLoadout` change.
- **Ranged slot** is treated as a normal stat slot (its auto-attack isn't modeled); it contributes only stats, so its ΔDPS is small but honest.

**Performance:** ~17 optimizable slots × ~100–250 candidates × capacity × ~3–6 convergence passes × (a `TotalDPSDual` eval = ~800-step rotation sim) ≈ a few million sim steps — sub-second in Go. No optimization needed.
