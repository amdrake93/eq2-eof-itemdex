# EQ2 EoF Assassin Best-in-Slot — System & Model Spec

**Status:** Living spec — the single source of truth for this system.
**Supersedes:** `docs/plans/design.md` and `docs/plans/design-plan2.md` (historical design records; kept as timeline only).
**Module:** `github.com/amdrake93/eq2-eof-itemdex` · **Go:** 1.26
**Provenance rule:** This spec describes what the code does. Where prose states a formula or constant, it matches the cited code symbol. Measurement provenance is summarized from code comments, `data/*.csv`, and tests.

---

# Part I — The System (how the program works)

## 1. Goal & Deliverables

The system answers one question for an EQ2 Eye of Fear (EoF) era Assassin on the Varsoon TLE server: which gear set maximizes relative DPS? It produces three artifacts:

1. **BiS markdown report** (`bis-report.md`, written into the character's directory — §6) — three-tier gear rankings (PRE-RAID / RAID / BEST-OF-BEST), each listing the converged best-in-slot pick per equipment slot plus the top-N alternatives ranked by in-context ΔDPS (`cmd/bis`).
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
| `internal/store` | SQLite schema and queries. Five tables: `items`, `item_stats`, `combat_arts`, `combat_art_components`, `scores`. Key operations: `LoadGear`, `LoadCombatArts`, `LoadLoadout` (highest-rank CAs), `LoadScorableItems` (Assassin items with stat blocks resolved, `ORDER BY id`), `WriteScores`. |
| `internal/spell` | Combat-art pull, parse, and manual supplement. `AssassinCombatArts` queries the Census `spell` collection for Assassin Expert-tier arts at level ≤ 70, then `FilterCombatArts` drops non-damaging and beneficial entries. `ParseDamage` and `ParseComponents` extract damage ranges and component kinds (DirectHit, DoT, Proc) from Census `effect_list` text. `ManualArts` returns two arts learned below the level-57 census floor, recovered via tooltip calibration. |
| `internal/model` | DPS arithmetic. `AutoDPS` (per-weapon sustained swing DPS), `AutoDPSDual` (dual-wield with ×1.33 off-hand delay penalty), `CADPS` (fight-length-smoothed combat-art DPS from rotation timeline), `TotalDPSDual` (auto + CA combined). `DeriveWeights` finite-differences marginal DPS per +1 to each stat. |
| `internal/bis` | Set builder, ranker, and renderer. `BuildSet` runs coordinate ascent (up to 12 passes) to fill each equipment slot with the DPS-maximizing pick given the current full-set context. `SlotCandidates` builds per-slot candidate pools (both weapon slots from the class weapon config, §6). `ConvergedWeights` derives stat weights at the converged set. `Render` produces the markdown report. |
| `internal/charconfig` | TOML character and class config. `Load` parses a character's `config.toml` (`characters/<census_name>/config.toml`, strict: unknown keys are errors), validating AA stats, per-art recast/potency mods, and named buff contexts (`solo`, `raid`). `LoadClass` reads `classes/<class>.toml` for the class auto-attack multiplier. `ApplyArtMods` stamps per-art AA modifiers onto the loaded combat-art pool. |
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

### Character import query

`itemdex import` (§8) reads the live equipped loadout for one character from the Census `character` collection. The query is keyed on the config's `census_name` and `world` (§6): `name.first_lower=<census_name lowercased>` + `locationdata.worldid=<world>`, with `c:show=equipmentslot_list,type,last_update`. The character name field is nested (`name.first` / `name.first_lower`); the world is `locationdata.worldid` (Wuoshi = 618). This is the only network step of the import; resolution and sim are offline. The `last_update` timestamp is carried into the loadout file so snapshot staleness is visible rather than hidden.

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
| `item_stats` | One row per (item_id, stat, source) triple — PRIMARY KEY `(item_id, stat, source)`. Modifier rows (source `modifier`) come from the Census modifiers block; effect rows (source `effect`) come from `effect_list` static grants. Rows with the same stat but different sources coexist and are summed when building a `StatBlock`. |
| `item_procs` | One row per cataloged gear proc: `(item_id, trigger, per_minute, dmg_type, min_dmg, max_dmg, raw)`. Captures triggered effects from `effect_list` — rate, damage, and the raw trigger text. Procs are cataloged but not yet scored. |
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

### Effect-list ingestion

An item's stats aggregate across multiple **sources**: `modifier` rows come from the Census `modifiers` block (the stats the catalog has always tracked); `effect` rows come from the item's `effect_list`, specifically "When Equipped: Increases/Decreases \<stat\> of caster by N" lines — static grants only (signed, point-values only, of-caster only). When building a `StatBlock` for a scored item, rows from all sources are summed. Adornments are not modeled anywhere — their stats are assumed roughly constant across the items competing for a slot, so they cancel in relative comparison (see §7, §16).

The shared parser `internal/catalog/effects.go ParseEffects` extracts static of-caster grants (mapped to census stat keys via `modifierToField`; non-mapped keys such as combat skills and single attributes are captured but not scored) and routes triggered effects to a proc record. Parsed effect-stats persist in `data/item-effects.csv`; gear procs persist in `data/item-procs.csv`; an `effect-audit.md` report summarizes the review. Triggered effects are cataloged as procs and are not folded into the item's stat totals.

### Imported loadout file

Import writes **`loadout.toml` into the config's own directory** (the per-character directory — §6; e.g. `characters/biffels/loadout.toml`) — one entry per kept slot with `item_id`, `name`, an `optimizable` flag, and a resolved `stats` block (the item's `modifiers` **plus** its `effect_list` "When Equipped" stat grants). Effect-list grants are folded in via `loadout.ItemStatBlock`, which takes the item plus an `effectStatsLookup` (built from `data/item-effects.csv`): freshly-fetched items carry their grants in `effect_list`; already-cataloged items (whose cached `effect_list` is empty) get theirs from the lookup — so e.g. Cloak of Flames' +25 haste is counted. The named "Haste" item effect routes to `StatBlock.HasteEffect` (non-stacking — §11). Weapon slots additionally carry `min_dmg`/`max_dmg`/`delay`. A top-level `last_update` records the Census snapshot time. The file is self-contained — `bis --loadout` consumes it with no network and no re-resolution.

**Adornments are not modeled.** Equipped adornment sockets are read from the character record but their stats are deliberately **not** counted, because adornments are assumed roughly constant across the items competing for a slot — so they cancel in relative ΔDPS comparison (§7, §16). There is no adornment catalog or adornment fetch.

Gear items equipped but absent from the catalog are fetched on demand and **appended to the existing item CSVs** (so `builddb` permanently picks them up). Import does not auto-run `builddb`; it prints a reminder, preserving the pull → builddb → bis flow (§8). Any item id Census cannot resolve is reported as "unresolved — stats not counted," never silently dropped.

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

### Character TOML structure

**Per-character directory.** Each character has one directory `characters/<census_name lowercased>/` holding everything for that character: the hand-authored `config.toml` (committed) plus all generated outputs — `loadout.toml` (import), `upgrade-report.md` (`bis --loadout`), and `bis-report.md` (`bis` from-scratch). All of these — `config.toml` and the generated files alike — are committed to the repo, so GitHub renders the reports with their EQ2U item links (§8). The directory is keyed by the character so multiple characters coexist without clobbering. **Path rule:** every generated output co-locates with the config's directory — commands derive the output directory from the path of the config (`--character`) or loadout (`--loadout`) file they were given, so nothing re-derives a name (§8). The committed example is `characters/biffels/config.toml` (the player is "Alex"; the in-game character is "Biffels", so the directory is keyed by the latter).

`internal/charconfig/charconfig.go Load` parses the `config.toml` (e.g. `characters/biffels/config.toml`) using `github.com/BurntSushi/toml` in strict mode: any TOML key that does not map to a struct field is reported as an error via `md.Undecoded()`. A typo'd stat name silently vanishes in most configs; here it fails loudly.

The top-level `Config` struct has four sections:

| Section | Type | Purpose |
|---|---|---|
| `[character]` | `Character` | Identity fields: `name`, `class`, `art_tier`; plus `census_name` and `world` for gear import (§4, §8). |
| `[stats]` | `StatGrants` | AA/innate stat grants applied to every evaluation context (baseline before gear and context buffs). |
| `[art_mods."Name"]` | `map[string]ArtMod` | Per-art AA modifiers keyed by rank-stripped art name. |
| `[contexts.X]` | `map[string]StatGrants` | Named buff packages. At least one context is required. |

**`Character` validation** — `Load` enforces two hard constraints that reflect the current implementation scope:

- `Class` must be `"assassin"` — any other value is a hard error (`"only assassin is implemented"`). See §16 — Open Items & Known Divergences for the class-agnosticism backlog item.
- `ArtTier` must be `"expert"` — any other value is a hard error (`"only expert is implemented"`). See §16 for the tier-flexibility backlog item.

**Gear-import fields.** `census_name` (string) and `world` (int) identify the live character for `itemdex import` (§4, §8). They are optional for non-import flows (existing configs load unchanged) and required when `import` runs. Gear is still never part of the character config — import writes a separate loadout file, and the optimizer adds gear (see `ContextBlock` below and §7).

**`StatGrants` validation** — every stat in a `[stats]` or `[contexts.X]` block must be non-negative. Config stats are grants; debuffs are not a config concept, and a negative `cast_speed` at or below −100 would make the rotation's divisor non-positive. The check is enforced by `StatGrants.nonNegative()` for both the base `[stats]` block and every context block individually.

**`ArtMod`** carries two fields, both validated at load time:

- `RecastMult float64` (`recast_mult` in TOML) — multiplies the art's base recast seconds. Must be in the range `(0, 1]`. A value of `0.5` halves the recast, which fills the art's 50% AA recast-reduction ceiling. `ApplyArtMods` converts this to `RecastReduction = 1 − RecastMult` on the `CombatArt`.
- `PotencyAdd float64` (`potency_add` in TOML) — additive potency rider pooled with the character's displayed potency. Must be non-negative.

`ApplyArtMods` also enforces that every key in the `[art_mods]` map matches at least one loaded combat art (by rank-stripped name). An unmatched key is a hard error — a typo'd art name failing loudly beats silently un-halving an art like Assassinate.

**`ContextBlock(name)`** returns the `model.StatBlock` used for one evaluation context: it combines `[stats]` + the named context's buff package via `StatGrants.Block().Add(ctx.Block())`. Gear is never part of the character config — the optimizer adds gear.

### Class TOML structure

`LoadClass(dir, class)` reads `classes/<class>.toml`. The schema is uniform across classes: every class file defines the same fields (a missing field is a hard error). The fields are:

- `auto_attack_multiplier float64` — the class-intrinsic auto-attack multiplier. Must be `> 0`. The Assassin value is `2.0`, measured via `/weaponstat` on 2026-06-13. This is distinct from per-character stats and never lives in the character TOML.
- `dual_wield bool` — whether the class wields a second (off-hand) weapon. The Assassin is `true`. When `false`, the `Secondary` weapon slot is not optimized and contributes no off-hand auto-attack.
- `weapon_wield_styles []string` — the catalog `wieldstyle` values valid in this class's **main-hand**, used to build the weapon candidate pools (§7). The Assassin is `["One-Handed"]`. (A future two-handed class would list `"Two-Handed"`; the two-handed-precludes-off-hand branch is not yet implemented — §16.)

`LoadClass` is also strict: unknown keys in the class TOML are errors.

### Accessibility tiers

`cmd/bis/main.go` defines three gear tiers that control which items are eligible in each optimization run:

| Tier | Stat context | Gear filter |
|---|---|---|
| `PRE-RAID` | `solo` context | Tier is `LEGENDARY` or `TREASURED`; avatar gear excluded; Hunter's sets excluded; curated exclusions excluded. |
| `RAID` | `raid` context | All tiers except avatar (MYTHICAL non-Soulfire); Hunter's sets excluded; curated exclusions excluded. |
| `BEST-OF-BEST` | `raid` context | All items including avatar (MYTHICAL); only Hunter's sets and curated exclusions removed. |

Avatar gear is defined by `IsAvatar`: `Tier == "MYTHICAL"` and `Name` does not start with `"Soulfire"`. Hunter's gear is defined by `IsHunters`: `Name` contains `"Hunter's"`. `Curated` is a hand-maintained map in `exclusions.go` for items that are discoverable on Varsoon but practically inaccessible; it is currently empty.

The main-hand (`Primary`) is an optimized weapon slot like any other (§7), subject to the same tier filters as every slot.

## 7. BiS Engine

### Set representation

`internal/bis/set.go Set` is the central data structure for an in-progress or converged gear loadout:

| Field | Type | Role |
|---|---|---|
| `Profile` | `model.StatBlock` | Baseline stat block from the character config + context (no gear). |
| `Arts` | `[]spell.CombatArt` | Combat-art pool with AA mods applied. |
| `AutoMult` | `float64` | Class auto-attack multiplier (`ClassData.AutoAttackMultiplier`). |
| `FightLen` | `float64` | Target fight length in seconds (rotation smoothing window). |
| `Equipped` | `map[string][]store.ScorableItem` | Currently equipped items per Census slot. Multi-capacity slots hold more than one item (see below). Both weapons live here too: the main-hand in `Equipped["Primary"]`, the off-hand in `Equipped["Secondary"]`. |

The main-hand weapon is **derived** from `Equipped["Primary"]` by `mainWeapon()`, exactly mirroring how `offWeapon()` derives the off-hand from `Equipped["Secondary"]`. The main-hand is thus an ordinary slot: its stat line flows through `restBase` like any other item, and its weapon damage/delay drives the main-hand auto-attack.

`Set.DPS()` evaluates the full set's modeled DPS by calling `model.TotalDPSDual` with the complete equipped stat totals, the derived main-hand (`mainWeapon()`), and the derived off-hand (`offWeapon()`).

`Set.CandidateDelta(slot, c)` computes the in-context ΔDPS of placing candidate `c` in `slot` against the rest of the fixed set. For **weapon** candidates it passes the candidate's weapon as a `*model.Weapon` override — `newMain` when `slot == "Primary"`, `newOff` when `slot == "Secondary"` (`model.ItemDelta` gains a `newMain` param to mirror its existing `newOff`); for all other slots it passes only the stat block. The same dual-weapon handling applies to `slotDPS`/`ReplaceInstanceDelta` (the per-instance path).

### Slot candidates

`SlotCandidates(items, keep, weapons)` builds the per-slot candidate pools passed to `BuildSet`. It groups all items that pass the `keep` predicate by their Census slot, then overrides the **weapon** slots from the class weapon config (`weapons`, §6): the **main-hand** (`Primary`) pool is every weapon whose `WieldStyle` is in the class's allowed `weapon_wield_styles`, and — when the class dual-wields — the **off-hand** (`Secondary`) pool is the same one-handed weapon set. A unique weapon the player owns only one of cannot fill both hands at once; that is enforced by the no-duplicate rule in the optimizer below, not by any pool-level exclusion. Both weapon pools are sorted by item id for deterministic ranking (§7 Tiered upgrade report).

### Imported loadout

`bis --loadout <file>` (§8) builds a `Set` from an imported loadout file (§4) instead of an empty set, classifying each kept slot:

- **Optimizable** — armor, jewelry, cloak, waist, and **both weapons** (`Primary` main-hand and `Secondary` off-hand). Each is pre-filled with the imported item and eligible for re-optimization against the catalog candidate pool. The imported `Primary` main-hand lands in `Equipped["Primary"]` (so `mainWeapon()` derives the real worn main-hand) and is a swap candidate like any slot — `OptimizableSlot` now includes `Primary`. The off-hand resolves to the `Secondary` slot via the character equipment slot name (`CharSlot`), since a one-handed weapon's census `slot_list` is `Primary|Secondary` and would otherwise mis-resolve to `Primary`.
- **Fixed** — ranged, ammo, charm/activated, event slots. Their stats contribute to the set total like locked items, but they are never swap candidates (no catalog pool, by design — §16). (Adornments are not counted at all — §4, §16.)
- **Skipped** — food, drink, mount slots, and empty sockets contribute nothing.

