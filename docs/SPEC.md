# EQ2 EoF Assassin Best-in-Slot — System & Model Spec

**Status:** Living spec — the single source of truth for this system.
**Supersedes:** `docs/plans/design.md` and `docs/plans/design-plan2.md` (historical design records; kept as timeline only).
**Module:** `github.com/amdrake93/eq2-eof-itemdex` · **Go:** 1.26
**Provenance rule:** This spec describes what the code does. Where prose states a formula or constant, it matches the cited code symbol. Measurement provenance is summarized from code comments, `data/*.csv`, and tests.

---

# Part I — The System (how the program works)

## 1. Goal & Deliverables

The system answers one question for an EQ2 Eye of Fear (EoF) era Assassin on the Varsoon TLE server: which gear set maximizes relative DPS? It produces three artifacts:

1. **BiS markdown report** (`bis-report.md`) — three-tier gear rankings (PRE-RAID / RAID / BEST-OF-BEST), each listing the converged best-in-slot pick per equipment slot plus the top-N alternatives ranked by in-context ΔDPS (`cmd/bis`).
2. **SQLite catalog** (`bis.db`) — queryable database of Assassin-usable gear, Assassin combat arts with parsed damage components, and per-item DPS scores under each tier baseline (`internal/store`).
3. **CSV gear catalogs** (`data/weapons.csv`, `data/armor.csv`, `data/jewelry-charms.csv`, `data/maxlife.csv`) — flat-file item cache used as offline Census snapshots and as the DB build input (`internal/catalog`).

**Non-goals:** The model produces relative DPS ordering only — it is not calibrated for parse-accurate absolute DPS numbers. Only the Assassin class is implemented; other scout subclasses are future work (see §16).

## 2. Architecture & Data Flow

### Pipeline

```
Census API
    │
    ▼
[Stage 1] Throttled pull + pagination
  internal/census   client.go — rate-limited HTTP client (1 req/6 s, retry on 429)
  internal/extract  extract.go — Varsoon-windowed pagination (world 614, EoF timestamp window)
  internal/classify eof.go — per-item EoF discovery-window gate (client-side refinement)
    │
    ▼  items []census.Item
[Stage 2] CSV cache read/write
  internal/source   source.go — Load(): serve from cache or trigger fresh pull;
                                 FreshPull(): paginate, split by category, write CSVs
  internal/catalog  csv.go    — WriteCSV / ReadCSV (wide format: fixed cols + stat-key union)
                    category.go — slot→category mapping, Census skilltype→armor-type labels
    │
    ▼  []census.Item (from cache)
[Stage 3] DB build
  internal/store    store.go  — SQLite schema (items, item_stats, combat_arts,
                                combat_art_components, scores); LoadGear, LoadCombatArts
  internal/spell    pull.go   — AssassinCombatArts (Census spell collection, Expert tier, level ≤70)
                    manual.go — ManualArts (two low-level-learned arts recovered via tooltip calibration)
                    parse.go  — ParseDamage / ParseComponents (effect_list text → Component structs)
    │
    ▼  bis.db
[Stage 4] Character & class config
  internal/charconfig charconfig.go — TOML character config (AA stats, art mods, buff contexts);
                                      LoadClass (classes/<class>.toml, auto_attack_multiplier)
[Stage 5] DPS model
  internal/constants  constants.go — locked combat constants (crit, flurry, haste cap, recast ceiling, …)
  internal/model      dps.go       — AutoDPS, AutoDPSDual, CADPS, TotalDPSDual
                      stats.go     — StatBlock, AddModifiers
                      curve.go     — HasteDpsModEffect, MainStatEffect (fitted quadratic)
                      weights.go   — DeriveWeights (finite-difference marginal DPS per +1 stat)
                      rotation.go  — rotationTimeline, fightLen-smoothed CA scheduling
    │
    ▼
[Stage 6] BiS optimizer + render
  internal/bis  build.go      — BuildSet (coordinate-ascent optimizer, up to 12 passes)
                candidates.go — SlotCandidates (slot grouping, off-hand weapon pool)
                report.go     — SlotReport, ConvergedWeights, BuildSlotReports
                render.go     — Render (markdown report)
                set.go        — Set (equipped loadout, DPS evaluation)
                exclusions.go — IsHunters, IsAvatar, Curated (tier/source filter predicates)
```

