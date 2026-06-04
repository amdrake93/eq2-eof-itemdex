# Plan 2e — Item Scoring & BiS Report Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Score every Assassin-usable EoF item against the (already-built) DPS model's per-baseline stat weights and emit an explainable, per-slot best-in-slot markdown report (top-3 Fabled + top-3 Legendary per slot) plus a `scores` table in `bis.db`.

**Architecture:** The model layer (`internal/model`: weights, DPS, curves) is complete and merged on `main`. This plan adds (1) a linear scoring function with an explainable per-stat breakdown in `internal/model`; (2) `scores` table + loader queries in `internal/store`; (3) a new `internal/bis` package that derives weights per baseline, scores items (with a DPS-aware path for weapons), ranks per slot, supports a locked-items re-model, and renders the report; (4) a `cmd/bis` orchestrator. Scoring is first-order `Σ(weight × itemStat)` per the spec; saturation/caps are handled by deriving weights at the two realistic baselines and by the locked-set re-model.

**Tech Stack:** Go 1.26, `modernc.org/sqlite` (pure-Go), `stretchr/testify`. Existing helpers: `model.DeriveWeights`, `model.TotalDPSDual`, `model.AutoDPS`, `model.StatBlock.AddModifiers`, `spell.HighestRanks`, `baseline.Solo`/`baseline.Raid`.

**Spec:** `docs/design-plan2.md` §3 (scoring), §6 (scores table), §7 (report + breakdowns), §8 (locked-set re-model), §9 (validation).

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/model/score.go` *(new)* | `ScoreItem` — linear `Σ weight×stat` + sorted per-stat breakdown (`ScoreTerm`). Pure. |
| `internal/store/store.go` *(modify)* | add `scores` table to schema; `ScoreRow`+`WriteScores`; `Loadout`+`LoadLoadout`; `ScorableItem`+`LoadScorableItems`. |
| `internal/bis/engine.go` *(new)* | `ScoredItem`; `Weights` (per-baseline); `ScoreAll` (armor linear + weapon DPS-aware); `LockedRemodel`. |
| `internal/bis/ranking.go` *(new)* | `SlotGroup`; `TopPerSlot` (group by slot, split by tier, top-N). |
| `internal/bis/report.go` *(new)* | `Render` — markdown: weight table, per-slot top-3 Fabled/Legendary with breakdowns, Mythical ceiling, assumptions block. |
| `cmd/bis/main.go` *(new)* | orchestrate: open `bis.db` → load loadout+items → per baseline derive weights, score, write `scores` → render `--out`; `--lock` re-model path. |

**Note on `cmd/weights`:** it stays as-is (a debug tool). `cmd/bis` is the real deliverable. The weapon/CA/weapon-loadout SQL currently inlined in `cmd/weights` is reimplemented as the tested `store.LoadLoadout` here; do not delete `cmd/weights`.

---

### Task 1: `model.ScoreItem` — linear scoring + breakdown

**Files:**
- Create: `internal/model/score.go`
- Test: `internal/model/score_test.go`

- [ ] **Step 1: Write the failing test**

```go
package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScoreItem(t *testing.T) {
	weights := map[string]float64{
		"reuse": 16.0, "potency": 7.0, "flurry": 3.0, "critchance": 2.0,
		"abilitymod": 1.0, "haste": 0.8, "dpsmod": 0.8, "multiattack": 0.7,
	}
	item := StatBlock{Potency: 35, CritChance: 22, Reuse: 4}

	total, terms := ScoreItem(weights, item)

	// 35*7 + 22*2 + 4*16 = 245 + 44 + 64 = 353
	require.InDelta(t, 353.0, total, 1e-9)
	// only the three nonzero stats produce terms, sorted by contribution desc
	require.Len(t, terms, 3)
	require.Equal(t, "potency", terms[0].Stat)
	require.InDelta(t, 245.0, terms[0].Contribution, 1e-9)
	require.InDelta(t, 35.0, terms[0].ItemValue, 1e-9)
	require.InDelta(t, 7.0, terms[0].Weight, 1e-9)
	require.Equal(t, "reuse", terms[1].Stat)   // 64
	require.Equal(t, "critchance", terms[2].Stat) // 44
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

// ScoreTerm is one stat's contribution to an item's score: itemValue × weight.
type ScoreTerm struct {
	Stat         string
	ItemValue    float64
	Weight       float64
	Contribution float64
}

// ScoreItem computes an item's score as Σ(weight × itemStat) over WeightStats,
// returning the total and the per-stat breakdown sorted by contribution desc.
// Stats the item does not carry (value 0) are omitted from the breakdown.
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
Expected: PASS (both subtests).

- [ ] **Step 5: Commit**

```bash
git add internal/model/score.go internal/model/score_test.go
git commit -m "Add ScoreItem: linear stat scoring with explainable breakdown"
```

---

### Task 2: `scores` table + `WriteScores`

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
		{ItemID: 1, Baseline: "solo", DPSScore: 100.5, Slot: "chest"},
		{ItemID: 1, Baseline: "raid", DPSScore: 220.0, Slot: "chest"},
		{ItemID: 2, Baseline: "solo", DPSScore: 80.0, Slot: "head"},
	}
	require.NoError(t, d.WriteScores(rows))

	var n int
	require.NoError(t, d.SQL().QueryRow(`SELECT COUNT(*) FROM scores`).Scan(&n))
	require.Equal(t, 3, n)

	var score float64
	require.NoError(t, d.SQL().QueryRow(
		`SELECT dps_score FROM scores WHERE item_id=1 AND baseline='raid'`).Scan(&score))
	require.InDelta(t, 220.0, score, 1e-9)

	// idempotent re-write (INSERT OR REPLACE on the composite key)
	require.NoError(t, d.WriteScores(rows))
	require.NoError(t, d.SQL().QueryRow(`SELECT COUNT(*) FROM scores`).Scan(&n))
	require.Equal(t, 3, n)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestWriteScores -v`
Expected: FAIL — `undefined: ScoreRow` / `WriteScores`, and `no such table: scores`.

- [ ] **Step 3: Write minimal implementation**

In `internal/store/store.go`, append to the `schema` const string (inside the backtick block, after the `combat_arts` table):

```sql
CREATE TABLE IF NOT EXISTS scores (
  item_id INTEGER, baseline TEXT, dps_score REAL, slot TEXT,
  PRIMARY KEY (item_id, baseline)
);
```

Then add:

```go
// ScoreRow is one item's score under one baseline.
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