This generalizes the existing `--lock` concept (lock specific ids) to "lock the whole imported set, re-optimizing only the chosen optimizable slots." The four import use cases are report modes over this `Set`, reusing existing machinery with no new model math: **score current set** (`Set.DPS()` + `DeriveWeights`), **what to upgrade next** (the tiered upgrade report — see below), **seed optimization** (`BuildSet` seeded from the set), and **validate vs parses** (emit absolute predicted DPS).

### Tiered upgrade report

`bis --loadout`'s "what to upgrade next" is an actionable, per-slot table: for each accessibility bucket it shows every optimizable slot's biggest upgrades against the imported set, answering "what to get now" vs "what to watch for in raids" — which the from-scratch tier BiS sets (the ideal end-states) do not.

- **Three buckets** reuse the existing accessibility-tier **filters** (§6), nested as-is: **Get now** (PRE-RAID filter — `LEGENDARY`/`TREASURED`, no avatar/Hunter's/curated), **Raid look-out** (RAID filter — all but avatar), **Best-of-best** (all, incl avatar/`MYTHICAL`). Nesting is intentional: a pre-raid item topping the Raid bucket is a deliberate "keeper" signal — it isn't replaced even by raid gear.
- **One context.** Every candidate in every bucket is evaluated against the imported set in the same (raid) context, so the +ΔDPS figures are comparable across buckets. (This differs from the from-scratch tiers, which sim PRE-RAID in `solo` context — appropriate there, not here.)
- **Per-instance rows.** The report's row unit is a physical **slot instance**, not a Census slot. A two-capacity slot (`Ear`/`Finger`/`Wrist`, per `capacityOf` in `build.go`) emits two rows — one per worn item — each with its own independent upgrade path, so a strong ring and a weak ring in the same slot are evaluated separately rather than collapsed to the weakest. Worn items fill rows in import order; any unfilled position up to the slot's capacity is shown as a synthetic **`Empty`** item with slot-DPS value `0` (surfacing an empty position as an upgrade opportunity).
- **Metric.** Each row's upgrade ΔDPS is the gain of swapping that one worn item (or filling its `Empty`) for the candidate while every other equipped item — **including the slot's other instance** — is held fixed: `Δ = DPS(swapped set) − Set.DPS()`. Positives only; per row keep the best candidate plus one alternative (2nd-best). The same candidate may legitimately top both rows of a two-capacity slot — that is left as-is (the report is advisory; the player buys one).
- **Ordering & coverage.** Rows are ranked across all instances by the **best (primary) candidate's** ΔDPS; the alternative's ΔDPS is displayed but **never** affects ordering. All instance rows are shown by default; `--top N` optionally caps the report to the top N rows. A **single-capacity** slot with no positive upgrade in a bucket is omitted (the report stays actionable). A **multi-capacity** slot (`Ear`/`Finger`/`Wrist`) instead **always emits a row for every worn instance**, even when that instance has no positive upgrade in the bucket — so both rings/ears/wrists are always visible and the two-slot nature is never hidden; the no-upgrade row shows `—` in the upgrade cells (ΔDPS 0, sorted last). Ordering is **deterministic**: equal ΔDPS is tie-broken by slot name then equipped item id, and a slot's candidate ranking (best vs alternative) is tie-broken by candidate item id — so regenerated, committed reports (§8) diff cleanly instead of reshuffling tied rows on every rebuild. The from-scratch `bis-report.md` ranking (`SlotCandidatesScored`) applies the same candidate-id tiebreak. Determinism also requires two upstream guarantees: `LoadScorableItems` loads items `ORDER BY id` (a bare scan reorders after `bis` writes score rows back to the DB, which would shuffle `pickBest`'s near-ties into a different converged set), and `Set.restBase` sums equipped stats in **sorted slot order** (float addition isn't associative, so map-iteration order otherwise wobbles `Set.DPS()`'s low bits and flips near-ties). Together these make a rebuilt report byte-identical run-to-run.
- **Columns.** A four-column markdown table per bucket: `| Slot | Wearing | Best upgrade | Alternative |`. **Wearing** is the worn item (or `Empty`) with its slot-DPS contribution; **Best upgrade** / **Alternative** show `name +ΔDPS (+pct)` where `pct = Δ / Set.DPS()` — the share of **total current-set DPS**, not slot-relative (slot-relative inflates low-impact slots). The best cell is emphasised; the alternative cell is blank when the row has only one positive upgrade, and the best cell shows `—` when a (multi-capacity) instance has no positive upgrade in the bucket. Item rarity tags are not shown.
- **Item links.** Every catalogued item name (in `Wearing` and the upgrade cells) links to its EQ2U page at `https://u.eq2wire.com/item/<id>`, using the Census item id (`ScorableItem.ID`, which is the first number of the in-game gamelink). Items without an id (e.g. `Empty`) render as plain text. The same EQ2U linking replaces the old, browser-useless in-game `\aITEM…\/a` gamelink in the from-scratch `bis-report.md` item lists.

The report also carries the current-set DPS line and the seeded-optimization total (`BuildSet` seeded from the imported set, fixed slots locked).

### Coordinate-ascent optimizer

`BuildSet(profile, lo, bySlot, locked, maxPasses, autoMult, fightLen)` converges a gear set via coordinate ascent:

1. A new `Set` is created with the given profile and loadout.
2. Any `locked` slots are pre-filled and excluded from optimization.
3. The two weapon slots (`Primary`, then `Secondary`) are optimized **first** each pass, before the armor slots — so the set always has weapons present when armor is evaluated, avoiding a skewed first pass with no auto-attack.
4. The remaining (armor/jewelry) slots are sorted alphabetically and iterated each pass.
5. For each slot, `pickBest` (greedy within-slot selection, see below) is called; if the result differs from the current occupant, the slot is updated and `changed` is set. When filling a weapon slot, the item currently in the **other** weapon slot is excluded as a candidate — the player owns one of each weapon, so main-hand and off-hand must be distinct; the coupled two-weapon choice converges through coordinate ascent like every other slot.
6. If no slot changed in a full pass the set has converged and iteration stops. Otherwise a new pass begins.
7. Iteration is capped at `maxBuildPasses = 12` passes (defined in `cmd/bis/main.go`).

**`pickBest`** greedily fills a slot up to its capacity by evaluating full-set DPS for each unused candidate in the current-set context and picking the best at each fill step. This means the second item in a two-capacity slot is evaluated with the first already in place, so within-slot stat interactions (e.g., double-ear crit stacking) are respected. After returning, the slot's original contents are restored via `defer` — callers see only the returned slice, not a side-effected set.

**Slot capacities** — `slotCapacity` in `build.go` defines which slots hold two items:

| Slot | Capacity |
|---|---|
| `Ear` | 2 |
| `Finger` | 2 |
| `Wrist` | 2 |
| `Charm` | 2 |

All other slots hold 1. `capacityOf(slot)` returns the capacity (defaulting to 1 for unlisted slots).

### Stat weights

`model.DeriveWeights(base, dps)` returns the marginal DPS per +1 unit of each stat in `WeightStats` at the given `base` stat block. The `dps` function is provided by the caller (bound to the current weapons + arts), enabling per-tier, per-set re-derivation.

**Curve stats** (`haste`, `dpsmod`, `multiattack`, `mainstat`) use a bracket-slope method rather than a simple +1 forward difference, because the in-game stat-to-effect conversion floors the effect to an integer and a naive +1 diff reads noisy. For `haste` and `dpsmod`, the bracket endpoints are the stat values that produce consecutive whole-percent effects on the fitted quadratic curve (see §11). For `multiattack` and `mainstat`, the bracket comes from the sample tables in `curve.go`. The bracket slope is then `(dps(hi) − dps(lo)) / (hi − lo)`. A stat already at its cap yields ~0 by construction.

**Linear stats** (`critchance`, `potency`, `reuse`, `flurry`, `abilitymod`, `castspeed`) use a plain +1 forward difference: `(dps(base + 1) − dps(base)) / 1`.

`WeightStats` is the fixed ordered slice that defines which stats receive weights; `potencybonus` and `recoveryspeed` are excluded (they are applied in the model but not independently weighted in the report).

Weights are outputs derived from the full model at the converged set — no stat is pre-judged. A stat that is already capped or that has no live interaction with the current loadout automatically yields a near-zero weight.

