# Design: Item `effect_list` Ingestion (Stat Sources + Procs)

**Date:** 2026-06-19
**Status:** Design — approved, pre-implementation
**Motivation:** SPEC §16 coverage gap — items granting stats via "When Equipped:" effects are undervalued; gear procs are unmodeled.

## Problem

The catalog ingests only an item's `modifiers` map. Stats and procs delivered through the census `effect_list` are never read (`census.Item` has no `effect_list`; `extract.showFields` omits it). So items that grant DPS stats via an equipped effect are scored too low, and gear procs are invisible.

**Confirmed impact:** Cloak of Flames grants +25 Haste via `effect_list` (not in its modifiers). Counted, it goes from +175.5 (3rd) to **+291.4 — PRE-RAID BiS Cloak**, beating V'Ncenzi's. It's a *class* of items, not one cloak.

## Principle: parse faithfully, model separately

Ingestion **captures everything the item grants, by source**; the model layer decides what to score. We do **not** pre-filter by DPS-relevance and do **not** flatten effect-stats into the raw modifier stats. This preserves provenance, avoids a brittle whitelist, and means future modeling changes (or new sources like adornments) need no re-ingestion.

## Data model: item stats have multiple sources

An item's effective stats aggregate across distinct **sources**:
- `modifier` — the census `modifiers` block (existing).
- `effect` — static `Increases/Decreases <stat> of caster by N` grants from `effect_list` (new).
- *(future:* `adornment` *— slots in as just another source, same aggregation, no rework.)*

Plus **procs** — triggered effects from `effect_list`, captured separately and **not scored yet** (the deferred proc layer reads them later).

### Storage
- **`item_stats` gains a `source` column** → `(item_id, stat, value, source)`. Existing rows = `modifier`; effect grants written as `effect`. **Nothing is pre-summed** — provenance survives in the data.
- **New `item_procs` table** keyed by `item_id`: `trigger` (raw text), `per_minute` (rate, nullable), `dmg_type` / `min_dmg` / `max_dmg` (nullable, for damage procs), `raw_effect` (full text so nothing is lost). Unscored.

### Aggregation (explicit calc step, source-agnostic)
Where the item's `StatBlock` is built (`store.LoadScorableItems` / `loadWeapon`), sum **all** `item_stats` rows for the item across sources (the existing `AddModifiers` path, now fed every source). The model gets the correct total; the source tag is available for reporting/audit. **Model code is unchanged.** Adornments later: same aggregation includes `source=adornment` rows.

## Parser (`internal/catalog/effects.go`, shared by both ingestion paths)

Walks the `effect_list` (each entry: `description` + `indentation`), mirroring the combat-art component parser. Classifies each `When Equipped:` subtree:

- **Static stat-grant** — a *direct* child (`indentation 1`) of the form `Increases|Decreases <Stat> of caster by N` with **no `%`** and **no trigger keyword**. Emit `(statKey, signedValue, source=effect)`. `<Stat>` is mapped to a census stat key via a **confirmed name→key table** (Haste→attackspeed, Multi Attack→doubleattackchance, Crit Chance→critchance, Potency→basemodifier, DPS→dps, Flurry→flurry, Ability Modifier→all, Reuse→spelltimereusepct, Casting Speed→spelltimecastpct, Agility / all-primary-attributes→strength, Combat Skills→combatskills, …). **No DPS-whitelist** — capture all named stats; the model's `modifierToField` is the sole filter for what's *scored* (so skills/attributes are captured but simply unused until modeling adds them).
- **Proc** — a line that is, or sits under, a trigger (`may cast`, `Triggers about N times per minute`, `On a spell cast`, `On a successful…`, a `Lasts for`/duration or chance clause). Capture as an `item_procs` record (reuse `spell/parse.go` `damageRe` / `perMinuteRe` for damage + rate; keep `raw_effect`). **Not folded into stats.**
- **Skip + log** anything unrecognized, `%`-valued, `of target`/`of group members`, or otherwise unmapped — **under-capture is safe; never guess.**

### Conservative-by-construction (the interpretation safeguard)
The genuine risk is *interpreting* wordings (e.g. Cloak of Unrest's "10% reuse" is a spell-cast **proc**, not a flat stat — correctly routed to `item_procs`, not stats). The parser only credits **confirmed** name→key mappings; everything else is logged. It emits an **audit report** — every distinct `When Equipped` wording, tagged static-stat (proposed key/value/sign/unit) vs proc vs skipped — as a committed artifact for human review. Mappings are corrected there and the backfill re-run (cheap).

## Components / files
- `internal/census/item.go` — add `EffectList []Effect` (`{Description string; Indentation int}`) to `Item`.
- `internal/extract/extract.go` — add `effect_list` to `showFields` (future pulls capture it natively).
- `internal/catalog/effects.go` (new) — `ParseEffects(effects []census.Effect) (stats map[string]float64, procs []Proc, audit []AuditLine)`; the shared name→key table; classification.
- `internal/source/source.go` — `FreshPull` applies `ParseEffects`, writing effect-stats (tagged) + procs (so the main pull captures both going forward).
- `cmd/itemdex` — `--effects` backfill mode: read catalog item IDs, by-ID fetch `effect_list` via the throttled census client (resume on quota like `FreshPull`), apply the **same** `ParseEffects`, write the audit report + effect-stats + procs.
- `internal/store/store.go` — `item_stats` gains `source`; new `item_procs` table + load/write; `LoadScorableItems`/`loadWeapon` aggregate across sources.
- `builddb` + `bis` — rebuild + re-score (model unchanged).

## Validation
- **Parser unit tests** over real wordings: Cloak of Flames → stat `{attackspeed:25, source:effect}`, no proc; Cloak of Unrest → no stat, one `item_procs` record (spell-cast, ~1.8/min, spell-reuse buff); a `Decreases` line → negative stat; a `%` line → skipped/logged; `of target` → skipped; an unrecognized stat → skipped + audit line.
- **Aggregation test:** an item with the same stat in modifier + effect sources sums correctly into the `StatBlock`.
- **Audit report** generated + reviewed post-hoc (parser conservative meanwhile).
- **Integration:** after backfill + rebuild, Cloak of Flames carries +25 haste (`source=effect`) and takes the PRE-RAID BiS Cloak slot; review the broader set of items that re-rank, plus the proc catalog.

## Out of scope
- **Proc scoring** — `item_procs` is captured-not-scored; the proc-layer (rate × damage, 0%-crit class) is a separate future feature (SPEC §16).
- **Crit bonus** — confirmed inert on this TLE server; captured if present but the model ignores `critbonus` (unchanged).
- **Combat-skill / attribute modeling** — captured, but the model doesn't score them yet (attack rating held constant; that's a separate modeling decision, SPEC §16).

## Spec updates
- **§4** (Catalog & Persistence) — document item `effect_list` ingestion: multi-source item stats (`modifier`/`effect`, aggregated), and `item_procs` capture.
- **§6** (Schema) — `item_stats.source` column; new `item_procs` table.
- **§16** — proc-scoring reads `item_procs` (deferred); **correct the `critBonus` item** (crit bonus inert; raid auto-crit ~1.64 is the wide-weapon range-shift floor, not crit bonus); note combat-skill/attribute effects captured-but-unscored.