### Task 3: `store.LoadLoadout` — weapons + collapsed combat arts

**Files:**
- Modify: `internal/store/store.go` (add `model` import, `Loadout`, `LoadLoadout`)
- Test: `internal/store/loadout_test.go`

**Context:** `cmd/weights` currently inlines this. The main-hand is the best Soulfire 1H Assassin weapon; the off-hand is the best Fabled 1H Assassin piercing/slashing weapon with delay 3.5–4.5. CAs are collapsed to highest rank via `spell.HighestRanks`.

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
	// main-hand Soulfire + a fabled off-hand piercer + a non-weapon (ignored)
	exec(`INSERT INTO items (id,name,slot,tier,itemlevel,armor_type,skill,wieldstyle,classes,gamelink,weapon_min_dmg,weapon_max_dmg,delay,damage_rating)
	      VALUES (1,'Soulfire Gladius','primary','MYTHICAL',70,'',? ,'One-Handed','assassin','',120,200,4.0,80)`, "slashing")
	exec(`INSERT INTO items (id,name,slot,tier,itemlevel,armor_type,skill,wieldstyle,classes,gamelink,weapon_min_dmg,weapon_max_dmg,delay,damage_rating)
	      VALUES (2,'Enchanted Grove Scimitar','secondary','FABLED',70,'','piercing','One-Handed','assassin','',118,198,4.4,75)`)
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
	require.InDelta(t, 160.0, lo.Main.AvgDamage, 1e-9) // (120+200)/2
	require.InDelta(t, 4.0, lo.Main.DelaySecs, 1e-9)
	require.Equal(t, "Enchanted Grove Scimitar", lo.OffName)
	require.InDelta(t, 158.0, lo.Off.AvgDamage, 1e-9) // (118+198)/2
	// HighestRanks collapses the two Assassinate ranks to one
	require.Len(t, lo.Arts, 1)
	require.Equal(t, "Assassinate II", lo.Arts[0].Name)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestLoadLoadout -v`
Expected: FAIL — `undefined: Loadout` / `LoadLoadout`.

- [ ] **Step 3: Write minimal implementation**

In `internal/store/store.go` add `"github.com/amdrake93/eq2-eof-itemdex/internal/model"` to the import block, then add:

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
// Assassin combat arts (collapsed to highest rank).
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
git commit -m "Add LoadLoadout: weapons + collapsed combat arts from the db"
```