### Scoring and ranking

`model.ScoreItem(weights, item)` returns `Σ(weight × itemStat)` over `WeightStats` plus a per-stat `ScoreTerm` breakdown (nonzero stats only, sorted by contribution descending). This produces the explainable breakdown shown in the report but is **not** used for set selection — `BuildSet` and `CandidateDelta` use `ItemDelta` (full before/after `TotalDPSDual`) for all optimization decisions.

`model.ItemDelta(restBase, main, restOff, arts, itemStats, newOff, classAutoMult, fightLen)` is the selection primitive: it diffs two full `TotalDPSDual` calls — one without the item, one with it added to `restBase`. Because it uses the live full-model evaluation, a stat already at its cap in `restBase` contributes ~0 and multiplicative auto-attack clusters (haste × MA × crit × flurry × dps-mod) make a stat worth more when its partners are already accumulated.

`ConvergedWeights(set)` derives weights at the converged full-set baseline (with the converged off-hand weapon) for use in the report's explainable breakdowns.

`BuildSlotReports` produces one `SlotReport` per slot, each carrying the converged pick (`Chosen`) and the top-N ranked alternatives (`Ranked`) by in-context ΔDPS via `SlotCandidatesScored`. The Primary main-hand is a ranked slot like any other.

## 8. Commands & Operations

### Command reference

| Command | Purpose | Flags (default) |
|---|---|---|
| `itemdex` | Pull EoF items from Census and write CSV catalog. Serves from cache if CSVs exist and `--refresh` is false. | `--out data` (CSV output dir / cache), `--refresh false` (force fresh Census pull), `--sid s:example` (Census service ID), `--page 1000` (items per Census request) |
| `builddb` | Build the SQLite database from the CSV cache. Reads gear from CSVs, pulls Assassin Expert CAs from Census, and writes `bis.db`. | `--data data` (CSV catalog dir), `--db bis.db` (output DB path), `--sid s:example` (Census service ID) |
| `bis` | Run the BiS optimizer and write the markdown report into the character's directory (§6). From-scratch mode writes `bis-report.md`; `--loadout` mode writes `upgrade-report.md` (the tiered upgrade report). | `--db bis.db` (scored DB), `--character characters/biffels/config.toml` (character config; its directory is the per-character output dir), `--out ""` (override report path/name; default = `<config-dir>/bis-report.md`, or `<loadout-dir>/upgrade-report.md` in `--loadout` mode), `--lock ""` (item IDs to lock for a re-model run), `--loadout ""` (imported loadout file to sim from — §7), `--top` (in `--loadout` mode: cap on instance rows per bucket, default **all**; in from-scratch mode: alternatives per slot, default 3), `--fight` (fight seconds; default `constants.FightDurationSecs`) |
| `itemdex import` | Pull one character's live equipped loadout from Census and write `loadout.toml` into the config's directory (§6). Fetches any uncataloged items. | `--character characters/biffels/config.toml` (config supplying `census_name`/`world`; its directory receives `loadout.toml`), `--out data` (catalog dir for appended items), `--sid s:example` (Census service ID) |
| `weights` | Print marginal DPS weights per stat at the current loadout for each context. Diagnostic tool; does not write any files. | `--db bis.db`, `--character characters/biffels/config.toml`, `--fight` (fight duration; defaults to `constants.FightDurationSecs`) |
| `fitcurve` | Fit the haste/DPS-mod quadratic curve from the readings CSV and print paste-ready constants for `internal/model/curve.go`. | `--readings data/curve-readings.csv` |

**Committed per-character artifacts.** Each character's generated `loadout.toml`, `upgrade-report.md`, and `bis-report.md` are committed to the repo (not gitignored), so GitHub renders the reports with their EQ2U item links. Re-running the commands regenerates them in place for review in the diff.

### Operational loops

**Data refresh loop** — run when item data may be stale (new Census data, post-patch):

1. `go run ./cmd/itemdex [--refresh]` — pulls EoF items from Census and writes (or refreshes) the CSV catalog under `data/`. Omit `--refresh` to serve from the existing cache.
2. `go run ./cmd/builddb` — ingests the CSV catalog + fresh Census CA pull and writes `bis.db`.
3. `go run ./cmd/bis` — runs the optimizer and writes `bis-report.md` into the character's directory (§6). Optionally run `go run ./cmd/weights` first to inspect stat weights.

**Gear-import loop** — run to sim from the live equipped set:

1. Set `census_name` and `world` in the character config (§6).
2. `go run ./cmd/itemdex import` — writes `loadout.toml` into the config's directory (`characters/<census_name>/loadout.toml`); appends any newly-fetched items to the CSVs and prints a `builddb` reminder if it did.
3. `go run ./cmd/builddb` — only if step 2 fetched new items.
4. `go run ./cmd/bis --loadout characters/<census_name>/loadout.toml` — sims from the real set; writes `upgrade-report.md` into that same directory.

**Curve re-fit loop** — run after collecting new haste/DPS-mod or main-stat tooltip readings:

1. Append new rows to `data/curve-readings.csv` (for haste/DPS-mod) or `data/mainstat-readings.csv` (for main-stat).
2. `go run ./cmd/fitcurve` — fits the quadratic and prints `HasteDpsModA` / `HasteDpsModB` constants.
3. Paste the printed constants into `internal/model/curve.go`. For main-stat readings also sync the `mainStatSamples` table in the same file.
4. Run `go test ./...` — `TestFittedConstantsMatchReadings` and `TestMainStatSamplesMatchReadings` both fail until the constants are updated, confirming the change is complete.

## 9. Testing & Calibration-Sync

### Test strategy

Tests are pure unit tests (no network, no on-disk DB required) and are organized close to the code they exercise, following Go conventions. Parameterized coverage uses table-driven tests. The test suite is fast enough to run on every change. No file with `"Harness"` in its name is ever run during normal testing — harness files hit real databases or live APIs and are potentially destructive; only mocked unit tests are run.

Tests span all internal packages: `internal/bis`, `internal/catalog`, `internal/census`, `internal/charconfig`, `internal/classify`, `internal/constants`, `internal/extract`, `internal/fit`, `internal/model`, `internal/source`, `internal/spell`, and `internal/store`. The `cmd/` packages have no test files.

### Code-data sync tests (calibration guardrail)

Two tests in `internal/fit` act as a code↔data guardrail: they fail the build whenever a curve CSV is updated without the corresponding code constant being re-fitted, preventing silent divergence between the committed readings and the live model.

**`TestFittedConstantsMatchReadings`** (`internal/fit/sync_test.go`) — loads `data/curve-readings.csv`, fits the joint quadratic `f(s) = A·s − B·s²` via `FitQuad`, and asserts that `model.HasteDpsModA` and `model.HasteDpsModB` match to the tolerances `1e-6` and `1e-8` respectively. Editing the CSV without re-fitting and updating `internal/model/curve.go` causes this test to fail.

**`TestMainStatSamplesMatchReadings`** (`internal/fit/mainstat_sync_test.go`) — loads `data/mainstat-readings.csv` and asserts that `model.MainStatEffect(r.Raw)` matches `r.Effect` to `1e-9` for every reading row. The readings must have `stat = "agi"`. Editing the CSV without syncing the `mainStatSamples` table in `internal/model/curve.go` (or vice versa) causes this test to fail.

### Calibration tests in `internal/model`

Several tests in `internal/model` pin the DPS model against live measured values:

- **`TestAutoWeaponMultiplierCalibration`** (`dps_test.go`) — pins `AutoWeaponMultiplier` against two `/weaponstat` readings taken 2026-06-13 at known stat combinations, confirming the auto-attack formula is correct end-to-end.
- **`TestGushingWoundCalibration`** and **`TestDeathMarkCalibration`** (`rotation_test.go`) — pin DoT and proc component outputs against measured in-game values.

---

# Part II — The Game (mechanics, calibrated)

## 10. The DPS Model

`internal/model/dps.go` composes total damage per second from two parallel timelines — auto-attack swings and the combat-art rotation. The two do **not** displace each other: casting a combat art does not consume an auto-attack swing, so the model sums them rather than interleaving:

