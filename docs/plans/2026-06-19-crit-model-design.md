# Design: Crit Model — Range-Shift Floor (Hybrid)

**Date:** 2026-06-19
**Status:** Design — approved, pre-implementation
**Implements:** `docs/SPEC.md` §16.1 (the crit known-divergence)

## Problem

Code and spec model a critical hit as a flat expected-value multiplier — `critFactor = 1 + critChance·(CritMultiplier−1)`, `CritMultiplier = 1.30` — applied to each CA cast total and to auto-attack (`internal/model/dps.go critFactor`, `internal/constants/constants.go`). The 2026-06-16 raid log flagged this as wrong (effective crit ~1.5+). Clean tooltip and combat-log reads on 2026-06-19 pinned the real mechanic.

## The mechanic (measured 2026-06-19)

A critical hit deals:

```
crit = max( prev_max + 1 , (1.5 + crit_bonus) × roll )
```

— the **higher of** (a) the damage range's `max + 1` (a floor) and (b) `1.5×` the rolled value — applied to the **final** per-hit damage, *after* potency × AGI **and** ability-mod (ability-mod rides the crit). `crit_bonus` is a buff/gear term, **0** in the clean reads (so base crit = ×1.5).

### Evidence

All reads naked / controlled (multipliers cancel in the crit-vs-non-crit comparison):

| read | range / value | result | interpretation |
|---|---|---|---|
| Quick Strike DirectHit | 285–475 (1.667:1) | crits 474–674 | 1.5×roll (floor barely grazes bottom) |
| Quick Strike DoT | single 53 | crit 80 | ×1.51 (single → 1.5×) |
| Strike of Consistency | single 61 | crit 92 | ×1.51 (single → 1.5×) |
| Hilt Strike | 316–386 (1.2:1) | crits 477, 513 | **exceed range-shift ceiling (457)** → flat 1.5×roll, not pure range-shift |
| SoC + 68 abmod | non-crit 121 | crit 182 | ×1.50 → **abmod rides** |
| **Auto (Modinthalis)** | 139–775 (5.58:1) | **11/14 crits = exactly 776 (= max+1)**; rest 858/860/906 = 1.5×high-roll | **the floor** — wide range, floor branch dominates |

Quick Strike *alone* couldn't distinguish flat-×1.5 from the floor (its 1.667:1 range makes them coincide on average and within rounding at the floor). Hilt Strike ruled out *pure* range-shift (crits above its ceiling). The **wide auto range** is what exposed the floor decisively (the 776 pile-up). Together they uniquely fix the `max(...)` hybrid.

### Why the raid log misread it as range-dependent

abmod (a flat add) and raid crit-bonus buffs vary by ability, so the *effective* crit ratio looked like it tracked range width — when it's actually the single `max(floor, 1.5×roll)` rule. Clean unbuffed reads show the uniform mechanic.

## The model (expected DPS)

The model computes expected (sustained) DPS, so per source we need the **expected** crit multiplier over the roll distribution — computed **live, per source, from its own final range** `[lo, hi]` (no hardcoded multiplier). Assuming uniform rolls and base crit factor `c = 1.5 + crit_bonus`:

```
t = hi / c                                   # roll where 1.5× overtakes the floor
critMult(lo,hi) =
    c                                                       if lo >= t      (whole range clears floor)
    [ hi·(t−lo) + c·(hi²−t²)/2 ] / ( (hi−lo) · (lo+hi)/2 )  otherwise       (floor zone + 1.5× zone)
```

Outputs: ~×1.5 for single-valued and ≤1.5:1 ranges; ~×1.51 for the 1.667:1 CA family; floor-boosted for wide weapons (Modinthalis 139–775 → **×1.87**).

Expected per-source damage = `avg × ( 1 + min(critChance,100)/100 · (critMult − 1) )`.

### Validation (auto-attack log, `data/autoattacktest.txt`, 106 non-crit + 14 crit)

