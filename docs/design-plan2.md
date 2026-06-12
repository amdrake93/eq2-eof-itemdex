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

`TotalDPS = AutoDPS + CADPS`, computed in **parallel** — auto-attack and combat-art casting run on independent timelines, so casting does **not** displace auto swings (a CA's cast+recovery only paces the *CA* side; see §3.1). Locked combat values: crit ×1.30; **flurry ×4** (a flurry proc does +100%–500%, averaging +300% = ×4); **ability-mod cap = 50% of the potency-adjusted CA base**; reuse halves recast at 100%; **potency applies to CAs only**; `critbonus` ignored. Haste, multi-attack, and dps-mod are **non-linear** (§3.1) — *not* `1 + stat/100`. Haste & dps-mod share one **fitted** diminishing curve (hard cap **300** stat; the old `200 → 125%` anchor is disproven — see §3.1); multi-attack has its own gentler curve (runs to 3400 with triple overcap). Stat-conversion mechanics, the AA cooldown/recovery effects, and the curve table are in **§3.1** (the authoritative stat-mechanics block, revised from the Varsoon playtest session).

**Why the Assassin CA query is integral (not just additive damage):** the CA term needs each Combat Art's *base* damage to model potency and ability-mod correctly — potency scales the base `×(1+potency)`, and ability-mod caps at **50% of that potency-adjusted base**. Without the per-CA base damages, neither term is computable. So `internal/spell` pulls the Assassin's CAs (Expert tier, level ≤70), regex-parses damage from `effect_list`, and applies the TLE translations (§ Assumptions).

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

The tool fits each form three ways — **haste-only, dpsmod-only, and joint** — and reports per-fit residuals. This is the ongoing shared-curve verification: if the two single-stat fits agree within flooring noise, the joint fit is the curve; if they ever diverge as data accumulates, split into per-stat parameter sets (the model carries per-stat parameters that may alias one shared set). The winning form's parameters are recorded as constants in `internal/model/curve.go`, annotated with the residual and dataset size; appending readings + re-running the tool is the whole refresh loop.

**Application (unchanged mechanics, new curve underneath):**

- **Haste** — `effDelay = baseDelay / (1 + hasteEffect/100)`.
- **DPS-mod** — auto-damage multiplier = `1 + dpsModEffect/100`. **Auto-attack only.**
- Effect is floored to a whole % (in-game behavior): `effect = floor(f(min(stat, 300)))`. Overcap (stat > 300) clamps at `f(300)`; haste overcap does **NOT** convert to flurry (confirmed removed).

**Data still wanted (non-blocking — append to the CSV and re-fit):** the 153–238 and 281–300 gaps; a reading at/near exactly 300; one deliberately *overcapped* pair (e.g. `320 → X%`) to verify the clamp.

### Linear / direct stats
- **Crit chance** — linear, `crit% = stat`. `critFactor = 1 + (crit%/100)·(1.30−1)`.
- **Potency** — linear, `potency% = stat`; scales CA base `×(1 + potency/100)`.
- **Flurry** — **gear % only** (no haste contribution); `flurryFactor = 1 + (flurry%/100)·(4−1)`.
- **Ability-mod** — flat add to each CA, capped at `0.50 × CA_maxDamage × (1+potency/100)` (per-CA; potency floats the cap up, so it binds mainly on small filler arts). At a fully-geared baseline the *stacked* ability-mod (~700, since every item carries it) caps most small/frequent arts, so its marginal weight reads small even though the big nukes still have headroom — that aggregate, not any one item, is the saturator.

### Reuse (combat-art recast %)
- `effRecast = baseRecast × (1 − 0.5·min(reuse,100)/100)` — 100% reuse halves recast, capped at 100. Applied **after** the per-art AA cooldown halving (Assassinate/Mortal Blade `×0.5`, *then* reuse). Constants: `ReuseHalveCoeff = 0.50`, `ReuseHalvesAt = 100`. Affects the **CA timeline only** (recast → cast count).
- **Reuse is the most gear-state-dependent stat.** On a bare character (sparse, idle rotation) it is the single highest weight (~25/pt — every point buys more casts into empty time). Fully geared and cast-saturated it collapses toward 0; its converged-set weight can even read slightly **negative** — a discrete-sim quantization artifact (a +1 reuse step usually can't fit even one more *whole* cast into the fixed 600s window; see weight-derivation note below), NOT a real penalty. Gear reuse is also scarce in the EoF pool (~2/item), so it rarely wins a slot regardless of weight.

### Putting it together (formulas as implemented — `internal/model`)
```
critFactor   = 1 + (crit/100)·0.30
flurryFactor = 1 + (flurry/100)·3.0                  # gear flurry only (no haste→flurry)
dpsModFactor = 1 + curveHD(dpsMod)/100               # shared haste/dps-mod curve
effDelay     = weaponDelay / (1 + curveHD(haste)/100)
AutoDPS(w)   = (w.avgDmg / effDelay) · (1 + curveMA(MA)/100) · critFactor · flurryFactor · dpsModFactor
AutoDPSDual  = AutoDPS(main) + AutoDPS(off)           # both weapons swing on own delay, treated equally
CAeffective  = ((min+max)/2 · (1+potency/100) + min(abilityMod, 0.5·max·(1+potency/100))) · critFactor
CADPS        = RotationSim(arts, fight=600s)          # priority by CAeffective / (cast+recovery)
TotalDPS     = AutoDPSDual + CADPS                    # PARALLEL — CA casting costs zero auto swings
```

### Combat-art rotation / recovery
- **Recovery time** is real and folded into CA pacing: each cast occupies `cast (0.5s) + recovery (0.25s) = 0.75s` of the **CA** timeline before the next cast (weapon DPS amortizes over full delay; CA throughput must amortize over full cast+recovery). Recovery is a flat per-cast constant for CA-vs-CA ranking, but it is **not** a common factor for CA-vs-auto, because it sizes total CA damage over the fight. The base recovery is 0.5s, **halved to 0.25s by an AA**.
- **AA cooldown reduction:** a large AA halves the **base** recast of **Assassinate** and **Mortal Blade** by 50% (300→150s, 180→90s). **Reuse applies to the already-halved base** (order: ×0.5 AA, then reuse).
- **Rotation simulation (CADPS):** a discrete priority sim over the fight. Each slot, fire the off-cooldown art with the highest **damage-per-cast-time** (`CAeffective / slot`), advance time by that art's slot, set its `effRecast`; idle-jump when nothing is up. Priority is by damage-per-**time**, not raw damage, because a slow high-damage art can be worse per second than a fast one. Only fired casts count.
- **Art pool (`internal/spell`):** Assassin Expert-tier arts, **level ≥ 57** (`minDamageArtLevel` — below is vestigial low-level filler), damaging (parseable `effect_list` damage), **not beneficial** (buffs/stances excluded), **highest rank per base name**. **Ranged bow shots ARE kept** — no minimum range, zero melee-auto cost, so they're free CA damage that fills idle (Head/Spine/Deadly Shot). Low-level *scaling* arts that census files at base level (Hilt Strike, Strike of Consistency) are NOT yet included — see backlog §3.
- **Idle is structural:** with the real recasts the CA timeline sits idle ~45–50% of a long fight (cooldown-bound, not cast-bound); auto-attack fills the gaps in parallel — correct, not a bug. (Adding the 3 ranged shots cut idle ~52%→46% and raised total DPS ~6.7%.)
- **Stealth — currently assumed free:** many arts require stealth ("must be sneaking"); the model assumes it's always available. The real economy (stealth breaks on any CA cast; granters = Masked Strike / Stalk / the 7s-burst Concealment) is parked in backlog §4 — it only sharpens reuse's exact weight, it does not change gear picks.

### Weight derivation under non-linear stats
**Multi-attack** (still a sample table) takes its marginal weight from the **sample-to-sample slope** at the baseline — evaluate DPS at the sample points bracketing the baseline and divide by the stat gap. At sample points the floored effect equals the table value exactly, so this yields the true segment slope with no flooring noise (the floor makes real gains lumpy, but the per-point weight should read as the smooth "going rate").

**Haste/dps-mod** (fitted equation — no sample brackets) get the same smooth-going-rate intent directly: the marginal is a DPS finite difference evaluated on the **unfloored** fitted curve `f(s)` (the flooring was the whole reason for the bracket trick; the equation lets us bypass it). Marginals clamp to 0 at the 300 cap.

Linear stats use the standard +1 finite difference.

### Removed / changed constants
- **Removed:** `HasteCapPct` (100), `HasteToFlurry` (10:1), `DPSModEffectAtCap` (1.25), the haste→flurry term, the linear dps-mod form, and (2026-06 refit) the piecewise haste/dps-mod sample table with its `(200,125)` anchor.
- **Added:** the multi-attack interpolated+floored sample curve; the **fitted** haste/dps-mod equation (form + parameters derived by `cmd/fitcurve` from `data/curve-readings.csv`); `HasteStatCap` = 300; `DPSModCap` = 300; `CARecoverySecs` = 0.25; per-art AA cooldown halving for Assassinate/Mortal Blade.
- **Changed:** flurry ×5 → **×4**; dps-mod → the **shared fitted diminishing curve** (hard cap 300), not linear.

---

## 4. The Two Baselines

A labeled constants block (`internal/baseline`) defines two input stat profiles. Each is **only an input stat block**; the model derives the weights from it — this spec asserts nothing about which stats end up mattering under either.

- **Solo:** the Assassin's self-buffs only (Villainy → +34.2 Multi-Attack; the temporary self-haste/DPS self-buff while active). No group DPS-mod.
- **Raid:** self-buffs + group package; **DPS-mod = 200** (group package value — *not* capped; the cap is 300, so dps-mod keeps real marginal value at this baseline); the Velocity-style group contribution; crit elevated by AAs/buffs. Haste from the same comp (no maintained group haste buff).

Each baseline yields its own derived weight set → its own BiS list. The **cross-context difference is an output** (it shows which stats change between contexts), not an assumption.

### Accessibility tiers (report structure)

The report segments into **three accessibility tiers**, each = one stat baseline + a gear keep-filter (`internal/bis`):
- **PRE-RAID** — Solo baseline; only `LEGENDARY`/`TREASURED` items (dungeon-accessible); no avatar/Hunter's.
- **RAID** — Raid baseline; all items **minus** avatar mythicals and the Hunter's set.
- **BEST-OF-BEST** — Raid baseline; all items minus the Hunter's set (avatar mythicals **kept**).

Exclusion predicates: **avatar** = `MYTHICAL` except the Soulfire; **Hunter's** set excluded everywhere; plus a small **curated** exclude list. The main-hand is pinned to the **Soulfire Sabre** (its multi-attack beats the Gladius's block) and its full stat line folds into every tier's baseline; the off-hand pool is all 1H weapons **except** Soulfire (the player gets exactly one Soulfire — the fixed main-hand).

Baseline numeric values are documented best-guesses tagged for later confirmation (guild leader / Varsoon parse) per the provenance hierarchy in `docs/design.md` §2.1.

---

## 5. Architecture / Components (Go)

Reuse Plan-1 packages; add:

| Package | Responsibility |
|---|---|
| `internal/census` (extend) | add `Skill` + `WieldStyle` to `TypeInfo` |
| `internal/catalog` (extend) | add `skill`/`wieldstyle` CSV columns |
| `internal/spell` *(new)* | pull Assassin CAs (Expert, ≤70); regex damage from `effect_list`; apply TLE translations |
| `internal/model` *(new)* | DPS equations, marginal-weight derivation (iterated), item scoring, per-slot ranking, **locked-items** constraint (§8) |
| `internal/baseline` *(new)* | the two baseline profiles + the labeled combat-constants block |
| `internal/store` *(new)* | `modernc.org/sqlite` — schema, load gear + per-baseline scores, ranking/coverage queries |
| `internal/bis` *(new)* | accessibility keep-filters + exclusions, converging set-builder (coordinate ascent), per-slot report + progression render |
| `cmd/bis` *(new)* | orchestrate: load gear (from CSV cache) → pull CAs → build/score per tier (3 accessibility tiers over the 2 stat baselines) → load SQLite → emit report + DB. Flags: `--out`, `--lock` (§8), `--db`, `--top`. |

Data flow: gear (Plan-1 cache) + Assassin CAs → `model` derives weights per baseline and scores each Assassin-usable item → `store` loads gear + scores into SQLite → `cmd/bis` runs ranking SQL and renders the markdown report.

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

All from `docs/design.md` §2.1 / §4, reproduced in `internal/baseline` with provenance + validation flags:

- **Combat constants (see §3.1 for the authoritative, revised mechanics):** crit ×1.30; **flurry ×4**; **haste & dps-mod** share a **fitted diminishing curve** (equation derived from `data/curve-readings.csv`; hard cap **300 stat**, effect at cap = `f(300)`, auto-attack only); **multi-attack** has its own gentler diminishing curve (runs to 3400 with triple overcap); **haste overcap does NOT convert to flurry**; ability-mod cap = 50% of potency-adjusted CA base; reuse: 100% → half recast (applied *after* the Assassinate/Mortal Blade AA cooldown halving); potency on CAs only; **CA cast+recovery = 0.75s** paces the CA timeline (auto runs in parallel); attributes excluded (track itemlevel).
- **Resolved (2026-06 curve refit):** cap = **300** confirmed (former-dev statement + readings growing past 238 and flattening by 281); the patch-note `(200 → 125%)` anchor **disproven** by `haste 281 → 124%` (monotonicity); curve re-derived as a fitted equation per §3.1.
- **Rotation (as implemented):** CADPS = priority sim (fire highest **damage-per-cast-time** off-cooldown art; 600s fight; ~45–50% structural idle, auto fills it). Art pool = Expert, **level ≥ 57**, damaging, non-beneficial, highest-rank, **ranged shots included**. Stealth assumed always available (real stealth-grant economy parked — backlog §4).
- **TLE translations:** `doubleattackchance` → **Multi-Attack** (legacy key; displayname already "Multi Attack"); `critbonus` → **ignored entirely**; Fervor → does not exist.
- **CA tier:** Expert (classic "Adept III" — the farmable raiding baseline).
- **Baselines:** Solo (self-buffs only) and Raid (self + group, DPS-mod = 200 — below the 300 cap) — values tagged "confirm vs guild leader / Varsoon parse."

---

## 12. Open / To-Confirm (non-blocking)

- Baseline numeric values (confirm with guild leader / Varsoon parse) — parameterized, so refinement is a re-run.
- Exact crit baseline per context (AAs vs buffs) — feeds the baselines.
- **Haste/dps-mod curve — remaining gaps** (cap = 300 and the refit itself are resolved, §3.1): readings in 153–238 and 281–300, the effect at exactly 300, and one overcapped pair to verify the clamp. Append to `data/curve-readings.csv` + re-run `cmd/fitcurve`; gatherable any time via buff/CD-stacking. Also keep watching the haste-vs-dpsmod separate-fit residuals as data lands — split the shared curve if they diverge.
- **Reuse weight is gear-conditional + discrete** — bare ≈ top stat (~25/pt), geared ≈ 0/slightly-negative (quantization). Only matters precisely for the character-pull case (backlog §1).
- **Parked rotation-realism** (`docs/backlog.md`): character-pull seeding (§1), lore-equip doubling (§2), manual scaling arts (§3), stealth-grant modeling (§4), launch-day gear-cache re-export (§5).
