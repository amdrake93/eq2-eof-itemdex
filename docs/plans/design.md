> **HISTORICAL — superseded by `docs/SPEC.md` (the source of truth). Kept as timeline history; do not treat as current.**

# EoF Assassin Best-in-Slot via a Derived-Weight DPS Model

**Date:** 2026-06-02
**Status:** Design / spec (pre-implementation)
**Type:** Personal project (non-work)
**Implementation language:** Go (chosen for learning; design itself is language-neutral)

---

## 1. Goal & Scope

Produce a **best-in-slot (BiS) gear list, per equipment slot, for a level-70 Echoes of Faydwer (EoF) Assassin** as it exists on a Daybreak Census *Time-Locked Expansion* (TLE) server (Varsoon-style), ranked by a **relative DPS model**, together with the **derived per-stat weights** that drive the ranking.

- **Deliverable:** an analysis/report (per-slot BiS table + derived stat weights + a fully provenance-tagged assumptions block). Not a reusable product, not a raw dataset dump.
- **Secondary deliverable:** shareable CSV exports (§10) — an **all-class EoF *gear* catalog by category** (every itemlevel/tier; non-gear excluded), plus a cross-class Max Health list for tanks (optionally KoS-extended). The Assassin-usability filter applies only to the stat-weight/BiS math, never to the CSVs.
- **Class:** Assassin only (Scout → Predator; melee DPS).
- **Item universe:** all EoF items the Assassin can equip, **from all sources** (raid / group / quest / dropped), each **tagged** by source/tier so the raid-BiS and the no-raid-realistic pick are both visible.
- **Item version:** the **Varsoon-current** version of each item (the version actually present on the target server), selected automatically by the classifier in §3.

### Non-goals
- Absolute/parse-accurate DPS numbers. The model produces **relative ordering only** — which is all BiS requires.
- Other classes, other expansions (the method generalizes, but this spec covers EoF Assassin).

---

## 2. Data Sources

Daybreak Census API (`https://census.daybreakgames.com`), namespace `eq2`.

- **Service ID:** public `s:example` only — throttled to **10 requests/min per IP**. No private key available. The pipeline must be throttle-aware (page small, prune server-side, back off on 429).
- **Collections used:**
  - `item` (~510K records) — gear stats, weapon data, discovery provenance.
  - `spell` (~167K records) — Combat Arts (damage in `effect_list` text, plus `recast`/`cast` timing, `level`, `tier_name`).
  - `world` — world-id lookup (Varsoon = **614**).
- **Verbs/operators relied on:** `get`, `count` (times out on large sets — avoid), field projection `c:show`, paging `c:limit`/`c:start`, match operators `^` (starts-with) and `*` (contains), range operators (`<`, `>`, `[`, `]`).
- **Known schema facts:**
  - Item name search key: `displayname_lower`. Item type name field: `displayname` (not `displayed_name`).
  - Spell name search key: `name_lower`.
  - Equip usability is encoded per item in `typeinfo.classes` (a map of class → min level). Filtering to `assassin` here resolves armor-type rules automatically.

### 2.1 The modern-Census ↔ TLE translation problem (critical)
Census returns the **current/modern** version of every item and spell. The target server runs the **modern combat engine with EoF content/level gated on top**, but **not every modern stat is active**, and some modern stats are **remaps of era stats**. Therefore Census data must be **translated** to TLE behavior before use.

**Provenance hierarchy (highest → lowest authority):**
1. Varsoon parse / in-game testing data
2. Guild leader / active TLE raider knowledge
3. User (project owner) memory
4. EoF-era deep research (forum/archive theorycraft)
5. Census modern-inferred values (lowest — must be translated, never trusted raw)

**Known translations (each validated individually, not by blanket rule):**

| Census shows | On TLE | Provenance |
|---|---|---|
| Fervor / Fervor Overcap | **does not exist** → ignore | user |
| `critbonus` (item modifier) | **ignored entirely** — stripped on server | user |
| Velocity (Coercer) "Multi-Attack +57.6" | **DPS-mod +57.6** (era remap, same value) | user |
| Villainy (Assassin) "Multi-Attack +34.2" | **stays Multi-Attack** | Varsoon testing |
| Double Attack (engine) | **removed from engine** | user |
| `doubleattackchance` (item modifier key) | **= Multi-Attack** — legacy key; its Census `displayname` is already "Multi Attack" (`multiattackchance`/`multiattack` keys do not exist) | Census probe |