---

### Task 4: `store.ScorableItem` + `LoadScorableItems`

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
	// assassin chest with potency+crit; an assassin off-hand weapon; a non-assassin item (excluded)
	exec(`INSERT INTO items (id,name,slot,tier,itemlevel,armor_type,skill,wieldstyle,classes,gamelink,weapon_min_dmg,weapon_max_dmg,delay,damage_rating)
	      VALUES (10,'Fabled Chest','chest','FABLED',70,'Leather','','','assassin|ranger','link10',0,0,0,0)`)
	exec(`INSERT INTO item_stats (item_id,stat,value) VALUES (10,'basemodifier',35)`)
	exec(`INSERT INTO item_stats (item_id,stat,value) VALUES (10,'critchance',22)`)
	exec(`INSERT INTO items (id,name,slot,tier,itemlevel,armor_type,skill,wieldstyle,classes,gamelink,weapon_min_dmg,weapon_max_dmg,delay,damage_rating)
	      VALUES (11,'Fabled Dirk','secondary','FABLED',70,'','piercing','One-Handed','assassin','link11',118,198,4.4,75)`)
	exec(`INSERT INTO item_stats (item_id,stat,value) VALUES (11,'flurry',5)`)
	exec(`INSERT INTO items (id,name,slot,tier,itemlevel,armor_type,skill,wieldstyle,classes,gamelink,weapon_min_dmg,weapon_max_dmg,delay,damage_rating)
	      VALUES (12,'Wizard Hat','head','FABLED',70,'Cloth','','','wizard','link12',0,0,0,0)`)

	items, err := d.LoadScorableItems()
	require.NoError(t, err)
	require.Len(t, items, 2) // wizard item excluded

	byID := map[int]ScorableItem{}
	for _, it := range items {
		byID[it.ID] = it
	}
	chest := byID[10]
	require.Equal(t, "Fabled Chest", chest.Name)
	require.Equal(t, "chest", chest.Slot)
	require.Equal(t, "FABLED", chest.Tier)
	require.Equal(t, "link10", chest.GameLink)
	require.InDelta(t, 35.0, chest.Stats.Potency, 1e-9)   // basemodifier -> Potency
	require.InDelta(t, 22.0, chest.Stats.CritChance, 1e-9) // critchance -> CritChance
	require.False(t, chest.IsWeapon())

	dirk := byID[11]
	require.True(t, dirk.IsWeapon())
	require.InDelta(t, 158.0, dirk.WeaponAvg, 1e-9) // (118+198)/2
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
// into a StatBlock plus the raw modifier map (for display).
type ScorableItem struct {
	ID          int
	Name        string
	Slot        string
	Tier        string
	GameLink    string
	WeaponAvg   float64 // (min+max)/2; 0 for non-weapons
	WeaponDelay float64 // 0 for non-weapons
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

### Task 5: `bis` engine — `Weights` + `ScoreAll` (armor + weapon-aware)

**Files:**
- Create: `internal/bis/engine.go`
- Test: `internal/bis/engine_test.go`

**Context:** Armor/jewelry score = `Σ weight×stat`. Weapons additionally carry their auto-attack value (`model.AutoDPS(baseline, weapon)`), which `Σ weight×stat` misses — so weapon score adds an `auto-attack` breakdown term. The `baseline` is passed so the weapon's auto term reflects the buff context (haste/dps-mod) it's evaluated in.

- [ ] **Step 1: Write the failing test**

```go
package bis

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
	"github.com/stretchr/testify/require"
)

func TestScoreAllArmorAndWeapon(t *testing.T) {
	weights := map[string]float64{"potency": 7.0, "flurry": 3.0}
	base := model.StatBlock{} // zero baseline keeps weapon auto math simple

	armor := store.ScorableItem{ID: 1, Name: "Chest", Slot: "chest", Tier: "FABLED",
		Stats: model.StatBlock{Potency: 10}}
	weapon := store.ScorableItem{ID: 2, Name: "Dirk", Slot: "secondary", Tier: "FABLED",
		WeaponAvg: 160, WeaponDelay: 4, Stats: model.StatBlock{Flurry: 5}}

	scored := ScoreAll([]store.ScorableItem{armor, weapon}, weights, base)
	require.Len(t, scored, 2)

	// armor: 10 * 7 = 70, no auto term
	require.InDelta(t, 70.0, scored[0].Score, 1e-9)
	require.Len(t, scored[0].Terms, 1)

	// weapon: AutoDPS(zero stats, 160/4) = 160/4 = 40 swings * all-1.0 factors = 40
	// + flurry 5*3 = 15  => 55
	require.InDelta(t, 55.0, scored[1].Score, 1e-6)
	require.Equal(t, "auto-attack", scored[1].Terms[0].Stat) // auto term first (largest)
	require.InDelta(t, 40.0, scored[1].Terms[0].Contribution, 1e-6)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bis/ -run TestScoreAll -v`
Expected: FAIL — package/symbols undefined.

- [ ] **Step 3: Write minimal implementation**

```go
// Package bis derives per-baseline stat weights, scores Assassin-usable items,
// ranks them per slot, and renders the best-in-slot report.
package bis

import (
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
)

// ScoredItem is one item with its computed score and explainable breakdown.
type ScoredItem struct {
	Item  store.ScorableItem
	Score float64
	Terms []model.ScoreTerm
}

// Weights derives the marginal stat weights for a baseline against the loadout.
func Weights(lo store.Loadout, baseline model.StatBlock) map[string]float64 {
	dps := func(sb model.StatBlock) float64 {
		return model.TotalDPSDual(sb, lo.Main, lo.Off, lo.Arts)
	}
	return model.DeriveWeights(baseline, dps)
}

// ScoreAll scores items at the given weights. Weapons add an auto-attack term
// (their dominant value) evaluated at the baseline's buff context.
func ScoreAll(items []store.ScorableItem, weights map[string]float64, baseline model.StatBlock) []ScoredItem {
	out := make([]ScoredItem, 0, len(items))
	for _, it := range items {
		total, terms := model.ScoreItem(weights, it.Stats)
		if it.IsWeapon() {
			auto := model.AutoDPS(baseline, model.Weapon{AvgDamage: it.WeaponAvg, DelaySecs: it.WeaponDelay})
			total += auto
			terms = append([]model.ScoreTerm{{Stat: "auto-attack", ItemValue: it.WeaponAvg, Weight: 0, Contribution: auto}}, terms...)
		}
		out = append(out, ScoredItem{Item: it, Score: total, Terms: terms})
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bis/ -run TestScoreAll -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/bis/engine.go internal/bis/engine_test.go
git commit -m "Add bis engine: per-baseline Weights and ScoreAll (weapon-aware)"
```

---

### Task 6: `bis.LockedRemodel` — constrained re-model (§8)

**Files:**
- Modify: `internal/bis/engine.go`
- Test: `internal/bis/locked_test.go`

**Context:** Locking N items folds their stats into the baseline, re-derives weights at that adjusted baseline, and re-scores only the *unlocked* items. The user supplies locked IDs from game knowledge; the tool needs no set-membership data.

- [ ] **Step 1: Write the failing test**

```go
package bis

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
	"github.com/stretchr/testify/require"
)

func TestLockedRemodel(t *testing.T) {
	lo := store.Loadout{
		Main: model.Weapon{AvgDamage: 160, DelaySecs: 4},
		Off:  model.Weapon{AvgDamage: 158, DelaySecs: 4.4},
	}
	base := model.StatBlock{}
	items := []store.ScorableItem{
		{ID: 1, Name: "Locked Chest", Slot: "chest", Tier: "FABLED", Stats: model.StatBlock{DPSMod: 200}},
		{ID: 2, Name: "Open Legs", Slot: "legs", Tier: "FABLED", Stats: model.StatBlock{Flurry: 10}},
	}

	res := LockedRemodel(lo, base, items, []int{1})

	require.Len(t, res.Locked, 1)
	require.Equal(t, 1, res.Locked[0].ID)
	// adjusted baseline absorbs the locked item's DPSMod
	require.InDelta(t, 200.0, res.AdjustedBaseline.DPSMod, 1e-9)
	// only the unlocked item is scored
	require.Len(t, res.Scored, 1)
	require.Equal(t, 2, res.Scored[0].Item.ID)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bis/ -run TestLockedRemodel -v`
Expected: FAIL — `undefined: LockedRemodel` / `RemodelResult`.

- [ ] **Step 3: Write minimal implementation**

Append to `internal/bis/engine.go`:

```go
// RemodelResult is the output of a locked-items constrained re-model.
type RemodelResult struct {
	AdjustedBaseline model.StatBlock
	Weights          map[string]float64
	Locked           []store.ScorableItem
	Scored           []ScoredItem // unlocked items, scored at the adjusted weights
}

// LockedRemodel folds the locked items' stats into the baseline, re-derives the
// weights there, and re-scores the remaining (unlocked) items.
func LockedRemodel(lo store.Loadout, baseline model.StatBlock, items []store.ScorableItem, lockedIDs []int) RemodelResult {
	locked := make(map[int]bool, len(lockedIDs))
	for _, id := range lockedIDs {
		locked[id] = true
	}
	adj := baseline
	var lockedItems, unlocked []store.ScorableItem
	for _, it := range items {
		if locked[it.ID] {
			adj = adj.Add(it.Stats)
			lockedItems = append(lockedItems, it)
		} else {
			unlocked = append(unlocked, it)
		}
	}
	weights := Weights(lo, adj)
	return RemodelResult{
		AdjustedBaseline: adj,
		Weights:          weights,
		Locked:           lockedItems,
		Scored:           ScoreAll(unlocked, weights, adj),
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bis/ -run TestLockedRemodel -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/bis/engine.go internal/bis/locked_test.go
git commit -m "Add LockedRemodel: constrained re-model around locked items"
```

---

### Task 7: `bis.TopPerSlot` — per-slot tier ranking

**Files:**
- Create: `internal/bis/ranking.go`
- Test: `internal/bis/ranking_test.go`

- [ ] **Step 1: Write the failing test**

```go
package bis

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
	"github.com/stretchr/testify/require"
)

func si(id int, slot, tier string, score float64) ScoredItem {
	return ScoredItem{Item: store.ScorableItem{ID: id, Slot: slot, Tier: tier}, Score: score}
}

func TestTopPerSlot(t *testing.T) {
	scored := []ScoredItem{
		si(1, "chest", "FABLED", 50), si(2, "chest", "FABLED", 90), si(3, "chest", "FABLED", 70), si(4, "chest", "FABLED", 10),
		si(5, "chest", "LEGENDARY", 40), si(6, "chest", "LEGENDARY", 60),
		si(7, "chest", "MYTHICAL", 200),
		si(8, "head", "FABLED", 30),
		si(9, "chest", "TREASURED", 999), // ignored tier
	}
	groups := TopPerSlot(scored, 3)

	// slots sorted alphabetically: chest, head
	require.Len(t, groups, 2)
	require.Equal(t, "chest", groups[0].Slot)

	// top-3 Fabled by score desc: 90,70,50 (the 10 dropped)
	require.Len(t, groups[0].Fabled, 3)
	require.Equal(t, []int{2, 3, 1}, []int{groups[0].Fabled[0].Item.ID, groups[0].Fabled[1].Item.ID, groups[0].Fabled[2].Item.ID})
	// top-3 Legendary (only 2 exist): 60,40
	require.Len(t, groups[0].Legendary, 2)
	require.Equal(t, 6, groups[0].Legendary[0].Item.ID)
	// all Mythical shown
	require.Len(t, groups[0].Mythical, 1)
	require.Equal(t, 7, groups[0].Mythical[0].Item.ID)

	require.Equal(t, "head", groups[1].Slot)
	require.Len(t, groups[1].Fabled, 1)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bis/ -run TestTopPerSlot -v`
Expected: FAIL — `undefined: TopPerSlot` / `SlotGroup`.

- [ ] **Step 3: Write minimal implementation**

```go
package bis

import "sort"

// SlotGroup holds the ranked items for one equipment slot, split by tier.
type SlotGroup struct {
	Slot      string
	Mythical  []ScoredItem // all (the ceiling)
	Fabled    []ScoredItem // top N
	Legendary []ScoredItem // top N
}

// TopPerSlot groups scored items by slot and, within each slot, returns all
// Mythical plus the top-n Fabled and top-n Legendary by score. Other tiers are
// ignored. Slots are returned in alphabetical order.
func TopPerSlot(scored []ScoredItem, n int) []SlotGroup {
	bySlot := map[string][]ScoredItem{}
	for _, s := range scored {
		bySlot[s.Item.Slot] = append(bySlot[s.Item.Slot], s)
	}
	slots := make([]string, 0, len(bySlot))
	for slot := range bySlot {
		slots = append(slots, slot)
	}
	sort.Strings(slots)

	topN := func(items []ScoredItem, tier string, limit int) []ScoredItem {
		var f []ScoredItem
		for _, it := range items {
			if it.Item.Tier == tier {
				f = append(f, it)
			}
		}
		sort.Slice(f, func(i, j int) bool { return f[i].Score > f[j].Score })
		if limit >= 0 && len(f) > limit {
			f = f[:limit]
		}
		return f
	}

	groups := make([]SlotGroup, 0, len(slots))
	for _, slot := range slots {
		items := bySlot[slot]
		groups = append(groups, SlotGroup{
			Slot:      slot,
			Mythical:  topN(items, "MYTHICAL", -1),
			Fabled:    topN(items, "FABLED", n),
			Legendary: topN(items, "LEGENDARY", n),
		})
	}
	return groups
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bis/ -run TestTopPerSlot -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/bis/ranking.go internal/bis/ranking_test.go
git commit -m "Add TopPerSlot: per-slot tier ranking"
```

---

### Task 8: `bis.Render` — markdown report

**Files:**
- Create: `internal/bis/report.go`
- Test: `internal/bis/report_test.go`

**Context:** One report covers both baselines. For each baseline: a weight table, then per-slot Mythical (ceiling) + top-3 Fabled + top-3 Legendary, each item showing name, score, gamelink, and its top breakdown terms. A trailing assumptions block names the key constants. Tests assert on substrings (markdown exact-match is brittle).

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
	groups := []SlotGroup{{
		Slot: "chest",
		Fabled: []ScoredItem{{
			Item:  store.ScorableItem{ID: 2, Name: "Fabled Chest", Tier: "FABLED", GameLink: "LINK2"},
			Score: 412.0,
			Terms: []model.ScoreTerm{{Stat: "potency", ItemValue: 35, Weight: 7.19, Contribution: 251.65}},
		}},
	}}

	out := Render([]BaselineReport{{Name: "RAID", Weights: weights, Groups: groups}})

	require.Contains(t, out, "## RAID")
	require.Contains(t, out, "### chest")
	require.Contains(t, out, "Fabled Chest")
	require.Contains(t, out, "412.0")              // score
	require.Contains(t, out, "LINK2")              // gamelink
	require.Contains(t, out, "potency 35 × 7.19")  // breakdown term
	require.Contains(t, out, "reuse")              // weight table
	require.Contains(t, out, "Assumptions")        // constants block
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
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
)

