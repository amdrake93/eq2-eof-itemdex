# Plan 2a — Model Data Layer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Assemble the data the DPS model needs — add weapon `skill`/`wieldstyle` to the catalog (+ re-pull), pull & parse the Assassin's Combat Arts from Census, and load gear + CAs into a SQLite DB.

**Architecture:** Extends Plan 1's `census`/`catalog` packages with two weapon fields; adds an `internal/spell` package (CA types, `effect_list` damage parser, class-filtered CA pull) and an `internal/store` package (pure-Go SQLite via `modernc.org/sqlite`, normalized schema). Output: `bis.db` with gear (`items` + `item_stats`) and Assassin CAs (`combat_arts`).

**Tech Stack:** Go 1.26; `modernc.org/sqlite` (pure-Go) + `database/sql`; reuses Plan 1's throttled `census` client, `stretchr/testify`, `golangci-lint`. Module `github.com/amdrake93/eq2-eof-itemdex`.

This is **Plan 2a of 2** (Plan 2b adds the DPS model, scoring, and report). Spec: `docs/design-plan2.md`. Conventions (testify, `rate.Limiter`, gofmt/lint, Go-specific idioms) carry over from `docs/plans/2026-06-02-item-catalog.md` — reuse them.

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/census/item.go` (modify) | add `Skill` + `WieldStyle` to `TypeInfo` |
| `internal/catalog/csv.go` (modify) | add `skill` + `wieldstyle` columns to weapon write/read |
| `internal/spell/spell.go` (new) | Census spell/CA types + `DecodeSpells` |
| `internal/spell/parse.go` (new) | `ParseDamage` — extract min/max from `effect_list` text |
| `internal/spell/pull.go` (new) | `PullAssassinCAs` — class-filtered, Expert-tier, ≤70 |
| `internal/store/store.go` (new) | SQLite schema + `LoadGear` + `LoadCombatArts` |
| `cmd/builddb/main.go` (new) | orchestrate: gear (CSV cache) + CAs (Census) → `bis.db` |

---

## Task 1: Add `Skill` + `WieldStyle` to `census.TypeInfo`

**Files:**
- Modify: `internal/census/item.go`
- Test: `internal/census/item_test.go`

- [ ] **Step 1: Write the failing test** (append to `item_test.go`)

```go
func TestDecodeWeaponSkillAndWieldStyle(t *testing.T) {
	body := `{"item_list":[{
	  "id":701979186,"displayname":"Grinning Dirk of Horror","type":"Weapon",
	  "typeinfo":{"name":"weapon","skill":"piercing","wieldstyle":"One-Handed","delay":4.0,"damagerating":72.1}
	}],"returned":1}`
	items, err := DecodeItems([]byte(body))
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "piercing", items[0].TypeInfo.Skill)
	require.Equal(t, "One-Handed", items[0].TypeInfo.WieldStyle)
}
```
(Add `"github.com/stretchr/testify/require"` to the test imports if not already present.)

- [ ] **Step 2: Run, verify FAIL**

Run: `go test ./internal/census/ -run TestDecodeWeaponSkillAndWieldStyle -v`
Expected: FAIL — `items[0].TypeInfo.Skill undefined`.

- [ ] **Step 3: Add the fields to `TypeInfo`** in `item.go`

In the `TypeInfo` struct, add these two fields (alongside the existing weapon fields):
```go
	Skill      string `json:"skill"`       // weapon skill: piercing/slashing/crushing/ranged…
	WieldStyle string `json:"wieldstyle"`  // One-Handed / Two-Handed