### Package Responsibilities

| Package | Responsibility |
|---|---|
| `internal/census` | Throttled Daybreak Census API client (`Client.Get`). Rate-limited to 1 request per 6 seconds with a single 30-second retry on HTTP 429 or network timeout. Decodes the `item_list` JSON envelope (`DecodeItems`); spell decoding lives in `internal/spell`. |
| `internal/extract` | Varsoon-windowed pagination over the Census `item` collection. Server-side pre-filter: world 614, EoF timestamp window, item level < 72. Handles incremental resume when the `s:example` service-ID quota cuts a session short (`PartialError`, `.census_next_offset` marker). |
| `internal/classify` | EoF and KoS expansion-window detection. Defines the Varsoon world ID (614), the EoF discovery window (2023-04-11 – 2023-08-08), and the KoS window immediately prior. `IsEoF` / `IsKoS` are the client-side per-item gates applied after server-side pre-filtering. |
| `internal/source` | CSV cache gatekeeper. `Load` serves items from the category CSVs when they exist and `--refresh` is false; otherwise calls `FreshPull`. `FreshPull` merges any prior partial cache, calls `extract.AllEoFFrom`, splits output by catalog category, and writes `weapons.csv`, `armor.csv`, `jewelry-charms.csv`, and `maxlife.csv`. |
| `internal/catalog` | CSV schema and item classification. `WriteCSV` / `ReadCSV` round-trip items in wide format (fixed columns + the union of all stat keys). `CategoryForSlot` maps slot names to the three catalog categories. `ArmorType` maps Census `typeinfo.skilltype` values to Cloth / Leather / Chain / Plate labels. |
| `internal/store` | SQLite schema and queries. Five tables: `items`, `item_stats`, `combat_arts`, `combat_art_components`, `scores`. Key operations: `LoadGear`, `LoadCombatArts`, `LoadLoadout` (Soulfire main-hand + highest-rank CAs), `LoadScorableItems` (Assassin items with stat blocks resolved), `WriteScores`. |
| `internal/spell` | Combat-art pull, parse, and manual supplement. `AssassinCombatArts` queries the Census `spell` collection for Assassin Expert-tier arts at level ≤ 70, then `FilterCombatArts` drops non-damaging and beneficial entries. `ParseDamage` and `ParseComponents` extract damage ranges and component kinds (DirectHit, DoT, Proc) from Census `effect_list` text. `ManualArts` returns two arts learned below the level-57 census floor, recovered via tooltip calibration. |
| `internal/model` | DPS arithmetic. `AutoDPS` (per-weapon sustained swing DPS), `AutoDPSDual` (dual-wield with ×1.33 off-hand delay penalty), `CADPS` (fight-length-smoothed combat-art DPS from rotation timeline), `TotalDPSDual` (auto + CA combined). `DeriveWeights` finite-differences marginal DPS per +1 to each stat. |
| `internal/bis` | Set builder, ranker, and renderer. `BuildSet` runs coordinate ascent (up to 12 passes) to fill each equipment slot with the DPS-maximizing pick given the current full-set context. `SlotCandidates` builds per-slot candidate pools (off-hand pool = all one-handed non-Soulfire weapons). `ConvergedWeights` derives stat weights at the converged set. `Render` produces the markdown report. |
| `internal/charconfig` | TOML character and class config. `Load` parses `characters/<name>.toml` (strict: unknown keys are errors), validating AA stats, per-art recast/potency mods, and named buff contexts (`solo`, `raid`). `LoadClass` reads `classes/<class>.toml` for the class auto-attack multiplier. `ApplyArtMods` stamps per-art AA modifiers onto the loaded combat-art pool. |
| `internal/constants` | Locked combat constants shared across all packages. Covers crit multiplier, flurry multiplier, haste/DPS-mod caps, recast-reduction ceiling, dual-wield delay penalty, fight duration, and CA cast time. Per-character values are not here — they live in TOML config. |
| `internal/fit` | Curve fitting for the haste/DPS-mod conversion. `FitQuad` fits `f(s) = A·s − B·s²` to tooltip readings in `data/curve-readings.csv`; `FitLog` fits the logarithmic alternative for residual comparison. `cmd/fitcurve` prints paste-ready constants for `internal/model/curve.go`; `TestFittedConstantsMatchReadings` (sync test) fails until those constants are updated after new readings. |