> Every translation is per-effect and must carry its provenance. Do **not** generalize one buff's remap to another.

---

## 3. Item Classifier — EoF on Varsoon (validated, locked)

For each candidate item, read `_extended.discovered.world_list`, find the entry with `id == 614` (Varsoon), and **keep the item iff** that entry's `timestamp` falls in:

```
[ 2023-04-11 , 2023-08-08 )       # EoF unlock window on Varsoon
```

- **Lower bound (2023-04-11):** White Oak Acorn, an **EoF-exclusive** collectable — content-gated, so EoF content cannot be discovered before this.
- **Upper bound (2023-08-08):** Tuft of Dark Brown Brute Fur, a **RoK-exclusive** collectable — the next expansion's content cannot appear before this, making it a hard exclusive ceiling.
- The `world_list[614]` entry naturally selects **the version present on Varsoon** (e.g. for Soulfire Hammer it picks the 2023 reissue, not the 2007 original or 2020 reissue — those have no 614 entry).

**Validation (5/5 calibration items consistent):**

| Item | Expansion | Varsoon discovery | In window? |
|---|---|---|---|
| Qeynos Claymore | KoS | 2022-12-13 | no (below) ✓ |
| Windrazor | KoS | 2022-12-11 | no (below) ✓ |
| White Oak Acorn | EoF | 2023-04-11 | yes ✓ |
| Soulfire Hammer | EoF | 2023-06-14 | yes ✓ |
| Tuft of Dark Brown Brute Fur | RoK | 2023-08-08 | no (at ceiling) ✓ |

Window width ≈ 17 weeks, matching the documented TLE cadence (16–18 weeks/expansion).

**Error modes:**
- *Recall tail:* an obscure EoF item first looted after 2023-08-08 would be misfiled as RoK. Cannot pad past the ceiling without catching RoK. Small, affects only rarely-looted items.
- *Precision:* a KoS item first looted on Varsoon during the EoF window (un-looted through all of KoS). First-discovery is sticky, so re-drops don't trigger this; only genuinely-never-looted-until-EoF items do. Rare.

---

## 4. The DPS Model

Relative sustained DPS for a fixed, buffed baseline Assassin. **All gear-variable stats are included; no stat is excluded a priori.** Stat value is an *output* (derived weight), never an input assumption.

### 4.1 Equations

```
TotalDPS = AutoDPS + CADPS

AutoDPS = (avgWeaponDmg / effDelay)
          × (1 + MA% / 100)            # Multi-Attack: extra auto swings (auto-attack only)
          × critFactor
          × flurryFactor
          × dpsmodFactor

   effDelay     = baseDelay / (1 + min(haste%, hasteCap))
   critFactor   = 1 + critChance% × (critMult − 1)          # critMult = 1.30
   flurryFactor = 1 + flurry%_total × (flurryMult − 1)      # flurryMult = 5.0
                  flurry%_total = flurry_gear% + max(0, haste% − hasteCap) / hasteToFlurryRatio
   dpsmodFactor = DR_curve(DPSmod, softcap, hardcap)        # diminishing-returns; overcap wasted

CADPS = Σ_CA  CAdmg(CA) / effRecast(CA)

   CAdmg     = base × (1 + potency)
               × (1 + min( abilityMod / (base × (1 + potency)), 0.50 ))   # +CA-dmg cap = 50% of potency-adjusted base
               × critFactor
   effRecast = baseRecast × (1 − 0.5 × min(reuse%, 100%))   # 100% reuse halves recast (cap)
```

### 4.2 Mechanics constants & rules (modern engine / TLE)

> **Superseded for stat mechanics** by `design-plan2.md` §3.1 (the authoritative, data-revised block). Known-stale rows here: flurry is ×4 (not ×5), haste does NOT overcap into flurry, the haste/dps-mod curve is a fitted equation with a **300** hard cap — the "hard cap ≈ 200 (= +125%)" research claim below was disproven by live readings — and (2026-06 rotation revision) reuse converts at **1%/pt capping at 50 stat** within a per-art 50% ceiling shared with AA reductions (not the half-strength rule below), recovery speed is a real subtractive stat (not "constant → ignored"), and cast speed is a real divisor stat on gear and AAs (not negligible). This table is kept as the original research record.