```
TotalDPS     = classAutoMult · AutoDPS(sb, w)            + CADPS(sb, cas, fightLen)
TotalDPSDual = AutoDPSDual(sb, main, off, classAutoMult) + CADPS(sb, cas, fightLen)
```

`TotalDPS` (single weapon) and `TotalDPSDual` (dual-wield) differ only in the auto term. `TotalDPSDual` is the production path for the EoF Assassin, who always dual-wields; it routes through `AutoDPSDual`, which applies the ×1.33 off-hand delay penalty to both weapons when a real off-hand weapon is present (§12). `classAutoMult` is the class-intrinsic auto-attack multiplier sourced from `classes/<class>.toml` (Assassin = 2.0, §6); `AutoDPS` deliberately does **not** apply it, so callers must.

`AutoDPS(sb, w)` is sustained per-weapon swing DPS — weapon damage over haste-adjusted delay, scaled by the multi-attack, AGI, dps-mod, crit, and flurry factors. Its exact multiplier stack is the authoritative §12; the per-stat factor functions are §11. This section is the top-level overview; §11–§14 carry the detail.

`CADPS(sb, cas, fightLen)` is the fight-length-smoothed combat-art DPS. A single fixed fight length quantizes the last cast of a long-cooldown art, so `CADPS` averages `cumCA(t)/t` over K samples (`fightSmoothingSamples = 9`) spanning a window of width R = the longest effective recast, centered on `fightLen`. The rotation timeline and per-cast damage are detailed in §13–§14.

## 11. Stat-Conversion Mechanics

This is the single authoritative statement of how each combat stat converts to a damage or timing effect. Every other section references this block rather than restating it. Each formula matches its cited code symbol exactly; constants live in `internal/constants/constants.go` and `internal/model/curve.go`.

### Crit

A critical hit re-rolls as the **higher of** the range ceiling and a flat multiple of the roll (measured 2026-06-19, naked/controlled tooltip + combat-log reads):

```
crit = max( rangeMax + 1 , (1.50 + critBonus) · roll )
```

applied to the **final** per-hit damage — after potency × AGI **and** ability-mod (ability-mod rides the crit). `critBonus` is structurally present in the formula but is inert on this server (vestigial field; see §16.5). The model computes the **expected** crit multiplier per source from its final range `[lo,hi]` (`critFactor`, `dps.go`), assuming uniform rolls:

- range ratio ≤ 1.5:1 (single-valued, narrow CAs) → the `1.50×` branch always wins → **×1.50**
- 1.667:1 (the typical CA range) → the floor grazes the low rolls → **~×1.51**
- wide weapon ranges (auto-attack) → the floor lifts most low rolls → **>×1.50** (Modinthalis 139–775 → ~×1.87)

