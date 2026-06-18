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

`TotalDPS = AutoDPS + CADPS`, computed in **parallel** — auto-attack and combat-art casting run on independent timelines, so casting does **not** displace auto swings (a CA's cast+recovery only paces the *CA* side; see §3.1). Locked combat values: crit ×1.30; **flurry ×4** (a flurry proc does +100%–500%, averaging +300% = ×4); **ability-mod applies in full — the old 50% cap is disproven** (§3.1); reuse converts at **1%/point capping at 50 stat**, sharing each art's **50% recast-reduction ceiling** with AA reductions (§3.1); **potency applies to CAs only**, pooled with a **calibrated hidden bonus (Wuoshi TLE damage adjustment, §12)**; **main stat (AGI) multiplies all CA damage** via its own capped curve (§3.1); `critbonus` ignored. Haste, multi-attack, and dps-mod are **non-linear** (§3.1) — *not* `1 + stat/100`. Haste & dps-mod share one **fitted** diminishing curve (hard cap **300** stat; the old `200 → 125%` anchor is disproven — see §3.1); multi-attack has its own gentler curve (runs to 3400 with triple overcap). Stat-conversion mechanics, the AA cooldown/recovery effects, and the curve table are in **§3.1** (the authoritative stat-mechanics block, revised from the Varsoon playtest session).

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

- **Main stat (AGI for scouts)** multiplies all CA damage via its own curve — **interpolated sample table from 13 committed live readings** (the curve flattens below ~600: readings at 73→6.08% and 156→15.01% sit under the high-range trend, so no global equation is assumed — `curveInterp` over the table, the multi-attack treatment). **Hard cap 1100 → 65% (user-confirmed).** High range (625–1100) happens to fit a quadratic peaking at ~1142 — the third "peak just past the cap" dev signature — recorded as commentary only. The AGI tooltip displays the conversion directly (*"Agility increases your damage by X%"*), so readings are cheap. The *"your damage"* wording **does** include auto-attack — confirmed 2026-06-13 via `/weaponstat` (see the auto-attack weapon-damage equation in §3.1): the **same** `mainStatCurve` scales both CA and auto-attack damage.
- **Main stat is a gear stat**: census files "+N to primary attributes" under the `strength` key (3,248 items) — point-for-point AGI for a scout. The ~70 explicit single-attribute items (`agility`/`wisdom`/`intelligence`, mostly avatar mythicals) are data-suspicious and excluded from the mainstat mapping pending the data-fixing pass.
- **The potency pool**: displayed potency + per-character `potencyBonus` (calibrated; see the ⚠ mystery below) + per-art AA riders (`[art_mods]` `potency_add` — e.g. the cooldown AA grants Assassinate/Mortal Blade +15 potency each, per its own text).
- **Hidden potency-pool bonus — SOURCE IDENTIFIED (§12):** a naked, AA-less, buff-less level-70 still swings CAs at `(1 + ~23.4/100) × (1 + agi%)` — **~23.4 hidden points in the potency pool that survive removing everything removable**. Measured across four states (23.4 no-AA naked / 24.6 naked / 24.4 full gear / 25.6 partial-gear outlier). It's a **Wuoshi TLE server-applied damage adjustment** (the TLE lineage's dev damage-balance pass), which is exactly why it survives stripping gear/AAs/buffs — it's not a stat. Captured empirically as the calibrated `potency_bonus` config value (~24.6); no published scalar, doesn't need one. Per-class-vs-uniform is the only open question (cross-class mage read; §12). Level-vs-art-level scaling was ruled out (probe arts at native levels 57/63/66 share one multiplier to ±0.1%).

### Multi-component abilities — components, agnostic behaviors, ability-mod rule (CALIBRATED 2026-06-15)

Many abilities deal damage through **more than one component**, and the model treats this **agnostically**: an ability is parsed into typed components, each component is simmed with the existing per-component equation, and the results sum to that ability's total damage. No ability is special-cased — "Gushing Wound" is simply an art that happens to carry a direct hit + a DoT + a termination. The pipeline generalizes: **query → parse into parts → sim each part → sum to the ability's total**, so future CAs/spells slot in by their component shape.

**Component taxonomy** — extracted from the census `effect_list`, which is **raw / pre-calculation** and therefore the trustworthy structural source. The in-game tooltip *merges and splits* lines based on live calculations (it fooled the original hypothesis — see below) and cannot be parsed structurally. Census encodes parent/child via an `indentation` field and exposes a structured `duration` (`sec_tenths/10`):