// BaselineReport is one baseline's fully-ranked result.
type BaselineReport struct {
	Name    string
	Weights map[string]float64
	Groups  []SlotGroup
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

func writeItem(b *strings.Builder, it ScoredItem) {
	fmt.Fprintf(b, "- **%s** — score %.1f", it.Item.Name, it.Score)
	if it.Item.GameLink != "" {
		fmt.Fprintf(b, " ([item](%s))", it.Item.GameLink)
	}
	b.WriteString("\n")
	for i, term := range it.Terms {
		if i >= 4 {
			break
		}
		if term.Stat == "auto-attack" {
			fmt.Fprintf(b, "  - auto-attack (avg %.0f) = %.1f\n", term.ItemValue, term.Contribution)
			continue
		}
		fmt.Fprintf(b, "  - %s %.0f × %.2f = %.1f\n", term.Stat, term.ItemValue, term.Weight, term.Contribution)
	}
}

func writeTier(b *strings.Builder, label string, items []ScoredItem) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "**%s**\n\n", label)
	for _, it := range items {
		writeItem(b, it)
	}
	b.WriteString("\n")
}

// Render produces the full markdown BiS report across all baselines.
func Render(reports []BaselineReport) string {
	var b strings.Builder
	b.WriteString("# Assassin EoF Best-in-Slot\n\n")
	for _, r := range reports {
		fmt.Fprintf(&b, "## %s\n\n", r.Name)
		b.WriteString("Derived stat weights (marginal DPS per +1 stat):\n\n")
		writeWeightTable(&b, r.Weights)
		for _, g := range r.Groups {
			fmt.Fprintf(&b, "### %s\n\n", g.Slot)
			writeTier(&b, "Mythical (ceiling)", g.Mythical)
			writeTier(&b, "Fabled", g.Fabled)
			writeTier(&b, "Legendary", g.Legendary)
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
	b.WriteString("- Scores are first-order Σ(weight × stat); see docs/design-plan2.md §3.1 for the full model.\n")
	_ = model.WeightStats // keep model imported for type references above
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bis/ -run TestRender -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/bis/report.go internal/bis/report_test.go
git commit -m "Add Render: markdown BiS report with breakdowns and assumptions"
```

---

### Task 9: `cmd/bis` — orchestrator

**Files:**
- Create: `cmd/bis/main.go`
- Test: manual (CLI integration; the logic is unit-tested in Tasks 1–8)

**Context:** Operates on an existing `bis.db` (built by `cmd/builddb`). Derives weights for both baselines, scores items, writes the `scores` table, and renders the report to `--out`. `--lock` (comma-separated item IDs) switches to the locked re-model for the raid baseline.

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

func scoreRows(scored []bis.ScoredItem, baselineName string) []store.ScoreRow {
	rows := make([]store.ScoreRow, 0, len(scored))
	for _, s := range scored {
		rows = append(rows, store.ScoreRow{
			ItemID: s.Item.ID, Baseline: baselineName, DPSScore: s.Score, Slot: s.Item.Slot,
		})
	}
	return rows
}

func main() {
	dbPath := flag.String("db", "bis.db", "scored SQLite db (built by builddb)")
	out := flag.String("out", "bis-report.md", "report output path")
	lock := flag.String("lock", "", "comma-separated item IDs to lock (raid re-model)")
	topN := flag.Int("top", 3, "items per tier per slot")
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

	baselines := []struct {
		name string
		sb   model.StatBlock
	}{{"SOLO", baseline.Solo}, {"RAID", baseline.Raid}}

	var reports []bis.BaselineReport
	var allRows []store.ScoreRow
	for _, b := range baselines {
		weights := bis.Weights(lo, b.sb)
		scored := bis.ScoreAll(items, weights, b.sb)
		allRows = append(allRows, scoreRows(scored, strings.ToLower(b.name))...)
		reports = append(reports, bis.BaselineReport{
			Name: b.name, Weights: weights, Groups: bis.TopPerSlot(scored, *topN),
		})
	}

	if len(lockIDs) > 0 {
		res := bis.LockedRemodel(lo, baseline.Raid, items, lockIDs)
		names := make([]string, 0, len(res.Locked))
		for _, it := range res.Locked {
			names = append(names, it.Name)
		}
		reports = append(reports, bis.BaselineReport{
			Name:    "RAID (locked: " + strings.Join(names, ", ") + ")",
			Weights: res.Weights,
			Groups:  bis.TopPerSlot(res.Scored, *topN),
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
Expected: prints the loadout line + `wrote bis-report.md and N score rows`. (If `bis.db` is missing, rebuild it first: `go run ./cmd/builddb`.)

- [ ] **Step 3: Sanity-check the output**

Run: `head -60 bis-report.md`
Expected: a `## SOLO` section with a weight table, then `### <slot>` blocks listing Fabled/Legendary items with `stat N × W = C` breakdown lines; the Soulfire weapon appearing under Mythical for its slot.

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
git commit -m "Add cmd/bis: score items, write scores table, render BiS report"
```

---

## Self-Review

**1. Spec coverage:**
- §3 scoring `Σ weight×stat` → Task 1 (`ScoreItem`); weapon DPS-awareness → Task 5. ✔
- §6 `scores (item_id, baseline, dps_score, slot)` table → Task 2; `items`/`item_stats` already exist. ✔
- §7 report: top-3 Fabled + top-3 Legendary per slot → Task 7; Mythical ceiling → Task 7/8; per-item breakdown → Tasks 1+8; weight table + assumptions → Task 8; `bis.db` scores artifact → Tasks 2+9. ✔
- §8 locked-items re-model → Task 6 + `--lock` in Task 9. ✔
- §9 validation: the explainable breakdown is the artifact; loop is human-driven (run → eyeball → adjust constants → re-run). ✔

**2. Placeholder scan:** No TBD/TODO; every code step shows complete code; every test has real assertions. ✔

**3. Type consistency:** `model.ScoreTerm` (Stat/ItemValue/Weight/Contribution) used identically in Tasks 1, 5, 8. `store.ScorableItem` (with `WeaponAvg`/`WeaponDelay`/`IsWeapon`/`Stats`/`Mods`) consistent across Tasks 4, 5, 6. `store.Loadout` fields (`Main`/`Off`/`MainName`/`OffName`/`Arts`) consistent in Tasks 3, 5, 9. `bis.ScoredItem`/`SlotGroup`/`BaselineReport`/`RemodelResult` consistent across Tasks 5–9. `store.ScoreRow` consistent in Tasks 2, 9. ✔

**Known first-order limitation (per spec §3, intentional):** the linear `Σ weight×stat` score does not re-evaluate caps/diminishing returns for the item's own stacked stats — it uses the marginal weight at the baseline. This is the spec's chosen explainable model; saturation is addressed by the two realistic baselines and the locked-set re-model (Task 6). Documented in the report's assumptions block.

**Out of scope (noted, not built):** automatic equip-best-then-re-derive convergence iteration (§3 mentions it; the locked-set provides manual iteration); set-bonus *value* (intentionally the user's subjective call per §8); weapon *main-hand* optimization (Soulfire is given).
