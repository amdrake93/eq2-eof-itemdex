# Character Gear Import — Design

**Date:** 2026-06-24
**Status:** Design (approved in brainstorm; pending spec review). This doc is the brainstorm record — rationale, feasibility probes, alternatives. The authoritative behavior is folded into `docs/SPEC.md` (Planned subsections in §3, §4, §6, §7, §8, §16).
**Backlog refs:** `docs/SPEC.md` §16, `docs/plans/design-plan2.md` backlog §1 (character import) and §10 (class-intrinsic values migration / wield-state detection).

## Goal

Import the player's *actual* equipped loadout from the live game (via census) so the BiS optimizer can run sims from the real gear set, rather than only optimizing a set from scratch. The single new capability is **the import**; four existing-machinery use cases ride on top of it:

1. **Score current set** — modeled DPS of the real loadout + stat weights *at that set*.
2. **What to upgrade next** — per optimizable slot, the best catalog alternative's ΔDPS vs the currently equipped item, ranked.
3. **Seed optimization** — use the real loadout as the optimizer's starting point for realistic incremental improvements.
4. **Validate vs parses** — surface the real set's absolute predicted DPS for comparison against in-game parses.

All four are report modes over the same imported `Set`; none introduce new model math.

## Feasibility (verified live during brainstorm, 2026-06-24)

- Census **does** index the TLE character: `character?name.first_lower=biffels&locationdata.worldid=618` returns exactly one **Biffels (Wuoshi)**, Assassin, level 70.
- `c:show=equipmentslot_list` returns all 27 slots, each with the equipped `item.id` and an `adornment_list`.
- Each `adornment_list` entry is a **socket**: filled sockets carry an `id` (resolvable to stats); empty sockets show only a `color`. Biffels: 29 filled (resolvable), 36 empty.
- Validation anchor: Biffels' `cloak` = item `264598753` = **Cloak of Flames**, the item our BiS report already crowns.
- Census name field is nested (`name.first` / `name.first_lower`); world is `locationdata.worldid`. Wuoshi = world id **618**.

## Architecture — Approach A: Loadout file

A network import step writes an inspectable loadout file; a deterministic sim step consumes it. This mirrors the project's existing separation of data-pull from build/score, and decouples the flaky public-census fetch from the reproducible sim.

```
character config ([character] census_name="Biffels", world=618)
        │
   itemdex import                       ← new command; the ONLY network step
        │  GET character?name.first_lower=<census_name>&locationdata.worldid=<world>
        │       &c:show=equipmentslot_list,type,last_update
        ▼
   equipmentslot_list (27 slots)
        │  • map census slot → model slot; skip food/drink/mount/empty
        │  • per item: resolve base stats (catalog hit, else fetch item by id → append to item catalog)
        │  • per FILLED adornment (has id): resolve stats (adornments.csv hit, else fetch → append to adornments.csv)
        │  • sum item + adornment stats per slot
        ▼
   characters/<name>-loadout.toml        ← inspectable artifact
        │
   bis --loadout <file>                  ← deterministic, offline
        ▼
   sims / weights / report against the REAL set
```

## Components & responsibilities

### `itemdex import` (new command)
- **Input:** `census_name` + `world` from the character config (`--character` flag selects the config; defaults as today).
- **Does:** one census `character` query → `equipmentslot_list`; resolves each kept slot's stats; writes the loadout file.
- **Depends on:** the existing `census.Client` (BaseURL-overridable for tests), the item-pull + `catalog.ParseEffects` machinery, the catalog CSVs.
- **Network isolation:** this is the only component that talks to census. It is re-runnable independently of any sim.

### Slot mapping
| Census slot(s) | Disposition |
|---|---|
| primary, secondary | **Optimizable weapons** — override the hardcoded Soulfire main-hand; carry `min_dmg/max_dmg/delay` |
| head, chest, shoulders, forearms, hands, legs, feet, left/right_ring, ears, ears2, neck, left/right_wrist, cloak, waist | **Optimizable gear** — have a catalog candidate pool |
| ranged, ammo, activate1, activate2, event_slot | **Fixed** — stats counted toward baseline, never swapped (no candidate pool) |
| all adornments (filled sockets) | **Fixed** — stats counted, never swapped |
| food, drink, mount_adornment, mount_armor | **Skipped** — stats not counted (food/drink change ad hoc; revisit in future expansions) |
| empty sockets | **Skipped** — color only, no stats |

