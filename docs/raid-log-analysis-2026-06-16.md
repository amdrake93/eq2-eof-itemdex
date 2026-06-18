# Raid Log Analysis — Mistmoore's Inner Sanctum (2026-06-16)

Source: `data/eq2log_Biffels.7z` → `eq2log_Biffels.txt` (717,983 lines). Raid window =
the Mistmoore's Inner Sanctum block, **lines 1599–683738** (entered 19:45, left to
Commonlands 22:22). All damage figures are **Biffels' outgoing damage** in that window.

**Caveat:** this is full-buff raid combat (≈88 reuse, ≈1800 AGI, group buffs) which the
model's RAID context does NOT yet carry, so **absolute numbers are not comparable** —
this analysis is about **structure, shares, ratios, coverage, and validating the
mechanics** we calibrated. No sim changes were made.

Total outgoing damage in the window: **40,431,107**.

---

## 1. Coverage — the model "sees" 88% of your raid damage

| | share |
|---|---|
| **Modeled** (CAs in the pool + Death Mark triggers + Gushing Wound detonate + auto) | **88.0%** |
| **Unmodeled** | **12.0%** |

The 12% gap, itemized:

| source | share | category |
|---|---|---|
| Caustic Poison | 3.1% | poison proc (0% crit) — out of scope |
| Vampiric Requiem | 2.8% | proc (0% crit) — out of scope |
| Swipe | 2.6% | Whirling Blades RateProc — **deferred** (we flagged this) |
| **Bladed Opening** | **1.8%** | **a CA missing from the pool** (see §4) |
| Greater Rune of Blasting | 0.5% | adornment proc (0% crit) |
| Incinerate Blood | 0.5% | proc (0% crit) |
| Shock | 0.3% | proc (0% crit) |
| **Point Blank Shot** | **0.3%** | **a CA missing from the pool** (see §4) |
| Dissonant / Precise Note | 0.2% | bard-ish procs |
| Trickster's Grasp / Linked Agony | ~0% | one-off procs |

So of the 12%: **~7.4% is procs/poisons/adornments** (a damage class the sim doesn't
model — notably **none of them crit**), **2.6% is the deferred Swipe RateProc**, and
**~2.1% is two combat arts the pool is missing** (Bladed Opening, Point Blank Shot).

---

## 2. What VALIDATED — the calibrated mechanics hold up

