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
## 4. Catalog & Persistence
## 5. Combat-Art Pipeline
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
