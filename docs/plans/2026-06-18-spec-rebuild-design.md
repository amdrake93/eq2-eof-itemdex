# Design: Whole-System Spec Rebuilt From Code (`docs/SPEC.md`)

**Date:** 2026-06-18
**Type:** Documentation rebuild (no code changes)
**Status:** Design — approved, pre-implementation

## Problem

The project's "spec" is fractured and drifting:

- There are **two** design docs, both marked `Status: Design / spec` — `docs/design.md` ("Plan 1": catalog/data pipeline) and `docs/design-plan2.md` ("Plan 2": DPS model/BiS). The `-plan2` name is misleading; both are specs, not implementation plans (the real implementation plans are `docs/plans/plan2{a..s}`).
- Within `design-plan2.md`, the combat mechanics are **independently restated in multiple places** that have drifted apart — §3.1 (declared "authoritative"), the §3 intro paragraph, and §11 (titled "single source of truth"). Two blocks each claim to be canonical and they disagree (e.g. reuse: §3.1 has the correct divisor; §11 still has the old "1%/pt cap-50"). `design.md §4` adds a fourth, frozen copy whose supersession banner is itself stale.

A prior audit (`docs/spec-sim-audit-2026-06-18.md`) catalogued the divergences but only covered `design-plan2.md §3.1/§11/§12`, missing several duplicate locations. Patching individual stale lines would leave the structural problem — multiple independent restatements — intact.

## Goal

Produce **one** spec, `docs/SPEC.md`, that is the single source of truth for the whole system, **derived from the code** so it cannot inherit the old docs' drift. Establish the durable model the user wants:

- **code = spec** (the spec describes what the code does),
- **`docs/SPEC.md` = current end-state truth**,
- **`docs/plans/` = the timeline** (historical plans; allowed to diverge from each other and from the spec — they are history, not truth).

## Scope

**Whole system.** `SPEC.md` replaces both `design.md` and `design-plan2.md`, covering: Census pull, TLE translation, item classification, catalog/CSV exports, SQLite store, the combat-art pipeline, the DPS model + rotation sim, weight derivation, the BiS engine, outputs, and the testing/calibration-sync approach.

**Out of scope:** any code change. Known code-vs-reality bugs (e.g. the crit model) are *documented as known divergences*, not fixed here.

## Method (source-of-truth discipline)

- **Spine = the repo's code, its inline comments, the `data/*.csv` calibration files, and the tests.** These already carry most provenance (measurement dates, readings, "disproven" notes).
- **Old docs (`design.md`, `design-plan2.md`) and memory are consulted only to enrich provenance** — measurement facts, disproven-hypothesis history — never to settle what the system *currently does*.
- **Current-state claims always defer to code.** Where a comment and an old doc conflict on present behavior, code wins.
- The spec is a **high-level system explanation** — how the system works and *why the numbers are what they are* — not a line-by-line code transcription. Provenance details are kept (they are valued), summarized to spec altitude.

## Deliverable: `docs/SPEC.md` structure

Two parts, per the user's framing — **the system (how the program works)** vs **the game (how EQ2's mechanics work, calibrated)**.

Front matter: status = living spec (source of truth); note that it supersedes `design.md` + `design-plan2.md` (now historical in `plans/`); module name + Go version.

### Part I — The System
1. **Goal & deliverables** — three-tier BiS report, queryable SQLite, CSV catalogs; non-goals (relative ordering only; Assassin only).
2. **Architecture & data flow** — pipeline (Census → classify → catalog/CSV → store → model → BiS report) + package-responsibility table.
3. **Data acquisition** — Census client (throttle, quota-resume), `extract` (Varsoon world 614, EoF discovery-window classification).
4. **Catalog & persistence** — wide-format CSV schema + categories; SQLite schema (items, item_stats, combat_arts, combat_art_components, scores); TLE Census-key translations.
5. **Combat-art pipeline** — pull (Assassin class id 40, Expert, level filters — described as the current hardcoded reality), parse (damage + component hierarchy), `HighestRanks`, `ManualArts`.
6. **Configuration** — character TOML (stats, contexts, art_mods), class TOML (auto-attack multiplier), the three accessibility tiers; the current `assassin`/`expert`-only validation described as-is.
7. **BiS engine** — set model, coord-ascent build, slot capacities & candidates, exclusions, weight derivation, scoring/render.
8. **Commands & operations** — the 5 `cmd/` entrypoints + flags; the refresh loop (`itemdex → builddb → bis/weights`); the `fitcurve` re-fit loop.
9. **Testing & calibration-sync** — test strategy; the sync tests pinning code↔data (fit constants, mainstat table); the "never run Harness files" rule.

