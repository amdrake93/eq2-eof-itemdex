# Plan 2 Design: EoF Assassin Best-in-Slot — DPS Model & Analysis

**Date:** 2026-06-03
**Status:** Design / spec (pre-implementation)
**Builds on:** Plan 1 (`docs/design.md`, the item catalog) — reuses its `census` / `classify` / `catalog` / `extract` / `source` packages, types, and the gear CSVs.
**Implementation language:** Go 1.26.

This is **Plan 2 of 2**. Plan 1 produced the EoF gear catalog; Plan 2 builds the relative-DPS model that ranks that gear into a best-in-slot list for an Assassin. The core model + combat constants are specified in `docs/design.md` §3–§4; this document carries them forward and adds the Plan-2-specific decisions (two baselines, SQLite analysis layer, locked-items re-model, outputs).

---

## 1. Goal & Deliverables

For a level-70 Echoes of Faydwer **Assassin**, compute **best-in-slot per equipment slot** across **three accessibility tiers** — **pre-raid**, **raid**, and **best-of-best** — ranked by a derived-weight relative DPS model. (The tiers reuse two underlying stat baselines — solo-buff and raid-buff — combined with gear-accessibility filters; see §4.)

**Deliverables:**
- A markdown **BiS report**: per-slot pick(s) for *each* of the three accessibility tiers, **top-N alternatives per slot** (not just #1), the **derived stat weights** per tier, a **per-slot progression** summary, and the **assumptions/constants block**.
- The queryable **SQLite DB** (scored gear) for the user's own exploration.

**Non-goals:** absolute/parse-accurate DPS (relative ordering only); other classes; set-bonus *scoring* (see §8).

---

## 2. Prerequisite: catalog re-pull (weapon `skill` + `wieldstyle`)

Plan 1's `census.TypeInfo` captured `skilltype` (armor) but **not** the weapon `skill` (piercing/slashing/crushing/…) or `wieldstyle` (One-Handed/Two-Handed). Both are needed: `wieldstyle` is a **model input** (dual-wielding two 1H vs one 2H changes the auto-attack term), and `skill` is informational (which combat-skill applies). Weapon *eligibility* still rides on the existing `typeinfo.classes` → `classes` column (no skill filter — Assassins use piercing **and** slashing).

**Change:** add `Skill` (`json:"skill"`) and `WieldStyle` (`json:"wieldstyle"`) to `census.TypeInfo`; add `skill` + `wieldstyle` columns to the weapon CSV write/read (Plan 1 §10 schema). Then **`go run ./cmd/itemdex --refresh`** to repopulate `data/`. (The discovery-window pull is the slow, multi-session part — a one-time prereq.)

---

## 3. The DPS Model

`TotalDPS = AutoDPS + CADPS`, computed in **parallel** — auto-attack and combat-art casting run on independent timelines, so casting does **not** displace auto swings (a CA's cast+recovery only paces the *CA* side; see §3.1). Locked combat values: crit ×1.30; **flurry ×4** (a flurry proc does +100%–500%, averaging +300% = ×4); **ability-mod applies in full — the old 50% cap is disproven** (§3.1); reuse converts at **1%/point capping at 50 stat**, sharing each art's **50% recast-reduction ceiling** with AA reductions (§3.1); **potency applies to CAs only**, pooled with a **calibrated hidden bonus (⚠ open mystery, §12)**; **main stat (AGI) multiplies all CA damage** via its own capped curve (§3.1); `critbonus` ignored. Haste, multi-attack, and dps-mod are **non-linear** (§3.1) — *not* `1 + stat/100`. Haste & dps-mod share one **fitted** diminishing curve (hard cap **300** stat; the old `200 → 125%` anchor is disproven — see §3.1); multi-attack has its own gentler curve (runs to 3400 with triple overcap). Stat-conversion mechanics, the AA cooldown/recovery effects, and the curve table are in **§3.1** (the authoritative stat-mechanics block, revised from the Varsoon playtest session).

**Why the Assassin CA query is integral (not just additive damage):** the CA term needs each Combat Art's *base* damage to model the multipliers correctly — the potency pool and the main-stat curve scale the base, and ability-mod adds flat on top. Without the per-CA base damages, neither term is computable. So `internal/spell` pulls the Assassin's CAs (Expert tier, level ≤70), regex-parses damage from `effect_list`, and applies the TLE translations (§ Assumptions).

**Derive-don't-declare:** for each baseline, compute the marginal DPS per stat (`∂TotalDPS/∂stat`) at a realistically-geared baseline → the **weights**; iterate (equip current best → recompute near caps → re-rank) to convergence so saturation/caps self-correct. Score each Assassin-usable item `= Σ(weight × itemStat)`; rank per slot. **No stat is pre-judged valuable or dead — the weights are computed outputs.**

**Weapons:** Soulfire (Mythical, from the raid questline) is the given main-hand; the off-hand is ranked across all Assassin-usable 1H weapons (any skill). The model accounts for dual-wield via `wieldstyle`.

---

## 3.1 Stat-Conversion Mechanics (authoritative — Varsoon playtest revisions)

Corrections gathered from in-game testing on Varsoon, corroborated by a patch note. These **supersede** the simpler assumptions in `docs/design.md` §4. All conversions below are now backed by live Varsoon data points — no remaining inferred assumptions. There are **two distinct diminishing curves**: one for multi-attack, a steeper one shared by haste and dps-mod.

### Multi-attack conversion curve

Multi-attack is **non-linear**: the stat converts to an effect % via a diminishing-returns curve. We have only sampled points (no formula), so the model **interpolates piecewise-linearly between the samples, anchored at (0,0), then floors to whole %** (the game evaluates a continuous formula and floors per integer %).

Sampled stat → effect %:

| stat | % | stat | % | stat | % |
|-----:|--:|-----:|--:|-----:|--:|
| 10 | 12 | 90 | 84 | 180 | 123 |
| 20 | 22 | 100 | 91 | 190 | 124 |
| 30 | 33 | 110 | 97 | 200 | 125 |
| 40 | 43 | 120 | 100+2 | 300 | 135 |
| 50 | 52 | 130 | 100+7 | 500 | 145 |
| 60 | 61 | 140 | 100+11 | 700 | 155 |
| 70 | 69 | 150 | 100+15 | 900 | 165 |
| 80 | 77 | 160 | 100+18 | 1200 | 175 |
|    |    | 170 | 100+21 | 3400 | 200 |

(Effect % is a single number = `min(double,100) + triple`, e.g. `120 → 102`.) **No hard cap** — runs the full curve to `3400 → 200%`. The portion **above 100% is triple-attack chance** (e.g. `120` = 100% double + 2% triple). Auto-attack swing multiplier = `1 + effect/100`. **Auto-attack only** (does not touch CAs). TLE key: `doubleattackchance`.

### Haste & DPS-mod conversion curve (shared, fitted)

Haste and dps-mod share a **single, steeper diminishing curve** — originally claimed by a patch note, now confirmed by **20 readings from both stats that interleave onto one strictly-monotonic line**, including fresh new-server readings from both stats overlaying cleanly in their 70–100 overlap band.

**The old patch-note anchor `(200 → 125%)` is disproven.** The reading `haste 281 → 124%` sits *below* 125, and the curve is monotonic — so 200 cannot give 125. The same patch note's "200 cap" was also wrong: the **hard cap is 300 stat** (former-game-developer statement, corroborated by readings still growing at 238.4 and flattening hard by 281 — local slope ≈0.14/pt there vs ≈0.75 at the bottom). The exact effect % at 300 is unmeasured.

**Readings are data, the equation is derived.** The canonical dataset lives in `data/curve-readings.csv` (columns: `stat` = haste|dpsmod, `raw`, `effect` = shown %, `era` = varsoon|live). Shown effects are UI-floored integers — a shown `E` means true effect ∈ `[E, E+1)`. The raw-stat *source* is irrelevant (gear, group buffs, temporary CDs all roll into the UI total, which is all the curve sees), so any stat range can be sampled on demand by buff/CD-stacking. Initial dataset (2026-06; `varsoon` = original-server readings, `live` = current TLE server):

| stat | effect % | source | era |
|-----:|---------:|:--|:--|
| 24 | 18 | haste | varsoon |
| 28.1 | 21 | dps-mod | varsoon |
| 48.3 | 35 | dps-mod | varsoon |
| 48.7 | 36 | haste | live |
| 53 | 38 | haste | live |
| 57.3 | 41 | haste | live |
| 61.6 | 44 | haste | live |
| 67.5 | 48 | haste | varsoon |
| 70.6 | 50 | dps-mod | live |
| 78.8 | 55 | dps-mod | live |
| 86.9 | 59 | dps-mod | live |
| 87.4 | 60 | haste | live |
| 91.7 | 62 | haste | live |
| 96 | 65 | haste | live |
| 100.3 | 67 | haste | live |
| 136.5 | 85 | dps-mod | live |
| 144.6 | 88 | dps-mod | live |
| 152.8 | 92 | dps-mod | live |
| 238.4 | 118 | dps-mod | varsoon |
| 281 | 124 | haste | varsoon |

**Fitting (`cmd/fitcurve`)** — floor-aware least squares over the CSV: each reading's fit target is `shown + 0.5` (midpoint of its floor interval). Two candidate forms — both standard diminishing-returns implementations — with residuals deciding the winner:

- **quadratic:** `f(s) = a·s − b·s²` (naturally peaks; a peak landing near the 300 cap is a plausible dev implementation — the current rough fit peaks ≈340)
- **logarithmic:** `f(s) = a·ln(1 + s/b)` (never peaks; the cap would be a hard clamp)

The tool fits each form three ways — **haste-only, dpsmod-only, and joint** — and reports per-fit residuals. This is the ongoing shared-curve verification: if the two single-stat fits agree within flooring noise, the joint fit is the curve; if they ever diverge as data accumulates, split into per-stat parameter sets (the model today carries **one shared parameter set**; splitting is a small refactor to make when divergence actually appears). The winning form's parameters are recorded as constants in `internal/model/curve.go`, annotated with the residual and dataset size; appending readings + re-running the tool is the whole refresh loop.

**Application (unchanged mechanics, new curve underneath):**

- **Haste** — `effDelay = baseDelay / (1 + hasteEffect/100)`.
- **DPS-mod** — auto-damage multiplier = `1 + dpsModEffect/100`. **Auto-attack only.**
- Effect is floored to a whole % (in-game behavior): `effect = floor(f(min(stat, 300)))`. Overcap (stat > 300) clamps at `f(300)`; haste overcap does **NOT** convert to flurry (confirmed removed).

**Data still wanted (non-blocking — append to the CSV and re-fit):** the 153–238 and 281–300 gaps; a reading at/near exactly 300; one deliberately *overcapped* pair (e.g. `320 → X%`) to verify the clamp.

### Linear / direct stats
- **Crit chance** — linear, `crit% = stat`. `critFactor = 1 + (crit%/100)·(1.30−1)`.
- **Potency** — linear, `potency% = stat`; enters the CA potency pool (below).
- **Flurry** — **gear % only** (no haste contribution); `flurryFactor = 1 + (flurry%/100)·(4−1)`.
- **Ability-mod** — flat add to each CA, **in full: the old 50% cap is disproven** (measured 2026-06-12: Quick Strike VI, base 215–359, tooltips a flat add of 818 at ability-mod 738 — the old rule would have capped it at ~283). A small measured per-art enhancer (`add ≈ AM × (1 + base_max/3400)`, ≤1% of tooltip on ranking) is documented but NOT modeled.

### Combat-art damage equation — measured 2026-06-12 (tooltip calibration, 4 gear/AA states × 3 probe arts)

```
CAdamage = base × (1 + (potency + potencyBonus + artPotencyAdd)/100)
                × (1 + mainStatCurve(mainstat)/100)
         + abilityMod
```

- **Main stat (AGI for scouts)** multiplies all CA damage via its own curve — **interpolated sample table from 13 committed live readings** (the curve flattens below ~600: readings at 73→6.08% and 156→15.01% sit under the high-range trend, so no global equation is assumed — `curveInterp` over the table, the multi-attack treatment). **Hard cap 1100 → 65% (user-confirmed).** High range (625–1100) happens to fit a quadratic peaking at ~1142 — the third "peak just past the cap" dev signature — recorded as commentary only. The AGI tooltip displays the conversion directly (*"Agility increases your damage by X%"*), so readings are cheap; the *"your damage"* wording may include auto-attack — **unverified, auto-attack NOT scaled** pending a combat-log test (§12).
- **Main stat is a gear stat**: census files "+N to primary attributes" under the `strength` key (3,248 items) — point-for-point AGI for a scout. The ~70 explicit single-attribute items (`agility`/`wisdom`/`intelligence`, mostly avatar mythicals) are data-suspicious and excluded from the mainstat mapping pending the data-fixing pass.
- **The potency pool**: displayed potency + per-character `potencyBonus` (calibrated; see the ⚠ mystery below) + per-art AA riders (`[art_mods]` `potency_add` — e.g. the cooldown AA grants Assassinate/Mortal Blade +15 potency each, per its own text).
- **⚠ OPEN MYSTERY (flagged at user insistence — see §12 and backlog):** a naked, AA-less, buff-less level-70 still swings CAs at `(1 + ~23.4/100) × (1 + agi%)` — **~23.4 hidden points in the potency pool that survive removing everything removable**. Measured across four states (23.4 no-AA naked / 24.6 naked / 24.4 full gear / 25.6 partial-gear outlier, suspected UI lag). Level-vs-art-level scaling is ELIMINATED (probe arts at native levels 57/63/66 share one multiplier to ±0.1%). Surviving hypotheses: combat-skill scaling at the 350 cap, or a flat level-70 innate (70/3 ≈ 23.3 — noted, not assumed). Until named, it is a calibrated per-character config value, prominently flagged in the report output. **It is not allowed to become furniture.**

### Reuse (combat-art recast %) — measured 2026-06, replaces the halved-coefficient guess
- **Conversion is full-strength: 1 point of reuse = 1% recast reduction, capping at 50 stat** (= the 50% ceiling). Measured: Eviscerate 60s base → **57.8s tooltip at 3.8 reuse** — the half-strength rule (`ReuseHalveCoeff = 0.50`, never directly verified) would have shown 58.9s; disproven and removed.
- **Each art has a 50% recast-reduction ceiling shared by ALL reduction sources.** Per-art AA reductions pre-fill it: Assassinate (AA-halved 300→150s) tooltips **exactly 2m30s with reuse gear equipped** — reuse adds nothing to an art already at its ceiling. `effRecast = base × (1 − min(0.50, artModReduction + min(reuse,50)/100))`. Supersedes the old "AA ×0.5, *then* reuse" stacking, which would have produced an impossible 75s Assassinate at high reuse.
- Affects the **CA timeline only** (recast → cast count). Consequence for weights: reuse's marginal now touches only arts below their ceiling (the 60s-and-under pool) — and since reuse can no longer shift Assassinate/Mortal Blade cast timing, the **boundary-drift quantization artifact** (converged-set reuse reading negative) should shrink or vanish; verify on the post-change report.
- **Reuse remains the most gear-state-dependent stat**: large on a bare character (fills idle), saturating as the rotation fills. Gear reuse is scarce in the EoF pool (~2/item).

### Cast speed & recovery speed (CA timeline stats) — added 2026-06, measured live
Two stats the model previously baked into constants; both now first-class `StatBlock` fields:
- **Cast speed** — **divisor rule, like haste**: `effCast = baseCast / (1 + castSpeed/100)`. Measured: Head Shot 2.0s base → **1.46s tooltip at 37.4% total** (= 2.0/1.374; the subtractive rule's 1.25s is ruled out). **Also a gear stat** (census key `spelltimecastpct`, 419 EoF items, 0.1–1.8% each) → ranked like any other stat; the sim derives its weight. **Cap unknown** (un-sampled above ~37%) — modeled uncapped, see §12.
- **Recovery speed** — **subtractive from the 0.5s base**: `effRecovery = 0.5 × (1 − min(recoverySpeed,100)/100)`. Measured: at 100% recovery speed tooltips read **"Recovery: Instant"** (a 0.25s divisor result would have displayed as 0.25). **Not a gear stat** in the EoF pool — config-only (AA-sourced). Replaces the hardcoded `CARecoverySecs = 0.25` ("base 0.5s halved by AA" — a guess; the real AA budget reaches 100%).
- Each cast's CA-timeline slot = `effCast + effRecovery`. Sub-second tooltips display at two decimals (e.g. 1.46) — no flooring evidence on the timing stats; treated continuous.

### Putting it together (formulas as implemented — `internal/model`)
```
critFactor   = 1 + (crit/100)·0.30
flurryFactor = 1 + (flurry/100)·3.0                  # gear flurry only (no haste→flurry)
dpsModFactor = 1 + curveHD(dpsMod)/100               # shared haste/dps-mod curve
effDelay     = weaponDelay / (1 + curveHD(haste)/100)
AutoDPS(w)   = (w.avgDmg / effDelay) · (1 + curveMA(MA)/100) · critFactor · flurryFactor · dpsModFactor
AutoDPSDual  = AutoDPS(main×1.33dly) + AutoDPS(off×1.33dly)   # dual-wield delay penalty on BOTH hands (§ below)
potPool      = potency + potencyBonus + artPotencyAdd   # potencyBonus: calibrated, ⚠ §12 mystery
CAeffective  = ((min+max)/2 · (1+potPool/100) · (1+curveMS(mainstat)/100) + abilityMod) · critFactor
effRecast    = base · (1 − min(0.50, artMod + min(reuse,50)/100))   # shared per-art ceiling
slot         = baseCast/(1 + castSpeed/100) + 0.5·(1 − min(recoverySpeed,100)/100)
CADPS        = RotationSim(arts, fight=600s)          # priority by CAeffective / slot
TotalDPS     = AutoDPSDual + CADPS                    # PARALLEL — CA casting costs zero auto swings
```

### Dual-wield delay penalty — measured 2026-06-13
Equipping an off-hand multiplies **each** weapon's auto-attack delay by **1.33** (`DualWieldDelayPenalty`), applied on top of haste and **independent of it**. Measured on the character sheet across two haste levels (Blood Fire base 6.0s, Shock base 4.0s):

| weapon | haste | sheet delay | implied penalty = sheet·(1+haste)/base |
|---|---|---|---|
| Blood Fire | 36% | 5.9 | 1.337 |
| Shock | 36% | 3.9 | 1.326 |
| Blood Fire | 60% | 5.0 | 1.333 |
| Shock | 60% | 3.3 | 1.320 |

Centroid **1.33**, flat across 36%→60% haste — confirming a delay multiplier, not a haste-effectiveness reduction (the penalty didn't shrink as haste rose). Independently corroborated: EQ2 documents a **+33% off-hand delay penalty**. Single-wield is unpenalized (Blood Fire reads 4.4s = 6.0/1.36 with the off-hand removed). The penalty is **detected, not assumed**: `AutoDPSDual` applies it only when a real off-hand weapon is equipped (`off.DelaySecs > 0`), so an empty off-hand or a non-weapon off-hand (shield/symbol) is correctly unpenalized, and a 2H weapon routes to single-weapon `AutoDPS` (no dual path). This matters for character import (backlog §1) — the model must read each imported loadout's actual wield state rather than presume the Assassin always dual-wields (it does today, but imported characters may not). The Assassin always dual-wields, so it always applies here — scaling the auto term ~0.75× uniformly and shifting the auto-vs-CA balance toward CAs (auto stats — haste/MA/flurry/dps-mod — weigh less; CA stats more). It is uniform on both hands, so it does **not** reorder the off-hand candidate pool. *Supersedes the earlier note that the off-hand's only penalty was the un-tracked weapon-multiplier stat.*

### Combat-art rotation / recovery
- **Recovery time** is real and folded into CA pacing: each cast occupies `effCast + effRecovery` of the **CA** timeline before the next cast (weapon DPS amortizes over full delay; CA throughput must amortize over the full slot). Both timing stats come from the character config (§4) — e.g. at 37.4% cast speed and 100% recovery speed, a 0.5s art occupies `0.36s + 0s`. Recovery is a flat per-cast add for CA-vs-CA ranking, but **not** a common factor for CA-vs-auto, because it sizes total CA damage over the fight.
- **Per-art recast reductions (config `[art_mods]`):** AA effects that reduce a specific art's recast — Assassinate ×0.5, Mortal Blade ×0.5 today (300→150s, 180→90s). These **count against the art's shared 50% reduction ceiling** (§ Reuse above), so reuse cannot stack on top of a full AA halving.
- **Rotation simulation (CADPS):** a discrete priority sim over the fight. Each slot, fire the off-cooldown art with the highest **damage-per-cast-time** (`CAeffective / slot`), advance time by that art's slot, set its `effRecast`; idle-jump when nothing is up. Priority is by damage-per-**time**, not raw damage, because a slow high-damage art can be worse per second than a fast one. Only fired casts count.
- **Art pool (`internal/spell`):** Assassin Expert-tier arts, **level ≥ 57** (`minDamageArtLevel` — below is vestigial low-level filler), damaging (parseable `effect_list` damage), **not beneficial** (buffs/stances excluded), **highest rank per base name**. **Ranged bow shots ARE kept** — no minimum range, zero melee-auto cost, so they're free CA damage that fills idle (Head/Spine/Deadly Shot). Low-level *scaling* arts that census files at base level (Hilt Strike, Strike of Consistency) are NOT yet included — see backlog §3.
- **Idle is structural:** with the real recasts the CA timeline sits idle ~45–50% of a long fight (cooldown-bound, not cast-bound); auto-attack fills the gaps in parallel — correct, not a bug. (Adding the 3 ranged shots cut idle ~52%→46% and raised total DPS ~6.7%.)
- **Stealth — currently assumed free:** many arts require stealth ("must be sneaking"); the model assumes it's always available. The real economy (stealth breaks on any CA cast; granters = Masked Strike / Stalk / the 7s-burst Concealment) is parked in backlog §4 — it only sharpens reuse's exact weight, it does not change gear picks.

### Weight derivation under non-linear stats
**Multi-attack** (still a sample table) takes its marginal weight from the **sample-to-sample slope** at the baseline — evaluate DPS at the sample points bracketing the baseline and divide by the stat gap. At sample points the floored effect equals the table value exactly, so this yields the true segment slope with no flooring noise (the floor makes real gains lumpy, but the per-point weight should read as the smooth "going rate").

**Haste/dps-mod** (fitted equation — no sample brackets) generalize the same trick to anywhere on the curve: the marginal is the DPS slope between the fitted curve's **adjacent integer-effect crossings** bracketing the baseline — endpoints land exactly on whole-percent effects, so the floor contributes no noise. Marginals clamp to 0 at the 300 cap, and are legitimately 0 in the **dead zone** past the last integer crossing (≈289, where `f` can no longer reach the next whole percent before the cap) — the floored in-game effect genuinely cannot improve there.

**Main stat** (sample table, like multi-attack) brackets between its table samples and clamps to 0 at the 1100 cap.

Linear stats use the standard +1 finite difference — except **cast speed**, which uses a **wide +10 difference** (slope over `[v, v+10]/10`). Diagnosed 2026-06: a +1 castspeed nudge shifts the rotation's decision lattice and can slide one long-cooldown cast across the 600s fight boundary, so the +1 diff reads ±(one cast's DPS) of quantization noise (observed ±10 at a converged raid set) while the genuine trend is ~0.05–0.5/pt — noise dwarfs signal. The wide bracket averages ~10 lattice shifts; same artifact family as the old reuse boundary drift.

**Reuse** uses a **centered, cap-clamped span** for the same reason (added 2026-06-12 after the CA-equation revision tripled per-cast damage): slope over `[max(0, v−4), min(50, v+4)]` divided by the actual clamped width. The +1 diff's noise is ±(one cast's DPS) ≈ ±50 at a converged raid set while the genuine marginal is ~74/pt (measured by sweep + analytic cross-check: ceiling-filled arts contribute reuse-insensitive DPS; the cooldown-bound remainder scales as `1/(1−r/100)`) — signal no longer dominates single-point reads. Centered-not-forward because converged baselines sit at 40–48, where a forward span would average the past-the-cap dead zone into the slope; the span reads a secant of a convex curve (slightly below the tangent), accepted — gear delivers reuse in ~2-pt chunks, so the secant is the operationally honest rate.

### Removed / changed constants
- **Removed:** `HasteCapPct` (100), `HasteToFlurry` (10:1), `DPSModEffectAtCap` (1.25), the haste→flurry term, the linear dps-mod form, (2026-06 refit) the piecewise haste/dps-mod sample table with its `(200,125)` anchor, and (2026-06 rotation revision) `ReuseHalveCoeff` (0.50) / `ReuseHalvesAt` (100) / `CARecoverySecs` (0.25) — all three disproven by live tooltip measurements.
- **Added:** the multi-attack interpolated+floored sample curve; the **fitted** haste/dps-mod equation (form + parameters derived by `cmd/fitcurve` from `data/curve-readings.csv`); `HasteStatCap` = 300; `DPSModCap` = 300; `ReuseCapStat` = 50 (1%/pt to the 50% ceiling); `RecastReductionCeiling` = 0.50 (per-art, shared by AA + reuse); `CARecoveryBaseSecs` = 0.5 (server base, reduced by the character's recovery-speed stat); `StatBlock.CastSpeed` / `StatBlock.RecoverySpeed`; per-art recast multipliers moved from a hardcoded map into config `[art_mods]`.
- **Changed:** flurry ×5 → **×4**; dps-mod → the **shared fitted diminishing curve** (hard cap 300), not linear; reuse → full-strength 1%/pt (was half-strength).
- **(2026-06-12 CA-equation revision)** — **Removed:** `AbilityModCapFrac` (0.50 — disproven by tooltip probes). **Added:** the main-stat interpolated sample table (13 readings, cap 1100 → 65%); `StatBlock.MainStat` / `StatBlock.PotencyBonus`; `spell.CombatArt.PotencyAdd` (config `[art_mods]` rider).
- **(2026-06-13 dual-wield revision)** — **Added:** `DualWieldDelayPenalty` = 1.33 — multiplies each weapon's delay in `AutoDPSDual`, applied **only when an off-hand weapon is present** (`off.DelaySecs > 0`), so the penalty is detected from the loadout, not assumed (measured 4 readings + documented +33%; § Dual-wield delay penalty).

---

## 4. Character Configuration & Contexts (replaces the hardcoded baselines)

Per-character inputs live in a **TOML config file** (`characters/<name>.toml`, loaded by `internal/charconfig`, selected via `-character` on `cmd/weights`/`cmd/bis`), replacing the `internal/baseline` constants block. The **code/config boundary**: server-wide mechanics (crit ×1.30, flurry ×4, the fitted curve, the reuse/cast/recovery conversion rules) stay in code — they're identical for every player; everything about *a* player and *their group* is config. The schema is **class-agnostic** (a `class` field exists; only `assassin` is implemented today — other classes are a future art-pool problem, not a schema break).

```toml
[character]
name  = "Alex"
class = "assassin"
art_tier = "expert"

[stats]                      # character-permanent: AA + innate bonuses (NOT gear, NOT buffs)
cast_speed     = 37.4
recovery_speed = 100
mainstat       = 156         # innate + AA agility (naked-with-AAs reading)
potency        = 5           # AA potency (displayed in the character window)
potency_bonus  = 24.6        # calibrated hidden potency-pool points (⚠ §12 open mystery; naked-tooltip procedure in §3.1)

[art_mods."Assassinate"]     # per-art AA effects
recast_mult = 0.5            # counts against the 50% recast ceiling
potency_add = 15             # the same AA's potency rider, per its own text
[art_mods."Mortal Blade"]
recast_mult = 0.5
potency_add = 15

[contexts.solo]              # each context = the FULL buff package on you in that situation
multiattack = 34.2           # Villainy (maintained self-buff — listed in every context it's up)
[contexts.raid]
multiattack = 34.2
dpsmod      = 114.2          # coercer 74 + inquis 30.2 + dirge 10 (live estimate)
critchance  = 31.0           # buffed estimate; split AA portion into [stats] when measured
```

**Decomposition rule — every number has exactly one home:** `[stats]` = what the character *is* (AAs/innate); `[contexts.X]` = what's *cast on them* there (self-buffs included in every context where active — a context is a literal, line-by-line match for the in-game buff bar); **gear** = the optimizer's output, never config. A model run's input stat block = `[stats]` + one context + the gear set under evaluation.

**AA normalization taxonomy:** AAs are (a) **global stat grants** → `[stats]`; (b) **per-art modifiers** → `[art_mods]` (today: `recast_mult`; the schema reserves `damage_add`/`damage_mult`/`cast_mult` for future AAs without a break); or (c) **mechanic-changers** (e.g. Concealment) → out of config scope, modeled in code or backlogged.

**Validation is strict**: unknown stat keys and malformed values are errors (a typo'd stat must not silently vanish); `art_mods` names are checked against the loaded art pool downstream (a typo'd "Assasinate" fails loudly rather than silently un-halving the big hit); `class` ≠ assassin → clear unsupported-class error.

Each context yields its own derived weight set → its own BiS list. The **cross-context difference is an output** (it shows which stats change between contexts), not an assumption. Group package provenance (raid context): live comp 2026-06, Coercer 74 + Inquisitor 30.2 + Dirge 10 = 114.2 dps-mod — supersedes the dead "buffs reach the cap → 200" assumption; the Coercer reading (74) exceeds the old census-derived Velocity IV value (57.6), likely a higher rank on the new server; refine per component as readings firm up.

### Accessibility tiers (report structure)

The report segments into **three accessibility tiers**, each = one config context + a gear keep-filter (`internal/bis`):
- **PRE-RAID** — `solo` context; only `LEGENDARY`/`TREASURED` items (dungeon-accessible); no avatar/Hunter's.
- **RAID** — `raid` context; all items **minus** avatar mythicals and the Hunter's set.
- **BEST-OF-BEST** — `raid` context; all items minus the Hunter's set (avatar mythicals **kept**).

Exclusion predicates: **avatar** = `MYTHICAL` except the Soulfire; **Hunter's** set excluded everywhere; plus a small **curated** exclude list. The main-hand is pinned to the **Soulfire Sabre** (its multi-attack beats the Gladius's block) and its full stat line folds into every tier's baseline; the off-hand pool is all 1H weapons **except** Soulfire (the player gets exactly one Soulfire — the fixed main-hand).

Config numeric values are the user's data, not spec — current-best live estimates with provenance comments in the TOML, refined whenever better readings land (no spec change needed).

---

## 5. Architecture / Components (Go)

Reuse Plan-1 packages; add:

| Package | Responsibility |
|---|---|
| `internal/census` (extend) | add `Skill` + `WieldStyle` to `TypeInfo` |
| `internal/catalog` (extend) | add `skill`/`wieldstyle` CSV columns |
| `internal/spell` *(new)* | pull Assassin CAs (Expert, ≤70); regex damage from `effect_list`; apply TLE translations |
| `internal/model` *(new)* | DPS equations, marginal-weight derivation (iterated), item scoring, per-slot ranking, **locked-items** constraint (§8) |
| `internal/charconfig` *(replaces `internal/baseline`, 2026-06)* | TOML character config: AA stats, `[art_mods]`, buff contexts (§4); `internal/constants` keeps the server-wide combat constants |
| `internal/store` *(new)* | `modernc.org/sqlite` — schema, load gear + per-baseline scores, ranking/coverage queries |
| `internal/bis` *(new)* | accessibility keep-filters + exclusions, converging set-builder (coordinate ascent), per-slot report + progression render |
| `cmd/bis` *(new)* | orchestrate: load gear (from CSV cache) → pull CAs → build/score per tier (3 accessibility tiers over the 2 stat baselines) → load SQLite → emit report + DB. Flags: `--out`, `--lock` (§8), `--db`, `--top`. |

Data flow: gear (Plan-1 cache) + Assassin CAs + character config → `model` derives weights per context and scores each usable item → `store` loads gear + scores into SQLite → `cmd/bis` runs ranking SQL and renders the markdown report.

---

## 6. SQLite Schema (modernc, pure-Go)

**Normalized schema:**
- `items` — one row per gear item: `id`, `name`, `slot`, `tier`, `itemlevel`, `armor_type`, `skill`, `wieldstyle`, `classes`, `gamelink`, plus weapon fields `weapon_min_dmg`, `weapon_max_dmg`, `delay`, `damage_rating`.
- `item_stats` — `(item_id, stat, value)`, one row per modifier (friendly for SQL aggregation / scoring).
- `combat_arts` — `(name, level, min_dmg, max_dmg, recast_secs, cast_secs_hundredths)`, the pulled Assassin arts that drive CADPS.
- `scores` — `(item_id, baseline, dps_score, slot)` so a single query ranks per slot per tier (the `baseline` column holds the accessibility-tier name), and the user can sort/filter/explore (coverage gaps, runner-ups, etc.) freely.

The DB is both an analysis engine and a shareable artifact.

---

## 7. Outputs

- **`bis-report.md`** — for each of the three accessibility tiers (pre-raid / raid / best-of-best): the **derived stat-weight table**, then per slot the converged **BiS pick(s)** plus the **top-N merged alternatives** ranked by in-context ΔDPS, each tagged with rarity (and `· avatar` where shown). The fixed **Primary** (Soulfire Sabre) renders as `(fixed)`. A **per-slot progression** section then summarizes the top pick per tier. Closes with the **assumptions/constants block**. *(The original design listed top-3-Fabled + top-3-Legendary per slot with Mythical shown as ceiling; superseded by this merged-top-N + 3-tier + progression layout in plans 2f–2i, which also excludes avatar/Hunter's per tier rather than showing every Mythical.)*
- **Every ranked item shows a score *breakdown*** — its contributing terms as `stat × weight` (e.g. `crit 2 × 5.67 = 11.3`), not just the total. This makes each ranking **explainable**, which is the point of §9: an expert can see *why* an item placed where it did and immediately spot a wrong weight/constant.
- **`bis.db`** — the scored SQLite DB (the `scores.baseline` column holds the tier name).

---

## 8. Set Bonuses — constrained re-model (human-driven)

Set bonuses are **not scored or cataloged** — the player judges their value from game knowledge. They're handled as an **iterative constrained re-model**:

1. Plan 2 first produces the **unconstrained** stat-based BiS (+ top-N per slot).
2. The user reviews set bonuses (with the assistant) and decides which set + how many pieces is worth it.
3. The model is **re-run with those N pieces locked** into their slots (`--lock <item-id>,…`), re-optimizing the *remaining* slots around them.

This requires one model capability: a **locked-items input** — force specific items into their slots, optimize the rest. The user supplies the locked item IDs from their own knowledge, so **the tool never needs set-membership data**. The capability is generally useful ("I already have item X — best build around it?"). Set-bonus *value* remains the user's subjective call.

---

## 9. Validation

Two layers, with different jobs. The unit tests prove the code computes the equations correctly; **the expert review proves the *model* is right** — and the second is the one that matters most, because years of play experience can judge a BiS list at a glance.

**Primary — expert review of explainable output (the main validation path).**
The report is built to be eyeballed against experience. The per-item **score breakdown** (§7) makes every ranking transparent, so a result that contradicts domain knowledge points *directly* at the culprit — a mis-weighted stat, a wrong constant, or a bad translation. The loop:
> generate report → expert reviews against experience → flags any ranking that "feels wrong" → the breakdown shows which stat/weight caused it → fix that input/constant → re-run.
This is iterative and human-in-the-loop by design. The model is "validated" when its rankings stop surprising the expert (and the surprises that remain are genuine insights, not bugs).

**Secondary — provable mechanics tests (prove the math, not the answer).**
No pre-assumed "item X ranks top" anchors — that's the "Grinning Dirk" mis-step from Plan 1. Instead:
- CA `effect_list` parser (known CA text → expected damage numbers).
- DPS equations (hand-computed input → expected output).
- Weight derivation + scoring on synthetic items with known stats.
- Hand-calc spot-check: recompute a couple of *real* items' DPS by hand, confirm the model matches.

The **cross-tier diff** (how weights and picks shift between pre-raid / raid / best-of-best) is reported as an output and sanity-read — never asserted in advance.

---

## 10. Implementation Notes (Go)

- `modernc.org/sqlite` (pure-Go, cgo-free) via `database/sql`.
- Reuse Plan-1 idioms (throttled client only needed for the CA pull + the re-pull; `slog` progress; testify tests; `golangci-lint`).
- The CA pull is small (a few dozen Assassin CAs) — one throttled query batch, not the full-catalog ordeal.
- Keep `model` pure/deterministic (no I/O) so the DPS math is unit-testable in isolation.

---

## 11. Assumptions & Constants Block (single source of truth)

Server-wide mechanics live in `internal/constants` + `internal/model`; per-character values live in the TOML config (§4):

- **Combat constants (see §3.1 for the authoritative, revised mechanics):** crit ×1.30; **flurry ×4**; **haste & dps-mod** share a **fitted diminishing curve** (equation derived from `data/curve-readings.csv`; hard cap **300 stat**, effect at cap = `f(300)`, auto-attack only); **multi-attack** has its own gentler diminishing curve (runs to 3400 with triple overcap); **haste overcap does NOT convert to flurry**; **ability-mod uncapped** (old 50% cap disproven 2026-06-12); **main stat (AGI) multiplies CA damage** via an interpolated 13-reading table, hard cap 1100 → 65% (gear source: census `strength` key = "+N primary attributes"); CA potency pool = displayed potency + calibrated `potency_bonus` (**⚠ §12 mystery**) + per-art `[art_mods]` riders; **reuse 1%/pt capping at 50 stat, sharing each art's 50% recast ceiling with `[art_mods]` reductions**; **cast speed divisor** / **recovery subtractive from 0.5s base** (both from config; cast speed also on gear); **dual-wield delay penalty ×1.33 on both weapons** (auto-attack only, dual path); potency on CAs only; non-primary attributes still excluded.
- **Resolved (2026-06 curve refit):** cap = **300** confirmed (former-dev statement + readings growing past 238 and flattening by 281); the patch-note `(200 → 125%)` anchor **disproven** by `haste 281 → 124%` (monotonicity); curve re-derived as a fitted equation per §3.1.
- **Resolved (2026-06 rotation revision):** reuse half-strength coefficient **disproven** (Eviscerate 57.8s @ 3.8 reuse); AA-then-reuse stacking **disproven** (Assassinate pinned at 2m30s with reuse gear → shared 50% ceiling); recovery "halved by AA" guess replaced by the measured recovery-speed stat (100% → "Recovery: Instant"); cast speed measured as a divisor (Head Shot 1.46s @ 37.4%).
- **Rotation (as implemented):** CADPS = priority sim (fire highest **damage-per-cast-time** off-cooldown art; 600s fight; structural idle, auto fills it — idle % shifts with the measured timing stats; re-derive on the post-change report). Art pool = Expert, **level ≥ 57**, damaging, non-beneficial, highest-rank, **ranged shots included**. Stealth assumed always available (real stealth-grant economy parked — backlog §4).
- **TLE translations:** `doubleattackchance` → **Multi-Attack** (legacy key; displayname already "Multi Attack"); `spelltimecastpct` → **Cast Speed** (gear stat); `strength` → **Main Stat** ("+N primary attributes" — AGI point-for-point for a scout; explicit `agility`/`wisdom`/`intelligence` keys data-suspicious, excluded pending fixing); `critbonus` → **ignored entirely**; Fervor → does not exist.
- **CA tier:** Expert (classic "Adept III" — the farmable raiding baseline); config `art_tier` field reserved.
- **Character/contexts:** all per-character values (AA stats, art mods, buff packages incl. raid group DPS-mod **114.2**) live in `characters/<name>.toml` — config is data with in-file provenance comments, refined without spec changes.

---

## 12. Open / To-Confirm (non-blocking)

- **⚠⚠ THE POTENCY-POOL MYSTERY — flagged for active hunting, not acceptance.** ~23.4 hidden potency-pool points on a naked, AA-less, buff-less level-70 (§3.1 measurement ledger). Eliminated: gear, AAs, buffs, level-vs-art-level scaling. Surviving suspects: combat-skill scaling at the 350 cap; a flat level-70 innate. **Kill experiments:** (a) a different-level character (alt or guildmate) runs the naked five-number protocol — if the constant tracks level (or skill cap = 5×level), it's named; (b) any skill-debuff opportunity. Mentoring is confounded (its own penalties). Until named, it lives as calibrated config (`potency_bonus`) and is printed in every report's assumptions. **Do not let this become furniture.**
- Config numeric values (buff packages, AA stats) — config is data; refinement is an edit + re-run. The raid crit estimate (31) still conflates AA crit with buff crit — split into `[stats]` vs `[contexts.raid]` when measured.
- **Main-stat curve gaps**: no readings in 156–625 (curve shape between the flat low range and the quadratic-like high range is interpolated; marginal weights for any baseline in that range ride the gap's 469-point secant — a mid-gap reading ~350–400 would pin it); a deliberately overcapped reading (AGI > 1100) to verify the clamp; whether the AGI multiplier also applies to **auto-attack** (*"increases your damage"* wording — needs a combat-log session; auto NOT scaled until verified).
- **Per-art potency riders unverified numerically**: the cooldown AA's "+15% potency to Assassinate/Mortal Blade" is modeled from its text; the regeared Mortal Blade tooltip read (~13.8k max predicted vs ~12.8k without the rider) confirms or refutes.
- **Cast-speed cap unknown** — measured as a divisor at 37.4%; un-sampled above. Modeled uncapped; grab tooltip readings if AA respec/gear pushes it higher (the haste-curve lesson: don't trust received caps).
- **Haste/dps-mod curve — remaining gaps** (cap = 300 and the refit itself are resolved, §3.1): readings in 153–238 and 281–300, the effect at exactly 300, and one overcapped pair to verify the clamp. Append to `data/curve-readings.csv` + re-run `cmd/fitcurve`; gatherable any time via buff/CD-stacking. Also keep watching the haste-vs-dpsmod separate-fit residuals as data lands — split the shared curve if they diverge.
- **Reuse weight under the measured rules** — now full-strength per point but blind to ceiling-filled arts (Assassinate/Mortal Blade); the old negative converged-weight artifact (boundary drift on the AA-halved arts) is predicted to shrink/vanish — verify on the post-change report.
- **Art-list audit pending** — diff the census Expert-tier pool against the live in-game skill list (ranks, missing/extra arts, Hilt Strike / Strike of Consistency level-70 damage — backlog §3). Deferred from the 2026-06 rotation revision by user choice.
- **Parked rotation-realism** (`docs/backlog.md`): character-pull seeding (§1), lore-equip doubling (§2), manual scaling arts (§3), stealth-grant modeling (§4), launch-day gear-cache re-export (§5).
