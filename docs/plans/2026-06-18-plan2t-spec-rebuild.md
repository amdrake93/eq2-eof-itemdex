# Plan 2t: Whole-System Spec Rebuilt From Code Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce `docs/SPEC.md` — one whole-system spec derived from the code — and retire the two drifting design docs into the historical `docs/plans/` timeline.

**Architecture:** This is a **documentation task — no code changes**. The spec is written by auditing each package against the actual code, its comments, the `data/*.csv` calibration files, and the tests. There is no TDD here; each section's "test" is a cross-check that every formula/constant/behavior it states matches the cited code symbol. Proof that no code changed: `go test ./...` stays green throughout. The spec has two parts — Part I (the system: how the program works) and Part II (the game: how EQ2 mechanics work, calibrated, as one authoritative block). Source of truth is the code; old docs/memory are consulted only to enrich provenance, never to settle current behavior.

**Tech Stack:** Markdown; Go 1.26 codebase (`github.com/amdrake93/eq2-eof-itemdex`); SQLite (modernc); Census API.

**Design reference:** `docs/plans/2026-06-18-spec-rebuild-design.md` (approved).

**Authoring rules for every section:**
- State only what the code does. If code comment and old doc disagree on present behavior, code wins.
- Cite the owning symbol inline (e.g. `internal/model/rotation.go effRecast`) so the claim is checkable.
- Keep provenance (measurement date, reading count, "disproven X") at spec altitude — summarize, don't transcribe.
- No placeholders. Every constant/formula stated must be confirmed against the cited code before the section is committed.
- Do NOT read `docs/design.md` / `docs/design-plan2.md` to author current-behavior prose; use them only to recover a provenance *fact* the code lacks.

---

## Task 1: Skeleton + front matter

**Files:**
- Create: `docs/SPEC.md`

- [ ] **Step 1: Create the file with front matter and all section headers**

Write `docs/SPEC.md` containing exactly this skeleton (sections filled in later tasks):