- Empirical critMult = avg_crit 797.1 / avg_noncrit 430.7 = **1.85** vs the formula's **1.87** — **<1% match**.
- Floor reconfirmed: 11/14 crits exactly 776 (= max+1).
- Roll distribution **consistent with uniform** (per-bin χ² p≈0.11 — cannot reject uniformity at this sample size; mean 430.7 vs midpoint 456 is only ~1.4 SD, noise-range).

So the uniform `critMult` is **data-backed**, not assumed.

### Roll-distribution assumption

Uniform rolls over `[min,max]` is the documented assumption (validated above). The rolled-vs-deterministic distinction (DirectHits roll; DoTs emit their average) changes a 1.667:1 component by <1% (×1.51 vs ×1.50), so **all components use the same uniform `critMult`** for simplicity. abmod is a flat add, so a DirectHit's crit is computed on its **abmod-inclusive** final range `[min·scaling+abmod, max·scaling+abmod]` (more abmod → narrower ratio → less floor boost — consistent with the raid log's lower high-abmod crit ratios).

## Changes

### Code (`internal/`)
- `constants.go`: `CritMultiplier 1.30 → 1.50` — now the base crit factor `c` (the `1.5` branch), not a flat expected multiplier.
- New `critMult(lo, hi, critBonus)` (closed form above) in `model`.
- `dps.go`: replace the flat `critFactor` with per-source `critMult`. `AutoDPS` computes its multiplier from the weapon's final swing range.
- `model.Weapon`: add `MinDamage`, `MaxDamage` (currently only `AvgDamage`). `store` loads them from the existing `weapon_min_dmg` / `weapon_max_dmg` columns into the loadout's `Weapon`.
- `rotation.go CAEffectiveDamage`: apply `critMult` per component, each on its **abmod-inclusive final range**, replacing the single `× critFactor` on the total.
- `StatBlock`: add `CritBonus` (default 0). Clamp crit chance at 100% in the multiplier.
- **Calibration test**: pin the mechanic to the reads — floor = `max+1`; single-valued → ×1.5; Modinthalis 139–775 → ≈1.87 (within tolerance of the measured 1.85); SoC+abmod → abmod rides (×1.5 on the abmod-inclusive total).

### Spec (`docs/SPEC.md`)
- **§11 Crit**: rewrite to the hybrid + per-source `critMult` + this provenance; drop the "known divergence" caveat.
- **§15 Constants**: `CritMultiplier = 1.50` (base crit factor).
- **§16**: remove the resolved crit divergence. Add (a) **auto roll-distribution / auto-avg-damage** refinement — gather more swings to confirm uniform; the model's `(min+max)/2` average may run ~6% high if a real low-lean exists (auto-*damage*, not crit); (b) **`crit_bonus`** as a future raid-context stat (raid crits exceed ×1.5); (c) the elevated **crit-adornment** question (crit chance's weight rises, so a crit adornment vs other stats becomes a real choice once the adornment layer exists — backlog).
- **§12 (Auto-Attack)**: note weapon `min/max` now feeds the model (schema already carried the columns).

## Out of scope
- **Adornment modeling** (backlog) — but crit's higher derived weight makes the crit-adornment-vs-other decision newly relevant; noted in §16.
- **Auto-average-damage skew** correction — a §16 refinement pending more auto samples; leave the midpoint average as-is for now.
- **Raid `crit_bonus` values** — a raid-context input, not this change.

## Expected impact
- Crit-chance derived weight **rises** — mostly via auto-attack (wide range, floor-boosted ~×1.87); CAs only mildly (~×1.5). Likely a noticeable jump in the weight table.
- **Gear picks ≈ stable** — gear crit is near-constant per item (like potency), a uniform additive across a slot's candidates, so it rarely changes which item wins; the fix is about absolute accuracy + auto-vs-CA balance + tilting genuine near-ties.

## Verification
- The calibration test pins the mechanic to the 2026-06-19 reads (regression-proof).
- `go test ./...` green; regenerate `cmd/weights` (crit weight up) and the BiS report (picks ≈ stable).