- **DirectHit** — `Inflicts A-B <type> damage on target` (no "every"). One instant application.
- **DoT** — `Inflicts A[-B] <type> damage on target [instantly and] every N seconds`, over the art's `duration`. Applications = `(hasInstant ? 1 : 0) + floor(duration / N)`. Two shapes occur: with-instant and periodic-only (periodic-only often shows a single value, not a range).
- **Termination** — `Applies <Spell> on termination` with the triggered spell's damage **inlined as the indented child** (ind+1) — no sub-pull needed. Fires once when the duration expires.
- **TriggerProc** (`Grants a total of N triggers` + inlined damage) and **RateProc** (`… ~N times per minute` + inlined damage) — **parsed and stored, not yet scored** (deferred below). Damage is inlined for the Assassin pool, so no sub-pull blocker.

**Per-component damage** is the existing direct-CA equation applied per component and summed (a DoT component contributes per-application damage × application count):

```
ability damage = Σ over components [ base_i × (1 + (potency + potencyBonus + artPotencyAdd)/100)
                                            × (1 + mainStatCurve(mainstat)/100)
                                      + abmod_i ] × critFactor
```

— reusing the already-calibrated potency pool, `mainStatCurve`, and crit unchanged.

**Ability-mod rule — placement is PER-MECHANIC (measured 2026-06-15 via live census + tooltip calibration; the prior "~50% splits onto a piercing line" hypothesis is DISPROVEN):**