```markdown
# EQ2 EoF Assassin Best-in-Slot — System & Model Spec

**Status:** Living spec — the single source of truth for this system.
**Supersedes:** `docs/plans/design.md` and `docs/plans/design-plan2.md` (historical design records; kept as timeline only).
**Module:** `github.com/amdrake93/eq2-eof-itemdex` · **Go:** 1.26
**Provenance rule:** This spec describes what the code does. Where prose states a formula or constant, it matches the cited code symbol. Measurement provenance is summarized from code comments, `data/*.csv`, and tests.

---

# Part I — The System (how the program works)

## 1. Goal & Deliverables
## 2. Architecture & Data Flow
## 3. Data Acquisition (Census)
## 4. Catalog & Persistence
## 5. Combat-Art Pipeline
## 6. Configuration & Accessibility Tiers
## 7. BiS Engine
## 8. Commands & Operations
## 9. Testing & Calibration-Sync

---

# Part II — The Game (mechanics, calibrated)

## 10. The DPS Model
## 11. Stat-Conversion Mechanics
## 12. Auto-Attack Model
## 13. Combat-Art Damage & Components
## 14. Rotation Model
## 15. Constants Block
## 16. Open Items & Known Divergences
```

- [ ] **Step 2: Verify the file exists and headers are present**

Run: `grep -c '^## ' docs/SPEC.md`
Expected: `16`

- [ ] **Step 3: Commit**

```bash
git add docs/SPEC.md
git commit -m "Spec: scaffold whole-system SPEC.md (plan 2t)"
```

---

## Task 2: §1 Goal & Deliverables + §2 Architecture & Data Flow

**Files:**
- Modify: `docs/SPEC.md` (§1, §2)
- Audit: `cmd/*/main.go`, and the package layout under `internal/`

- [ ] **Step 1: Audit the entrypoints and the pipeline**

Read each `cmd/*/main.go` end-to-end and confirm the stage ownership. Confirm the pipeline order: Census pull (`cmd/itemdex` → `source.Load` → `extract` → `classify`) → CSV cache (`data/*.csv`) → DB build (`cmd/builddb` → `store`) → model/BiS (`cmd/bis`, `cmd/weights`).

- [ ] **Step 2: Write §1 Goal & Deliverables**

Must cover: produces a three-tier BiS report (PRE-RAID / RAID / BEST-OF-BEST), a queryable SQLite DB, and CSV gear catalogs. Non-goals: relative DPS ordering only (not absolute/parse-accurate); Assassin only (other classes are future, see §16). Keep to ~120 words.

- [ ] **Step 3: Write §2 Architecture & Data Flow**

Must contain (a) a text pipeline diagram of the six stages above, naming the owning package per stage, and (b) a package-responsibility table covering every `internal/` package: `census` (throttled API client + decode), `extract` (Varsoon-windowed pagination), `classify` (EoF/KoS discovery-window detection), `source` (CSV cache load/fresh-pull), `catalog` (CSV schema + slot↔category + armor-type), `store` (SQLite schema + queries), `spell` (CA pull/parse/components/manual), `model` (DPS + weights), `bis` (set build + rank + render), `charconfig` (TOML character/class config), `constants` (locked combat constants), `fit` (curve fitting + sync tests).

- [ ] **Step 4: Verify each named package exists and the responsibility is accurate**

Run: `ls internal/`
Expected: the 12 packages above are all present. Spot-check two responsibility claims against the package's main file.

- [ ] **Step 5: Commit**

```bash
git add docs/SPEC.md
git commit -m "Spec: §1 goal/deliverables, §2 architecture & data flow (plan 2t)"
```

---

## Task 3: §3 Data Acquisition (Census)

**Files:**
- Modify: `docs/SPEC.md` (§3)
- Audit: `internal/census/client.go`, `internal/census/item.go`, `internal/extract/extract.go`, `internal/classify/eof.go`

- [ ] **Step 1: Audit**

Confirm from code: `census.Client` rate limit (~1 req/6s, 60s timeout, 30s backoff, retry once on 429/timeout — `client.go`); the public `s:example` ~10 req/min quota; `census.FlexString` (number-or-string JSON); `extract.collect` server-side prune (`world_list.id=614`, timestamp window, `itemlevel<72` via `maxItemLevel`), client-side `classify.IsEoF` refinement, `PartialError`/`ErrQuotaExceeded` resume-on-quota; `classify` constants `VarsoonWorldID=614`, `EoFStart`/`EoFEnd`, `KoSStart`/`KoSEnd`, and `VarsoonDiscovery`.

- [ ] **Step 2: Write §3**

Cover: the throttle-aware client and quota-resume behavior; the Varsoon (world 614) discovery-window classification that defines "EoF-era item"; the EoF/KoS date windows (cite the actual constant values from `eof.go`); the `itemlevel<72` server prune. Note KoS extension exists only for the max-life list.

- [ ] **Step 3: Verify the date-window and world-id claims against code**

Run: `grep -nE 'VarsoonWorldID|EoFStart|EoFEnd|KoSStart|maxItemLevel' internal/classify/eof.go internal/extract/extract.go`
Expected: prose values match the grep output exactly.

- [ ] **Step 4: Commit**

```bash
git add docs/SPEC.md
git commit -m "Spec: §3 data acquisition (plan 2t)"
```

---

## Task 4: §4 Catalog & Persistence

**Files:**
- Modify: `docs/SPEC.md` (§4)
- Audit: `internal/catalog/csv.go`, `internal/catalog/category.go`, `internal/store/store.go`, `internal/model/stats.go`

- [ ] **Step 1: Audit**

Confirm: CSV is wide-format with fixed columns + a union of stat columns (`catalog/csv.go` `WriteCSV`/`ReadCSV`); category files = `weapons.csv`, `armor.csv`, `jewelry-charms.csv` (+ `maxlife.csv` cross-cut); slot↔category and armor-type mappings (`category.go` `CategoryForSlot`, `ArmorType`, `SkillTypeFromArmorType`); SQLite tables `items`, `item_stats`, `combat_arts`, `combat_art_components`, `scores` (`store.go` `Init`); and the TLE Census-key → `StatBlock` translations from `stats.go` `modifierToField`.

- [ ] **Step 2: Write §4**

Cover the CSV schema + categories; the SQLite schema (list each table and its purpose, including `combat_art_components` and `duration_secs` on `combat_arts`); and a translation table of every Census modifier key in `modifierToField` → `StatBlock` field (`attackspeed→Haste`, `doubleattackchance→MultiAttack`, `critchance→CritChance`, `basemodifier→Potency`, `dps→DPSMod`, `spelltimereusepct→Reuse`, `flurry→Flurry`, `all→AbilityMod`, `spelltimecastpct→CastSpeed`, `strength→MainStat`). For `strength→MainStat`: record it as the **real, general** game encoding of "+N to all primary attributes" (not scout-specific); note explicit single-attribute keys (`agility`/`wisdom`/`intelligence`) are excluded as data-suspicious per the code.

- [ ] **Step 3: Verify the translation table against code**

Run: `grep -nE '".*":' internal/model/stats.go | grep -i 'Haste\|MultiAttack\|Crit\|Potency\|DPSMod\|Reuse\|Flurry\|AbilityMod\|CastSpeed\|MainStat'`
Expected: every key→field pair in the spec table appears in `modifierToField`. Also confirm the 5 SQLite tables via `grep -nE 'CREATE TABLE' internal/store/store.go`.

- [ ] **Step 4: Commit**

```bash
git add docs/SPEC.md
git commit -m "Spec: §4 catalog & persistence + TLE translations (plan 2t)"
```

---

## Task 5: §5 Combat-Art Pipeline

**Files:**
- Modify: `docs/SPEC.md` (§5)
- Audit: `internal/spell/pull.go`, `internal/spell/parse.go`, `internal/spell/component.go`, `internal/spell/rotation.go`, `internal/spell/manual.go`, `internal/spell/spell.go`

- [ ] **Step 1: Audit**

Confirm: `AssassinCombatArts` query uses `assassinClassID = 40` (note in code this differs from the item class id), `tier_name=Expert`, level `<71`; `FilterCombatArts` keeps damaging arts with `level ≥ minDamageArtLevel (57)`, `beneficial==0`, ranged bow shots kept; `HighestRanks`/`BaseName` collapse multi-rank arts; `ParseDamage`/`ParseComponents` extract the component hierarchy from `effect_list` via indentation; `ManualArts` appends the recovered low-level-learned arts (Hilt Strike, Strike of Consistency) with hand-entered L70 bases.

- [ ] **Step 2: Write §5**

Cover the pull → filter → highest-rank → parse → manual-append flow. **Describe `assassinClassID = 40` and the Expert/level filters as the current hardcoded reality** (with a forward pointer to §16 class-agnosticism). Explain why census effect_list is the structural source (raw/pre-calculation) and how indentation encodes parent/child for components.

- [ ] **Step 3: Verify**

Run: `grep -nE 'assassinClassID|minDamageArtLevel|tier_name|Expert|beneficial' internal/spell/pull.go`
Expected: `assassinClassID = 40` and `minDamageArtLevel = 57` confirmed; prose matches.

- [ ] **Step 4: Commit**

```bash
git add docs/SPEC.md
git commit -m "Spec: §5 combat-art pipeline (plan 2t)"
```

---

## Task 6: §6 Configuration & Accessibility Tiers

**Files:**
- Modify: `docs/SPEC.md` (§6)
- Audit: `internal/charconfig/charconfig.go`, `characters/alex.toml`, `classes/assassin.toml`, `cmd/bis/main.go` (tier definitions)

- [ ] **Step 1: Audit**

Confirm: `charconfig.Config` = Character + base `Stats` + `ArtMods` map + `Contexts` map; strict validation (unknown keys error; `Class` must be `"assassin"`, `ArtTier` must be `"expert"`; stats non-negative); `ArtMod` = `RecastMult` + `PotencyAdd`; `ContextBlock(name)` = base + context buff package; `LoadClass` reads `classes/assassin.toml` → `auto_attack_multiplier`; `ApplyArtMods` folds AA recast/potency riders into arts. Confirm the three accessibility tiers and their gear filters in `cmd/bis/main.go` (PRE-RAID = LEGENDARY+TREASURED; RAID = all; BEST-OF-BEST = avatar+all).

- [ ] **Step 2: Write §6**

Cover the character TOML structure (stats, `[contexts.X]` buff packages, `[art_mods."Name"]`), the class TOML (`auto_attack_multiplier`), and the three tiers with their exclusion filters. **Describe the `assassin`/`expert`-only validation as current reality** (pointer to §16).

- [ ] **Step 3: Verify**

Run: `grep -nE 'assassin|expert|auto_attack_multiplier|RecastMult|PotencyAdd' internal/charconfig/charconfig.go classes/assassin.toml`
Expected: validation strings and class field confirmed.

- [ ] **Step 4: Commit**

```bash
git add docs/SPEC.md
git commit -m "Spec: §6 configuration & accessibility tiers (plan 2t)"
```

---

## Task 7: §7 BiS Engine

**Files:**
- Modify: `docs/SPEC.md` (§7)
- Audit: `internal/bis/set.go`, `internal/bis/build.go`, `internal/bis/candidates.go`, `internal/bis/exclusions.go`, `internal/bis/render.go`, `internal/model/weights.go`, `internal/model/score.go`, `internal/model/itemdelta.go`

- [ ] **Step 1: Audit**

Confirm: `Set` (Profile/Main/Arts/AutoMult/FightLen/Equipped) + `DPS()` via `TotalDPSDual` + `CandidateDelta`; `BuildSet` coordinate-ascent with `maxBuildPasses = 12`; `pickBest` greedy per-slot with capacities (Ear/Finger/Wrist/Charm = 2, others = 1), Primary fixed; `SlotCandidates` injects 1H weapons into Secondary; `exclusions` (`IsAvatar`, `IsHunters`, `Curated`); `DeriveWeights`/`WeightStats` (curve stats via bracket slope, linear via +1 diff); `ScoreItem`/`ScoreTerm` breakdown; `ItemDelta` full before/after `TotalDPSDual`.

- [ ] **Step 2: Write §7**

Cover the optimization loop (coordinate ascent to convergence or 12 passes), slot capacities, candidate construction (off-hand = all non-Soulfire 1H), exclusions, the derive-don't-declare weight method, and the scored-breakdown rendering. Note weights are *outputs*, no stat pre-judged.

- [ ] **Step 3: Verify**

Run: `grep -nE 'maxBuildPasses|capacityOf|Ear|Finger|Wrist|Charm' internal/bis/build.go`
Expected: `maxBuildPasses = 12` and the multi-capacity slots confirmed.

- [ ] **Step 4: Commit**

```bash
git add docs/SPEC.md
git commit -m "Spec: §7 BiS engine (plan 2t)"
```

---

## Task 8: §8 Commands & Operations

**Files:**
- Modify: `docs/SPEC.md` (§8)
- Audit: `cmd/itemdex/main.go`, `cmd/builddb/main.go`, `cmd/bis/main.go`, `cmd/weights/main.go`, `cmd/fitcurve/main.go`

- [ ] **Step 1: Audit**

Confirm each command's flags from its `flag.` calls: `itemdex` (`--out`, `--refresh`, `--sid`, `--page`); `builddb` (`--data`, `--db`, `--sid`); `bis` (`--db`, `--out`, `--character`, `--lock`, `--top`, `--fight`); `weights` (`--db`, `--character`, `--fight`); `fitcurve` (`--readings`).

- [ ] **Step 2: Write §8**

A command reference table (command → purpose → flags + defaults), then the two operational loops: (a) data refresh — `itemdex [--refresh]` → `builddb` → `bis`/`weights`; (b) curve re-fit — append to `data/curve-readings.csv` → `go run ./cmd/fitcurve` → paste constants into `curve.go` → sync test goes green.

- [ ] **Step 3: Verify**

Run: `grep -rnE 'flag\.(String|Bool|Int|Float64)' cmd/`
Expected: every flag named in the spec table appears; no extra flags omitted.

- [ ] **Step 4: Commit**

```bash
git add docs/SPEC.md
git commit -m "Spec: §8 commands & operations (plan 2t)"
```

---

## Task 9: §9 Testing & Calibration-Sync

**Files:**
- Modify: `docs/SPEC.md` (§9)
- Audit: `internal/fit/sync_test.go`, `internal/fit/mainstat_sync_test.go`, plus the test inventory across packages

- [ ] **Step 1: Audit**

Confirm: `TestFittedConstantsMatchReadings` pins `HasteDpsModA/B` to `FitQuad(curve-readings.csv)`; `TestMainStatSamplesMatchReadings` pins `mainStatSamples` to `mainstat-readings.csv`. Note the calibration tests in `internal/model` (e.g. `TestAutoWeaponMultiplierCalibration`). Confirm there is no file named with "Harness" in the repo's runnable unit tests.

- [ ] **Step 2: Write §9**

Cover: test strategy (table/calibration tests close to code); the two sync tests as the code↔data guardrail (editing a CSV without re-fitting fails the build); the **"never run any file with 'Harness' in the name"** rule (hits real DBs/APIs — only mocked unit tests are run).

- [ ] **Step 3: Verify**

Run: `go test ./... 2>&1 | tail -5`
Expected: all packages PASS (proves the spec work changed no code).

- [ ] **Step 4: Commit**

```bash
git add docs/SPEC.md
git commit -m "Spec: §9 testing & calibration-sync (plan 2t)"
```

---

## Task 10: §10 The DPS Model

**Files:**
- Modify: `docs/SPEC.md` (§10)
- Audit: `internal/model/dps.go` (`TotalDPS`, `TotalDPSDual`, `AutoDPS`, `CADPS`)

- [ ] **Step 1: Audit**

Confirm: `TotalDPS = classAutoMult·AutoDPS + CADPS` and `TotalDPSDual = AutoDPSDual + CADPS` — auto and CA run in **parallel** (CA casting does not displace auto swings). Confirm the auto multiplier stack: `(weaponAvg/effDelay)·(1+MA%/100)·autoDamageMult·critFactor·flurryFactor`.

- [ ] **Step 2: Write §10**

State the top-level model: parallel auto + CA timelines, the multiplier stack, and that everything below in Part II feeds these two terms. Cross-reference §11–§14 for the per-factor detail.

- [ ] **Step 3: Verify**

Run: `grep -nE 'func (TotalDPS|TotalDPSDual|AutoDPS|CADPS)' internal/model/dps.go`
Expected: signatures match the prose; parallel composition confirmed.

- [ ] **Step 4: Commit**

```bash
git add docs/SPEC.md
git commit -m "Spec: §10 the DPS model (plan 2t)"
```

---

## Task 11: §11 Stat-Conversion Mechanics (the authoritative block)

**Files:**
- Modify: `docs/SPEC.md` (§11)
- Audit: `internal/model/curve.go`, `internal/model/dps.go` (factors), `internal/model/rotation.go` (`effRecast`, `slotSecs`), `internal/constants/constants.go`, `data/curve-readings.csv`, `data/mainstat-readings.csv`

- [ ] **Step 1: Audit each stat against code**

Confirm exactly: `critFactor = 1 + (crit/100)·(CritMultiplier−1)`, `CritMultiplier=1.30` (`dps.go`/`constants.go`); `flurryFactor = 1 + (flurry/100)·(FlurryMultiplier−1)`, `FlurryMultiplier=4.0`, gear-flurry only; haste/dps-mod shared fitted curve `f(s)=A·s−B·s²`, `A=0.800348`, `B=0.00127275`, floored, capped at 300 (`curve.go`); multi-attack piecewise table to 3400→200% (`multiAttackSamples`); main-stat AGI three-regime piecewise-**unfloored** table clamping at 1661 (`mainStatSamples` + `mainstat-readings.csv`); potency pool `(1+(Potency+PotencyBonus+PotencyAdd)/100)` (`rotation.go`); ability-mod per-mechanic (full DirectHit / ½ per trigger / zero DoT-Termination — detail lives in §13, summarize here); reuse divisor `max(0.5·base, base·(1−RecastReduction)/(1+Reuse/100))` (`effRecast`); cast speed divisor `baseCast/(1+castSpeed/100)`; recovery subtractive `0.5·(1−min(recovery,100)/100)` (`slotSecs`, `CARecoveryBaseSecs=0.5`).

- [ ] **Step 2: Write §11**

One subsection per stat, each: formula (from code) + constant + one-line provenance summarized from the code comment / CSV (measurement date, reading count, what was disproven). For AGI: describe the three regimes (climb to 65% @1100, deadzone 1100–1200, second climb to 1661, clamp above) and that tooltips are unfloored two-decimal. For the `strength` key, reference §4's translation note (do not restate the mapping mechanics). Add the `potencyBonus` provenance (Wuoshi TLE server damage adjustment, captured empirically, §16 open question on in-pool-vs-final).

- [ ] **Step 3: Verify every constant against code**

Run: `grep -nE 'CritMultiplier|FlurryMultiplier|HasteDpsModA|HasteDpsModB|HasteStatCap|RecastReductionCeiling|CARecoveryBaseSecs' internal/constants/constants.go internal/model/curve.go`
Expected: every numeric value in §11 matches. Also confirm the mainstat table head/tail values against `data/mainstat-readings.csv` (first row `73,6.08`; last `1661,79.54`).

- [ ] **Step 4: Commit**

```bash
git add docs/SPEC.md
git commit -m "Spec: §11 stat-conversion mechanics (plan 2t)"
```

---

## Task 12: §12 Auto-Attack Model

**Files:**
- Modify: `docs/SPEC.md` (§12)
- Audit: `internal/model/dps.go` (`AutoDPS`, `AutoDPSDual`, `autoDamageMult`, `AutoWeaponMultiplier`, `effDelay`), `internal/constants/constants.go` (`DualWieldDelayPenalty`)

- [ ] **Step 1: Audit**

Confirm: per-swing = `census_avg·(1+MainStatCurve/100)·dpsModFactor·classAutoMult`; AGI scales auto at the **same** curve as CAs; potency does **not** scale auto; `classAutoMult` (Assassin ×2.0) applied at the `AutoDPSDual`/`TotalDPS` boundary, NOT a `StatBlock` field; dual-wield `DualWieldDelayPenalty=1.33` applied to **both** weapon delays only when `off.DelaySecs>0`.

- [ ] **Step 2: Write §12**

Cover the weapon-damage equation, the `/weaponstat` decomposition that calibrated it (census-raw vs dps-mod-applied vs actual; residual ×2.0), why `classAutoMult` is threaded not stored, and the off-hand ×1.33 detection-not-assumption.

- [ ] **Step 3: Verify**

Run: `grep -nE 'DualWieldDelayPenalty|classAutoMult|autoDamageMult' internal/model/dps.go internal/constants/constants.go`
Expected: `DualWieldDelayPenalty=1.33`; classAutoMult applied in `AutoDPSDual`/`TotalDPS` only.

- [ ] **Step 4: Commit**

```bash
git add docs/SPEC.md
git commit -m "Spec: §12 auto-attack model (plan 2t)"
```

---

## Task 13: §13 Combat-Art Damage & Components

**Files:**
- Modify: `docs/SPEC.md` (§13)
- Audit: `internal/model/rotation.go` (`CAEffectiveDamage`, `dotTicks`), `internal/spell/component.go`, `internal/spell/parse.go`

- [ ] **Step 1: Audit**

Confirm `CAEffectiveDamage`: legacy single-line path (no components) = `(avgBase·scaling + abilityMod)·critFactor`; component path = Σ per component with per-mechanic abmod — DirectHit `base+abmod`; DoT `base·dotTicks(window)` (no abmod); Termination `base` only if held; TriggerProc `(base+0.5·abmod)·Triggers`; RateProc not scored. Confirm the `ComponentKind` enum and `Component` fields in `component.go`, and `dotTicks = floor(window/interval) + (hasInstant?1:0)`.

- [ ] **Step 2: Write §13**

Cover: the per-component sum equation; the taxonomy (DirectHit/DoT/Termination/TriggerProc/RateProc) with how each is parsed (indentation/duration); the per-mechanic ability-mod placement rule with its provenance (Gushing Wound full abmod on DirectHit; Death Mark ½×5≈2.4× across abmod 0/376/694; beneficial excluded); the base-source decision (highest-rank census bases for the sim; naked-recovered bases as calibration ground truth; the ~7% piercing/detonate reconcile note). State RateProc is parsed-but-deferred.

- [ ] **Step 3: Verify**

Run: `grep -nE 'DirectHit|DoT|Termination|TriggerProc|RateProc|0.5\*sb.AbilityMod' internal/model/rotation.go internal/spell/component.go`
Expected: the five kinds and the `0.5·abmod` per-trigger term confirmed.

- [ ] **Step 4: Commit**

```bash
git add docs/SPEC.md
git commit -m "Spec: §13 combat-art damage & components (plan 2t)"
```

---

## Task 14: §14 Rotation Model

**Files:**
- Modify: `docs/SPEC.md` (§14)
- Audit: `internal/model/rotation.go` (`rotationTimeline`, `CADPS`, `artCadence`, `hasTermination`, `maxEffRecast`, `fightSmoothingSamples`)

- [ ] **Step 1: Audit**

Confirm: priority sim fires the off-cooldown art with the highest `CAEffectiveDamage/slotSecs` each slot, idle-jumps when none ready, prefix-consistent; clip-vs-hold = `hasTermination` (termination → `artCadence = max(effRecast, duration)` held to full duration with detonate; else clip on `effRecast`); fight-length smoothing = mean of `cumCA(t)/t` over `fightSmoothingSamples=9` across `[fightLen−R/2, fightLen+R/2]`, `R = maxEffRecast`; `FightDurationSecs=600` default.

- [ ] **Step 2: Write §14**

Cover the priority sim (damage-per-time, not raw damage), clip-default/hold-on-termination rule with the conditional detonate, the fight-length smoothing rationale (single fixed length quantizes the last long-cooldown cast) and method, and the structural-idle + stealth-assumed-free notes. Forward-reference §16 for the residual non-monotonicity.

- [ ] **Step 3: Verify**

Run: `grep -nE 'fightSmoothingSamples|hasTermination|artCadence|maxEffRecast' internal/model/rotation.go internal/constants/constants.go`
Expected: `fightSmoothingSamples=9`, `FightDurationSecs=600` confirmed.

- [ ] **Step 4: Commit**

```bash
git add docs/SPEC.md
git commit -m "Spec: §14 rotation model (plan 2t)"
```

---

## Task 15: §15 Constants Block

**Files:**
- Modify: `docs/SPEC.md` (§15)
- Audit: `internal/constants/constants.go`

- [ ] **Step 1: Audit**

Read `constants.go` fully. Enumerate every exported constant with its value and inline-comment provenance.

- [ ] **Step 2: Write §15**

A table mirroring `internal/constants`: `CritMultiplier=1.30`, `FlurryMultiplier=4.0`, `HasteStatCap=300`, `DPSModCap=300`, `RecastReductionCeiling=0.50`, `CARecoveryBaseSecs=0.5`, `DualWieldDelayPenalty=1.33`, `FightDurationSecs=600`, `CACastTimeSecs=0.5` — each with a one-line provenance summary. Note this section is the spec mirror of the code constants; the code is authoritative.

- [ ] **Step 3: Verify the table is complete and exact**

Run: `grep -nE '=' internal/constants/constants.go | grep -vE '^\s*//'`
Expected: every constant in the grep appears in the §15 table with the same value; none missing.