### Part II — The Game (the single authoritative mechanics block)
Every formula/constant/curve stated **once**, with provenance inline.
10. **The DPS model** — `TotalDPS = auto ∥ CA`, the multiplier stack, parallel timelines.
11. **Stat-conversion mechanics** — per stat: crit, flurry, haste/dps-mod (fitted curve), multi-attack (table), main-stat/AGI (three-regime table), potency pool + `potencyBonus`, ability-mod (per-mechanic), reuse (divisor + 50% floor), cast speed (divisor), recovery (subtractive). Each with formula + constant + provenance. Records the `strength` census key as the **real, general** game encoding of "+N to all primary attributes" → MainStat (not a scout-specific assumption); explicit single-attribute keys excluded as data-suspicious, per current code.
12. **Auto-attack model** — weapon-damage equation, `classAutoMult` ×2.0, dual-wield ×1.33.
13. **Combat-art damage & components** — per-component sum, taxonomy (DirectHit/DoT/Termination/TriggerProc/RateProc), abmod placement, base-source decision.
14. **Rotation model** — priority sim (damage/slot), clip-vs-hold, fight-length smoothing, idle & stealth assumptions.
15. **Constants block** — mirrors `internal/constants`, each value with provenance.
16. **Open items & known divergences** — the still-live forward worklist:
    - **Tier 1 — crit model** (code uses flat ×1.30; raid data says ~1.50–1.55 via range-shift — documented as a known code bug, not fixed here).
    - **Tier 5 — coverage gaps** (missing CAs Bladed Opening / Point Blank Shot from the live-census-vs-TLE gap; unmodeled procs/poisons; `potencyBonus` in-pool-vs-final-multiplier question).
    - **Residual rotation non-monotonicity** (discrete greedy sim; needs a modeling decision).
    - **Class-agnosticism** — move the hardcoded Assassin specifics (`assassinClassID = 40`; the `assassin`/`expert`-only `charconfig` validation) into config so the system is config-driven; partially done (`classAutoMult` already in class TOML). Cross-references backlog §10. (The `strength → MainStat` mapping is **not** a coupling — it is a general game encoding.)
    - **Data wishlist** — e.g. an AGI reading >1661/1800 to anchor the second regime above the clamp; haste/dps-mod gap readings; cast-speed cap.

## File operations

- **Create** `docs/SPEC.md` (above).
- **Move** `docs/design.md` and `docs/design-plan2.md` → `docs/plans/`, each gaining a one-line header: *"Historical design record — superseded by `docs/SPEC.md`. Kept as timeline history."*
- **Move** `docs/spec-sim-audit-2026-06-18.md` → `docs/plans/` as a historical record of the drift that justified this rebuild. Its still-live items are carried forward into `SPEC.md §16`; its Tier 2–4 items are auto-resolved by deriving the spec from code.

## Why the old audit's Tier 2–4 drop out

Tiers 2–4 were all "spec lagged behind data-backed code." A spec **derived from** the code cannot exhibit those divergences by construction. Only the items where code itself diverges from reality (Tier 1 crit) or where work/data is genuinely outstanding (Tier 5, non-monotonicity, class-agnosticism, data wishlist) survive into §16.

## Verification approach

Each `SPEC.md` section is cross-checked against the owning package/file during writing (e.g. §11 reuse against `rotation.go effRecast` + `constants.go`; §13 against `dps.go CAEffectiveDamage` + `spell/component.go`). The spec asserts only what the code does; provenance facts are cross-checked against code comments and `data/*.csv`.