```

- [ ] **Step 4: Run, verify PASS**

Run: `go test ./internal/census/ -v`
Expected: PASS (all).

- [ ] **Step 5: Commit**

```bash
git add internal/census/item.go internal/census/item_test.go
git commit -m "feat: capture weapon skill and wieldstyle from census"
```

---

## Task 2: Add `skill` + `wieldstyle` to the catalog CSV

**Files:**
- Modify: `internal/catalog/csv.go`
- Test: `internal/catalog/csv_test.go`

The fixed columns must gain `skill` + `wieldstyle` so weapons round-trip them. `ReadCSV` derives stat columns as `header[len(fixedCols):]`, so updating `fixedCols` keeps reader/writer consistent automatically.

- [ ] **Step 1: Write the failing test** (append to `csv_test.go`)

```go
func TestRoundTripWeaponSkillWieldstyle(t *testing.T) {
	items := []census.Item{
		{ID: 1, DisplayName: "Dirk", Slots: []census.Slot{{Name: "Primary"}},
			TypeInfo: census.TypeInfo{Skill: "piercing", WieldStyle: "One-Handed", Classes: map[string]census.ClassReq{"assassin": {}}},
			Modifiers: map[string]census.Modifier{},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, WriteCSV(&buf, items))
	require.Contains(t, buf.String(), "skill")
	require.Contains(t, buf.String(), "wieldstyle")

	got, err := ReadCSV(&buf)
	require.NoError(t, err)
	require.Equal(t, "piercing", got[0].TypeInfo.Skill)
	require.Equal(t, "One-Handed", got[0].TypeInfo.WieldStyle)
}
```

- [ ] **Step 2: Run, verify FAIL**

Run: `go test ./internal/catalog/ -run TestRoundTripWeaponSkillWieldstyle -v`
Expected: FAIL — `skill` not in header / `Skill` empty after read.

- [ ] **Step 3: Update `csv.go`**

Add the two columns to `fixedCols` (after `damage_rating`, before `gamelink`):
```go
var fixedCols = []string{
	"id", "name", "slot", "tier", "itemlevel", "armor_type", "classes",
	"weapon_min_dmg", "weapon_max_dmg", "delay", "damage_rating", "skill", "wieldstyle", "gamelink",
}
```
In `WriteCSV`, add `it.TypeInfo.Skill` and `it.TypeInfo.WieldStyle` to the row, in the same positions (after `damage_rating`, before `gamelink`):
```go
		f(it.TypeInfo.DamageRating),
		it.TypeInfo.Skill,
		it.TypeInfo.WieldStyle,
		it.GameLink,
```
In `ReadCSV`, set them on the reconstructed `TypeInfo`:
```go
		it.TypeInfo.Skill = row[idx["skill"]]
		it.TypeInfo.WieldStyle = row[idx["wieldstyle"]]
```

- [ ] **Step 4: Run, verify PASS**

Run: `go test ./internal/catalog/ -v`
Expected: PASS (all — existing round-trip tests still pass since `idx[...]` lookups are name-based).

- [ ] **Step 5: Commit**

```bash
git add internal/catalog/csv.go internal/catalog/csv_test.go
git commit -m "feat: round-trip weapon skill and wieldstyle in catalog csv"
```

---

## Task 3: Re-pull the catalog (populate skill/wieldstyle)

**Files:** none (data refresh).

The CSVs in `data/` predate the new columns, so weapons there have empty `skill`/`wieldstyle`. A fresh pull repopulates them.

- [ ] **Step 1: Refresh (throttled, multi-session)**

Run: `go run ./cmd/itemdex --refresh`
Note: the public `s:example` quota cuts sessions short (~10 requests), so this resumes across several runs — **re-run the same command until it prints a completion line and `data/.census_next_offset` is gone** (see the resume logic in `internal/source`). Expect multiple invocations over a while.

- [ ] **Step 2: Verify weapons now carry skill/wieldstyle**

Run: `head -1 data/weapons.csv` and confirm `skill,wieldstyle` are in the header.
Run: `python3 -c "import csv; r=list(csv.DictReader(open('data/weapons.csv'))); print(sum(1 for x in r if x['skill']), 'of', len(r), 'weapons have a skill')"`
Expected: a large majority of weapons have a non-empty `skill`.

- [ ] **Step 3: Commit the refreshed data**

```bash
git add data/
git commit -m "data: re-pull catalog with weapon skill + wieldstyle"
```

---

## Task 4: Spell/CA types + `DecodeSpells`

**Files:**
- Create: `internal/spell/spell.go`
- Test: `internal/spell/spell_test.go`

- [ ] **Step 1: Write the failing test**

```go
package spell

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const sampleSpells = `{"spell_list":[{
  "name":"Assassinate","name_lower":"assassinate","level":50,"tier_name":"Expert","type":"arts","beneficial":0,
  "cast_secs_hundredths":50,"recast_secs":300.0,
  "classes":{"assassin":{"displayname":"Assassin","id":40,"level":50}},
  "effect_list":[{"description":"Inflicts 3,011 - 5,018 melee damage on target"},{"description":"You must be sneaking to use this ability."}]
}],"returned":1}`

func TestDecodeSpells(t *testing.T) {
	sp, err := DecodeSpells([]byte(sampleSpells))
	require.NoError(t, err)
	require.Len(t, sp, 1)
	require.Equal(t, "Assassinate", sp[0].Name)
	require.Equal(t, "Expert", sp[0].TierName)
	require.Equal(t, "arts", sp[0].Type)
	require.Equal(t, 300.0, sp[0].RecastSecs)
	require.Len(t, sp[0].Effects, 2)
	require.Contains(t, sp[0].Classes, "assassin")
}

func TestDecodeSpellsError(t *testing.T) {
	_, err := DecodeSpells([]byte(`{"error":"Missing Service ID"}`))
	require.Error(t, err)
}
```

- [ ] **Step 2: Run, verify FAIL**

Run: `go test ./internal/spell/ -run TestDecodeSpells -v`
Expected: FAIL — `undefined: DecodeSpells`.

- [ ] **Step 3: Implement `spell.go`**

```go
package spell

import (
	"encoding/json"
	"fmt"
)

type Effect struct {
	Description string `json:"description"`
}

type ClassReq struct {
	DisplayName string `json:"displayname"`
	ID          int    `json:"id"`
	Level       int    `json:"level"`
}

// Spell is the subset of the Census spell record this project needs.
type Spell struct {
	Name               string              `json:"name"`
	NameLower          string              `json:"name_lower"`
	Level              int                 `json:"level"`
	TierName           string              `json:"tier_name"`
	Type               string              `json:"type"`
	Beneficial         int                 `json:"beneficial"`
	CastSecsHundredths int                 `json:"cast_secs_hundredths"`
	RecastSecs         float64             `json:"recast_secs"`
	Classes            map[string]ClassReq `json:"classes"`
	Effects            []Effect            `json:"effect_list"`
}

type spellListResponse struct {
	Spells    []Spell `json:"spell_list"`
	Returned  int     `json:"returned"`
	ErrorCode string  `json:"errorCode"`
	Error     string  `json:"error"`
}

// DecodeSpells parses a Census spell_list payload, surfacing API error envelopes.
func DecodeSpells(body []byte) ([]Spell, error) {
	var r spellListResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	if r.ErrorCode != "" || r.Error != "" {
		return nil, fmt.Errorf("census error: %s%s", r.ErrorCode, r.Error)
	}
	return r.Spells, nil
}
```

- [ ] **Step 4: Run, verify PASS**

Run: `go test ./internal/spell/ -v`
Expected: PASS (both).

- [ ] **Step 5: Commit**

```bash
git add internal/spell/spell.go internal/spell/spell_test.go
git commit -m "feat: census spell types and decoder"
```

---

## Task 5: Parse damage from `effect_list`

**Files:**
- Create: `internal/spell/parse.go`
- Test: `internal/spell/parse_test.go`

CA damage lives in effect text like `"Inflicts 3,011 - 5,018 melee damage on target"`. Parse the min/max (commas stripped). A CA with no such line is non-damaging (a buff/utility) — return `ok=false`.

- [ ] **Step 1: Write the failing test**

```go
package spell

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseDamage(t *testing.T) {
	cases := []struct {
		name           string
		effects        []string
		wantMin, wantMax float64
		wantOK         bool
	}{
		{"melee", []string{"Inflicts 3,011 - 5,018 melee damage on target", "You must be sneaking"}, 3011, 5018, true},
		{"piercing single-line", []string{"Inflicts 800 - 1,200 piercing damage on target"}, 800, 1200, true},
		{"no damage", []string{"Increases Haste of caster by 30.6."}, 0, 0, false},
	}
	for _, c := range cases {
		min, max, ok := ParseDamage(c.effects)
		require.Equal(t, c.wantOK, ok, c.name)
		require.Equal(t, c.wantMin, min, c.name)
		require.Equal(t, c.wantMax, max, c.name)
	}
}
```

- [ ] **Step 2: Run, verify FAIL**

Run: `go test ./internal/spell/ -run TestParseDamage -v`
Expected: FAIL — `undefined: ParseDamage`.

- [ ] **Step 3: Implement `parse.go`**

```go
package spell

import (
	"regexp"
	"strconv"
	"strings"
)

// damageRe matches "Inflicts 3,011 - 5,018 <type> damage..." capturing min and max.
var damageRe = regexp.MustCompile(`Inflicts\s+([\d,]+)\s*-\s*([\d,]+)\s+\w+\s+damage`)

func toFloat(s string) float64 {
	v, _ := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	return v
}

// ParseDamage scans effect descriptions for the first damage line and returns
// its min/max. ok is false when no damage line is present (buff/utility art).
func ParseDamage(effects []string) (min, max float64, ok bool) {
	for _, e := range effects {
		if m := damageRe.FindStringSubmatch(e); m != nil {
			return toFloat(m[1]), toFloat(m[2]), true
		}
	}
	return 0, 0, false
}
```

- [ ] **Step 4: Run, verify PASS**

Run: `go test ./internal/spell/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/spell/parse.go internal/spell/parse_test.go
git commit -m "feat: parse combat-art damage from effect_list"
```

---

## Task 6: Pull the Assassin's Combat Arts

**Files:**
- Create: `internal/spell/pull.go`
- Test: `internal/spell/pull_test.go`

Combat Arts are damaging (`type=arts`, `beneficial=0`), class-gated (`classes.assassin`), and we want the Expert tier the Assassin can use by level 70. Build a `CombatArt` (name + timing + parsed damage), keeping only damaging arts whose Expert version the Assassin learns by 70.

- [ ] **Step 1: Write the failing test**

```go
package spell

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

func TestAssassinCombatArts(t *testing.T) {
	// One damaging Expert CA (kept) + one buff art (dropped: no damage line).
	page := `{"spell_list":[
	  {"name":"Assassinate","level":50,"tier_name":"Expert","type":"arts","beneficial":0,"recast_secs":300.0,"cast_secs_hundredths":50,
	   "classes":{"assassin":{"id":40,"level":50}},
	   "effect_list":[{"description":"Inflicts 3,011 - 5,018 melee damage on target"}]},
	  {"name":"Honed Reflexes","level":40,"tier_name":"Expert","type":"arts","beneficial":1,"recast_secs":0.0,"cast_secs_hundredths":0,
	   "classes":{"assassin":{"id":40,"level":40}},
	   "effect_list":[{"description":"Increases Haste of caster by 30.6."}]}
	],"returned":2}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, page)
	}))
	defer srv.Close()

	c := census.New("s:example")
	c.BaseURL = srv.URL
	c.Limiter = rate.NewLimiter(rate.Inf, 1)

	cas, err := AssassinCombatArts(context.Background(), c)
	require.NoError(t, err)
	require.Len(t, cas, 1) // only the damaging one
	require.Equal(t, "Assassinate", cas[0].Name)
	require.Equal(t, 5018.0, cas[0].MaxDamage)
	require.Equal(t, 300.0, cas[0].RecastSecs)
}
```

- [ ] **Step 2: Run, verify FAIL**

Run: `go test ./internal/spell/ -run TestAssassinCombatArts -v`
Expected: FAIL — `undefined: AssassinCombatArts`.

- [ ] **Step 3: Implement `pull.go`**

```go
package spell

import (
	"context"
	"fmt"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
)

// assassinClassID is the Assassin's class id in the SPELL collection (40);
// note it differs from the item collection's class id (15) — a Census quirk.
const assassinClassID = 40

const caShowFields = "name,level,tier_name,type,beneficial,cast_secs_hundredths,recast_secs,classes,effect_list"

// CombatArt is a damaging Assassin ability with the fields the DPS model needs.
type CombatArt struct {
	Name               string
	Level              int
	MinDamage          float64
	MaxDamage          float64
	RecastSecs         float64
	CastSecsHundredths int
}

// AssassinCombatArts pulls the Assassin's Expert-tier combat arts usable by
// level 70, keeping only damaging ones (a parseable effect_list damage line).
func AssassinCombatArts(ctx context.Context, c *census.Client) ([]CombatArt, error) {
	// classes.assassin.id=40 selects Assassin abilities; type=arts = combat arts;
	// tier_name=Expert = the modeled tier; level=<71 = learnable by 70.
	query := fmt.Sprintf(
		"classes.assassin.id=%d&type=arts&tier_name=Expert&level=%%3C71&c:limit=500&c:show=%s",
		assassinClassID, caShowFields)
	body, err := c.Get(ctx, "get", "spell", query)
	if err != nil {
		return nil, err
	}
	spells, err := DecodeSpells(body)
	if err != nil {
		return nil, err
	}
	var arts []CombatArt
	for _, s := range spells {
		min, max, ok := ParseDamage(effectStrings(s.Effects))
		if !ok {
			continue // buff/utility art — not damage
		}
		arts = append(arts, CombatArt{
			Name:               s.Name,
			Level:              s.Level,
			MinDamage:          min,
			MaxDamage:          max,
			RecastSecs:         s.RecastSecs,
			CastSecsHundredths: s.CastSecsHundredths,
		})
	}
	return arts, nil
}

func effectStrings(effs []Effect) []string {
	out := make([]string, len(effs))
	for i, e := range effs {
		out[i] = e.Description
	}
	return out
}
```

- [ ] **Step 4: Run, verify PASS**

Run: `go test ./internal/spell/ -v`
Expected: PASS.

- [ ] **Step 5: Verify the server-side filter against live Census (then note the result)**

Run:
```bash
curl -s "https://census.daybreakgames.com/s:example/get/eq2/spell/?classes.assassin.id=40&type=arts&tier_name=Expert&level=%3C71&c:limit=5&c:show=name,level,tier_name,effect_list" | python3 -m json.tool | head -40
```
Expected: a handful of Assassin Expert combat arts with `effect_list` damage lines — **not** `{"errorCode":...}` and not an empty list. If `classes.assassin.id` filtering is rejected, fall back to `classes.assassin.level=%3C71` (presence-by-level) or pull `type=arts&tier_name=Expert` broadly and filter `s.Classes["assassin"]` client-side; keep `AssassinCombatArts`'s signature unchanged.

- [ ] **Step 6: Commit**

```bash
git add internal/spell/pull.go internal/spell/pull_test.go
git commit -m "feat: pull assassin expert combat arts (damaging only)"
```

---

## Task 7: SQLite store — schema + `LoadGear`

**Files:**
- Create: `internal/store/store.go`
- Test: `internal/store/store_test.go`

Normalized schema per spec §6: `items` (one row/item) + `item_stats(item_id, stat, value)`. Use `modernc.org/sqlite` (pure-Go; driver name `"sqlite"`).

- [ ] **Step 1: Add the dependency**

Run: `go get modernc.org/sqlite`
Expected: added to `go.mod`/`go.sum`.

- [ ] **Step 2: Write the failing test**

```go
package store

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/stretchr/testify/require"
)

func TestLoadGear(t *testing.T) {
	db, err := Open(":memory:")
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.Init())

	items := []census.Item{
		{ID: 1, DisplayName: "Dirk", Tier: "FABLED", ItemLevel: 70,
			Slots:    []census.Slot{{Name: "Primary"}},
			TypeInfo: census.TypeInfo{Skill: "piercing", WieldStyle: "One-Handed", Classes: map[string]census.ClassReq{"assassin": {}}},
			Modifiers: map[string]census.Modifier{"strength": {Value: 32}, "critchance": {Value: 1.2}},
		},
	}
	require.NoError(t, db.LoadGear(items))

	var name, skill string
	require.NoError(t, db.SQL().QueryRow(`SELECT name, skill FROM items WHERE id=1`).Scan(&name, &skill))
	require.Equal(t, "Dirk", name)
	require.Equal(t, "piercing", skill)

	var n int
	require.NoError(t, db.SQL().QueryRow(`SELECT COUNT(*) FROM item_stats WHERE item_id=1`).Scan(&n))
	require.Equal(t, 2, n)
}
```

- [ ] **Step 3: Implement `store.go`**

```go
package store

import (
	"database/sql"
	"sort"
	"strings"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	_ "modernc.org/sqlite" // pure-Go driver, registers "sqlite"
)

type DB struct{ db *sql.DB }

func Open(path string) (*DB, error) {
	d, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	return &DB{db: d}, nil
}

func (d *DB) SQL() *sql.DB { return d.db }
func (d *DB) Close() error { return d.db.Close() }

const schema = `
CREATE TABLE IF NOT EXISTS items (
  id INTEGER PRIMARY KEY, name TEXT, slot TEXT, tier TEXT, itemlevel INTEGER,
  armor_type TEXT, skill TEXT, wieldstyle TEXT, classes TEXT, gamelink TEXT,
  weapon_min_dmg REAL, weapon_max_dmg REAL, delay REAL, damage_rating REAL
);
CREATE TABLE IF NOT EXISTS item_stats (
  item_id INTEGER, stat TEXT, value REAL,
  PRIMARY KEY (item_id, stat)
);
CREATE TABLE IF NOT EXISTS combat_arts (
  name TEXT PRIMARY KEY, level INTEGER, min_dmg REAL, max_dmg REAL,
  recast_secs REAL, cast_secs_hundredths INTEGER
);`

func (d *DB) Init() error {
	_, err := d.db.Exec(schema)
	return err
}

func firstSlot(it census.Item) string {
	if len(it.Slots) > 0 {
		return it.Slots[0].Name
	}
	return ""
}

func classList(it census.Item) string {
	names := make([]string, 0, len(it.TypeInfo.Classes))
	for k := range it.TypeInfo.Classes {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, "|")
}

// LoadGear inserts items + their modifier stats in one transaction.
func (d *DB) LoadGear(items []census.Item) (err error) {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	for _, it := range items {
		if _, err = tx.Exec(
			`INSERT OR REPLACE INTO items
			 (id,name,slot,tier,itemlevel,armor_type,skill,wieldstyle,classes,gamelink,weapon_min_dmg,weapon_max_dmg,delay,damage_rating)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			it.ID, it.DisplayName, firstSlot(it), it.Tier, it.ItemLevel,
			it.TypeInfo.SkillType, it.TypeInfo.Skill, it.TypeInfo.WieldStyle, classList(it), it.GameLink,
			it.TypeInfo.MinBaseDamage, it.TypeInfo.MaxBaseDamage, it.TypeInfo.Delay, it.TypeInfo.DamageRating,
		); err != nil {
			return err
		}
		for stat, m := range it.Modifiers {
			if _, err = tx.Exec(`INSERT OR REPLACE INTO item_stats (item_id,stat,value) VALUES (?,?,?)`,
				it.ID, stat, m.Value); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
```
Note: `armor_type` is stored as the derived label via `it.TypeInfo.SkillType`? No — store the *armor_type label*. Use `catalog.ArmorType(it.TypeInfo.SkillType)` instead of `it.TypeInfo.SkillType` in the insert (import `internal/catalog`). Correct that line to:
```go
			catalog.ArmorType(it.TypeInfo.SkillType), it.TypeInfo.Skill, it.TypeInfo.WieldStyle, classList(it), it.GameLink,
```
and add the import `"github.com/amdrake93/eq2-eof-itemdex/internal/catalog"`.

- [ ] **Step 4: Run, verify PASS**

Run: `go test ./internal/store/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go go.mod go.sum
git commit -m "feat: sqlite store with normalized gear schema (modernc)"
```

---

## Task 8: SQLite store — `LoadCombatArts`

**Files:**
- Modify: `internal/store/store.go`
- Test: `internal/store/combatarts_test.go`

- [ ] **Step 1: Write the failing test**

```go
package store

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	"github.com/stretchr/testify/require"
)

func TestLoadCombatArts(t *testing.T) {
	db, err := Open(":memory:")
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.Init())

	arts := []spell.CombatArt{
		{Name: "Assassinate", Level: 50, MinDamage: 3011, MaxDamage: 5018, RecastSecs: 300, CastSecsHundredths: 50},
	}
	require.NoError(t, db.LoadCombatArts(arts))

	var max float64
	require.NoError(t, db.SQL().QueryRow(`SELECT max_dmg FROM combat_arts WHERE name='Assassinate'`).Scan(&max))
	require.Equal(t, 5018.0, max)
}
```

- [ ] **Step 2: Run, verify FAIL**

Run: `go test ./internal/store/ -run TestLoadCombatArts -v`
Expected: FAIL — `undefined: ... LoadCombatArts`.

- [ ] **Step 3: Implement `LoadCombatArts`** (append to `store.go`)

```go
// LoadCombatArts inserts the Assassin's combat arts.
func (d *DB) LoadCombatArts(arts []spell.CombatArt) (err error) {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	for _, a := range arts {
		if _, err = tx.Exec(
			`INSERT OR REPLACE INTO combat_arts (name,level,min_dmg,max_dmg,recast_secs,cast_secs_hundredths)
			 VALUES (?,?,?,?,?,?)`,
			a.Name, a.Level, a.MinDamage, a.MaxDamage, a.RecastSecs, a.CastSecsHundredths,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}
```
Add the import `"github.com/amdrake93/eq2-eof-itemdex/internal/spell"` to `store.go`.

- [ ] **Step 4: Run, verify PASS**

Run: `go test ./internal/store/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/combatarts_test.go
git commit -m "feat: load combat arts into sqlite store"
```

---

## Task 9: `builddb` command — assemble `bis.db`

**Files:**
- Create: `cmd/builddb/main.go`

- [ ] **Step 1: Implement `main.go`**

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/amdrake93/eq2-eof-itemdex/internal/source"
	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
)

func main() {
	var (
		dataDir = flag.String("data", "data", "catalog CSV directory")
		dbPath  = flag.String("db", "bis.db", "output SQLite DB path")
		sid     = flag.String("sid", "s:example", "Census service ID")
	)
	flag.Parse()

	gear, err := source.LoadCache(*dataDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load gear:", err)
		os.Exit(1)
	}
	c := census.New(*sid)
	arts, err := spell.AssassinCombatArts(context.Background(), c)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pull combat arts:", err)
		os.Exit(1)
	}

	_ = os.Remove(*dbPath)
	db, err := store.Open(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open db:", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := db.Init(); err != nil {
		fmt.Fprintln(os.Stderr, "init db:", err)
		os.Exit(1)
	}
	if err := db.LoadGear(gear); err != nil {
		fmt.Fprintln(os.Stderr, "load gear -> db:", err)
		os.Exit(1)
	}
	if err := db.LoadCombatArts(arts); err != nil {
		fmt.Fprintln(os.Stderr, "load CAs -> db:", err)
		os.Exit(1)
	}
	fmt.Printf("built %s: %d gear items, %d combat arts\n", *dbPath, len(gear), len(arts))
}
```

- [ ] **Step 2: Build + lint**

Run: `make build && make lint`
Expected: exit 0.

- [ ] **Step 3: Run it for real**

Run: `go run ./cmd/builddb`
Expected: `built bis.db: <N> gear items, <M> combat arts` with N in the thousands and M a few dozen. (The CA pull is small — one throttled batch; if quota interrupts, re-run.)

- [ ] **Step 4: Spot-check the DB**

Run:
```bash
sqlite3 bis.db "SELECT name,skill,wieldstyle FROM items WHERE slot='Primary' LIMIT 5;"   # (or: go run a tiny query)
sqlite3 bis.db "SELECT name,max_dmg,recast_secs FROM combat_arts ORDER BY max_dmg DESC LIMIT 5;"
```
Expected: weapons show skill/wieldstyle; combat arts show damage + recast. (If `sqlite3` CLI isn't installed, query via a one-off Go snippet or `make`-able helper — the DB is standard SQLite.)

- [ ] **Step 5: Decide on tracking `bis.db`**

`bis.db` is a generated artifact derived from `data/` + a CA pull. Add `bis.db` to `.gitignore` (regenerable; don't bloat the repo) — unless you want to publish it like the CSVs. Default: gitignore it.
```bash
echo "/bis.db" >> .gitignore
git add .gitignore cmd/builddb/main.go
git commit -m "feat: builddb command assembles gear + combat arts into sqlite"
```

---

## Self-Review

**Spec coverage (design-plan2.md → tasks):**
- §2 re-pull (weapon skill/wieldstyle) → Tasks 1, 2, 3.
- §3 CA query drives potency/ability-mod (pull + parse Assassin CAs) → Tasks 4, 5, 6.
- §5 `internal/spell` → Tasks 4–6; `internal/store` → Tasks 7, 8.
- §6 normalized SQLite schema (`items` + `item_stats`, plus `combat_arts`; `scores` deferred to 2b) → Tasks 7, 8.
- §11 CA tier = Expert, ≤70, damaging only → Task 6.
- **Deferred to Plan 2b** (correctly out of scope here): baselines, DPS equations, weight derivation, scoring, locked-items, ranking, report, the `scores` table.

**Placeholder scan:** none — the one live-API uncertainty (server-side `classes.assassin.id` filter) has an explicit verify-then-fallback step (Task 6 Step 5); the `armor_type` insert correction is spelled out with the exact line + import.

**Type consistency:** `census.Item`/`TypeInfo.Skill`/`TypeInfo.WieldStyle` consistent across Tasks 1–2 and 7. `spell.CombatArt` fields (`Name`,`Level`,`MinDamage`,`MaxDamage`,`RecastSecs`,`CastSecsHundredths`) defined in Task 6 and consumed identically in Task 8. `store.DB` methods (`Open`,`Init`,`SQL`,`Close`,`LoadGear`,`LoadCombatArts`) consistent across Tasks 7–9. `source.LoadCache` (Plan 1) reused in Task 9 with its existing signature.