- [ ] **Step 4: Commit**

```bash
git add docs/SPEC.md
git commit -m "Spec: §15 constants block (plan 2t)"
```

---

## Task 16: §16 Open Items & Known Divergences

**Files:**
- Modify: `docs/SPEC.md` (§16)
- Reference: `docs/spec-sim-audit-2026-06-18.md` (its Tier 1 & Tier 5 items), `docs/backlog.md` (§10 class-agnostic)

- [ ] **Step 1: Compile the still-live items**

From the audit doc, carry forward only the surviving items (Tier 2–4 are auto-resolved by deriving from code):
- **Tier 1 — crit model:** code uses flat ×1.30 (`constants.CritMultiplier`); the 2026-06-16 raid log measured ~1.50–1.55 via a range-shift (crit re-rolls in a band shifted up by ≈ the range width; not a gear stat). Documented as a known code bug; fix deferred. Cite `docs/plans/raid-log-analysis-2026-06-16.md` §3.
- **Tier 5 — coverage gaps:** Bladed Opening (~1.8% raid dmg) & Point Blank Shot (~0.3%) missing because the pool pulls live census but the server is TLE/EoF (recover EoF L70 bases manually, §9-path); ~7% unmodeled procs/poisons (flat, non-critting class); `potencyBonus` in-pool-vs-final-multiplier question.
- **Residual rotation non-monotonicity:** discrete greedy sim leaves slight non-monotone CADPS in reuse/cast-speed (strict-dominance inversions ~1.5–29 DPS); needs a modeling decision; picks via full resim still valid.
- **Class-agnosticism:** move `assassinClassID=40` and the `assassin`/`expert`-only `charconfig` validation into config; partially done (`classAutoMult` already in class TOML). Cross-reference `backlog.md §10`. Note `strength→MainStat` is a general game encoding, **not** a coupling.
- **Data wishlist:** an AGI reading >1661/1800 to anchor the second regime above the clamp; haste/dps-mod gap readings (153–238, 281–300); cast-speed cap above ~37%.