| Mechanic | Treatment | Provenance |
|---|---|---|
| **Critical hit** | Fixed ×1.30 multiplier; `critFactor = 1 + critChance% × 0.30`. Applies to auto-attacks **and** CAs. Crit **Bonus is not a gear stat** (baseline only). | user |
| **Multi-Attack** | `autoSwings = 1 + MA%/100`. 1–100% = chance of a 2nd swing; >100% = guaranteed 2nd + chance of a 3rd, etc. Auto-attack only. | user |
| **Double Attack** | Removed from engine — not modeled. | user |
| **Flurry** | Chance to apply a fixed **5×** multiplier to the auto-attack, applied **after** all other multipliers. No flurry-multiplier stat. Possibly bursts up to ~4 swings (unconfirmed). | user / guildmates |
| **Haste** | `effDelay = baseDelay / (1 + min(haste%, cap))`. Cap and overcap→flurry ratio are parameters (user: cap ≈ 100%, overcap → flurry at 10:1; research: 200 pts = +125%, soft ~80 — reconcile, see §7). | user + research |
| **DPS-mod** | Diminishing-returns curve; **hard cap ≈ 200 (= +125%)**, overcap wasted; soft-cap value TBD. | research + guild leader |
| **Potency** | Abilities only (not auto-attack): `CAbase × (1 + potency)`. Also raises the +CA-dmg cap (via the larger adjusted base). | user |
| **Ability mod (+CA dmg)** | Flat add to a CA, capped at **50% of the potency-adjusted base** (`0.5 × base × (1+potency)`). DoT front-loading of 50% is a timing detail, ignored for total output. | user + research |
| **Reuse** | `effRecast = baseRecast × (1 − 0.5 × min(reuse%, 100%))`. 100% reuse → recast halved (cap). | user |
| **Recovery speed** | Constant across CAs → ignored (constant factor). | user |
| **Cast speed** | Negligible (CA base casts ≈ 0.5s) → ignored. | user |
| **Primary attributes (STR/AGI/STA)** | **Excluded** — they track itemlevel and barely vary between same-ilvl items, so no discriminating power. | user |
| **Attack rating vs target mitigation/avoidance** | Held constant (fixed raid target) → cancels in relative ranking. | assumption |

### 4.3 Derivation of stat weights (the method)
1. Establish the **buffed baseline** stat block (§5): group buffs + AAs + self-buffs + a realistically-geared starting set.
2. Compute each stat's **marginal DPS** numerically: `∂TotalDPS/∂stat` at the baseline.
3. **Iterate:** equip the current best item per slot → recompute the baseline → recompute weights → re-rank. Repeat to convergence. (Necessary because caps/DR make weights gear-state-dependent: a stat already at its cap from buffs + gear yields a derived weight near 0 — *computed, not declared*.)
4. **Score** each candidate item per slot as `Σ (weight_stat × item_stat)`; rank within slot.

> Saturation emerges from the math. If a baseline input is wrong, the derived weight self-corrects once the input is fixed. The spec asserts **no** conclusions about which stats are valuable.

---

## 5. Baseline Buff / AA Package (parameterized input)

Group composition: **Berserker + Dirge + Coercer + Assassin**. Buffs are level-based; the EoF-cap-relevant versions are level **57–70**, at the **Expert** tier (consistent with the CA tier in §6). Values are pulled from the `spell` collection and **translated** per §2.1.

| Source | Buff (EoF-cap version) | TLE effect (post-translation) | Maintained? | Provenance |
|---|---|---|---|---|
| Coercer | Velocity IV (L62) | **+57.6 DPS-mod** (Census shows MA; remapped) | yes | user |
| Assassin | Villainy IV (L55) | **+34.2 Multi-Attack** (stays MA) | yes (self) | Varsoon testing |
| Dirge | Cacophony of Blades (L58) | +52.2 Haste | **temporary** → **excluded** from sustained baseline | Census + user |
| Berserker + full raid package | *aggregate, not itemized* | **brings DPS-mod to its ~200 cap** → baseline DPS-mod = **200**, pre-gear | yes | assumption (user) |
| Assassin self (haste) | (temporary) | excluded from sustained baseline | no | user |

**Notes / open items folded into §7:**
- **Working assumption (resolves the Berserker placeholder):** in a raid setting the **aggregate buff package brings DPS-mod to its ~200 cap with no gear contribution**, so the baseline DPS-mod input = **200**. We do not itemize the Berserker buff. Per the derive-don't-declare principle, the model then computes whatever gear-DPS-mod weight follows from a capped baseline — expected near-zero, but *derived, not asserted*.
  - **Superseded (2026-06, see `design-plan2.md` §4):** the cap is 300 (not 200), the live comp has no Berserker, and the measured group DPS-mod is **114.2** (Coercer 74 + Inquisitor 30.2 + Dirge 10) — mid-curve, so gear dps-mod is *not* near-zero in raid.
