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

`TotalDPS = AutoDPS + CADPS`, computed in **parallel** — auto-attack and combat-art casting run on independent timelines, so casting does **not** displace auto swings (a CA's cast+recovery only paces the *CA* side; see §3.1). Locked combat values: crit ×1.30; **flurry ×4** (a flurry proc does +100%–500%, averaging +300% = ×4); **ability-mod cap = 50% of the potency-adjusted CA base**; reuse halves recast at 100%; **potency applies to CAs only**; `critbonus` ignored. Haste, multi-attack, and dps-mod are **non-linear** (§3.1) — *not* `1 + stat/100`. Haste & dps-mod share one diminishing curve (hard cap 200 → 125% = ×2.25 — **but see §3.1 pending note: the cap is likely 300, deferred for data**); multi-attack has its own gentler curve (runs to 3400 with triple overcap). Stat-conversion mechanics, the AA cooldown/recovery effects, and the curve table are in **§3.1** (the authoritative stat-mechanics block, revised from the Varsoon playtest session).

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

### Haste & DPS-mod conversion curve (shared)

Haste and dps-mod share a **single, steeper diminishing curve** — confirmed by a patch note (*"Haste and DPS mods use a diminishing returns curve … 125% at the 200 cap"*) and by live Varsoon readings that interleave cleanly onto one line:

| stat | effect % | source |
|-----:|---------:|:--|
| 24 | 18 | haste |
| 28.1 | 21 | dps-mod |
| 48.3 | 35 | dps-mod |
| 67.5 | 48 | haste |
| 200 | 125 | both (hard cap) |

Same interpolate-anchored-at-(0,0)-then-floor treatment. This curve is **distinct from (and steeper than) multi-attack** (24→18 here vs ~26 on the MA curve) and **hard-caps at 200 stat → 125%**; overcap is wasted (haste does **NOT** convert to flurry on Varsoon — confirmed removed).

- **Haste** — `effDelay = baseDelay / (1 + hasteEffect/100)` (100% → half delay; 125% cap → /2.25).
- **DPS-mod** — auto-damage multiplier = `1 + dpsModEffect/100` (200 cap → **×2.25**). **Auto-attack only.** Supersedes *both* the earlier +125% guess and the linear +1%/point reading (that linear set ran past the 200 cap, so it's treated as a different system / misread).

> **⚠ PENDING REVISION (learned 2026-06, NOT yet applied — deferred for data):** the haste and dps-mod hard cap is actually **300 stat**, not 200. The shared curve above tops out at `200 → 125%`; the true cap is higher and the 200→300 segment is un-sampled. When readings above 200 are available: re-fit the curve and bump `HasteStatCap` / `DPSModCap` (both currently `200.0`) to 300. Until then the model uses the 200-cap curve, so any baseline/gear at or above 200 haste or dps-mod is *undervalued* (treated as capped when it isn't yet).

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
For the three curve-stats (haste, multi-attack, dps-mod) the marginal weight is taken from the **sample-to-sample slope** at the baseline — evaluate DPS at the sample points bracketing the baseline and divide by the stat gap. At sample points the floored effect equals the table value exactly, so this yields the true segment slope with no flooring noise (the floor makes real gains lumpy, but the per-point weight should read as the smooth "going rate"). Haste/dps-mod marginals clamp at the 200 cap (→ 0 beyond). Linear stats use the standard +1 finite difference.

### Removed / changed constants
- **Removed:** `HasteCapPct` (100), `HasteToFlurry` (10:1), `DPSModEffectAtCap` (1.25), the haste→flurry term, and the linear dps-mod form.
- **Added:** two interpolated+floored sample curves — multi-attack, and the shared haste/dps-mod curve; `HasteStatCap` = 200; `DPSModCap` = 200; `CARecoverySecs` = 0.25; per-art AA cooldown halving for Assassinate/Mortal Blade.
- **Changed:** flurry ×5 → **×4**; dps-mod → the **shared diminishing curve** (×2.25 at the 200 cap), not linear.

---

## 4. The Two Baselines

A labeled constants block (`internal/baseline`) defines two input stat profiles. Each is **only an input stat block**; the model derives the weights from it — this spec asserts nothing about which stats end up mattering under either.

- **Solo:** the Assassin's self-buffs only (Villainy → +34.2 Multi-Attack; the temporary self-haste/DPS self-buff while active). No group DPS-mod.
- **Raid:** self-buffs + group package; **DPS-mod = 200 (capped)**; the Velocity-style group contribution; crit elevated by AAs/buffs. Haste from the same comp (no maintained group haste buff).

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

- **Combat constants (see §3.1 for the authoritative, revised mechanics):** crit ×1.30; **flurry ×4**; **haste & dps-mod** share a **diminishing curve** (hard cap 200 → 125% = ×2.25, auto-attack only); **multi-attack** has its own gentler diminishing curve (runs to 3400 with triple overcap); **haste overcap does NOT convert to flurry**; ability-mod cap = 50% of potency-adjusted CA base; reuse: 100% → half recast (applied *after* the Assassinate/Mortal Blade AA cooldown halving); potency on CAs only; **CA cast+recovery = 0.75s** paces the CA timeline (auto runs in parallel); attributes excluded (track itemlevel).
- **⚠ PENDING (deferred for data):** haste & dps-mod hard cap is actually **300**, not 200 — re-fit the shared curve and bump `HasteStatCap`/`DPSModCap` once readings above 200 exist (see §3.1 note).
- **Rotation (as implemented):** CADPS = priority sim (fire highest **damage-per-cast-time** off-cooldown art; 600s fight; ~45–50% structural idle, auto fills it). Art pool = Expert, **level ≥ 57**, damaging, non-beneficial, highest-rank, **ranged shots included**. Stealth assumed always available (real stealth-grant economy parked — backlog §4).
- **TLE translations:** `doubleattackchance` → **Multi-Attack** (legacy key; displayname already "Multi Attack"); `critbonus` → **ignored entirely**; Fervor → does not exist.
- **CA tier:** Expert (classic "Adept III" — the farmable raiding baseline).
- **Baselines:** Solo (self-buffs only) and Raid (self + group, DPS-mod = 200 capped) — values tagged "confirm vs guild leader / Varsoon parse."

---

## 12. Open / To-Confirm (non-blocking)

- Baseline numeric values (confirm with guild leader / Varsoon parse) — parameterized, so refinement is a re-run.
- Exact crit baseline per context (AAs vs buffs) — feeds the baselines.
- **Haste & dps-mod cap = 300, not 200** — confirmed in play but un-sampled above 200; needs data points in the 200–300 range to re-fit the shared curve and raise the cap constants (§3.1 pending note). Highest-priority data to gather.
- **Reuse weight is gear-conditional + discrete** — bare ≈ top stat (~25/pt), geared ≈ 0/slightly-negative (quantization). Only matters precisely for the character-pull case (backlog §1).
- **Parked rotation-realism** (`docs/backlog.md`): character-pull seeding (§1), lore-equip doubling (§2), manual scaling arts (§3), stealth-grant modeling (§4), launch-day gear-cache re-export (§5).