- [ ] **Step 2: Write §16**

Group the above under clear headings (Known code divergences / Coverage gaps / Modeling decisions / Future: class-agnosticism / Data wishlist). Each item: one line of what + one line of status/next step.

- [ ] **Step 3: Verify the crit constant claim**

Run: `grep -n 'CritMultiplier' internal/constants/constants.go`
Expected: confirms code still uses `1.30` (the divergence is real and current).

- [ ] **Step 4: Commit**

```bash
git add docs/SPEC.md
git commit -m "Spec: §16 open items & known divergences (plan 2t)"
```

---

## Task 17: Retire old docs into the timeline

**Files:**
- Move: `docs/design.md` → `docs/plans/design.md`
- Move: `docs/design-plan2.md` → `docs/plans/design-plan2.md`
- Move: `docs/spec-sim-audit-2026-06-18.md` → `docs/plans/spec-sim-audit-2026-06-18.md`

- [ ] **Step 1: Move the three files with git**

```bash
git mv docs/design.md docs/plans/design.md
git mv docs/design-plan2.md docs/plans/design-plan2.md
git mv docs/spec-sim-audit-2026-06-18.md docs/plans/spec-sim-audit-2026-06-18.md
```

- [ ] **Step 2: Add a superseded header to each moved design doc**