- This group has **no maintained haste buff** (only temporary), so sustained haste comes from gear/AA — the model will reflect that in the derived haste weight.
- The buff package is an **explicit input**: changing the comp (e.g. swapping in a Troubadour with maintained haste) changes the baseline and therefore the derived weights. The saturation set is a *function of the comp*, not a constant.

---

## 6. Combat-Art (Spell) Pipeline

- Pull the Assassin's Combat Arts from `spell`: `type = arts`, `level ≤ 70`, **tier_name = "Expert"** (classic "Adept III" → Census "Expert"; user-confirmed). Expert is the realistic raiding baseline on a 3-month TLE window (Master is not reliably obtainable; Adept III/Expert is the farmable expectation).
- **Parse damage** from `effect_list` descriptions via regex, e.g. `"Inflicts X - Y <type> damage on target"` → use the **max** (Y) as the base for the +CA-dmg cap, average for DPS.
- **Apply TLE translations** (§2.1): strip non-existent stats (e.g. Fervor), respect per-effect remaps.
- Use the structured `recast_secs`, `cast_secs_hundredths`, `recovery_secs_tenths`, `level`, `tier_name` fields for timing.
- Long-recast nukes (e.g. Assassinate, 300s) contribute little to *sustained* DPS — they enter the model via `dmg / recast` naturally, so no special-casing needed.

---

## 7. Open Parameters & Validation TODOs