> Ability mod applies only to **detrimental** abilities, and *where* it lands depends on the component shape:
> - **DirectHit** → **full** abmod (Gushing Wound's front-load melee absorbed the entire 694 as a flat add — range width preserved, so a flat add not a multiplier).
> - **DoT ticks (instant + periodic) and Termination/detonate alongside a DirectHit** → **zero** abmod (they scaled purely multiplicatively).
> - **Triggers (no DirectHit)** → **≈ half abmod per trigger** (Death Mark: k≈0.48 measured across abmod 0/376/694; ×5 triggers ≈ **2.4× abmod** per cast).
> - **Beneficial abilities** (self/ally buffs, e.g. Apply Poison's proc) → **no** caster abmod (and they're dropped by `beneficial != 0` anyway).

So `abmod_i` is per-component: full for a DirectHit, `0.5 × abmod` per trigger for a TriggerProc, `0` for DoT ticks / Termination. The "piercing line" that seeded the old hypothesis was never ability mod — it's the **instant tick of the piercing DoT**, which the tooltip splits out only when an overtime-potency AA inflates the periodic ticks above the instant (census shows instant+periodic as one component). **No ability-mod cap** observed up to 694. **Uncalibrated (no clean Assassin vehicle):** a *detrimental* DoT-with-instant that lacks a standalone DirectHit — hypothesis is abmod → the DoT's instant application (the Hemotoxin test for this was invalidated: it's beneficial + ally-applied + a poison proc, so its zero is uninformative). Deferred.

**Component bases — source decision (2026-06-15).** Each art's **highest-rank** census entry (collapsed via `spell.HighestRanks`; numerals vary — Gushing Wound VI, Death Mark IV, Impale V, Quick Strike VI) carries level-70-appropriate component bases, already loaded into `combat_art_components` by Increment A. The **rotation sim uses those highest-rank census bases**; the **Gushing Wound calibration test uses the naked-recovered bases** as ground truth. They mostly agree (Gushing Wound VI melee 49–83 ≈ recovered 49.2–82.5) but **piercing/detonate run ~7% higher in census** than the naked reads imply — flagged as a known reconcile-later item, not a blocker. Recovered ground-truth bases: Gushing Wound front-load melee **49.2–82.5**, piercing (instant+periodic) **69.6–116.5**, detonate **326.1–543.2**; Death Mark per-trigger **329–545**.

**Rotation behavior (agnostic, revised 2026-06-15).** Clip-vs-hold is decided by **one switch — does the art have a Termination component**:
- **No Termination → CLIP.** Pressed on its `effRecast` cooldown like any DirectHit art (the priority sim recasts it when its initial hit is the next-best cast). Each cast delivers its DirectHit + the periodic ticks inside the cast window: `(hasInstant ? 1 : 0) + floor(min(effRecast, duration) / interval)` applications. The running DoT is refreshed on recast → it **never terminates** (no detonate; these have none anyway).
- **Termination → HOLD to full duration.** Cadence = `max(effRecast, duration)`; the cast delivers DirectHit + full-duration ticks + the **detonate**, which fires precisely *because* the DoT runs to termination. **Manually calibrated** (Gushing Wound is the worked example). Gushing Wound's `effRecast` currently exceeds its 24s duration so it terminates regardless — but that is incidental to *this reuse stat*, **not** a law; the law is "hold iff it has a termination effect."

The **detonate/termination component is conditional**: counted iff the DoT completes within the cast cadence — always true for held termination-DoTs, never for clipped ones (so when a clipped DoT's cadence math changes with reuse, the detonate correctly stays absent). The clip-vs-hold *optimization* for termination-less DoTs (could holding ever beat clipping?) and per-class variants are backlog §10.

**Implementation lands in two increments:** (A) parser + structured component data model + DB schema (splits abilities into parts; documents how abilities work) — **shipped, plan 2q**; (B) the agnostic damage/rotation sim (each part simmed, summed) + per-mechanic abmod placement + detrimental **TriggerProc scoring (Death Mark)** + clip-vs-hold rotation + Gushing Wound & Death Mark calibration tests.

**In scope for Increment B (now calibrated):** DirectHit / DoT / Termination per-component damage; abmod placement (full DirectHit, ½×N triggers, zero DoT/Termination); detrimental TriggerProc scoring with "all N triggers land" (Death Mark, ×5); clip-default / hold-on-termination rotation.

**Deferred (own calibration / spec, not Increment B):**
- **RateProc scoring** (`~N/min` procs) — needs proc-rate-over-fight modeling; and beneficial/ally procs never score regardless.
- **Detrimental DoT-with-instant lacking a DirectHit** — abmod placement on its instant is unmeasured (no clean Assassin vehicle; Hemotoxin invalidated as beneficial+poison+proc).
- **Clip-vs-hold optimization** for termination-less DoTs + per-class variants (backlog §10) — Increment B uses the fixed clip-default / hold-on-termination rule.
- **Overtime-potency / per-skill behavior AAs** (the ~×1.14 Gushing Wound DoT bump) — build-state modifier, ranking-neutral; future "AA type-system" spec.
- **AoE multi-target** components (`… on targets in Area of Effect`).

### Reuse (combat-art recast %) — RECALIBRATED 2026-06-18: a DIVISOR (not subtractive), floored at 50% of base
- **Reuse is a DIVISOR, like haste and cast speed:** `recast = base / (1 + reuse/100)`. Measured across six Eviscerate (60s base) tooltip points from 2.4%→61.8% reuse — all fit `60/(1+reuse/100)` to a hundredth of a second (2.4→58.6, 10.3→54.4, **61.8→37.1** = 60/1.618). Supersedes the old subtractive "1 pt = 1% reduction, cap 50 stat" rule, which had a single low data point (3.8→57.8) where divisor and subtractive are indistinguishable (`60/1.038 = 57.80`); the 61.8% point breaks the tie (divisor 37.1 vs capped-subtractive 30.0).
- **Hard floor at 50% of base recast, reached at reuse = 100%** (`base/(1+100/100) = base/2`). Confirmed by Assassinate: its cooldown AA alone drops 300→150s (= exactly 50% of base = the floor) and it stays pinned there regardless of reuse ("capped to all hell"); Mortal Blade likewise (180→90). The AA recast reduction puts those arts at the floor; reuse cannot go below it.
- **Combined:** `effRecast = max(0.5·base, base·(1 − artModReduction) / (1 + reuse/100))`. (The AA-reduction × reuse interaction for a *partial* AA art is uncalibrated — only the endpoints are measured: no-AA Eviscerate, and full-AA-floored Assassinate/Mortal Blade. No real art is partial today.)
- **No reuse-stat cap.** Reuse never hard-saturates below 100% reuse — it keeps cutting recast with diminishing returns (divisor) until the 50% floor. A near-floor raid build still gains a small positive marginal from reuse; only at/above 100% reuse is non-AA reuse value dead. *(Corrects the earlier "reuse weight collapses at the 50-stat cap" claim — there is no 50-stat cap; the cap is 50% of base recast at 100% reuse.)*
- Affects the **CA timeline only** (recast → cast count). Reuse remains the most gear-state-dependent stat.

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
autoFactor   = (1+curveMS(mainstat)/100) · dpsModFactor · classAutoMult   # weapon-damage multipliers (§ below)
AutoDPS(w)   = (w.avgDmg/effDelay) · (1 + curveMA(MA)/100) · autoFactor · critFactor · flurryFactor
AutoDPSDual  = AutoDPS(main×1.33dly) + AutoDPS(off×1.33dly)   # dual-wield delay penalty on BOTH hands (§ below)
potPool      = potency + potencyBonus + artPotencyAdd   # potencyBonus: calibrated, ⚠ §12 mystery
CAeffective  = ((min+max)/2 · (1+potPool/100) · (1+curveMS(mainstat)/100) + abilityMod) · critFactor
effRecast    = max(0.5·base, base·(1 − artMod) / (1 + reuse/100))   # reuse DIVISOR (recalibrated 2026-06-18), floored at 50% of base
slot         = baseCast/(1 + castSpeed/100) + 0.5·(1 − min(recoverySpeed,100)/100)
CADPS        = mean over K lengths in [L−R/2, L+R/2] of cumCA(t)/t   # fight-length smoothed (§ below); L=target (default 600), R=longest eff recast
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

### Auto-attack weapon-damage equation — measured 2026-06-13 (`/weaponstat` + 3 gear states)
A weapon's per-swing damage is the census raw base times three multipliers:

```
per-swing avg = census_avg × (1 + curveMS(mainstat)/100) × dpsModFactor × classAutoMult
```

- **Main stat (AGI)** scales auto-attack at the **same** `mainStatCurve` used for CAs — the AGI tooltip's *"increases your damage"* was literal, and applies to weapons too. (Resolves the §12 "does AGI scale auto?" question: yes.) `AutoDPS` reads `sb.MainStat`; a zero-MainStat block gives factor 1.0, so unrelated tests are unaffected.
- **dps-mod** already lived in `AutoDPS` (`dpsModFactor`) and stays — no double-count, because the input is census raw (not the pre-multiplied `/weaponstat` value).
- **`classAutoMult`** — the **class-intrinsic innate auto-attack multiplier**, ×2.0 for the Assassin (the gear "melee mult" stat is a *separate* additive thing, 0 for us). It is **NOT** a `StatBlock` field — a multiplier's zero value (0.0) would zero out auto damage everywhere `StatBlock{}` is used; instead it comes from the class data (§4 class files) and is applied at the `AutoDPS`/`TotalDPS(Dual)` boundary, carried on the bis `Set`.
- **Potency does NOT scale auto** — verified: folding potency in makes the residual drift across gear states instead of holding constant.

**Calibration (the `/weaponstat` decomposition):** `/weaponstat` reports `base` (= census × dps-mod) and `actual` (= base × AGI × classAutoMult). At dps-mod 0 the `base` collapses to census raw exactly, isolating the rest. Across three gear states the non-AGI/non-dps-mod residual held **dead constant at 2.00** (e.g. Blood Fire census max 290 → actual max 882 at AGI 51.74%/dps-mod 0 → `882/290/1.5174 = 2.004`; → 1442 at AGI 64.06%/dps-mod 51%). Confirmed by blind prediction. The ×2.0 was missing from the model entirely (along with AGI), so this **~triples** modeled auto damage and swings stat weights back toward auto stats — more than reversing the dual-wield penalty.

**Min-compression aside (not modeled):** the sheet's *min* is raised toward max by the weapon-skill-over-cap floor mechanic (sheet min/max 0.378 vs census 0.334), which shifts with combat skills. `AutoDPS` uses the **average**, where this largely washes out; the calibration uses **max** to sidestep it cleanly.

### Combat-art rotation / recovery
- **Recovery time** is real and folded into CA pacing: each cast occupies `effCast + effRecovery` of the **CA** timeline before the next cast (weapon DPS amortizes over full delay; CA throughput must amortize over the full slot). Both timing stats come from the character config (§4) — e.g. at 37.4% cast speed and 100% recovery speed, a 0.5s art occupies `0.36s + 0s`. Recovery is a flat per-cast add for CA-vs-CA ranking, but **not** a common factor for CA-vs-auto, because it sizes total CA damage over the fight.
- **Per-art recast reductions (config `[art_mods]`):** AA effects that reduce a specific art's recast — Assassinate ×0.5, Mortal Blade ×0.5 today (300→150s, 180→90s). These **count against the art's shared 50% reduction ceiling** (§ Reuse above), so reuse cannot stack on top of a full AA halving.
- **Rotation simulation (CADPS):** a discrete priority sim over the fight. Each slot, fire the off-cooldown art with the highest **damage-per-cast-time** (`CAeffective / slot`), advance time by that art's slot, set its `effRecast`; idle-jump when nothing is up. Priority is by damage-per-**time**, not raw damage, because a slow high-damage art can be worse per second than a fast one. Only fired casts count.
- **Fight-length smoothing (added 2026-06-13):** a single fixed fight length quantizes CADPS — the last cast of a long-cooldown art either fits the window or doesn't, a discrete cliff. Worst on **Assassinate** (longest effective recast, ~150s): the default 600s sits *exactly* on an Assassinate cast time, so whether the 5th cast counts is arbitrary, making reuse's value lumpy (two near-identical reuse items scored 240 vs 177). **Fix:** CADPS is the **mean of `cumCA(t)/t` over K samples evenly spanning `[L − R/2, L + R/2]`**, where `L` = target fight length (configurable, default 600) and `R` = the longest effective recast in the art set (auto-computed). The window is one recast wide, so it always brackets exactly one big-cast step wherever `L` lands — averaging "almost another Assassinate" into an honest expected value (e.g. a 270s target → ~2.4 expected Assassinates, not a hard 2). Mortal Blade's ~90s boundary (damage-per-cooldown ~40% of Assassinate's) is a minor ripple inside the window, covered for free. **Computed in one sim pass:** the priority sim is prefix-consistent (stopping early just truncates the same cast sequence), so a single run to `L + R/2` recording each cast's start time + running total yields `cumCA(t)` for every `t`; the K samples are cheap lookups, not K sims. `K` is an internal constant. All scoring (`pickBest`, `CandidateDelta`/ΔDPS, `DeriveWeights`) routes through CADPS, so picks, deltas, and weights are smoothed *and* mutually consistent.
- **Target length is configurable** via `-fight <seconds>` on `cmd/bis`/`cmd/weights` (default 600) — for optimizing a known fight length. Sub-~90s fights fire the big arts 0–1 times (no boundary to smooth); the window lower bound clamps `> 0`.
- **Art pool (`internal/spell`):** Assassin Expert-tier arts, **level ≥ 57** (`minDamageArtLevel` — below is vestigial low-level filler), damaging (parseable `effect_list` damage), **not beneficial** (buffs/stances excluded), **highest rank per base name**. **Ranged bow shots ARE kept** — no minimum range, zero melee-auto cost, so they're free CA damage that fills idle (Head/Spine/Deadly Shot). Low-level *scaling* arts that census files at base level (Hilt Strike, Strike of Consistency) are NOT yet included — see backlog §3.
- **Idle is structural:** with the real recasts the CA timeline sits idle ~45–50% of a long fight (cooldown-bound, not cast-bound); auto-attack fills the gaps in parallel — correct, not a bug. (Adding the 3 ranged shots cut idle ~52%→46% and raised total DPS ~6.7%.)
- **Stealth — currently assumed free:** many arts require stealth ("must be sneaking"); the model assumes it's always available. The real economy (stealth breaks on any CA cast; granters = Masked Strike / Stalk / the 7s-burst Concealment) is parked in backlog §4 — it only sharpens reuse's exact weight, it does not change gear picks.

### Weight derivation under non-linear stats
**Multi-attack** (still a sample table) takes its marginal weight from the **sample-to-sample slope** at the baseline — evaluate DPS at the sample points bracketing the baseline and divide by the stat gap. At sample points the floored effect equals the table value exactly, so this yields the true segment slope with no flooring noise (the floor makes real gains lumpy, but the per-point weight should read as the smooth "going rate").

**Haste/dps-mod** (fitted equation — no sample brackets) generalize the same trick to anywhere on the curve: the marginal is the DPS slope between the fitted curve's **adjacent integer-effect crossings** bracketing the baseline — endpoints land exactly on whole-percent effects, so the floor contributes no noise. Marginals clamp to 0 at the 300 cap, and are legitimately 0 in the **dead zone** past the last integer crossing (≈289, where `f` can no longer reach the next whole percent before the cap) — the floored in-game effect genuinely cannot improve there.

**Main stat** (sample table, like multi-attack) brackets between its table samples and clamps to 0 at the 1100 cap.

Linear stats use the standard +1 finite difference.

> **Wide-span marginal band-aids (cast speed `±10`, reuse centered `±4`) — superseded by fight-length smoothing (2026-06-13).** These wide finite-difference spans were introduced to average out the *same* single-fixed-600s cast-boundary quantization that the smoothing now removes at the source (CADPS itself is smoothed). With smoothed CADPS the +1 diff should read clean, making the spans redundant. **Verified at implementation:** reverting both to the plain +1 diff is tested — kept reverted if the smoothed marginals stay clean (simpler), retained only if a residual artifact remains. (Historical rationale, for reference: the +1 diff on the un-smoothed sim read ±(one cast's DPS) of noise — ±10 for castspeed, ±50 for reuse at a converged raid set — dwarfing the genuine ~0.1/pt and ~74/pt trends; reuse's span was cap-clamped to `[max(0,v−4), min(50,v+4)]` because converged baselines sit near the 50 cap.)

### Removed / changed constants
- **Removed:** `HasteCapPct` (100), `HasteToFlurry` (10:1), `DPSModEffectAtCap` (1.25), the haste→flurry term, the linear dps-mod form, (2026-06 refit) the piecewise haste/dps-mod sample table with its `(200,125)` anchor, and (2026-06 rotation revision) `ReuseHalveCoeff` (0.50) / `ReuseHalvesAt` (100) / `CARecoverySecs` (0.25) — all three disproven by live tooltip measurements.
- **Added:** the multi-attack interpolated+floored sample curve; the **fitted** haste/dps-mod equation (form + parameters derived by `cmd/fitcurve` from `data/curve-readings.csv`); `HasteStatCap` = 300; `DPSModCap` = 300; `ReuseCapStat` = 50 (1%/pt to the 50% ceiling); `RecastReductionCeiling` = 0.50 (per-art, shared by AA + reuse); `CARecoveryBaseSecs` = 0.5 (server base, reduced by the character's recovery-speed stat); `StatBlock.CastSpeed` / `StatBlock.RecoverySpeed`; per-art recast multipliers moved from a hardcoded map into config `[art_mods]`.
- **Changed:** flurry ×5 → **×4**; dps-mod → the **shared fitted diminishing curve** (hard cap 300), not linear; reuse → full-strength 1%/pt (was half-strength).
- **(2026-06-12 CA-equation revision)** — **Removed:** `AbilityModCapFrac` (0.50 — disproven by tooltip probes). **Added:** the main-stat interpolated sample table (13 readings, cap 1100 → 65%); `StatBlock.MainStat` / `StatBlock.PotencyBonus`; `spell.CombatArt.PotencyAdd` (config `[art_mods]` rider).
- **(2026-06-13 dual-wield revision)** — **Added:** `DualWieldDelayPenalty` = 1.33 — multiplies each weapon's delay in `AutoDPSDual`, applied **only when an off-hand weapon is present** (`off.DelaySecs > 0`), so the penalty is detected from the loadout, not assumed (measured 4 readings + documented +33%; § Dual-wield delay penalty).
- **(2026-06-13 auto-attack equation)** — **Added:** AGI now scales auto-attack (`AutoDPS` reads `sb.MainStat` through `mainStatCurve`, same curve as CAs); `classAutoMult` (Assassin ×2.0) from the new `classes/<class>.toml`, applied at the `AutoDPS`/`TotalDPS(Dual)` boundary (NOT a `StatBlock` field — multiplier zero-value would zero auto damage). Together these ~triple modeled auto damage. dps-mod unchanged (already applied); potency confirmed NOT on auto.
- **(2026-06-13 fight-length smoothing)** — **Changed:** `FightDurationSecs` (600) from a hardcoded constant to the default for a configurable `-fight` flag (the target length `L`). **Added:** CADPS smoothing — mean of `cumCA(t)/t` over K samples across `[L−R/2, L+R/2]`, `R` = longest effective recast (auto-computed), one sim pass via the recorded cast timeline. **Likely removed** (verified at implementation): the wide-span marginal band-aids `castSpeedMarginalSpan` (10) / `reuseMarginalHalfSpan` (4) — superseded by smoothing.

---

## 4. Character Configuration & Contexts (replaces the hardcoded baselines)

Per-character inputs live in a **TOML config file** (`characters/<name>.toml`, loaded by `internal/charconfig`, selected via `-character` on `cmd/weights`/`cmd/bis`), replacing the `internal/baseline` constants block. The **code/config boundary**: server-wide mechanics (crit ×1.30, flurry ×4, the fitted curve, the reuse/cast/recovery conversion rules) stay in code — they're identical for every player; everything about *a* player and *their group* is config. The schema is **class-agnostic** (a `class` field exists; only `assassin` is implemented today — other classes are a future art-pool problem, not a schema break).

**Class-intrinsic data** lives in a separate **`classes/<class>.toml`**, looked up by the character config's `class` field — values that are the same for every character *of that class* but differ *between* classes (not per-character, not universal). Uniform strict schema: every class file defines the same fields, and a **missing field is a hard error** (the sim is incomplete without them). v1 holds one field:

```toml
# classes/assassin.toml — class-intrinsic measured constants (uniform schema across classes)
auto_attack_multiplier = 2.0   # innate auto-attack multiplier (measured /weaponstat 2026-06-13; Enchanter ≈0.7 for reference)
```

As character import (backlog §1, §10) nears, more class-intrinsic values move here (`census_class_id`, etc.) — all "same field, different value per class." See backlog §10 for the migration list. `auto_attack_multiplier` feeds the auto-attack equation (§3.1) as `classAutoMult`.

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
potency_bonus  = 24.6        # Wuoshi TLE server damage adjustment, captured empirically (§12; naked-tooltip procedure in §3.1)

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
| `internal/charconfig` *(replaces `internal/baseline`, 2026-06)* | TOML character config: AA stats, `[art_mods]`, buff contexts (§4); plus class-data loader for `classes/<class>.toml` (class-intrinsic constants, e.g. `auto_attack_multiplier`); `internal/constants` keeps the server-wide combat constants |
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

- **Combat constants (see §3.1 for the authoritative, revised mechanics):** crit ×1.30; **flurry ×4**; **haste & dps-mod** share a **fitted diminishing curve** (equation derived from `data/curve-readings.csv`; hard cap **300 stat**, effect at cap = `f(300)`, auto-attack only); **multi-attack** has its own gentler diminishing curve (runs to 3400 with triple overcap); **haste overcap does NOT convert to flurry**; **ability-mod uncapped** (old 50% cap disproven 2026-06-12); **main stat (AGI) multiplies BOTH CA and auto-attack damage** via an interpolated 13-reading table, hard cap 1100 → 65% (gear source: census `strength` key = "+N primary attributes"); CA potency pool = displayed potency + calibrated `potency_bonus` (**⚠ §12 mystery**) + per-art `[art_mods]` riders; **auto-attack = census_raw × AGI × dps-mod × classAutoMult** (Assassin ×2.0 from `classes/<class>.toml`; potency does NOT scale auto); **reuse 1%/pt capping at 50 stat, sharing each art's 50% recast ceiling with `[art_mods]` reductions**; **cast speed divisor** / **recovery subtractive from 0.5s base** (both from config; cast speed also on gear); **dual-wield delay penalty ×1.33 on both weapons** (auto-attack only, dual path); non-primary attributes still excluded.
- **Resolved (2026-06 curve refit):** cap = **300** confirmed (former-dev statement + readings growing past 238 and flattening by 281); the patch-note `(200 → 125%)` anchor **disproven** by `haste 281 → 124%` (monotonicity); curve re-derived as a fitted equation per §3.1.
- **Resolved (2026-06 rotation revision):** reuse half-strength coefficient **disproven** (Eviscerate 57.8s @ 3.8 reuse); AA-then-reuse stacking **disproven** (Assassinate pinned at 2m30s with reuse gear → shared 50% ceiling); recovery "halved by AA" guess replaced by the measured recovery-speed stat (100% → "Recovery: Instant"); cast speed measured as a divisor (Head Shot 1.46s @ 37.4%).
- **Rotation (as implemented):** CADPS = priority sim (fire highest **damage-per-cast-time** off-cooldown art; 600s fight; structural idle, auto fills it — idle % shifts with the measured timing stats; re-derive on the post-change report). Art pool = Expert, **level ≥ 57**, damaging, non-beneficial, highest-rank, **ranged shots included**. Stealth assumed always available (real stealth-grant economy parked — backlog §4).
- **TLE translations:** `doubleattackchance` → **Multi-Attack** (legacy key; displayname already "Multi Attack"); `spelltimecastpct` → **Cast Speed** (gear stat); `strength` → **Main Stat** ("+N primary attributes" — AGI point-for-point for a scout; explicit `agility`/`wisdom`/`intelligence` keys data-suspicious, excluded pending fixing); `critbonus` → **ignored entirely**; Fervor → does not exist.
- **CA tier:** Expert (classic "Adept III" — the farmable raiding baseline); config `art_tier` field reserved.
- **Character/contexts:** all per-character values (AA stats, art mods, buff packages incl. raid group DPS-mod **114.2**) live in `characters/<name>.toml` — config is data with in-file provenance comments, refined without spec changes.

---

## 12. Open / To-Confirm (non-blocking)

- **The "potency-pool" hidden bonus — SOURCE IDENTIFIED 2026-06-15 (was the §12 mystery).** ~23.4 hidden potency-pool points on a naked, AA-less, buff-less level-70 (§3.1 measurement ledger). We eliminated gear/AAs/buffs/level-scaling and were left with "innate, unsourceable." **It's a Wuoshi (TLE) server-applied damage adjustment** — the TLE lineage carries a dev "balance pass that increased outgoing damage across most classes" (GU104-era, Caith/Fyreflyte; the Assassin was specifically buffed). That's exactly why it survived stripping everything: it's applied by the server, not by any gear/AA/buff. **No clean published scalar exists**, but it doesn't need one — its *effect* is already captured empirically in the calibrated `potency_bonus` (~24.6) and the auto-attack `auto_attack_multiplier` (×2.0), both measured against live tooltips. So the model reproduces the game's real numbers regardless. Downgraded from "actively hunt" to "identified source, captured empirically; exact value undocumented." **Open only:** whether it's a *uniform* global multiplier (shared across all classes) or *per-class* tuning — resolved by a **cross-class read (ideally a mage**: spells have no auto/weapon confounder). If shared, it's one global factor; if not, it's each class's own balance value (backlog §10/§11). Either way it's ranking-neutral for the Assassin (a constant on Assassin damage cancels in relative gear comparisons), so this is curiosity/absolute-DPS only.
- Config numeric values (buff packages, AA stats) — config is data; refinement is an edit + re-run. The raid crit estimate (31) still conflates AA crit with buff crit — split into `[stats]` vs `[contexts.raid]` when measured.
- **Main-stat curve gaps**: no readings in 156–625 (curve shape between the flat low range and the quadratic-like high range is interpolated; marginal weights for any baseline in that range ride the gap's 469-point secant — a mid-gap reading ~350–400 would pin it); a deliberately overcapped reading (AGI > 1100) to verify the clamp. (**Resolved 2026-06-13:** AGI *does* scale auto-attack, same curve as CAs — see the §3.1 auto-attack weapon-damage equation.)
- **Per-art potency riders unverified numerically**: the cooldown AA's "+15% potency to Assassinate/Mortal Blade" is modeled from its text; the regeared Mortal Blade tooltip read (~13.8k max predicted vs ~12.8k without the rider) confirms or refutes.
- **Residual rotation non-monotonicity (backlog §11)** — fight-length smoothing reduced the discrete-sim cast-boundary quantization (~18% variance, fixed the Wrist over-ranking) but did NOT eliminate it: CADPS is still slightly non-monotone in reuse/cast-speed, causing strict-dominance inversions in the ΔDPS scores (~1.5–29 DPS, ≈ one mid-art cast, on near-tied items). Increasing the smoothing sample count does not help — it's intrinsic to the discrete greedy sim. Needs a spec-level decision (accept+document / model expected fractional casts / other); the weight-side band-aids would NOT fix it (they smoothed displayed weights, not the resim scores where inversions live). Picks via full resim are unaffected (still a local optimum).
- **Cast-speed cap unknown** — measured as a divisor at 37.4%; un-sampled above. Modeled uncapped; grab tooltip readings if AA respec/gear pushes it higher (the haste-curve lesson: don't trust received caps).
- **Haste/dps-mod curve — remaining gaps** (cap = 300 and the refit itself are resolved, §3.1): readings in 153–238 and 281–300, the effect at exactly 300, and one overcapped pair to verify the clamp. Append to `data/curve-readings.csv` + re-run `cmd/fitcurve`; gatherable any time via buff/CD-stacking. Also keep watching the haste-vs-dpsmod separate-fit residuals as data lands — split the shared curve if they diverge.
- **Reuse weight under the measured rules** — now full-strength per point but blind to ceiling-filled arts (Assassinate/Mortal Blade); the old negative converged-weight artifact (boundary drift on the AA-halved arts) is predicted to shrink/vanish — verify on the post-change report.
- **Art-list audit pending** — diff the census Expert-tier pool against the live in-game skill list (ranks, missing/extra arts, Hilt Strike / Strike of Consistency level-70 damage — backlog §3). Deferred from the 2026-06 rotation revision by user choice.
- **DoT/multi-component ability-mod rule — CALIBRATED 2026-06-15** (§3.1 "Multi-component abilities"). Measured via live census + tooltip reads: abmod placement is **per-mechanic** — full on a **DirectHit**, **≈½ per trigger** on a TriggerProc (Death Mark, ×5 ≈ 2.4×), **zero** on DoT ticks/Termination, and **none** on beneficial abilities. (Old "~50% onto a piercing line" disproven — that line was the DoT's instant tick, a tooltip artifact.) Rotation: **clip DoTs by default, hold only termination-DoTs to full duration** (detonate conditional on completion). Increment A (parser, plan 2q) shipped; Increment B (per-component sim + abmod placement + Death Mark TriggerProc scoring + clip/hold + calibration tests) in progress (plan 2r). Still deferred: RateProc scoring; detrimental no-DirectHit DoT-with-instant abmod; clip-vs-hold optimization (backlog §10); per-skill behavior AAs (~×1.14 overtime bump); AoE multi-target.
- **Raid-wide bleed damage-taken debuff — OUT OF SCOPE (deferred).** The Assassin's single-target bleeds (e.g. Gushing Wound) apply a +0.2%-from-everyone damage-taken debuff. Its raid-wide value is structurally unmodelable here (no model of the rest of the raid) and makes bleed uptime mandatory by construction. Deferred tail worth modeling later: the debuff's *self*-benefit (+0.2% × bleed stacks on the Assassin's own hits against the target) and reuse-for-uptime valuation (at base reuse, recast ~29s > duration 24s forces a ~5s gap). Parked with the per-ability rotation revisit.
- **Clip-viability for other classes — backlog TODO gated on multi-class support.** For the Assassin, hold-vs-clip is fully settled (never clip — duration ≪ break-even AND the raid debuff mandates uptime). Other classes' bleeds/DoTs may lack the debuff AA, so clip-vs-hold becomes a real per-class DPS optimization. Revisit when implementing additional classes.
- **Parked rotation-realism** (`docs/backlog.md`): character-pull seeding (§1), lore-equip doubling (§2), manual scaling arts (§3), stealth-grant modeling (§4), launch-day gear-cache re-export (§5).