## 3. Data Acquisition (Census)

### Client

`internal/census/client.go New` creates a throttled HTTP client with a 60-second request timeout, a token-bucket limiter set to one request every 6 seconds (`rate.Every(6*time.Second), 1`), and a 30-second backoff (`Backoff: 30 * time.Second`). This keeps the caller comfortably within the public `s:example` service-ID quota of approximately 10 requests/minute.

`Client.Get` retries once on HTTP 429 (Too Many Requests) or any network-level error (timeout, connection reset). On a retriable error it sleeps the full backoff before the second attempt; after two attempts it returns an error. The request URL format is `{BaseURL}/{SID}/get/eq2/{collection}/?{query}`.

### Flexible JSON — `census.FlexString`

`internal/census/item.go FlexString` is a `string` alias whose `UnmarshalJSON` handles the Census API's inconsistent field encoding: some fields (notably `displayname` on unnamed items) are emitted as bare JSON numbers rather than quoted strings. When the first byte is not `"`, the raw JSON scalar is stored verbatim as its decimal string representation.

### Server-side pre-filter (`extract.collect`)

`internal/extract/extract.go collect` pages the Census `item` collection with three server-side predicates in the query string:

- `_extended.discovered.world_list.id=614` — Varsoon TLE server only (`classify.VarsoonWorldID = 614`)
- `_extended.discovered.world_list.timestamp` inside the expansion window (operators URL-encoded as `%3E` / `%3C`)
- `itemlevel<%3C72` (`maxItemLevel = 72` — the EoF item level ceiling)

The timestamp window used for the server-side filter is intentionally loose: Census array matching cannot bind `id==614` and the timestamp range to the same element in a single query, so the window pre-filters broadly and `classify.IsEoF` (or `classify.IsKoS`) does the precise per-item classification client-side.

### Client-side classification (`classify.IsEoF`)

`internal/classify/eof.go VarsoonDiscovery` scans an item's `_extended.discovered.world_list` for an entry with `ID == 614` and returns its Unix timestamp. `IsEoF` accepts an item whose Varsoon discovery timestamp falls in `[EoFStart, EoFEnd)`:

- `EoFStart = 2023-04-11` (White Oak Acorn, first EoF-exclusive collectable)
- `EoFEnd   = 2023-08-08` (Tuft of Dark Brown Brute Fur, first RoK-exclusive collectable — exclusive upper bound)

`IsKoS` uses the window `[KoSStart, KoSEnd)` where `KoSStart = 2022-12-11` and `KoSEnd = EoFStart`. The KoS window is only exercised by `extract.AllKoS`, which feeds `maxlife.csv` (the cross-cut max-life list).

### Pagination and quota resume

`extract.AllEoFFrom` (and `AllEoF` which wraps it at offset 0) calls `collect` with a caller-supplied page size and start offset. If Census returns its quota-exceeded sentinel ("Missing Service ID…") mid-run, `collect` returns a `*PartialError` carrying the items already collected and the `NextOffset` at which the session ended. The caller can re-run with `--refresh` and pass `NextOffset` to `AllEoFFrom` to accumulate additional pages in a future session. The package-level sentinel `ErrQuotaExceeded` is wrapped inside `PartialError` and can be tested with `errors.Is`.

## 4. Catalog & Persistence

### CSV cache — wide format

`internal/catalog/csv.go WriteCSV` writes items in a wide (denormalized) CSV format. The file has a fixed column block followed by a dynamic stat-key block:

**Fixed columns:** `id`, `name`, `slot`, `tier`, `itemlevel`, `armor_type`, `classes`, `weapon_min_dmg`, `weapon_max_dmg`, `delay`, `damage_rating`, `skill`, `wieldstyle`, `gamelink`.

**Stat columns:** the union of all Census modifier keys present across the items in the batch, sorted alphabetically. Cells are empty for items that lack a given modifier.

`ReadCSV` reconstructs `[]census.Item` from a `WriteCSV` stream by reading the header to discover which columns are fixed vs stat, then rebuilding each item's `Modifiers` map from the non-empty stat cells. The `armor_type` label is round-tripped back to the Census `skilltype` string via `SkillTypeFromArmorType` so that slot-to-category lookups continue to work after a cache load.

### Category files

Items are split across four CSV files based on their first slot:

| File | Category | Slots |
|---|---|---|
| `data/weapons.csv` | `weapons` | primary, secondary, ranged |
| `data/armor.csv` | `armor` | head, shoulders, chest, forearms, hands, legs, feet |
| `data/jewelry-charms.csv` | `jewelry-charms` | neck, ears, wrist, ring, finger, charm, waist, belt, cloak |
| `data/maxlife.csv` | cross-cut | KoS-window items, used for the max-life tier |

`internal/catalog/category.go CategoryForSlot` performs the slot→category lookup (case-insensitive); unrecognized slots map to `"other"` so nothing is silently dropped.

### Armor-type mapping

`ArmorType` maps Census `typeinfo.skilltype` values to human-readable labels stored in the CSV `armor_type` column:

| Census `skilltype` | Label |
|---|---|
| `verylightarmor` | `Cloth` |
| `lightarmor` | `Leather` |
| `mediumarmor` | `Chain` |
| `heavyarmor` | `Plate` |

`SkillTypeFromArmorType` is the inverse, used during CSV load-back.

### SQLite schema

`internal/store/store.go` defines five tables, created by `Init()`:

| Table | Purpose |
|---|---|
| `items` | One row per gear item: identity fields (id, name, slot, tier, itemlevel), armor_type, weapon damage/delay fields (0 for non-weapons), class list, gamelink. |
| `item_stats` | One row per (item_id, stat) pair — the modifier key/value pairs from Census, normalized out of the wide CSV into a two-column table. |
| `combat_arts` | One row per combat art: name (PK), level, damage range (min/max), recast_secs, cast_secs_hundredths, duration_secs (effect duration; 0 if none). |
| `combat_art_components` | One row per parsed damage component: art_name + idx form the PK; kind (integer-encoded `ComponentKind`), dmg_type, damage range, interval_secs, has_instant, aoe, triggered_spell, triggers, per_minute. |
| `scores` | One row per (item_id, baseline) pair: the item's in-context ΔDPS score and the slot it was evaluated for. |

### Census modifier key → StatBlock field

`internal/model/stats.go modifierToField` maps every Census item modifier key the system tracks to the `StatBlock` field it increments. Unlisted keys (critbonus, resistances, hp/power, specific attributes) are silently skipped.

| Census key | StatBlock field | Notes |
|---|---|---|
| `attackspeed` | `Haste` | |
| `doubleattackchance` | `MultiAttack` | Legacy Census key name; maps to the Multi-Attack stat |
| `critchance` | `CritChance` | |
| `basemodifier` | `Potency` | |
| `dps` | `DPSMod` | |
| `spelltimereusepct` | `Reuse` | |
| `flurry` | `Flurry` | |
| `all` | `AbilityMod` | displayname "All" = Ability Modifier |
| `spelltimecastpct` | `CastSpeed` | |
| `strength` | `MainStat` | Game encoding for "+N to all primary attributes" — EQ2 files all-primary-attribute bonuses under the `strength` key. For a scout the relevant primary attribute is Agility, and the game grants it point-for-point from items keyed `strength`. Explicit single-attribute keys (`agility`, `wisdom`, `intelligence`) are excluded as data-suspicious (see §11). |

## 5. Combat-Art Pipeline

### Overview

