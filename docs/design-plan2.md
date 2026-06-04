# Plan 2 Design: EoF Assassin Best-in-Slot — DPS Model & Analysis

**Date:** 2026-06-03
**Status:** Design / spec (pre-implementation)
**Builds on:** Plan 1 (`docs/design.md`, the item catalog) — reuses its `census` / `classify` / `catalog` / `extract` / `source` packages, types, and the gear CSVs.
**Implementation language:** Go 1.26.

This is **Plan 2 of 2**. Plan 1 produced the EoF gear catalog; Plan 2 builds the relative-DPS model that ranks that gear into a best-in-slot list for an Assassin. The core model + combat constants are specified in `docs/design.md` §3–§4; this document carries them forward and adds the Plan-2-specific decisions (two baselines, SQLite analysis layer, locked-items re-model, outputs).

---

## 1. Goal & Deliverables

For a level-70 Echoes of Faydwer **Assassin**, compute **best-in-slot per equipment slot** under **two baselines** — **solo** and **raid** — ranked by a derived-weight relative DPS model.

**Deliverables:**
- A markdown **BiS report**: per-slot best item for *each* baseline, **top-N per slot** (not just #1), the **derived stat weights** per baseline, and the **assumptions/constants block**.
- The queryable **SQLite DB** (scored gear) for the user's own exploration.

**Non-goals:** absolute/parse-accurate DPS (relative ordering only); other classes; set-bonus *scoring* (see §8).

---

## 2. Prerequisite: catalog re-pull (weapon `skill` + `wieldstyle`)

Plan 1's `census.TypeInfo` captured `skilltype` (armor) but **not** the weapon `skill` (piercing/slashing/crushing/…) or `wieldstyle` (One-Handed/Two-Handed). Both are needed: `wieldstyle` is a **model input** (dual-wielding two 1H vs one 2H changes the auto-attack term), and `skill` is informational (which combat-skill applies). Weapon *eligibility* still rides on the existing `typeinfo.classes` → `classes` column (no skill filter — Assassins use piercing **and** slashing).

**Change:** add `Skill` (`json:"skill"`) and `WieldStyle` (`json:"wieldstyle"`) to `census.TypeInfo`; add `skill` + `wieldstyle` columns to the weapon CSV write/read (Plan 1 §10 schema). Then **`go run ./cmd/itemdex --refresh`** to repopulate `data/`. (The discovery-window pull is the slow, multi-session part — a one-time prereq.)

---

## 3. The DPS Model

`TotalDPS = AutoDPS + CADPS`, computed in **parallel** — auto-attack and combat-art casting run on independent timelines, so casting does **not** displace auto swings (a CA's cast+recovery only paces the *CA* side; see §3.1). Locked combat values: crit ×1.30; **flurry ×4** (a flurry proc does +100%–500%, averaging +300% = ×4); **ability-mod cap = 50% of the potency-adjusted CA base**; reuse halves recast at 100%; **potency applies to CAs only**; `critbonus` ignored. Haste and multi-attack are **non-linear** (shared conversion curve, §3.1) — *not* `1 + stat/100`; dps-mod is **linear** (+1%/point, hard cap 200 → ×3). Stat-conversion mechanics, the AA cooldown/recovery effects, and the curve table are in **§3.1** (the authoritative stat-mechanics block, revised from the Varsoon playtest session).

**Why the Assassin CA query is integral (not just additive damage):** the CA term needs each Combat Art's *base* damage to model potency and ability-mod correctly — potency scales the base `×(1+potency)`, and ability-mod caps at **50% of that potency-adjusted base**. Without the per-CA base damages, neither term is computable. So `internal/spell` pulls the Assassin's CAs (Expert tier, level ≤70), regex-parses damage from `effect_list`, and applies the TLE translations (§ Assumptions).

**Derive-don't-declare:** for each baseline, compute the marginal DPS per stat (`∂TotalDPS/∂stat`) at a realistically-geared baseline → the **weights**; iterate (equip current best → recompute near caps → re-rank) to convergence so saturation/caps self-correct. Score each Assassin-usable item `= Σ(weight × itemStat)`; rank per slot. **No stat is pre-judged valuable or dead — the weights are computed outputs.**

**Weapons:** Soulfire (Mythical, from the raid questline) is the given main-hand; the off-hand is ranked across all Assassin-usable 1H weapons (any skill). The model accounts for dual-wield via `wieldstyle`.

---

## 3.1 Stat-Conversion Mechanics (authoritative — Varsoon playtest revisions)

Corrections gathered from in-game testing on Varsoon. These **supersede** the simpler assumptions in `docs/design.md` §4. One value (haste sharing the multi-attack curve) is **inferred** and flagged as such — trivially revised if better numbers surface. (dps-mod was initially assumed to share the curve too, but playtest data showed it is linear — see Linear stats below.)

### Shared "combat-mod" conversion curve — haste, multi-attack

These two stats are **non-linear**: the stat value converts to an effect % via a single diminishing-returns curve. Both land on **200 stat → 125% effect**, which is why we treat them as one shared curve (multi-attack is measured; haste is *inferred* to share it — flagged above). We have only sampled points (no formula), so the model **interpolates piecewise-linearly between the samples, anchored at (0,0), then floors to whole %** (the game evaluates a continuous formula and floors per integer %).

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

(Effect % is stored as a single number = `min(double,100) + triple`, e.g. `120 → 102`.)

Per-stat cap behavior:
- **Multi-attack** — no hard cap; runs the full curve to `3400 → 200%`. The portion **above 100% is triple-attack chance** (e.g. `120` = 100% double + 2% triple). Auto-attack swing multiplier = `1 + effect/100`. **Auto-attack only** (does not touch CAs). TLE key: `doubleattackchance`.
- **Haste** — **hard cap at 200 stat (125%)**; over 200 is **wasted** (it does **NOT** convert to flurry — that earlier assumption is removed). `effDelay = baseDelay / (1 + hasteEffect/100)`.

### Linear / direct stats
- **DPS-mod** — **linear, +1% auto-damage per point** (playtest: 100→×2, 200→×3, 300→×4, 500→×6). Auto-damage multiplier = `1 + min(dpsmod, 200)/100`; **hard cap 200 → ×3 (+200%)**, over 200 wasted. **Auto-attack only.** Supersedes the earlier +125%@cap guess.
- **Crit chance** — linear, `crit% = stat`. `critFactor = 1 + (crit%/100)·(1.30−1)`.
- **Potency** — linear, `potency% = stat`; scales CA base `×(1 + potency/100)`.
- **Flurry** — **gear % only** (no haste contribution); `flurryFactor = 1 + (flurry%/100)·(4−1)`.
- **Ability-mod** — flat add to each CA, capped at `0.50 × CA_maxDamage × (1+potency/100)` (per-CA; potency floats the cap up, so it binds mainly on small filler arts).

### Combat-art rotation / recovery
- **Recovery time** is real and folded into CA pacing: each cast occupies `cast (0.5s) + recovery (0.25s) = 0.75s` of the **CA** timeline before the next cast (weapon DPS amortizes over full delay; CA throughput must amortize over full cast+recovery). Recovery is a flat per-cast constant for CA-vs-CA ranking, but it is **not** a common factor for CA-vs-auto, because it sizes total CA damage over the fight. The base recovery is 0.5s, **halved to 0.25s by an AA**.
- **AA cooldown reduction:** a large AA halves the **base** recast of **Assassinate** and **Mortal Blade** by 50% (300→150s, 180→90s). **Reuse applies to the already-halved base** (order: ×0.5 AA, then reuse).

### Weight derivation under non-linear stats
For the two curve-stats (haste, multi-attack) the marginal weight is taken from the **sample-to-sample slope** at the baseline — evaluate DPS at the sample points bracketing the baseline and divide by the stat gap. At sample points the floored effect equals the table value exactly, so this yields the true segment slope with no flooring noise (the floor makes real gains lumpy, but the per-point weight should read as the smooth "going rate"). Linear stats — including **dps-mod** — use the standard +1 finite difference (capped stats yield ~0 at the cap by construction).

### Removed / changed constants
- **Removed:** `HasteCapPct` (100), `HasteToFlurry` (10:1), `DPSModEffectAtCap` (1.25), and the haste→flurry term.
- **Added:** the shared curve table + interpolation (haste, MA); `HasteStatCap` = 200; `CARecoverySecs` = 0.25; per-art AA cooldown halving for Assassinate/Mortal Blade. `DPSModCap` = 200 stays (now a linear clamp → ×3).
- **Changed:** flurry ×5 → **×4**; dps-mod +125%@cap → **+200%@cap** (linear +1%/point).

---

## 4. The Two Baselines

A labeled constants block (`internal/baseline`) defines two input stat profiles. Each is **only an input stat block**; the model derives the weights from it — this spec asserts nothing about which stats end up mattering under either.

- **Solo:** the Assassin's self-buffs only (Villainy → +34.2 Multi-Attack; the temporary self-haste/DPS self-buff while active). No group DPS-mod.
- **Raid:** self-buffs + group package; **DPS-mod = 200 (capped)**; the Velocity-style group contribution; crit elevated by AAs/buffs. Haste from the same comp (no maintained group haste buff).

Each baseline yields its own derived weight set → its own BiS list. The **solo-vs-raid difference is an output** (it shows which stats change between contexts), not an assumption.

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
| `cmd/bis` *(new)* | orchestrate: load gear (from CSV cache) → pull CAs → derive weights ×2 baselines → score → load SQLite → emit report + DB. Flags: `--out`, `--lock` (§8), `--db`. |

Data flow: gear (Plan-1 cache) + Assassin CAs → `model` derives weights per baseline and scores each Assassin-usable item → `store` loads gear + scores into SQLite → `cmd/bis` runs ranking SQL and renders the markdown report.

---

## 6. SQLite Schema (modernc, pure-Go)

**Normalized schema:**
- `items` — one row per gear item: `id`, `name`, `slot`, `tier`, `itemlevel`, `armor_type`, `skill`, `wieldstyle`, `classes`, `gamelink`.
- `item_stats` — `(item_id, stat, value)`, one row per modifier (friendly for SQL aggregation / scoring).
- `scores` — `(item_id, baseline, dps_score, slot)` so a single query ranks per slot per baseline, and the user can sort/filter/explore (coverage gaps, runner-ups, etc.) freely.

The DB is both an analysis engine and a shareable artifact.

---

## 7. Outputs

- **`bis-report.md`** — for each baseline (solo, raid), a per-slot listing of the **top 3 Fabled + top 3 Legendary** items (name, tier, DPS score, key stat line, gamelink), plus the **derived stat-weight table** and the **assumptions/constants block**. Rationale: Legendary ≈ dungeon gear, Fabled ≈ raid gear, so the split gives non-raid options alongside raid options — and surfaces the cases where a Legendary out-scores a Fabled (a flat top-N would hide that). Any **Mythical** in a slot (e.g. the Soulfire weapon) is shown at the top as the ceiling. Showing multiple per tier also supports the set-bonus overlay (§8).
- **Every ranked item shows a score *breakdown*** — its top contributing terms as `stat × weight` (e.g. `crit 1.8 × W_crit = …`, `MA 4.0 × W_ma = …`), not just the total. This makes each ranking **explainable**, which is the point of §9: an expert reading the report can see *why* an item placed where it did and immediately spot a wrong weight/constant.
- **`bis.db`** — the scored SQLite DB.

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

The **solo-vs-raid diff** is reported as an output and sanity-read — never asserted in advance.

---

## 10. Implementation Notes (Go)

- `modernc.org/sqlite` (pure-Go, cgo-free) via `database/sql`.
- Reuse Plan-1 idioms (throttled client only needed for the CA pull + the re-pull; `slog` progress; testify tests; `golangci-lint`).
- The CA pull is small (a few dozen Assassin CAs) — one throttled query batch, not the full-catalog ordeal.
- Keep `model` pure/deterministic (no I/O) so the DPS math is unit-testable in isolation.

---

## 11. Assumptions & Constants Block (single source of truth)

All from `docs/design.md` §2.1 / §4, reproduced in `internal/baseline` with provenance + validation flags:

- **Combat constants (see §3.1 for the authoritative, revised mechanics):** crit ×1.30; **flurry ×4**; **haste & multi-attack** via the **shared non-linear curve** (haste caps at 200 stat → 125%, MA runs to 3400 with triple overcap); **dps-mod linear** (+1%/point, hard cap 200 → ×3); **haste overcap does NOT convert to flurry**; ability-mod cap = 50% of potency-adjusted CA base; reuse: 100% → half recast (applied *after* the Assassinate/Mortal Blade AA cooldown halving); potency on CAs only; **CA cast+recovery = 0.75s** paces the CA timeline (auto runs in parallel); attributes excluded (track itemlevel).
- **TLE translations:** `doubleattackchance` → **Multi-Attack** (legacy key; displayname already "Multi Attack"); `critbonus` → **ignored entirely**; Fervor → does not exist.
- **CA tier:** Expert (classic "Adept III" — the farmable raiding baseline).
- **Baselines:** Solo (self-buffs only) and Raid (self + group, DPS-mod = 200 capped) — values tagged "confirm vs guild leader / Varsoon parse."

---

## 12. Open / To-Confirm (non-blocking)

- Baseline numeric values (confirm with guild leader / Varsoon parse) — parameterized, so refinement is a re-run.
- Exact crit baseline per context (AAs vs buffs) — feeds the baselines.