Prepend this line as the new first line of `docs/plans/design.md` and `docs/plans/design-plan2.md` (use Edit, matching each file's existing first heading below it):

```markdown
> **HISTORICAL — superseded by `docs/SPEC.md` (the source of truth). Kept as timeline history; do not treat as current.**
```

And prepend to `docs/plans/spec-sim-audit-2026-06-18.md`:

```markdown
> **HISTORICAL — the spec↔sim drift audit that motivated the `docs/SPEC.md` rebuild (plan 2t). Live items carried into `docs/SPEC.md` §16.**
```

- [ ] **Step 3: Verify nothing else references the old top-level paths**

Run: `grep -rnE 'docs/design\.md|docs/design-plan2\.md|docs/spec-sim-audit' --include=*.md --include=*.go . | grep -v 'docs/plans/'`
Expected: no stray references to the old `docs/` paths (matches inside the moved files that point to siblings are acceptable; flag any in code or `MEMORY.md`).

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "Docs: retire design.md, design-plan2.md, audit into plans/ as historical (plan 2t)"
```

---

## Task 18: Whole-doc consistency review

**Files:**
- Modify: `docs/SPEC.md` (fixes only)

- [ ] **Step 1: Placeholder scan**

Run: `grep -nE 'TBD|TODO|FIXME|XXX|\bfill in\b|\.\.\.\s*$' docs/SPEC.md`
Expected: no matches. Fix any inline.

- [ ] **Step 2: Cross-reference integrity**

Manually confirm every "see §N" pointer resolves and that no two sections state the same formula/constant independently (mechanics stated once in Part II; Part I references, never restates). Fix duplicates by replacing with a pointer.

- [ ] **Step 3: Confirm no code changed**

Run: `go test ./... 2>&1 | tail -5 && git status --porcelain`
Expected: all tests PASS; `git status` shows only `docs/` changes across the whole plan.

- [ ] **Step 4: Final read-through against the design doc**

Open `docs/plans/2026-06-18-spec-rebuild-design.md` and confirm every promised section and file operation landed. Fix gaps inline.

- [ ] **Step 5: Commit**

```bash
git add docs/SPEC.md
git commit -m "Spec: consistency review + placeholder/duplication sweep (plan 2t)"
```

---

## Self-Review

**Spec coverage** (design doc → tasks):
- Part I §1–§9 → Tasks 2–9. ✓
- Part II §10–§16 → Tasks 10–16. ✓
- File operations (move 3 docs + headers) → Task 17. ✓
- Audit-TODO carry-forward → Task 16 (live items into §16) + Task 17 (original preserved). ✓
- "No code changes" guarantee → Tasks 9 & 18 (`go test ./...` green; `git status` docs-only). ✓
- Class-agnostic correction + `strength`-is-not-a-coupling → Tasks 4, 5, 6, 16. ✓

**Placeholder scan:** No "TBD/implement later" — each task lists exact files, exact grep verifications, exact commit messages. Section *prose* is intentionally produced at execution (it requires reading the code), but each task pins precisely which symbols to audit and what facts must appear, plus a grep that fails if a stated constant is wrong. This is the honest doc-task analog of TDD.

**Type/name consistency:** Code symbols cited (`effRecast`, `CAEffectiveDamage`, `assassinClassID`, `DualWieldDelayPenalty`, `fightSmoothingSamples`, `modifierToField`, `mainStatSamples`) match the names confirmed in the codebase map and direct reads. Section numbers are consistent between the skeleton (Task 1) and the per-section tasks.