**Death Mark = 5 triggers (✓ and it's huge).** Logged as "Agonizing Pain":
**3,993,066 dmg (9.9%, the #2 source), 999 hits.** `999 / 5 = 199.8` → ~200 Death Mark
casts × exactly 5 triggers. The near-perfect divisibility by 5 confirms the 5-trigger
model, and its #2 ranking confirms that calibrating its ½-abmod-per-trigger (×5 ≈ 2.4×
abmod) was worth doing — it's a top-tier source, not a footnote.

**Gushing Wound = front-load + DoT + detonate (✓ structure) and 7 ticks (✓).** The log
splits it exactly as the component model predicts:
- front-load melee → **slashing**, 484,985, 222 hits (= ~222 casts)
- DoT (instant+periodic) → **piercing**, 973,360, 1143 hits
- detonate → logged as its own spell **"Untreated Bleeding"**, 209,580, 64 hits

Ticks per **terminated** application (ran the full ~24s): **min 7**, median 8 (the 8 is
my 25s analysis window catching ~1 tick from the adjacent held cast). So the model's
`instant + 6 = 7` is correct for full-duration applications. The *aggregate* is only
5.15 ticks/cast and detonate fires on just **29%** of casts — because **71% of casts hit
trash that died before 24s**. Takeaway: the model's full-lump Gushing Wound is right for
the **single-target boss** case; it **overvalues the DoT/detonate in trash-heavy
combat** (the detonate especially — modeled as always-on, real-world 29%).

**Manual scaling arts (§9) are real contributors (✓).** Hilt Strike 622,569 (1.5%, 205
hits) and Strike of Consistency 409,620 (1.0%, 199 hits) — both firing and pulling their
weight, validating the §9 manual-arts addition.

**Auto-attack = 18.8%** (7,593,859; 3,437 hits; 67% crit; avg ~2,210). A clean reference
for the auto-vs-CA balance to check once the raid context is set.

---

## 3. What's OFF — the crit model (biggest issue)

**Real crit multiplier ≈ 1.50–1.55; the model assumes 1.30.** Measured as
avg(crit)/avg(non-crit) per ability (model = `1 + critChance×0.30`, i.e. a flat +30%
crit bonus, with **no Crit Bonus stat tracked**):

| ability | crit n | mult | | ability | crit n | mult |
|---|---|---|---|---|---|---|
| Auto-attack | 2319 | **1.64** | | Quick Strike | 797 | 1.59 |
| Impale | 1032 | 1.55 | | Stealth Assault | 994 | 1.51 |
| Gushing Wound | 882 | 1.55 | | Agonizing Pain | 670 | 1.50 |
| Mortal Blade | 27 | 1.51 | | Eviscerate | 46 | 1.47 |

**The mechanic (community knowledge, log-consistent):** a crit doesn't multiply by a
constant — it **re-rolls in a range shifted up by the range width**: base `[min,max]` →
crit `[max+1, 2·max−min+1]`. So a crit **adds ≈ the range width** to the rolled damage.
That makes the effective multiplier `1 + width/avg`, which **varies by ability** (wide
range → big crit, narrow → small) — exactly the 1.43–1.64 spread above, not a constant.
For a typical CA range (~1.67:1 max:min) `width/avg ≈ 0.5` → ×1.5; auto (two weapons,
wide effective range) → 1.64.

Consequences:
- The model's **flat `1 + critChance×0.30` is wrong** — it under-scales crit by
  ~17–25% on the crit portion (~10–12% on total in raid).
- **No new gear stat needed** (crit bonus is NOT itemized here — correction). The crit
  bonus is **intrinsic to each ability's damage range**, which the parser already
  stores as `MinDamage/MaxDamage` per component. The fix is to model crit as an
  **additive range-width shift per component** instead of a flat ×1.30.
- Log noise (raid buffs scale each hit, widening observed ranges) prevents pinning the
  exact `+1` / whether abmod participates in the shift — a **clean tooltip read** of one
  ability's non-crit vs crit range (no buff noise) would nail it, same as our other
  calibrations.

This is the clearest model-vs-reality miss — affects absolute DPS, and slightly affects
ranking (wide-range abilities gain more from crit than the flat model credits).

---

## 4. The data-source problem — live census vs the TLE server

Bladed Opening (1.8%) and Point Blank Shot (0.3%) are **missing from the pool**, and the
reason is structural: the model builds its art pool from the **live** EQ2 census
(`census.daybreakgames.com`), but Biffels plays on the **Wuoshi TLE (EoF-era) server**.
In the live census both of these are **level 100–110** abilities (Bladed Opening even
lists `assassin=False`) — i.e. **post-EoF redesigns** — so the model's
`classes.assassin & level<71` pull legitimately skips them, even though they're real
arts in the EoF-era kit Biffels is using.

Implication: the live census is an **imperfect source** for the TLE kit. For abilities
unchanged since pre-EoF (Gushing Wound, Assassinate, …) it matches; for abilities
added/redesigned after EoF it diverges. Bladed Opening + Point Blank Shot need their
EoF L70 bases **recovered manually** (same path as the §9 arts), and it's worth a broader
audit of whether any *pooled* arts also carry wrong live-census numbers.

---

## 5. Recommendations (for review — no sim changes made)

1. **Crit model** — replace the flat `1 + critChance×0.30` with the **range-shift**
   mechanic: a crit adds ≈ the component's damage-range width (crit re-rolls in
   `[max+1, 2·max−min+1]`). No new gear stat — uses the `min/max` the parser already has.
   Highest-impact fix (~10–12% absolute in raid). *(Confirm the exact form — the `+1`,
   and whether ability-mod participates in the shift — with one clean tooltip read of an
   ability's non-crit vs crit range.)*
2. **Missing CAs** — recover EoF L70 bases for Bladed Opening + Point Blank Shot
   (manual-arts path); audit the pool against the log for any other live-vs-TLE drift.
3. **Swipe (2.6%)** — the deferred RateProc is non-trivial; worth scoring.
4. **Procs/poisons (~7%)** — a separate flat (non-critting) damage class; decide whether
   a proc/poison layer is worth it for absolute accuracy (ranking-light since most are
   not gear-driven).
5. **Boss-only validation** — when the raid buff package lands, validate against a single
   sustained boss fight, not this trash-mixed aggregate (where DoT/detonate are cut short
   by trash mortality).

## Appendix — full per-ability breakdown (raid window)

Top sources: Auto 18.8% · Agonizing Pain 9.9% · Stealth Assault 6.5% · Quick Strike 5.2%
· Assassinate 5.2% (only 26 casts, max 98,785) · Impale 5.1% · Gushing Wound 3.6% ·
Masked Strike 3.6% · Ambush 3.4% · Mortal Blade 3.1% (48 casts) · Caustic Poison 3.1% ·
Vampiric Requiem 2.8% · Swipe 2.6% · Improvised Weapon 2.5% · Eviscerate 2.5% · Deadly
Shot 2.4% · Paralyzing Strike 2.3% · Head Shot 2.2% · Crippling Strike 2.2% · Spine Shot
2.1% · Death Blow 2.0% · Jugular Slice 1.8% · Bladed Opening 1.8% · Hilt Strike 1.5% ·
Strike of Consistency 1.0% · (others <1%).