### Stat resolution
- **Item base stats:** look up in the catalog by id; on miss, fetch the item from census by id (reusing the item-pull + `ParseEffects` path) and **append it to the existing item catalog CSVs** so it is permanently cataloged and `builddb` picks it up.
- **Adornment stats:** for each filled socket, look up the adornment id in `data/adornments.csv`; on miss, fetch the adornment (it is an `item` in census) → parse stats → **append to `data/adornments.csv`** (new catalog, reusable cache). Fetch each unique adornment id once.
- **Per-slot resolved stats** = item base + sum of its filled-adornment stats, folded into a single stat block. v1 does not track which portion came from adornments — their stats simply count toward the slot total.

### Loadout file — `characters/<name>-loadout.toml`
Per kept slot:
```toml
last_update = 1782258823          # census snapshot timestamp, surfaced for staleness
[slots.cloak]
item_id    = 264598753
name       = "Cloak of Flames"
optimizable = true
# resolved stats = item base + filled adornments
[slots.cloak.stats]
haste = 25
# … other stats
[slots.primary]                   # weapon slots additionally carry:
item_id = 1606057721
name = "…"
optimizable = true
min_dmg = <from census>
max_dmg = <from census>
delay   = <from census>
```
Self-contained: carries resolved stats so `bis --loadout` needs no network and no re-resolution.

### `bis --loadout <file>` (new flag)
- Builds a `Set` from the loadout: optimizable slots are pre-filled with the equipped item; fixed slots contribute their stats to the baseline like locked items but are excluded from optimization (extends the existing `--lock` concept from "lock these ids" to "lock the whole imported set, optionally re-optimizing chosen slots").
- Imported primary/secondary populate the `model.Weapon` structs (damage/delay), **overriding** the hardcoded Soulfire main-hand.
- Drives the four report modes (below) over this `Set`.

### The four uses (thin layers on existing machinery)
- **Score current set** → `Set.DPS()` + `model.DeriveWeights` at the imported set.
- **What to upgrade next** → for each optimizable slot, `Set.CandidateDelta` over the catalog pool; report best alternative ΔDPS vs the equipped item, ranked across slots.
- **Seed optimization** → `BuildSet` seeded from the imported set.
- **Validate vs parses** → emit the absolute predicted DPS of the real set; the user compares to parses manually.

### Config changes
- `[character]` gains `census_name` (string) and `world` (int).
- `charconfig` loader reads them; validation requires both present when `import` runs. Existing configs without them still load for non-import flows.

## Testing (TDD)

Census calls are mocked via the client's `BaseURL` override (already used in `internal/census/client_test.go`). Unit coverage:
- slot mapping (each census slot → optimizable / fixed / skipped);
- item + adornment stat summing (incl. empty-socket skip and repeated-adornment caching);
- loadout TOML round-trip (write → read → identical `Set`);
- fixed-vs-optimizable routing in `bis --loadout` (fixed slots add stats but never appear as swap candidates);
- missing-item / missing-adornment fetch path (fetch → append-to-catalog → resolve);
- weapon override (imported primary/secondary replace Soulfire in the `Weapon` structs).

## Risks — surfaced, not silently absorbed
- **Census snapshot staleness:** `last_update` is written to the loadout file and shown in the report, so the player knows how current the import is.
- **Unresolvable item/adornment:** any id census can't return is **listed as "unresolved — stats not counted"** in the import output, never silently dropped.
- **Census-vs-TLE differences:** item *modifiers* appear consistent between census and the TLE server (unlike the CA-base-damage gap handled by `potency_bonus`), but this is flagged as a watch item to confirm against an in-game stat read.

## Non-goals (YAGNI)
- Adornment optimization / upgrade suggestions (stats count; suggestions wait).
- Food/drink slot stats.
- Multi-character batch import.
- Any GUI.
- Reslotting/forecasting adornment swaps.

## Decisions (flag if you disagree)
- **Loadout file naming:** `characters/<census_name lowercased>-loadout.toml` (e.g., `characters/biffels-loadout.toml`).
- **builddb sequencing:** `import` does **not** auto-run `builddb`. When it fetches new items/adornments it appends them to the CSVs and prints a reminder to run `builddb` before `bis`, consistent with today's pull → builddb → bis flow.
