# Plan 2v: Item effect_list Ingestion (Stat Sources + Procs) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ingest item `effect_list` faithfully — fold "When Equipped: Increases <stat> of caster by N" static grants into the item's stats as a distinct `effect` source (aggregated with `modifier` stats at calc time), and catalog triggered procs into a new `item_procs` table (captured, not scored).

**Architecture:** Parse faithfully, model separately. One shared parser (`internal/catalog/effects.go`) classifies each `When Equipped:` subtree as a static stat-grant (→ stat keyed by source `effect`) or a proc (→ `item_procs`). `item_stats` gains a `source` column; the item `StatBlock` is built by aggregating all stat rows across sources (model code unchanged). The backfill (`itemdex --effects`) and the main pull share the parser. Conservative: unmapped/`%`/`of target`/Decrease-ambiguous lines are logged to an audit report and skipped — never guessed.

**Tech Stack:** Go 1.26; census API (throttled client); SQLite (modernc); existing `internal/{census,extract,catalog,source,store}` + `cmd/{itemdex,builddb,bis}`.

**Design reference:** `docs/plans/2026-06-19-effect-list-ingestion-design.md` (approved).

**Author/runtime notes for the implementer:**
- The user is away; run autonomously. The audit report is produced for **post-hoc** review — the parser must be conservative (skip+log when unsure).
- `store.go`: `item_stats` table at line ~39; `LoadGear` inserts at ~350 (`INSERT OR REPLACE INTO item_stats (item_id, stat, value)`); `LoadScorableItems` reads stats at ~299; `combat_art_components` (line ~47, loaded ~111) is the **template** for the new `item_procs` table.
- `census` decode: `Item` has `Modifiers map[string]Modifier`; add `EffectList`.
- Reuse comma-tolerant numeric parsing (census uses `1,161`): strip commas before `ParseFloat` (this was a real bug source).

---

## Task 1: Census `effect_list` field + pull it

**Files:** `internal/census/item.go`, `internal/extract/extract.go`, `internal/census/item_test.go`

- [ ] **Step 1: Write the failing decode test**

