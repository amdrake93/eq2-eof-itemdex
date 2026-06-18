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

### Character TOML structure

`internal/charconfig/charconfig.go Load` parses `characters/<name>.toml` using `github.com/BurntSushi/toml` in strict mode: any TOML key that does not map to a struct field is reported as an error via `md.Undecoded()`. A typo'd stat name silently vanishes in most configs; here it fails loudly.

The top-level `Config` struct has four sections:

| Section | Type | Purpose |
|---|---|---|
| `[character]` | `Character` | Identity fields: `name`, `class`, `art_tier`. |
| `[stats]` | `StatGrants` | AA/innate stat grants applied to every evaluation context (baseline before gear and context buffs). |
| `[art_mods."Name"]` | `map[string]ArtMod` | Per-art AA modifiers keyed by rank-stripped art name. |
| `[contexts.X]` | `map[string]StatGrants` | Named buff packages. At least one context is required. |

**`Character` validation** — `Load` enforces two hard constraints that reflect the current implementation scope:

- `Class` must be `"assassin"` — any other value is a hard error (`"only assassin is implemented"`). See §16 — Open Items & Known Divergences for the class-agnosticism backlog item.
- `ArtTier` must be `"expert"` — any other value is a hard error (`"only expert is implemented"`). See §16 for the tier-flexibility backlog item.

**`StatGrants` validation** — every stat in a `[stats]` or `[contexts.X]` block must be non-negative. Config stats are grants; debuffs are not a config concept, and a negative `cast_speed` at or below −100 would make the rotation's divisor non-positive. The check is enforced by `StatGrants.nonNegative()` for both the base `[stats]` block and every context block individually.

**`ArtMod`** carries two fields, both validated at load time:

- `RecastMult float64` (`recast_mult` in TOML) — multiplies the art's base recast seconds. Must be in the range `(0, 1]`. A value of `0.5` halves the recast, which fills the art's 50% AA recast-reduction ceiling. `ApplyArtMods` converts this to `RecastReduction = 1 − RecastMult` on the `CombatArt`.
- `PotencyAdd float64` (`potency_add` in TOML) — additive potency rider pooled with the character's displayed potency. Must be non-negative.

`ApplyArtMods` also enforces that every key in the `[art_mods]` map matches at least one loaded combat art (by rank-stripped name). An unmatched key is a hard error — a typo'd art name failing loudly beats silently un-halving an art like Assassinate.

**`ContextBlock(name)`** returns the `model.StatBlock` used for one evaluation context: it combines `[stats]` + the named context's buff package via `StatGrants.Block().Add(ctx.Block())`. Gear is never part of the character config — the optimizer adds gear.

### Class TOML structure

`LoadClass(dir, class)` reads `classes/<class>.toml`. The schema is uniform across classes: every class file defines the same fields (a missing field is a hard error). Currently the only field is:

- `auto_attack_multiplier float64` — the class-intrinsic auto-attack multiplier. Must be `> 0`. The Assassin value is `2.0`, measured via `/weaponstat` on 2026-06-13. This is distinct from per-character stats and never lives in the character TOML.

`LoadClass` is also strict: unknown keys in the class TOML are errors.

### Accessibility tiers

`cmd/bis/main.go` defines three gear tiers that control which items are eligible in each optimization run:

| Tier | Stat context | Gear filter |
|---|---|---|
| `PRE-RAID` | `solo` context | Tier is `LEGENDARY` or `TREASURED`; avatar gear excluded; Hunter's sets excluded; curated exclusions excluded. |
| `RAID` | `raid` context | All tiers except avatar (MYTHICAL non-Soulfire); Hunter's sets excluded; curated exclusions excluded. |
| `BEST-OF-BEST` | `raid` context | All items including avatar (MYTHICAL); only Hunter's sets and curated exclusions removed. |

Avatar gear is defined by `IsAvatar`: `Tier == "MYTHICAL"` and `Name` does not start with `"Soulfire"`. Hunter's gear is defined by `IsHunters`: `Name` contains `"Hunter's"`. `Curated` is a hand-maintained map in `exclusions.go` for items that are discoverable on Varsoon but practically inaccessible; it is currently empty.

The Soulfire main-hand is the fixed primary weapon across all tiers — it is not subject to tier filtering and is never re-optimized (see §7).

## 7. BiS Engine

### Set representation

`internal/bis/set.go Set` is the central data structure for an in-progress or converged gear loadout:

| Field | Type | Role |
|---|---|---|
| `Profile` | `model.StatBlock` | Baseline stat block from the character config + context (no gear). |
| `Main` | `model.Weapon` | Fixed Soulfire main-hand weapon; never changed by the optimizer. |
| `Arts` | `[]spell.CombatArt` | Combat-art pool with AA mods applied. |
| `AutoMult` | `float64` | Class auto-attack multiplier (`ClassData.AutoAttackMultiplier`). |
| `FightLen` | `float64` | Target fight length in seconds (rotation smoothing window). |
| `Equipped` | `map[string][]store.ScorableItem` | Currently equipped items per Census slot. Multi-capacity slots hold more than one item (see below). |

`Set.DPS()` evaluates the full set's modeled DPS by calling `model.TotalDPSDual` with the complete equipped stat totals, the Soulfire main-hand, and the currently equipped off-hand weapon.

`Set.CandidateDelta(slot, c)` computes the in-context ΔDPS of placing candidate `c` in `slot` against the rest of the fixed set. For off-hand weapon candidates it passes the weapon's stats as both a stat block and a `*model.Weapon`. For all other slots it passes only the stat block (no weapon struct). This calls through to `model.ItemDelta`.

### Slot candidates

`SlotCandidates(items, keep)` builds the per-slot candidate pools passed to `BuildSet`. It groups all items that pass the `keep` predicate by their Census slot. The off-hand slot (`Secondary`) is then overridden: its candidate pool is every one-handed weapon that passes `keep` except Soulfire weapons (the player never owns a second Soulfire, and the main-hand Soulfire is fixed). The Primary slot is left as-is in the map but `BuildSet` ignores it.

### Coordinate-ascent optimizer

`BuildSet(profile, lo, bySlot, locked, maxPasses, autoMult, fightLen)` converges a gear set via coordinate ascent:

1. A new `Set` is created with the given profile and loadout.
2. Any `locked` slots are pre-filled and excluded from optimization.
3. The Primary (main-hand) slot is always excluded.
4. Remaining slots are sorted alphabetically and iterated each pass.
5. For each slot, `pickBest` (greedy within-slot selection, see below) is called; if the result differs from the current occupant, the slot is updated and `changed` is set.
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

`BuildSlotReports` produces one `SlotReport` per slot, each carrying the converged pick (`Chosen`) and the top-N ranked alternatives (`Ranked`) by in-context ΔDPS via `SlotCandidatesScored`. The Primary slot is excluded from reports (it is prepended separately by `cmd/bis`'s `withFixedPrimary`).

## 8. Commands & Operations

### Command reference

| Command | Purpose | Flags (default) |
|---|---|---|
| `itemdex` | Pull EoF items from Census and write CSV catalog. Serves from cache if CSVs exist and `--refresh` is false. | `--out data` (CSV output dir / cache), `--refresh false` (force fresh Census pull), `--sid s:example` (Census service ID), `--page 1000` (items per Census request) |
| `builddb` | Build the SQLite database from the CSV cache. Reads gear from CSVs, pulls Assassin Expert CAs from Census, and writes `bis.db`. | `--data data` (CSV catalog dir), `--db bis.db` (output DB path), `--sid s:example` (Census service ID) |
| `bis` | Run the BiS optimizer and write the markdown report. Runs all three accessibility tiers plus any locked re-model. | `--db bis.db` (scored DB), `--out bis-report.md` (report path), `--character characters/alex.toml` (character config), `--lock ""` (comma-separated item IDs to lock for a re-model run), `--top 3` (alternatives per slot), `--fight` (fight duration in seconds; defaults to `constants.FightDurationSecs`) |
| `weights` | Print marginal DPS weights per stat at the current loadout for each context. Diagnostic tool; does not write any files. | `--db bis.db`, `--character characters/alex.toml`, `--fight` (fight duration; defaults to `constants.FightDurationSecs`) |
| `fitcurve` | Fit the haste/DPS-mod quadratic curve from the readings CSV and print paste-ready constants for `internal/model/curve.go`. | `--readings data/curve-readings.csv` |

### Operational loops

**Data refresh loop** — run when item data may be stale (new Census data, post-patch):

1. `go run ./cmd/itemdex [--refresh]` — pulls EoF items from Census and writes (or refreshes) the CSV catalog under `data/`. Omit `--refresh` to serve from the existing cache.
2. `go run ./cmd/builddb` — ingests the CSV catalog + fresh Census CA pull and writes `bis.db`.
3. `go run ./cmd/bis` — runs the optimizer and writes `bis-report.md`. Optionally run `go run ./cmd/weights` first to inspect stat weights.

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
## 11. Stat-Conversion Mechanics
## 12. Auto-Attack Model
## 13. Combat-Art Damage & Components
## 14. Rotation Model
## 15. Constants Block
## 16. Open Items & Known Divergences
