# Design: Item `effect_list` Stat Ingestion

**Date:** 2026-06-19
**Status:** Design — approved, pre-implementation
**Motivation:** SPEC §16 coverage gap — items granting stats via "When Equipped:" effects are undervalued.

## Problem

The catalog ingests only an item's `modifiers` map. Stats granted through the census `effect_list` (e.g. `"When Equipped: Increases Haste of caster by 25.0"`) are never read — `census.Item` has no `effect_list` field and `extract.showFields` doesn't request it. So any item delivering DPS stats via an equipped effect is scored too low.

**Confirmed impact:** Cloak of Flames carries +25 Haste via `effect_list` (not in its modifiers). With it counted, the item goes from +175.5 DPS (3rd) to **+291.4 (PRE-RAID BiS Cloak)**, beating V'Ncenzi's Voluminous Cape — a real BiS change, not just absolute DPS. It is a *class* of items, not one cloak.

## The interpretation hazard (the real risk)

The parsing is trivial; correctly **interpreting** each wording is not. Confirmed pitfalls from live census reads:

- **Procs masquerading as stats.** Cloak of Unrest's `effect_list`: `When Equipped: → On a spell cast … may cast … Triggers about 1.8 times per minute → Decrease the caster's spell reuse time by 10%`. A naive "10% reuse" credit is triple-wrong: it's a proc, spell-cast-gated, temporary, and *spell* reuse. (Its real flat reuse is already in the modifiers.)
- **Increases vs Decreases** (items self-debuff as a tradeoff: "Decreases Weapon Damage of caster").
- **Points vs percent** ("by 25.0" = stat points like gear; "by N%" is a different quantity).
- **Conditionals** ("If profession other than Fighter, …").
- **Stat identity** — confirm the effect's "Haste" is the melee `attackspeed` haste the model curves, not a lookalike; don't double-count a stat that's also in modifiers.
- **Inert stats** — Crit Bonus (confirmed stripped on this TLE server), Fervor (inert), combat/spell skills, attributes — all excluded.

## Design

### Approach
Fold effect-granted stats into the item's catalog stats at ingestion (downstream `builddb`/`store`/`model` unchanged — effect stats become indistinguishable from modifiers). One **shared parser**, used by both the main pull (future-proof) and a fast by-ID backfill (current data without a slow full re-scan).

### Classification rule — indentation + trigger keywords
Mirrors the combat-art component parser. Walk the `effect_list` (each entry has `description` + `indentation`):

- **CREDIT** a line iff it is a **direct child of `When Equipped:`** (indentation 1) of the form **`Increases <Stat> of caster by <N>`** — `Increases` (not `Decreases`), no `%`, `<Stat>` in the whitelist, `of caster` (not `of target`/`of group members`).
- **SKIP** (and skip its whole subtree) any line that is — or sits under — a **proc/trigger**: contains `may cast`, `Triggers about N times per minute`, `On a spell cast`, `On a successful`, a `Lasts for`/duration, or a chance clause. These are the deferred proc class (SPEC §16).
- **SKIP + log** anything unrecognized, conditional, `%`-valued, a Decrease, or a non-whitelisted stat. **Under-credit (safe), never guess.**

### Whitelist (effect stat display-name → modifier key)
| effect "Increases ___ of caster" | modifier key | model field |
|---|---|---|
| Haste | `attackspeed` | Haste |
| Multi Attack | `doubleattackchance` | MultiAttack |
| Crit Chance | `critchance` | CritChance |
| Potency | `basemodifier` | Potency |
| DPS | `dps` | DPSMod |
| Flurry | `flurry` | Flurry |
| Ability Modifier | `all` | AbilityMod |
| Reuse* | `spelltimereusepct` | Reuse |
| Casting Speed* | `spelltimecastpct` | CastSpeed |

\* exact display strings (Reuse / Casting Speed and their variants) to be **confirmed against real census wordings during the audit** before mapping. Excluded: Crit Bonus, Combat Skills, Focus/Disruption/Ministration/Subjugation/Ordination, Slashing/Piercing/Crushing/Aggression/Ranged, STR/STA/attributes, regen, mitigation, Fervor, run/mount speed.

### Interpretation-audit gate (human-reviewed, before crediting)
The implementation first produces an **audit report**: every distinct ind-1 `When Equipped` wording across the full catalog, each tagged **static-stat → proposed (key, value, unit)** or **proc/skip / unrecognized**. The user reviews and confirms/corrects the mapping table. The parser is then built from the **confirmed** table; unrecognized lines stay logged-and-skipped.

### Components / files
- `internal/census/item.go` — add `EffectList []Effect` (`{Description string; Indentation int}`) to `Item`.
- `internal/extract/extract.go` — add `effect_list` to `showFields`.
- `internal/catalog/effects.go` (new) — `ParseEffectStats(effects []census.Effect) map[string]float64` (the shared parser) + an audit/classify helper that returns the per-line tagging for the report.
- `internal/source/source.go` — `FreshPull` applies `ParseEffectStats` and merges results into each item's stats before writing CSVs (so future pulls capture effects natively).
- `cmd/itemdex` — an `--effects` backfill mode: read the catalog's item IDs, by-ID fetch `effect_list` via the existing throttled client, apply the **same** `ParseEffectStats`, rewrite the catalog CSVs.
- `builddb` + `bis` — unchanged; rebuild + re-score.

### Data flow
census `effect_list` → `ParseEffectStats` (classify + whitelist + of-caster/Increases/points/static) → stats merged into the item → catalog CSV → `builddb` → `store` → `model` → `bis` re-score.

## Validation
- **Parser unit tests** over real wordings: Cloak of Flames → `{attackspeed: 25}`; Cloak of Unrest → `{}` (proc skipped); a Decrease line → skipped; a `%` line → skipped; an `of target` line → skipped; an unrecognized stat → skipped + logged.
- **Audit report** reviewed by the user (the interpretation gate).
- **Integration:** after backfill + rebuild, Cloak of Flames carries +25 haste and takes the PRE-RAID BiS Cloak slot; review the broader set of items that re-rank.

## Out of scope
- **Proc scoring** (`Inflicts …damage`, spell-cast procs) — the deferred RateProc/unmodeled-proc class (SPEC §16).
- **Crit bonus** — confirmed inert on this TLE server; excluded.
- **Single-attribute / non-DPS effects** — excluded.

## Spec updates
- **§4** (Catalog & Persistence) — document item `effect_list` ingestion (shared parser, whitelist, of-caster static stat-grants folded into item stats).
- **§16** — add "unmodeled gear procs (`Inflicts…` / spell-cast procs in item `effect_list`)" to the deferred-procs class; **correct the `critBonus` raid-stat item** (crit bonus confirmed inert; the raid auto-crit ~1.64 is the wide-weapon range-shift floor, not crit bonus — the `CritBonus` field is vestigial on this server).