The pipeline runs: pull from Census → filter → collapse to highest rank → parse damage → append manual arts.

### Pull — `AssassinCombatArts`

`internal/spell/pull.go AssassinCombatArts` queries the Census `spell` collection with these fixed predicates:

- `classes.assassin.id=40` (`assassinClassID = 40` — the Assassin class id in the spell collection; note this differs from the item collection's class id of 15, a Census quirk)
- `type=arts`
- `tier_name=Expert`
- `level<71` (level ≤ 70)

This is the current hardcoded reality: the pipeline is Assassin-only and Expert-tier-only. Class-agnosticism and tier flexibility are deferred (see §16 — Open Items & Known Divergences).

### Filter — `FilterCombatArts`

`FilterCombatArts` applies three conditions to keep only arts that belong in a melee rotation:

1. `level >= minDamageArtLevel` (57) — below this threshold abilities are vestigial low-level filler that never scales to level 70.
2. `beneficial == 0` — drops buffs, debuffs, and utility arts.
3. A parseable damage line must be present in `effect_list` (confirmed by `ParseDamage`) — drops non-damaging arts.

Ranged bow shots pass all three tests and are intentionally kept: they fire with no minimum range restriction and do not consume melee auto-attack time, so they contribute free bonus CA damage that fills idle slots in the rotation.

### Rank collapse — `HighestRanks` / `BaseName`

Census returns multiple ranks of each ability line (e.g. "Mortal Blade III", "Mortal Blade IV"). `BaseName` strips trailing roman-numeral or arabic-digit rank suffixes. `HighestRanks` collapses all ranks of a line to the single entry with the highest `MaxDamage`. All ranks of a line share one cooldown, so only the highest rank is ever cast.

### Parse — `ParseDamage` / `ParseComponents`

`internal/spell/parse.go ParseDamage` scans an art's `effect_list` for the first line matching `"Inflicts N - M <type> damage"` and returns the min/max as the art's headline damage range.

`ParseComponents` extracts the full typed component hierarchy. Census `effect_list` encodes structure through indentation: each `Effect` carries an `Indentation` integer, and a child line (indentation N) is interpreted against its parent (the last line seen at indentation N−1). The resolution rules are:

- **Indentation 0, no periodic clause** → `DirectHit` component.
- **Indentation 0, "every N seconds" clause** → `DoT` component (`HasInstant=true` when the clause reads "instantly and every").
- **Child of "Applies \<Spell\> on termination"** → `Termination` component (fires once when the effect expires).
- **Child of "may/will cast \<Spell\> on target" with "Triggers about N times per minute"** → `RateProc` component.
- **Child of "may/will cast \<Spell\> on target" without a rate** → `TriggerProc` component.
- **"Grants a total of N triggers"** at the same indentation as a proc component → sets that component's `Triggers` count.

`effect_list` is sourced from Census raw data, pre-calculation — it represents the structural content of the ability (component kinds, damage types, delivery patterns) without character-stat scaling applied, making it the correct structural source for building the component model.

### Manual supplement — `ManualArts`

`internal/spell/manual.go ManualArts` returns two arts that the Census pull misses because they are learned below the level-57 floor. Census reports their low-level base damage; the level-70 effective bases were recovered via tooltip calibration (attribute-divided back to the Census-equivalent value):

| Art | L70 min | L70 max | Recast | Cast |
|---|---|---|---|---|
| Hilt Strike | 262 | 315 | 20 s | 0.5 s |
| Strike of Consistency | 199 | 199 | 12 s | 0.5 s |

Both are single `DirectHit` melee components. `ManualArts` returns a deep copy to prevent callers from mutating the package-level constants.

## 6. Configuration & Accessibility Tiers
## 7. BiS Engine
## 8. Commands & Operations
## 9. Testing & Calibration-Sync

---

# Part II — The Game (mechanics, calibrated)

## 10. The DPS Model
## 11. Stat-Conversion Mechanics
## 12. Auto-Attack Model
## 13. Combat-Art Damage & Components
## 14. Rotation Model
## 15. Constants Block
## 16. Open Items & Known Divergences