In `internal/census/item_test.go`, add:
```go
func TestDecodeItemsEffectList(t *testing.T) {
	body := []byte(`{"item_list":[{"id":1,"displayname":"Cloak of Flames","effect_list":[
		{"description":"When Equipped:","indentation":0},
		{"description":"Increases Haste of caster by 25.0.","indentation":1}]}]}`)
	items, err := DecodeItems(body)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Len(t, items[0].EffectList, 2)
	require.Equal(t, "Increases Haste of caster by 25.0.", items[0].EffectList[1].Description)
	require.Equal(t, 1, items[0].EffectList[1].Indentation)
}
```
(Use the existing test's import style; add `github.com/stretchr/testify/require` if not present.)

- [ ] **Step 2: Run it — expect failure** (`EffectList` undefined)

Run: `go test ./internal/census/ -run TestDecodeItemsEffectList 2>&1 | tail -5`

- [ ] **Step 3: Add the type + field**

In `internal/census/item.go`, add an `Effect` struct and an `EffectList` field on `Item`:
```go
// Effect is one line of an item's effect_list (raw text + nesting depth).
type Effect struct {
	Description string `json:"description"`
	Indentation int    `json:"indentation"`
}
```
And in the `Item` struct, alongside `Modifiers`:
```go
	Modifiers   map[string]Modifier `json:"modifiers"`
	EffectList  []Effect            `json:"effect_list"`
```

- [ ] **Step 4: Add effect_list to the pull**

In `internal/extract/extract.go`, change `showFields` to append `,effect_list`:
```go
	showFields = "displayname,id,tier,itemlevel,gamelink,slot_list,typeinfo,modifiers,effect_list,_extended.discovered.world_list"
```

- [ ] **Step 5: Run + commit**

Run: `go test ./internal/census/ 2>&1 | tail -3` (PASS)
```bash
git add internal/census/item.go internal/census/item_test.go internal/extract/extract.go
git commit -m "Census: decode item effect_list + pull it (plan 2v)"
```

---

## Task 2: The effect parser (`internal/catalog/effects.go`) — CORE

**Files:** Create `internal/catalog/effects.go`, `internal/catalog/effects_test.go`

This is the heart and the interpretation-risk area. Conservative by construction.

- [ ] **Step 1: Write the failing tests (real wordings)**

Create `internal/catalog/effects_test.go`:
```go
package catalog

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/stretchr/testify/require"
)

func eff(pairs ...any) []census.Effect {
	var out []census.Effect
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, census.Effect{Description: pairs[i].(string), Indentation: pairs[i+1].(int)})
	}
	return out
}

func TestParseEffects_StaticStat(t *testing.T) {
	// Cloak of Flames: a direct When-Equipped stat grant → effect stat, no proc.
	stats, procs, _ := ParseEffects(eff(
		"When Equipped:", 0,
		"Increases Haste of caster by 25.0.", 1))
	require.Equal(t, map[string]float64{"attackspeed": 25.0}, stats)
	require.Empty(t, procs)
}

func TestParseEffects_Proc(t *testing.T) {
	// Cloak of Unrest: a spell-cast proc → item_procs, NOT a stat.
	stats, procs, _ := ParseEffects(eff(
		"When Equipped:", 0,
		"On a spell cast this spell may cast Harnessed Power of the Estate on caster.  Lasts for 30.0 seconds.  Triggers about 1.8 times per minute.", 1,
		"Decrease the caster's spell reuse time by 10%.", 2))
	require.Empty(t, stats)
	require.Len(t, procs, 1)
	require.InDelta(t, 1.8, procs[0].PerMinute, 1e-9)
	require.Contains(t, procs[0].Trigger, "On a spell cast")
}

func TestParseEffects_DamageProc(t *testing.T) {
	stats, procs, _ := ParseEffects(eff(
		"When Equipped:", 0,
		"On a successful melee attack this spell may cast Flame on target.  Triggers about 3.0 times per minute.", 1,
		"Inflicts 1,200 - 2,000 heat damage on target", 2))
	require.Empty(t, stats)
	require.Len(t, procs, 1)
	require.InDelta(t, 3.0, procs[0].PerMinute, 1e-9)
	require.Equal(t, "heat", procs[0].DmgType)
	require.InDelta(t, 1200, procs[0].MinDmg, 1e-9)
	require.InDelta(t, 2000, procs[0].MaxDmg, 1e-9)
}

func TestParseEffects_DecreaseIsNegative(t *testing.T) {
	stats, _, _ := ParseEffects(eff(
		"When Equipped:", 0,
		"Decreases Haste of caster by 5.0.", 1))
	require.Equal(t, map[string]float64{"attackspeed": -5.0}, stats)
}

func TestParseEffects_SkipsPercentTargetUnknown(t *testing.T) {
	stats, procs, audit := ParseEffects(eff(
		"When Equipped:", 0,
		"Increases Haste of caster by 10%.", 1, // percent → skip+audit
		"Increases STA of target by 40.", 1, // of target → skip
		"Increases Bogus Stat of caster by 7.", 1)) // unknown name → skip+audit
	require.Empty(t, stats)
	require.Empty(t, procs)
	require.GreaterOrEqual(t, len(audit), 2) // percent + unknown surfaced
}
```

- [ ] **Step 2: Run — expect compile failure** (`ParseEffects`, `Proc` undefined)

Run: `go test ./internal/catalog/ -run TestParseEffects 2>&1 | tail -5`

- [ ] **Step 3: Implement the parser**

Create `internal/catalog/effects.go`:
```go
package catalog

import (
	"regexp"
	"strings"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
)

// Proc is a triggered item effect, cataloged but not scored (the deferred
// proc layer reads these; see SPEC §16).
type Proc struct {
	Trigger   string  // the trigger line ("On a spell cast …", "On a successful melee attack …")
	PerMinute float64 // rate from "Triggers about N times per minute" (0 if none stated)
	DmgType   string  // for damage procs ("heat", "melee", …); "" otherwise
	MinDmg    float64
	MaxDmg    float64
	Raw       string  // full effect text (trigger + children), so nothing is lost
}

// AuditLine records a When-Equipped wording and how the parser classified it,
// for human review (SPEC §16 audit gate).
type AuditLine struct {
	Description string
	Kind        string // "stat" | "proc" | "skip"
	Detail      string // e.g. "attackspeed +25", "percent unit", "unknown stat", "of target"
}

// effectStatKey maps an effect's stat display-name to the census modifier key
// the rest of the pipeline uses. Captured faithfully — the model's
// modifierToField decides what is actually scored (so skills/attributes are
// captured here but simply unused until modeling adds them).
var effectStatKey = map[string]string{
	"haste":                  "attackspeed",
	"multi attack":           "doubleattackchance",
	"crit chance":            "critchance",
	"potency":                "basemodifier",
	"dps":                    "dps",
	"flurry":                 "flurry",
	"ability modifier":       "all",
	"reuse":                  "spelltimereusepct",
	"casting speed":          "spelltimecastpct",
	"agility":                "strength", // scout primary; aggregates with the "+all primary attributes" key → MainStat
	"all primary attributes": "strength",
	"combat skills":          "combatskills",
	"slashing":               "slashing",
	"piercing":               "piercing",
	"crushing":               "crushing",
	"ranged":                 "ranged",
	"aggression":             "aggression",
}

var (
	// "Increases|Decreases <stat> of caster by <N>." (point value, no % — % excluded by the absence of '%').
	effStatRe  = regexp.MustCompile(`^(Increases|Decreases) (.+?) of caster by ([\d,]+(?:\.\d+)?)\.?$`)
	effPctRe   = regexp.MustCompile(`of caster by [\d,.]+%`)
	procRateRe = regexp.MustCompile(`Triggers about ([\d.]+) times per minute`)
	procDmgRe  = regexp.MustCompile(`Inflicts\s+([\d,]+)\s*-\s*([\d,]+)\s+(\w+)\s+damage`)
	triggerRe  = regexp.MustCompile(`(?i)may cast|Triggers about|On a spell cast|On a successful|when (?:struck|striking)`)
)

func toFloat(s string) float64 {
	s = strings.ReplaceAll(s, ",", "")
	var f float64
	// best-effort; effStatRe/procDmgRe already guarantee numeric shape
	for _, fn := range []func() {} { _ = fn }
	_, _ = fmtSscan(s, &f)
	return f
}

// ParseEffects walks an item's effect_list and returns: static stat grants
// (census-key → summed value, source "effect"), cataloged procs, and an audit
// trail. Only DIRECT When-Equipped children that are unambiguous static stat
// grants become stats; anything under a trigger is a proc; everything else is
// skipped and logged. Conservative: never guess.
func ParseEffects(effects []census.Effect) (map[string]float64, []Proc, []AuditLine) {
	stats := map[string]float64{}
	var procs []Proc
	var audit []AuditLine

	for i := 0; i < len(effects); i++ {
		e := effects[i]
		if e.Indentation != 1 { // only direct When-Equipped children are classified at top level
			continue
		}
		desc := strings.TrimSpace(e.Description)

		// Proc: this line is a trigger → capture it + its deeper children, skip past them.
		if triggerRe.MatchString(desc) {
			p := Proc{Trigger: desc, Raw: desc}
			if m := procRateRe.FindStringSubmatch(desc); m != nil {
				p.PerMinute = toFloat(m[1])
			}
			j := i + 1
			for j < len(effects) && effects[j].Indentation > 1 {
				child := strings.TrimSpace(effects[j].Description)
				p.Raw += " | " + child
				if m := procDmgRe.FindStringSubmatch(child); m != nil {
					p.MinDmg, p.MaxDmg, p.DmgType = toFloat(m[1]), toFloat(m[2]), m[3]
				}
				j++
			}
			procs = append(procs, p)
			audit = append(audit, AuditLine{desc, "proc", "trigger"})
			i = j - 1
			continue
		}

		// Static stat grant?
		if m := effStatRe.FindStringSubmatch(desc); m != nil && !effPctRe.MatchString(desc) {
			name := strings.ToLower(strings.TrimSpace(m[2]))
			key, ok := effectStatKey[name]
			if !ok {
				audit = append(audit, AuditLine{desc, "skip", "unknown stat: " + name})
				continue
			}
			v := toFloat(m[3])
			if m[1] == "Decreases" {
				v = -v
			}
			stats[key] += v
			audit = append(audit, AuditLine{desc, "stat", key})
			continue
		}

		if effPctRe.MatchString(desc) {
			audit = append(audit, AuditLine{desc, "skip", "percent unit"})
			continue
		}
		audit = append(audit, AuditLine{desc, "skip", "unrecognized"})
	}
	return stats, procs, audit
}
```
Replace the placeholder `toFloat` body with a real implementation using `strconv`:
```go
import "strconv"
func toFloat(s string) float64 {
	f, _ := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	return f
}
```
(Delete the bogus `fmtSscan`/loop scaffold above — it was illustrative; the `strconv` version is the real one. Ensure imports are exactly `regexp`, `strconv`, `strings`, and the census package.)

- [ ] **Step 4: Run the parser tests** — `go test ./internal/catalog/ -run TestParseEffects 2>&1 | tail -15` → all PASS. Do not change asserted values; fix the implementation to match.

- [ ] **Step 5: Commit**
```bash
git add internal/catalog/effects.go internal/catalog/effects_test.go
git commit -m "Catalog: effect_list parser — static stats (effect source) + proc catalog (plan 2v)"
```

---

## Task 3: Effect/proc CSV artifacts + audit report writer

**Files:** `internal/catalog/effects_io.go` (new), `internal/catalog/effects_io_test.go`

The wide item CSVs stay modifier-only; effects/procs persist in their own files so `builddb` can rebuild deterministically.

- [ ] **Step 1: Failing round-trip test**

`internal/catalog/effects_io_test.go`:
```go
package catalog

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEffectStatsCSVRoundTrip(t *testing.T) {
	in := []EffectStat{{ItemID: 1, Stat: "attackspeed", Value: 25}}
	var b bytes.Buffer
	require.NoError(t, WriteEffectStatsCSV(&b, in))
	out, err := ReadEffectStatsCSV(&b)
	require.NoError(t, err)
	require.Equal(t, in, out)
}

func TestProcsCSVRoundTrip(t *testing.T) {
	in := []ItemProc{{ItemID: 2, Trigger: "On a spell cast", PerMinute: 1.8, Raw: "x"}}
	var b bytes.Buffer
	require.NoError(t, WriteProcsCSV(&b, in))
	out, err := ReadProcsCSV(&b)
	require.NoError(t, err)
	require.Equal(t, in, out)
}
```

- [ ] **Step 2: Run — expect compile failure.**

- [ ] **Step 3: Implement** `internal/catalog/effects_io.go`:
- Define `EffectStat{ItemID int; Stat string; Value float64}` and `ItemProc{ItemID int; Trigger string; PerMinute float64; DmgType string; MinDmg, MaxDmg float64; Raw string}`.
- `WriteEffectStatsCSV(io.Writer, []EffectStat) error` / `ReadEffectStatsCSV(io.Reader) ([]EffectStat, error)` — header `item_id,stat,value`.
- `WriteProcsCSV(io.Writer, []ItemProc) error` / `ReadProcsCSV(io.Reader) ([]ItemProc, error)` — header `item_id,trigger,per_minute,dmg_type,min_dmg,max_dmg,raw`.
- `WriteAuditReport(io.Writer, map[int][]AuditLine) error` — a markdown table grouped by classification (stat/proc/skip), each row: item id, kind, detail, the raw wording; mirror the prose style of existing docs. Follow the `encoding/csv` usage in `internal/catalog/csv.go`.

- [ ] **Step 4: Run** `go test ./internal/catalog/ 2>&1 | tail -5` → PASS.
- [ ] **Step 5: Commit** `git add internal/catalog/effects_io.go internal/catalog/effects_io_test.go && git commit -m "Catalog: effect-stats/procs CSV I/O + audit report writer (plan 2v)"`

---

## Task 4: Store — `source` column, `item_procs` table, source-aggregated stats

**Files:** `internal/store/store.go`, `internal/store/scorable_test.go` (or a new store test)

- [ ] **Step 1: Failing aggregation test**

Add to a store test (use the existing in-memory/temp DB pattern in `internal/store/*_test.go`):
```go
func TestStatsAggregateAcrossSources(t *testing.T) {
	db := openTestDB(t) // use the existing test helper pattern in this package
	require.NoError(t, db.Init())
	// one item, haste from modifier (10) + effect (25) → StatBlock.Haste 35.
	_, err := db.db.Exec(`INSERT INTO items (id,name,slot,tier,classes) VALUES (1,'X','Cloak','LEGENDARY','assassin')`)
	require.NoError(t, err)
	_, err = db.db.Exec(`INSERT INTO item_stats (item_id,stat,value,source) VALUES (1,'attackspeed',10,'modifier'),(1,'attackspeed',25,'effect')`)
	require.NoError(t, err)
	items, err := db.LoadScorableItems()
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.InDelta(t, 35.0, items[0].Stats.Haste, 1e-9)
}
```
(If the package lacks an `openTestDB` helper, mirror however the existing store tests construct a `*DB` over a temp file.)

- [ ] **Step 2: Run — expect failure** (no `source` column).

- [ ] **Step 3: Schema + writes + aggregation**
- In the `item_stats` `CREATE TABLE` (line ~39), add `source TEXT NOT NULL DEFAULT 'modifier'`.
- Add an `item_procs` table modeled on `combat_art_components`:
```sql
CREATE TABLE IF NOT EXISTS item_procs (
  item_id INTEGER, trigger TEXT, per_minute REAL,
  dmg_type TEXT, min_dmg REAL, max_dmg REAL, raw TEXT
);
```
- `LoadGear`'s insert (line ~350) → `INSERT OR REPLACE INTO item_stats (item_id, stat, value, source) VALUES (?, ?, ?, 'modifier')`.
- Add `LoadItemEffects([]catalog.EffectStat) error` (insert each as `source='effect'`) and `LoadItemProcs([]catalog.ItemProc) error` (insert into `item_procs`). Follow the transaction pattern of `LoadCombatArts`.
- `LoadScorableItems` reads `SELECT stat, value FROM item_stats WHERE item_id = ?` (line ~299) — this already sums all rows regardless of source via `AddModifiers`; **confirm** it folds every row (modifier + effect) into the `StatBlock`. `loadWeapon` similarly aggregates (weapon min/max/delay are columns, unaffected; weapon stat rows aggregate the same way). No model change.

- [ ] **Step 4: Run** `go test ./internal/store/ 2>&1 | tail -5` → PASS.
- [ ] **Step 5: Commit** `git add internal/store/store.go internal/store/*_test.go && git commit -m "Store: item_stats.source + item_procs table + source-aggregated stats (plan 2v)"`

---

## Task 5: builddb wires effect-stats + procs

**Files:** `cmd/builddb/main.go`

- [ ] **Step 1: Load the effect artifacts**

After `db.LoadGear(gear)` in `cmd/builddb/main.go`, load the effect CSVs if present (`data/item-effects.csv`, `data/item-procs.csv`) via `catalog.ReadEffectStatsCSV`/`ReadProcsCSV` and `db.LoadItemEffects`/`db.LoadItemProcs`. Treat missing files as empty (not an error) so builddb works before the first backfill. Mirror the existing error-handling style in the file.

- [ ] **Step 2: Build + smoke**

Run: `go build ./... && go run ./cmd/builddb --data data --db /tmp/build_check.db 2>&1 | tail -3`
Expected: builds; runs (effect files may be absent → loaded as empty; no error).

- [ ] **Step 3: Commit** `git add cmd/builddb/main.go && git commit -m "builddb: load item effect-stats + procs when present (plan 2v)"`

---

## Task 6: `itemdex --effects` backfill + main-pull integration

**Files:** `internal/source/source.go`, `cmd/itemdex/main.go`

- [ ] **Step 1: Shared write helper**

In `internal/source/source.go`, add `WriteEffectArtifacts(items []census.Item, dir string) error`: for each item run `catalog.ParseEffects(item.EffectList)`, accumulate `[]catalog.EffectStat` (source effect), `[]catalog.ItemProc`, and `map[int][]catalog.AuditLine`; write `data/item-effects.csv`, `data/item-procs.csv`, `data/effect-audit.md` via the Task-3 writers. (Have `FreshPull` call this after writing the wide CSVs, so full pulls capture effects natively.)

- [ ] **Step 2: Backfill mode**

In `cmd/itemdex/main.go`, add an `--effects` bool flag. When set: `source.LoadCache(dir)` → collect item IDs → batch-fetch `effect_list` from census by id (`c:show=id,effect_list`, ~50 ids/query) through the existing throttled `census.Client` (reuse its retry/backoff; on quota, log how many done and exit non-zero so a re-run resumes from the remaining IDs — track which IDs already have rows in `data/item-effects.csv`/audit, or simply re-fetch all on re-run since it's idempotent overwrite) → `census.DecodeItems` → `source.WriteEffectArtifacts`.

- [ ] **Step 3: Build + dry sanity (no network assertion in tests)**

Run: `go build ./... 2>&1 | tail -3` (builds). Unit-test `WriteEffectArtifacts` over two in-memory `census.Item`s (one with the Cloak-of-Flames effect, one with the Cloak-of-Unrest proc) asserting the three files' contents — no network.

- [ ] **Step 4: Commit** `git add internal/source/source.go cmd/itemdex/main.go internal/source/*_test.go && git commit -m "itemdex: --effects backfill + FreshPull effect ingestion (plan 2v)"`

---

## Task 7: Run the backfill + rebuild + re-score (data operation)

**Files:** none (produces `data/item-effects.csv`, `data/item-procs.csv`, `data/effect-audit.md`, refreshed `bis.db`, `bis-report.md`)

- [ ] **Step 1: Backfill effect data from census**

Run: `go run ./cmd/itemdex --effects --out data 2>&1 | tail -10`
Expected: writes `data/item-effects.csv`, `data/item-procs.csv`, `data/effect-audit.md`. **If it exits on census quota, re-run until it completes** (the public `s:example` throttle may require a few passes). Confirm row counts: `wc -l data/item-effects.csv data/item-procs.csv`.

- [ ] **Step 2: Rebuild + re-score**

Run: `go run ./cmd/builddb --data data --db bis.db 2>&1 | tail -3 && go run ./cmd/bis --db bis.db --character characters/alex.toml --out bis-report.md 2>&1 | tail -3`

- [ ] **Step 3: Confirm Cloak of Flames carries the effect haste and re-ranks**

Run: `sqlite3 bis.db "select stat,value,source from item_stats where item_id=264598753 and source='effect';"` → expect `attackspeed|25.0|effect`.
Run: `awk '/^## PRE-RAID/{p=1} p&&/^### Cloak/{f=1} f&&/^### /&&!/Cloak/{f=0} f' bis-report.md | head -6` → Cloak of Flames should be the BiS Cloak.

- [ ] **Step 4: Commit the data**
```bash
git add data/item-effects.csv data/item-procs.csv data/effect-audit.md
git commit -m "Data: backfill item effect_list (effect-stats + procs + audit) (plan 2v)"
```

---

## Task 8: Spec updates

**Files:** `docs/SPEC.md`

- [ ] **Step 1:** §4 — document item `effect_list` ingestion: multi-source item stats (`item_stats.source` ∈ modifier/effect, aggregated at calc time, adornment-ready), the shared `ParseEffects` (static stat-grants `of caster`, signed, points-only), and `item_procs` capture.
- [ ] **Step 2:** §6 — add `item_stats.source` column and the `item_procs` table to the schema.
- [ ] **Step 3:** §16 — proc-scoring reads `item_procs` (deferred); **correct the `critBonus` item** (crit bonus inert; raid auto-crit ~1.64 is the wide-weapon range-shift floor); note combat-skill/attribute effects captured-but-unscored.
- [ ] **Step 4:** `go test ./... 2>&1 | tail -3` (still PASS) + `grep -c '^## ' docs/SPEC.md` (16). Commit `git add docs/SPEC.md && git commit -m "Spec: §4/§6/§16 — item effect_list ingestion + proc catalog (plan 2v)"`

---

## Task 9: Final verification + audit/re-rank summary

**Files:** none

- [ ] **Step 1:** `go vet ./... && go test ./... 2>&1 | tail -8` → vet clean; all PASS.
- [ ] **Step 2:** Summarize for the user's post-hoc review: (a) `data/effect-audit.md` — counts of stat/proc/skip and the full skip list (anything mis-classified to fix + re-run); (b) which BiS slots re-ranked vs the pre-backfill report (esp. Cloak of Flames → Cloak); (c) `item_procs` row count (the new proc catalog). 
- [ ] **Step 3:** `git status --porcelain` → only intended changes; `bis-report.md` untracked.

---

## Self-Review

**Spec coverage:** census effect_list (T1) · parser static+proc+audit (T2) · CSV/audit I/O (T3) · source column + item_procs + aggregation (T4) · builddb wiring (T5) · backfill + FreshPull (T6) · run/backfill/rebuild (T7) · spec §4/§6/§16 (T8) · verify+audit summary (T9). All design sections covered.

**Placeholder scan:** Task 2 Step 3 contains an intentionally-flagged scaffold (`toFloat` placeholder) with the explicit `strconv` replacement called out in the same step — the engineer must use the `strconv` version; no other TBDs. CSV writers (T3) specify exact headers + the file to mirror.

**Type/name consistency:** `census.Effect{Description,Indentation}`, `catalog.ParseEffects → (map[string]float64, []Proc, []AuditLine)`, `catalog.EffectStat`, `catalog.ItemProc`, `store.LoadItemEffects`/`LoadItemProcs`, `item_stats.source`, `item_procs` used consistently across T1–T8. `effectStatKey` maps to census keys that match `modifierToField` (attackspeed/critchance/basemodifier/dps/flurry/all/spelltimereusepct/spelltimecastpct/strength) so scored stats aggregate; non-scored keys (combatskills/slashing/…) are captured but ignored by the model, as designed.