To confirm with **guild leader / Varsoon parses** (in priority order of impact):
1. **Buffed DPS-mod baseline = 200 (cap), pre-gear** — working assumption (replaces itemizing the Berserker buff). Confirm the raid package actually reaches cap; if it falls short, set the real value and the model re-derives the gear-DPS-mod weight.
2. **Berserker buff (optional refinement)** — itemizing the actual Berserker group buff would let us model sub-cap cases, but is unnecessary while the cap assumption holds.
3. **DPS-mod soft-cap value** on TLE (the ~200-hardcap system's soft cap; the EoF ~80 / generic-modern values do not match and should not be assumed).
4. **Haste reconciliation** — cap as % effect (user: ~100%) vs points (research: 200 = +125%), and the exact overcap→flurry ratio (user: 10:1).
5. **Flurry** — confirm the **5×** multiplier and whether it bursts up to 4 swings.
6. **Crit sources/baseline** — AA tree vs gear vs buffs; baseline crit chance and the (item-absent) crit-bonus baseline.
7. **Reuse formula** — confirm the 100%→half-recast cap and linearity.

Each parameter lives in a single labeled constants block in the implementation, tagged with value + provenance + a "validate against Varsoon" flag.

---

## 8. Extraction Pipeline (throttle-aware)

### Data-source modes
The pipeline reads item data from one of two sources, so we don't re-hit the throttled API needlessly:
- **Default — load from local CSVs if present.** If the §10 catalog CSVs exist, the run reads item data from them (zero Census queries). The CSVs are the **local cache / source of truth**.
- **Explicit fresh pull — `--refresh` (or `--source=census`).** Forces a fresh Census extraction (the steps below), which then **(re)writes** the catalog CSVs. Used for first run, corrections, or extending coverage (e.g. the optional KoS pass).

A fresh pull also persists the **CA + buff spell data** (§5, §6) to a small cache (CSV/JSON) so default runs are fully offline. The DPS model applies TLE translations (§2.1) **on load**, regardless of source — the CSVs always store **raw, untranslated** values. EoF/KoS data on a TLE server is effectively static content, so cached CSVs don't go stale within a progression phase.

### Fresh Census pull
**One broad EoF pull feeds every deliverable**; class/slot/stat filters are applied **downstream**, so the expensive throttled data is queried once.

1. **Verify** the server-side nested filter works: `item/?_extended.discovered.world_list.id=614`. If unsupported, fall back to an `itemlevel` ceiling + full client-side filtering.
2. **Prune server-side:** `world_list.id=614` **+** an `itemlevel` ceiling (~≤72, safe for the level-70 EoF cap) to drop level-80+ content → bounded candidate pool.
3. **Page** with modest `c:limit` (~100), `c:show` trimmed to needed fields (incl. `displayname`, `modifiers`, `typeinfo`, `slot_list`, `tier`, `itemlevel`, `gamelink`, `_extended.discovered.world_list`), **≤10 req/min**, backing off on 429.
4. **Apply the discovery-window classifier (§3)** → the **full EoF item set, all classes**. *No class filter at query time.*
5. **Derive each deliverable downstream from that one set:**
   - *CSV catalog (category + max-life, §10):* the **full EoF set, all classes** — no class filter. Split by slot category; max-life filtered to items with a Max Health modifier.
   - *Assassin BiS + stat weights:* filter to `typeinfo.classes` contains `assassin` — this subset feeds the DPS model (§4) and BiS output (§9) **only**.
6. **Pull CAs** once (§6).
7. **(Optional) KoS pass for max-life** (§10): if EoF is sparse, repeat steps 2–5 for the KoS window `[~2022-12-11, 2023-04-11)`, keeping only items with a Max Health modifier.
8. **Translate** Census values to TLE (§2.1) **only** on the DPS-model path — **never** for the CSV exports (§10), which stay faithful/untranslated.

---

## 9. Output

- **Per-slot BiS table:** item name, source, tier, DPS score, key stat line, runner-up.
- **Derived stat-weight table:** the converged marginal-DPS weight per stat (the auditable byproduct).
- **Assumptions/parameters block:** every constant, buff, and translation with its value, provenance, and validation status.

---

## 10. Shareable Item Data Export (CSV)

A persistence/sharing deliverable, **independent of the DPS model**: a faithful catalog of the queried items so others can browse items by slot and see their stats. Because Census queries are throttled (§2), this also preserves the expensive query results for reuse without re-querying.

### Scope
**The CSV catalog is every EoF-era *gear* item and its stats, all classes.** Non-gear (slot category "other": collectibles, house items, food/ammo) is excluded; **gear of every itemlevel and tier is kept — no itemlevel floor**, because flooring the dataset would blind the model to low-itemlevel/high-quality pieces. The Assassin-usability filter (`typeinfo.classes` contains `assassin`) is applied **only** to the stat-weight / DPS model / BiS analysis (§4, §9) — **never** to the CSV exports. All exports derive from the single broad EoF pull (§8):
- **Category files** (weapons / armor / jewelry-charms): all EoF *gear*, split by slot category — any class.
- **Max-life cross-cut** (see below): every EoF *gear* item with a Max Health stat — any class.

One row per item, using the Varsoon-current version.

**Why no scalar floor:** quality is driven by **tier** (Treasured < Legendary < Fabled < Mythical — combat-power medians ≈ 24 / 96 / 166 / 217) far more than itemlevel, and tier × itemlevel compound (a 60 Fabled ≈ a 70 Treasured), so no single cutoff (itemlevel *or* stat-total) separates good from bad — and a stat-total is resist-skewed besides. The **class-weighted DPS model (§4) is the comparator**; `tier` is a CSV column for human sorting. **Optional future trimming (Plan 2 era):** strict-domination dedup (drop an item another strictly beats on every stat — e.g. a low-ilvl piece with an identical-but-smaller stat line) and per-slot/armor-type stat-coverage-gap flags. Neither is an itemlevel floor. A SQLite-backed load (`modernc.org/sqlite`, pure-Go) is a candidate to make this analysis easier.

### Faithful & untranslated (key rule)
Item stats are exported **exactly as Census reports them — no TLE translations.** Stats "are what they say they are." The modern-Census↔TLE translation layer (§2.1) applies **only to the DPS model**, never to this CSV. The export is an honest "what the item says" catalog; a reader wanting server-accurate behavior consults §2.1 separately.

### Columns (wide format)
Focused on what's useful for browsing/sharing:
- `name` (displayname)
- `slot` (from `slot_list`; multi-slot items list all their slots)
- `tier`, `itemlevel` — light identifying context
- **one column per stat** — the union of all `modifiers` keys across the dataset; blank where an item lacks that stat; values exactly as Census returns them.
- **category-specific columns:**
  - *weapons:* `weapon_min_dmg`, `weapon_max_dmg`, `delay`, `damage_rating` (from `typeinfo`)
  - *armor:* `armor_type` — **Cloth / Leather / Chain / Plate**, derived from `typeinfo.skilltype` (e.g. `heavyarmor` → Plate) / `typeinfo.knowledgedesc`
- `classes` (or a `usable_by_assassin` flag) — equip eligibility from `typeinfo.classes`; valuable since the catalog spans all classes
- `gamelink` — the in-game item link string (Census `gamelink` field)
- `id` — Census item id, for traceability

**No DPS score, rank, or model output** — this file is deliberately decoupled from the model.

### Grouping (workshop — adjustable)
Following the prior convention, split by equipment **category** (not one-file-per-slot, not a single combined file). Proposed mapping:
- **weapons.csv** — primary, secondary, ranged
- **armor.csv** — head, shoulders, chest, forearms, hands, legs, feet
- **jewelry-charms.csv** — neck, ears, wrists, rings, charms, waist/belt, cloak

*Open for the build:* exact home of **cloak** and **waist/belt** (armor vs jewelry-charms).

### Cross-cut export: every item with Max Health (`maxlife.csv`)
A special-interest list cutting across **all categories and all classes**: every EoF-era item carrying a **Max Health** modifier (Census `maxhpperc` = "Max Health", plus any flat health stat), regardless of who can equip it. Intended as a **tank/survivability** reference, not for the Assassin. Same column treatment as the category files (untranslated: name / slot / tier / itemlevel / stats / `gamelink` / id). Items here may also appear in the category files.

**Optional KoS extension:** if EoF max-life pickings are slim, extend this list to **KoS-era** items by also accepting the KoS Varsoon window `[~2022-12-11, 2023-04-11)` (KoS unlock → EoF unlock). Tag each row with its source expansion (`eof` / `kos`). Caveat: the KoS **lower** bound is not yet pinned by a KoS-exclusive collectable (only bracketed by Qeynos Claymore 2022-12-13 / Windrazor 2022-12-11) — pinning it precisely would need a KoS-exclusive calibration item. Conditional: only if EoF alone is too sparse.

### Notes
- Wide format ⇒ the stat-column set is the union across each category file; the header row documents it.
- CSVs are generated from the **same in-memory item set the model consumes** — no extra Census queries needed to produce them.
- **Dual-duty:** these CSVs are also the pipeline's **local data cache** (§8 data-source modes). By default, subsequent runs read items from them instead of re-querying Census; a fresh pull requires the explicit `--refresh` flag and rewrites them. To round-trip cleanly as a cache, the schema must preserve everything the model needs (all stats, weapon fields, `armor_type`, `classes`, slot) — not just human-friendly columns.

---

## 11. Implementation Notes (Go)

Design is language-neutral; chosen implementation is **Go** (learning goal, mirrors a work webapp migration).

- **Dependencies (lean idiomatic, not stdlib-only):** `golang.org/x/time/rate` (throttling), `github.com/stretchr/testify` (tests), `log/slog` (logging); tooling `golangci-lint` + `Makefile`. Everything else stdlib.
- HTTP: stdlib `net/http` with a `rate.Limiter` (10 req/min) and a 429 backoff retry.
- JSON: `encoding/json` with typed structs for the item / spell / world_list shapes; keep a permissive decoder for the variable `typeinfo`/`modifiers`/`flags` sub-objects (`map[string]json.RawMessage` or targeted structs).
- Parsing: `regexp` for `effect_list` damage extraction.
- Paging: bounded goroutine concurrency is unnecessary given the 10/min cap — sequential paging with the throttle is simplest and correct.
- Model: a small numeric package for the DPS equations + the iterative weight solve.
- Constants/translations: one well-labeled source file as the single source of truth (mirrors the §4.2 / §5 / §7 tables), each with provenance + validation flag.

---

## 12. Validation / Testing Approach

- **Parser unit tests** against known calibration data: the §3 calibration items (Soulfire, Qeynos Claymore, the two collectables, Windrazor) for the `world_list[614]` classifier; the Assassinate CA for the `effect_list` damage regex.
- **Classifier check:** the 5 calibration items must classify exactly as in §3's table.
- **Sanity anchors:** known EoF Assassin gear should surface where expected — e.g. **Grinning Dirk of Horror** (a top EoF piercing weapon per era theorycraft) should rank near the top for the weapon slot under a high-crit baseline.
- **Weight sanity:** present the derived weights to the guild leader for a gut-check against lived raid experience.
- **Sensitivity:** because several inputs are TLE-uncertain (§7), report how the BiS ordering shifts under the plausible range of each open parameter, so the conclusions' robustness is visible.