Combat arts apply it per component on each component's ability-mod-inclusive scaled range (§13); auto-attack applies it on the weapon's damage range (`Weapon.MinDamage/MaxDamage`). Crit chance is clamped at 100% (a crit can't happen more than every hit).

**Provenance:** flat ×1.30 disproven; range-shift-only disproven (Hilt Strike crits exceeded its ceiling); the `max(...)` hybrid confirmed by the auto-attack floor pile-up. Validated on ~230 auto swings (`data/autoattacktest.txt` + `autoattacktest2.txt`; 194 non-crit + 35 crit): rolls are **uniform** (χ²=5.86 over the 194 non-crit hits, well under the 15.5 rejection threshold), the auto average equals the range midpoint (451 ≈ 457), 23/35 crits land exactly on the `max+1` floor (≈ uniform's expected 59%), and **critMult = 1.84 measured vs 1.87 modeled** (~1.7%). Uniform rolls — the model's lone assumption — is confirmed; it applies identically to every component (DirectHit, DoT, Termination, TriggerProc) and to auto-attack.

### Flurry

```
flurryFactor = 1 + (flurry/100) · (FlurryMultiplier − 1)  FlurryMultiplier = 4.0
```

`flurryFactor` (`dps.go`) is gear flurry only — haste overcap no longer converts to flurry. A flurry proc deals +100%–500% extra (2×–6×); `FlurryMultiplier = 4.0` is the mean used for expected DPS (`constants.go`).

### Haste / DPS-mod

Haste and DPS-mod share one fitted curve, `f(s) = A·s − B·s²` (`HasteDpsModEffect`, `curve.go`):

```
A = HasteDpsModA = 0.800348        B = HasteDpsModB = 0.00127275
```

The result is floored to a whole percent (UI behavior). Both stats hard-cap at 300 (`HasteStatCap` / `DPSModCap`, `constants.go`); the curve is clamped to the cap before flooring, giving `f(300) ≈ 125.56` → displays 125%. Haste divides weapon delay (`effDelay`); DPS-mod multiplies per-swing damage (`dpsModFactor`).

Provenance: a 2026-06 joint floor-aware quadratic refit over the 20 readings in `data/curve-readings.csv` (RMS 0.29 vs 1.93 for the logarithmic alternative, which it replaced; haste-only and dps-mod-only fits agreed within flooring noise, confirming the shared curve). `internal/fit`'s `TestFittedConstantsMatchReadings` pins the constants to the CSV (§9).

**Non-stacking "Haste" item effect (max-wins).** An *item effect* here means an entry in an item's `effect_list` (the indented "When Equipped:" lines), as distinct from the item's `modifiers` block. Beyond the haste *stat* (AA + `modifiers`-block haste, which stacks additively in `StatBlock.Haste`), some items carry a named **"Haste" item effect** — an `effect_list` "When Equipped: Increases Haste of caster by N" line. In game this is one discrete, named effect that takes the **highest** value equipped, not the sum — two such items (e.g. Cloak of Flames +25 and a +21 glove) grant only the max. The model captures this in a separate field `StatBlock.HasteEffect`: effect-sourced haste routes there (modifier-sourced haste stays in `Haste`), and `StatBlock.Add` **maxes** `HasteEffect` across combined blocks while summing every other field. Effective haste — used by `effDelay` and the `DeriveWeights` haste-curve bracket — is `Haste + HasteEffect`. Because all set aggregation and per-item ΔDPS flow through `Add`, the non-stacking is context-correct everywhere: a redundant second effect-haste item contributes no haste (its ΔDPS reflects only its other stats), and removing the dominant haste item makes the other's ΔDPS rise. Source routing happens at every `StatBlock` construction point that knows the source — `store.LoadScorableItems` (the `item_stats.source` column) and the import path (`loadout.ItemStatBlock`). **Scope:** only the "Haste" effect is modeled as non-stacking; all other `effect_list` stat grants still sum (the general mixed-stacking case is deferred — §16.1).

### Multi-attack

`MultiAttackEffect` (`curve.go`) interpolates a piecewise-linear sample table (`multiAttackSamples`), anchored at (0, 0) and floored to a whole percent. The table runs to 3400 → 200% with no hard cap; effect above 100% is triple-attack. The report brackets this table with a slope method rather than a +1 difference to avoid flooring noise (§7).

### Main stat (AGI)

`MainStatEffect` (`curve.go`) interpolates the piecewise-linear `mainStatSamples` table, **unfloored** (AGI tooltips display two decimals), clamped to the top sample above 1661. Three regimes:

- **Climb to the cap** — rises to 65% at 1100 (decelerating, slope ~0.008 near 1100).
- **Deadzone ~1100–1200** — flat at 65% (1109 read 65%; the {1200, 65} sample marks the plateau end).
- **Second regime >1200** — climbs again at ~0.027–0.031 %/AGI (1294 → 69.45 … 1661 → 79.54), ~3–4× the cap-approach slope.

Provenance: live tooltip readings measured 2026-06-16, which disproved an earlier "hard cap at 1100" model. `data/mainstat-readings.csv` spans `73 → 6.08` (first) to `1661 → 79.54` (last); raid AGI reaches ~1800, so a >1661 reading is still needed to anchor that range (§16). `internal/fit`'s `TestMainStatSamplesMatchReadings` pins the table to the CSV (§9). AGI from gear arrives via the `strength` modifier key — see §4 for the all-primary-attribute translation note.

### Potency pool

```
potPool = 1 + (Potency + PotencyBonus + PotencyAdd) / 100        (rotation.go, CAEffectiveDamage)
```

Within the pool the three terms **add**; the pool then multiplies a combat art's component bases. The AGI curve multiplies separately (`scaling = potPool · mainStat`), so potency and main stat compound. `Potency` is the displayed character potency, `PotencyAdd` is the art's AA potency rider (§6), and `PotencyBonus` is an empirically captured hidden pool adjustment applied by the TLE server (≈ 24.6 in `characters/biffels/config.toml`). Whether the pool multiplies in-component bases or the final cast total is the open question in §16.

### Ability-mod

Ability-mod placement is per-mechanic, applied inside `CAEffectiveDamage` (`rotation.go`): a DirectHit takes the ability mod **in full**; a TriggerProc takes **half** the ability mod per trigger; DoT ticks and on-Termination detonates take **none**. Arts with no parsed components fall back to a single damage line that takes the full ability mod. This is summarized here; §13 carries the per-component detail. The old 50% ability-mod cap is disproven.

### Reuse

```
effRecast = max(0.5 · base, base · (1 − RecastReduction) / (1 + Reuse/100))   (rotation.go)
RecastReductionCeiling = 0.50
```

Reuse is a **divisor** (like haste), applied after the per-art AA recast reduction (a multiplier), and floored at 50% of the art's base recast (`RecastReductionCeiling`). The divisor reaches that floor at 100% reuse; an AA-halved art (Assassinate 300→150, Mortal Blade 180→90) already sits on the floor, so reuse cannot reduce it further. Provenance: recalibrated 2026-06-18 from six Eviscerate readings (to 61.8%), which disproved the old subtractive 1%-per-point / 50-stat-cap model.

### Cast speed

```
effCast = baseCast / (1 + castSpeed/100)        (slotSecs, rotation.go)
```

Cast speed is a divisor on the art's base cast time (measured: Head Shot 2.0s → 1.46s @ 37.4%). It is also a gear stat, mapped from the `spelltimecastpct` Census key (§4). The cap is unknown (§16).

### Recovery speed

```
effRecovery = CARecoveryBaseSecs · (1 − min(recovery, 100)/100)        (slotSecs, rotation.go)
CARecoveryBaseSecs = 0.5
```

Recovery speed subtractively shrinks the 0.5s server base post-cast recovery; at 100% the recovery is zero ("Recovery: Instant"). It is config-only — there is no gear modifier key for it.

## 12. Auto-Attack Model

`internal/model/dps.go` models auto-attack as sustained per-weapon swing DPS. The per-swing damage of one weapon is its census base average lifted by the wielder's stats:

```
perSwing = weaponAvg · (1 + MainStatEffect(AGI)/100) · dpsModFactor · classAutoMult     (autoDamageMult × classAutoMult)
```

`weaponAvg` is the weapon's census `(min+max)/2` base; `autoDamageMult = (1 + MainStatEffect(AGI)/100) · dpsModFactor` (`autoDamageMult`). **AGI scales auto-attack on the same main-stat curve as combat arts** — `MainStatEffect` is the §11 curve, shared verbatim. **Potency does *not* scale auto-attack** — the auto term carries only AGI and dps-mod; folding potency into the per-swing damage was tried and drifts the `/weaponstat` residual, so it is deliberately excluded. The weapon's full `MinDamage`/`MaxDamage` range (`Weapon.MinDamage/MaxDamage`, loaded from the `weapon_min_dmg/weapon_max_dmg` catalog columns) feeds the range-shift crit model (§11); for auto-attack the wide weapon range means the `1.50×` floor lifts most low rolls, driving the effective auto crit multiplier well above ×1.50.

`classAutoMult` is the class-intrinsic auto multiplier (Assassin = 2.0, from `classes/<class>.toml`, §6). It is applied **at the `AutoDPSDual` / `TotalDPS` boundary, not as a `StatBlock` field**: a multiplier carried on the stat block would zero auto-attack at the block's zero value (the StatBlock zero value is a valid "no-gear" baseline). `AutoDPS(sb, w)` therefore does **not** apply it and callers must — `AutoDPSDual`, `TotalDPS`, and `TotalDPSDual` all multiply it in. `AutoWeaponMultiplier(sb, classAutoMult) = autoDamageMult · classAutoMult` is the full census-base-to-actual multiplier, used as the `/weaponstat` verification anchor (`TestAutoWeaponMultiplierCalibration`), not a production path.

Full sustained single-weapon DPS (`AutoDPS`):

```
AutoDPS = (weaponAvg / effDelay) · (1 + MultiAttackEffect(MA)/100) · autoDamageMult · critFactor · flurryFactor
effDelay = w.DelaySecs / (1 + HasteDpsModEffect(haste)/100)
```

A zero/negative weapon delay yields 0 (no weapon equipped). The MA / crit / flurry / haste / dps-mod factors are the §11 conversions.

**Dual-wield** (`AutoDPSDual`) sums both weapons and applies the off-hand delay penalty:

```
if off.DelaySecs > 0:  main.DelaySecs ·= DualWieldDelayPenalty;  off.DelaySecs ·= DualWieldDelayPenalty
AutoDPSDual = classAutoMult · (AutoDPS(sb, main) + AutoDPS(sb, off))
```

`DualWieldDelayPenalty = 1.33` (`constants.go`) multiplies **both** weapons' delays, on top of and independent of haste (measured 1.32–1.34 across two haste levels, 2026-06-13). The penalty is **detected, not assumed**: it fires only when a real off-hand weapon is present (`off.DelaySecs > 0`), so an empty off-hand or a non-weapon off-hand (shield/symbol) is correctly unpenalized — important for imported loadouts that may not be dual-wielding. Main and off are otherwise treated identically; the off-hand's weapon-multiplier-stat penalty is not tracked and nets out for relative comparison.

### `/weaponstat` decomposition (provenance)

The auto-attack multiplier stack is anchored to in-game `/weaponstat` readings, which decompose into three measured quantities at a fixed gear state:

- **census-raw** — the weapon's catalog base damage (the `weaponAvg` input).
- **"base"** — census-raw with the dps-mod factor applied (the dps-mod-scaled per-swing damage the game reports as the weapon's base after stats).
- **"actual"** — the fully-buffed per-swing damage including the AGI curve and the class auto multiplier.

The residual between the AGI-and-dps-mod-scaled value and "actual" resolves to the **×2.0 class auto multiplier**, which held constant across the measured gear states — confirming it is class-intrinsic, not a stat-derived factor, and justifying its placement outside `autoDamageMult` and `StatBlock`.

## 13. Combat-Art Damage & Components

`CAEffectiveDamage` (`internal/model/rotation.go`) is one cast's total damage. Every component's base is scaled by `scaling = potPool · mainStat`, where the potency pool and the main-stat curve are both defined in §11. Crit multiplies the cast **total** (`· critFactor`), not each component.

### Legacy single-line path

An art with no parsed components (`len(ca.Components) == 0`) uses one damage line that takes the **full** ability mod:

```
avgBase = (MinDamage + MaxDamage)/2 · scaling
cast    = (avgBase + AbilityMod) · critFactor
```

This keeps pre-component callers and tests unchanged.

### Component path

An art with parsed components sums per component, with ability-mod placement **per mechanic**. For each component, `base = (MinDamage + MaxDamage)/2 · scaling`:

| `ComponentKind` | Contribution |
|---|---|
| `DirectHit` | `base + AbilityMod` (full ability mod) |
| `DoT` | `base · dotTicks(c, window)` (no ability mod) |
| `Termination` | `base` **only if the art is held** (detonate lands); else 0 |
| `TriggerProc` | `(base + 0.5·AbilityMod) · Triggers` (half ability mod per trigger) |
| `RateProc` | not scored (parsed-but-deferred) |

```
cast = Σ component_contribution · critFactor
```

`ComponentKind` (`internal/spell/component.go`) enumerates the five kinds (`DirectHit`, `DoT`, `Termination`, `TriggerProc`, `RateProc`). `Component` carries the kind plus per-kind fields: `MinDamage`/`MaxDamage`, `IntervalSecs` and `HasInstant` (DoT), `TriggeredSpell`/`Triggers` (Termination/proc), `PerMinute` (RateProc), `DamageType`, and `AoE`.

**DoT tick count** is `dotTicks(c, window)`:

```
dotTicks = floor(window / IntervalSecs) + (HasInstant ? 1 : 0)        (0 if IntervalSecs ≤ 0)
```

i.e. one application per completed interval, plus one instant tick if the line reads "instantly and every" rather than bare "every". The `window` is set by the hold/clip rule (§14): a termination art is **held** to its full `DurationSecs` (all ticks fire and the detonate lands); every other DoT is **clipped** to `min(effRecast, DurationSecs)` (only ticks inside the recast window count; no detonate).

### Per-mechanic ability-mod rule (provenance)

Ability-mod placement is calibrated, not assumed:

- **DirectHit takes ability mod in full** — Gushing Wound's DirectHit absorbs the full 694 ability mod measured on a single hit.
- **TriggerProc takes half the ability mod per trigger** — Death Mark, a 5-trigger proc, scales ≈ 2.4× across ability mod 0 / 376 / 694, matching `(base + 0.5·abmod)·5` rather than full or zero abmod per trigger.
- **DoT ticks and on-Termination detonates take none.**
- **Beneficial abilities are excluded** from scoring entirely (filtered upstream).

The old flat 50%-of-base ability-mod *cap* is disproven; the rule is per-mechanic, not a global cap.

### Base sources (provenance)

Component bases come from the **highest-rank census damage lines** for the sim. **Naked-recovered bases** (read from tooltips with all gear removed) are the calibration ground truth that the census bases are reconciled against; the two agree to ~7% on the piercing / detonate lines, a residual tracked as a known reconcile note (§16). `RateProc` is **parsed but deferred** — proc-rate scoring is not modeled, so RateProc components contribute nothing to a cast's damage.

## 14. Rotation Model

`rotationTimeline` (`internal/model/rotation.go`) is a priority simulation. Each timeline slot it fires the off-cooldown art with the highest **damage-per-slot-second** `CAEffectiveDamage(sb, ca) / slotSecs(sb, ca)`; ties resolve to the first such art. When no art is ready it **idle-jumps** the clock to the soonest art's availability rather than advancing by a fixed step. The sim is **prefix-consistent**: a fight of length `s` credits exactly the casts whose start time is `< s` (`cumCAAt`), so one pass to the window top yields `cumCA(t)` for every `t`.

### Clip vs hold

Whether a DoT art is clipped on cooldown or held to its full duration is decided by `hasTermination(ca)` (true iff the art carries a `Termination` component). The scheduling interval is `artCadence`:

```
artCadence = hasTermination ? max(effRecast, DurationSecs) : effRecast
```

A **termination art is held**: its cadence is `max(effRecast, DurationSecs)`, so the DoT runs to its full duration and the on-termination detonate fires (a long cooldown still gates re-cast). **Every other art clips on `effRecast`** — it re-casts as soon as cooldown allows, and its DoT (if any) only counts ticks inside that window with no detonate (§13). `effRecast` is the §11 reuse divisor floored at 50% of base.

### Fight-length smoothing

`CADPS` (`internal/model/dps.go`) averages CA DPS across a window of fight lengths to remove last-cast quantization. A single fixed fight length quantizes the final cast of a long-cooldown art (whether it just fits or just misses swings the result), so:

```
R   = maxEffRecast(sb, cas)                       (longest artCadence in the set)
lo  = max(fightLen − R/2, 1.0),  hi = fightLen + R/2
CADPS = mean over fightSmoothingSamples samples of cumCA(s)/s,  s evenly spaced in [lo, hi]
```

`fightSmoothingSamples = 9` (`rotation.go`); the window width `R` is one full big-cast boundary cycle. A short-recast-only art set has `R ≈ 0` and is effectively unsmoothed (`CADPS` falls back to the single-length value when `hi ≤ lo`). The default fight length is `FightDurationSecs = 600.0` (`constants.go`) — a 10-minute fight, long-fight-aware yet short enough that one extra big nuke still matters. `RotationCADPS` is the single-fixed-length (unsmoothed) variant, retained for direct tests.

### Per-slot timing

Each cast occupies `slotSecs(sb, ca) = effCast + effRecovery`, the §11 cast-speed and recovery-speed conversions of the art's base cast time and the 0.5s server recovery. `maxEffRecast` scans `artCadence` over the art set for the smoothing width.

### Structural idle and stealth

On long fights the priority sim leaves **~45–50% of the timeline idle** — the high-value arts are on cooldown and nothing else is worth pressing. This is by design: auto-attack runs in parallel (§10) and fills that idle time, so CA idle is not lost DPS. Stealth-opener arts are **assumed free** (the stealth setup cost is not modeled). The priority-by-rate heuristic plus the clip/hold cadence can leave the smoothed `CADPS` slightly non-monotonic in a stat across gear states; that residual is tracked in §16.

## 15. Constants Block

This table mirrors `internal/constants/constants.go`. **The code is authoritative** — every value below is copied from the file; where they ever disagree, the file wins.

| Constant | Value | Provenance |
|---|---|---|
| `CritMultiplier` | `1.50` | Base crit factor (the `1.50×` branch of the range-shift floor model; measured 2026-06-19, §11). Not a flat expected multiplier. |
| `FlurryMultiplier` | `4.0` | A flurry proc does +100%–500% (2×–6×); 4× is the mean used for expected DPS. |
| `HasteStatCap` | `300.0` | Haste stat hard cap; fitted curve gives f(300) ≈ 125.56 → 125%; overcap wasted. |
| `DPSModCap` | `300.0` | Dps-mod stat hard cap; shares the haste curve and cap. |
| `RecastReductionCeiling` | `0.50` | Recast floor = 50% of base; reuse divisor reaches it at 100% reuse (recalibrated 2026-06-18, six Eviscerate reads). |
| `CARecoveryBaseSecs` | `0.5` | Server base post-cast recovery, reduced subtractively by recovery-speed stat (100 → "Recovery: Instant"). |
| `DualWieldDelayPenalty` | `1.33` | Off-hand multiplies each weapon's auto delay ×1.33 (measured 1.32–1.34 across two haste levels). |
| `FightDurationSecs` | `600.0` | 10-minute default fight (long-fight-aware; short enough that one extra big nuke matters). |
| `CACastTimeSecs` | `0.5` | Combat arts share ~0.5s base cast time. |

## 16. Open Items & Known Divergences

This section is the forward worklist. Items are grouped by kind; each gives the divergence or gap and the concrete next step.

### 16.1 Known Code Divergences

**Reconcile residual on Gushing Wound piercing / detonate bases.** Naked-recovered tooltip bases and the highest-rank census damage lines agree to ~7–11% on the piercing DoT and detonate components. Both directions of read noise are plausible (tooltip noise from hidden buff sources; census inconsistency at tier boundaries). The current code uses census bases; the discrepancy is tracked but not acted on. Status: accepted; re-examine if the crit fix or a fresh naked read changes the picture.

**Non-stacking item effects — Haste handled; general case deferred.** Items grant stats via the `modifiers` block (stacks) and via *item effects* — `effect_list` "Increases \<stat\> of caster" lines (source `effect`). The named **"Haste"** item effect is *max-wins* (highest single value equipped, not the sum) and is **now modeled** via `StatBlock.HasteEffect` (max-combined in `Add`; effective haste = `Haste + HasteEffect` — see §11 "Non-stacking 'Haste' item effect"). This resolves the double-count that made the gear-import report recommend a second haste item effect that doesn't stack with the equipped cloak. **Still deferred:** whether the *other* stat-granting item effects (DPS, Multi-Attack, Crit Chance) are also non-stacking is unverified — the player only has Haste item effects in current-era EoF, so the rest were deliberately not modeled from memory — and later expansions add item effects with mixed stacking rules. Census exposes no stacking metadata (only the description text), so any extension needs in-game confirmation per effect (double up an item, read max vs sum) then either widening the max-wins rule or adding a per-effect annotation. Route through brainstorm when a non-Haste case becomes verifiable.

### 16.2 Coverage Gaps

**Missing CAs — Bladed Opening and Point Blank Shot.** Bladed Opening (~1.8% of raid damage) and Point Blank Shot (~0.3%) are absent from the art pool. Both appear in the live EQ2 census as level 100–110 abilities (post-EoF redesigns), so the `classes.assassin & level<71` pull in `internal/spell/pull.go` legitimately skips them — even though both are real arts in the TLE/EoF kit. Next step: recover their EoF L70 bases manually via tooltip reads and add them through the `ManualArts` path (`internal/spell/manual.go`), the same route used for Hilt Strike and Strike of Consistency (§9). Also worth a broader audit of whether any currently-pooled arts carry wrong live-census damage numbers due to the same live-vs-TLE drift.

**Unmodeled procs and poisons (~7% of raid damage).** Caustic Poison (3.1%), Vampiric Requiem (2.8%), Greater Rune of Blasting (0.5%), Incinerate Blood (0.5%), Shock (0.3%), and similar sources form a flat, non-critting damage class the sim does not model. Because these are not gear-driven they have little effect on stat-weight *ordering* — but they pull absolute DPS down by ~7% versus a parse. Next step: decide whether a proc/poison scoring layer is worth adding (likely only valuable once absolute calibration is needed, not for relative BiS ranking).

**Gear procs captured, not scored.** Triggered gear effects (from `effect_list`) are now cataloged in `item_procs` (`data/item-procs.csv`) with rate, damage range, damage type, and raw trigger text. Scoring them (rate × expected damage, 0%-crit class) is deferred — procs do not affect current BiS rankings until a scoring layer is added.

**Buff procs (stat-granting) unscored — LIKELY NEXT BUILD.** Some procs cast a *self-buff* that grants stats rather than dealing damage — e.g. Adrenaline Mitts' **Adrenaline Rush**: "may cast … Lasts 10.0s … ~2.0/min" with child lines `Increases DPS of caster by 25.0` / `Increases Haste of caster by 25.0`. Two gaps: (1) **parser** — `catalog.ParseEffects` only reads indentation-1 lines, so the buff's indentation-2 stat grants aren't even captured (only the trigger text + dmg 0–0 land in `item_procs`); (2) **scoring** — procs aren't valued. Impact is large and can flip a recommendation: measured 2026-06-25, Adrenaline Rush is worth **~+70 avg DPS** on Biffels' set (+210 while active × 33% uptime), so the gear-import report wrongly flagged a Hands swap as +42 when it's actually ≈ −28 (Adrenaline Mitts is a keeper). **Proposed approach (user, easy):** parse the buff's stat grants + rate/duration, compute `uptime = min(1, rate × duration / 60)` (here 2×10/60 = 0.33), and credit the item the uptime-weighted buff stats (e.g. +25 haste → +8.3 effective always-on haste folded into the StatBlock). Damage procs can fold into the same scoring layer later. Route through brainstorm when picked up.

**Adornments not modeled (gear-import scope).** Character import (§4, §7) does **not** count adornment stats at all: adornments are assumed roughly constant across the items competing for a slot, so they cancel in relative ΔDPS and would only add an unfair equipped-vs-candidate asymmetry (the equipped item is adorned; catalog candidates are not). There is no adornment catalog or fetch. Revisit only if genuinely slot-specific or build-defining adornments emerge; that would pair with the crit-adornment question in §16.3.

**Food/drink slots excluded from import.** Import skips food and drink slots — their stats are not counted, since food/drink change ad hoc and shouldn't perturb the modeled set. Revisit if a future expansion makes consumable stats stable and BiS-relevant.

**Combat-skill and single-attribute effects captured, not scored.** Static `effect_list` grants whose Census key does not appear in `modifierToField` (e.g. combat-skill lines, explicit single-attribute grants like `agility`) are captured under source `effect` but silently skipped when building a `StatBlock`. The model holds attack rating constant and scores only the keys `modifierToField` maps; deciding whether and how to score combat skills or individual attributes is a separate modeling decision.

**`potencyBonus` placement.** Both spec and code (`internal/model/dps.go`) add `PotencyBonus` into the potency pool alongside displayed `Potency` and per-art AA riders. An alternative interpretation is that it is a final after-everything multiplier (equivalent if no cross-terms exist, non-equivalent if it does). The two forms are distinguishable with reads at two different potency levels — the cross-term `potency × potencyBonus/100` only appears in the additive-pool version. Next step: run two reads (one low-potency, one high-potency) and check which form fits.

### 16.3 Modeling Decisions

**Crit-adornment question.** Crit chance's derived weight rises under the hybrid model (especially via wide-range auto-attack), so a crit adornment vs other stats becomes a real choice once the adornment layer (backlog) exists. Route through brainstorm when the adornment layer is scoped.

**Residual rotation non-monotonicity.** `CADPS` (`internal/model/dps.go`) is slightly non-monotone in reuse and cast-speed at fine resolution: increasing reuse can locally lower `CADPS` by approximately one mid-art cast, because the greedy lattice re-orders under the shifted cooldown availability. This is intrinsic to the discrete simulation — increasing `fightSmoothingSamples` (`internal/model/rotation.go`) does not fix it, only sampling resolution on a problem that is discrete at the source. Consequence: strict-dominance inversions in the per-slot `ΔDPS` scores, magnitudes ~1.5–29 DPS on near-tied items (~37 inversions found across slot/baseline groups as of 2026-06-13). The converged BiS picks from the coordinate-ascent resim (`internal/bis/build.go`) are a valid local optimum regardless. Options: accept and document (cheapest — inversions only flip near-ties); replace the discrete greedy sim with an analytic expected-fractional-cast model (removes quantization at the source; significant rewrite); note that weight-side smoothing does NOT fix this (the inversions live in the resim `ΔDPS`, not the weight display). Route through brainstorm before implementing.

### 16.4 Future: Class-Agnosticism

The system is currently Assassin-specific in two hardcoded places:

- **`assassinClassID = 40`** in `internal/spell/pull.go` — the Census `classes.assassin` filter that gates which combat arts are fetched. Moving this to `classes/<class>.toml` as `census_class_id` makes `spell.AssassinCombatArts` (and `builddb`) class-parameterized; no CA pull for another class is possible without it.
- **`expert`-tier and `assassin`-class validation** in `internal/charconfig` — guards that reject non-Assassin configs. These become per-class config checks once the class field drives the lookup.

`classAutoMult = 2.0` already lives in `classes/assassin.toml` and is the template for the above; `dual_wield` and `weapon_wield_styles` (§6) also live there, making weapon eligibility and the main/off slot structure class-driven. **Deferred:** the two-handed-main-hand path (a 2H pick zeroing out the off-hand slot) is not implemented — only one-handed dual-wield is wired today; a 2H class would need that branch in the candidate-pool builder, the optimizer, and the auto-attack model. See `docs/backlog.md §10` for the full class-intrinsic data plan.

Note: the `strength` → `MainStat` mapping (`internal/stats` / Census item stat key) is a **general game encoding** — the Census files all "+N primary attributes" under the `strength` key for all classes; scouts receive AGI point-for-point from it. This is not an Assassin coupling and is not a blocker for class-agnosticism (see §4 and §11).

### 16.5 Data Wishlist

- **`critBonus` is inert on this server.** `StatBlock.CritBonus` is vestigial on the Varsoon TLE server — no crit-bonus gear or buff has been observed in practice. The raid auto-crit multiplier of ~1.64 is the **wide-weapon range-shift floor** (§11): Modinthalis's 139–775 range lifts most low auto rolls, not a crit-bonus addend. The field remains in `StatBlock` as a zero no-op; do not wire it into `[contexts.raid]` — it would silently double-count the range-shift effect.
- **AGI reading above 1661.** The main-stat curve (`internal/model/curve.go`) clamps above 1661 (the highest measured reading). Raid AGI is ~1800, so the clamped regime covers real play. A reading at ~1800+ would anchor whether the second-regime slope continues, flattens further, or caps before 1800.
- **Haste/dps-mod gap fills.** The fitted curve has measurement gaps at haste/dps-mod 153–238 and 281–300. These are low-priority (the current fit is smooth through the gaps) but a pair of readings per gap would tighten the curve residuals.
- **Cast-speed cap.** The current model treats cast-speed as uncapped above ~37%. If a cap exists, an overcapped read would reveal it as a tooltip plateau. Low probability of binding in EoF gear, but worth confirming if cast-speed items appear in BiS contention.
