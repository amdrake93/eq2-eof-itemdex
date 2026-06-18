> **HISTORICAL — the spec↔sim drift audit that motivated the `docs/SPEC.md` rebuild (plan 2t). Live items carried into `docs/SPEC.md` §16.**

# Spec ↔ Sim Audit + Fix Checklist (2026-06-18)

Audit of `docs/design-plan2.md` (spec) against the actual simulator (`internal/model/{dps,rotation,curve,stats,weights}.go`, `internal/constants`, `internal/spell`, `internal/store`). **Principle: trust neither source where they diverge — resolve with data, then fix.** Most divergences are the spec lagging data-backed code; one (crit) is both sources wrong vs the raid log.

Deep-audited: §3.1, §11, §12 + the model code. **Not yet audited:** §4 (config), §5 (architecture), §6 (schema — flagged below), §7 (outputs), §8 (set bonuses).

Line numbers are approximate (they shift as edits land) — locate by section + quoted text.

---

## 🔴 Tier 1 — both spec AND code wrong vs data (needs a read before fixing)

- [ ] **Crit model.** Spec (§3.1 "Linear/direct stats" + "Putting it together"; §11) and code (`constants.CritMultiplier = 1.30`, `dps.go critFactor = 1 + critChance·0.30`) **both** use a flat ×1.30. **The raid log (2026-06-16) says ~1.50–1.55, varying by ability** — the community **range-shift** mechanic: a crit re-rolls in `[base_max+1, 2·base_max−base_min+1]`, i.e. **adds ≈ the range width**, so the effective multiplier is `1 + width/avg` (wide range → bigger crit; auto-attack 1.64; typical CA ~1.5). Crit bonus is **not** a gear stat here, so no new stat is needed — the bonus is intrinsic to each component's `min/max` (already parsed).
  - **Resolve:** one clean tooltip read of an ability's non-crit vs crit damage range (no buff noise) to pin the exact form (the `+1`; whether ability-mod participates in the shift).
  - **Fix:** replace the flat `critFactor` with a per-component additive-range-width crit; update §3.1/§11. Highest-impact item (~10–12% absolute in raid; slightly affects ranking — wide-range abilities gain more from crit than the flat model credits).
  - Full evidence: `docs/raid-log-analysis-2026-06-16.md` §3.

---

## 🟠 Tier 2 — spec stale, code is the data-backed truth (doc-only fixes)

- [ ] **AGI curve.** §3.1 ("Combat-art damage equation" main-stat bullet) and §11 still say *"interpolated 13-reading table, hard cap 1100 → 65%, high range fits a quadratic peaking ~1142 (commentary)."* Code (`curve.go mainStatSamples`, `data/mainstat-readings.csv`) is now **18 readings with a measured second regime**: climb to 65% at 1100, **flat deadzone ~1100–1200**, then climbs again ~0.027–0.031 %/AGI to **1661** (clamps above; raid AGI ~1800 still wants a >1800 reading). Fix the spec prose in both places to the three-regime curve + new reading count.

- [ ] **Reuse.** §11 (*"reuse 1%/pt capping at 50 stat, sharing each art's 50% recast ceiling"*) and §12 (*"Reuse weight … full-strength per point"*) are stale. Code + §3.1 "Reuse" section + the "Putting it together" `effRecast` line are the recalibrated **divisor**: `effRecast = max(0.5·base, base·(1−artMod)/(1+reuse/100))` — floored at 50% of base, reached at 100% reuse, no 50-stat cap. Update §11 and the §12 reuse-weight bullet to match.

---

## 🟡 Tier 3 — spec contradicts itself (code follows the newer rule)

- [ ] **Ability-mod, §3.1 "Linear/direct stats" bullet** says *"flat add to each CA, in full."* This is the old blanket rule; the per-mechanic rule in §3.1 "Multi-component abilities" supersedes it (DirectHit full / TriggerProc ½ per trigger / DoT-tick + Termination zero / beneficial none). Code = per-mechanic. Edit the bullet to cross-reference the per-mechanic rule.

- [ ] **CAeffective formula, §3.1 "Putting it together"** still shows the single-component equation `CAeffective = ((min+max)/2 · (1+potPool) · (1+mainstat) + abilityMod) · crit`. Code (Increment B `CAEffectiveDamage`) is the **per-component sum** with per-mechanic abmod, DoT windowing, conditional detonate, trigger scoring. Update the summary line to the per-component form (or annotate it as the per-component *base* equation).

- [ ] **Clip-vs-hold, §12** ("Clip-viability for other classes") says the Assassin *"hold-vs-clip is fully settled — never clip."* This contradicts the revised rotation rule in §3.1 "Rotation behavior": **clip DoTs by default, hold only termination-DoTs to full duration.** Code = clip-default. Fix the §12 bullet.

---

## 🟢 Tier 4 — status staleness (shipped work marked in-progress)

- [ ] **Increment B is shipped/merged** (per-component sim, plan 2r) — §3.1 "Implementation lands in two increments" and §12 DoT bullet still say *"in progress (plan 2r)."* Mark shipped.
- [ ] **§9 manual arts shipped** (Hilt Strike, Strike of Consistency in the pool via `spell.ManualArts()`) — §12 "Art-list audit pending" still lists their L70 damage as pending. Update.
- [ ] **§12 main-stat overcap** ("want a deliberately overcapped AGI>1100 reading") — got it; now measured to 1661. Update to "want a >1800 reading to anchor above the clamp."

---

## 🔵 Tier 5 — loose ends from the raid log, absent from the spec (need decisions/data)

- [ ] **Missing CAs: Bladed Opening (1.8% of raid dmg), Point Blank Shot (0.3%).** In the log, absent from the pool because the model pulls from the **live** census but you're on the **TLE/EoF** server — both show as L100+/post-EoF redesigns in live census. Add a spec note + recover their EoF L70 bases manually (§9 path); audit whether other pooled arts carry wrong live-census data.
- [ ] **~7% procs/poisons unmodeled** (Caustic Poison, Vampiric Requiem, Swipe RateProc, Incinerate Blood, Greater Rune of Blasting, Shock…) — a flat, non-critting damage class the sim doesn't model. Add a scope note; decide whether a proc/poison layer is worth it.
- [ ] **potencyBonus: in-pool vs final-multiplier.** Both spec and code add it into the potency pool; you flagged it might be a final after-everything multiplier. Add to §12 as a to-confirm (distinguishable with reads across potency levels — the two forms differ by a cross-term).

---

## 🟣 §6 SQLite schema — verify (likely stale)

- [ ] §6 predates Increment A, which added the **`combat_art_components`** table and a **`duration_secs`** column on `combat_arts` (see `internal/store/store.go`). Confirm §6 reflects these; update if not.

---

## ✅ Verified consistent (spec ↔ code — no action)

flurry ×3 bonus (`FlurryMultiplier 4.0`); haste/multi-attack curves + 300 cap; `classAutoMult` ×2.0; dual-wield ×1.33; cast-speed divisor; recovery subtractive (0.5s base); slot formula; **abmod per-mechanic** (code matches §3.1 multi-component); **effRecast divisor** (just recalibrated, matches); `WeightStats` correctly excludes recoveryspeed/potencybonus; `critbonus` ignored in `modifierToField` (consistent with "not a gear stat"); potency-pool composition (potency + potencyBonus + artPotencyAdd).

---

## Suggested order

1. **Doc-only fixes (Tiers 2–4 + §6):** safe now — code is the data-backed side, spec just lagged. No calibration needed.
2. **Tier 1 (crit):** get the one tooltip read, then fix code + spec. Highest DPS impact.
3. **Tier 5:** decisions (proc layer? add missing CAs?) + manual base reads for Bladed Opening / Point Blank Shot.
4. Extend the audit to §4/§5/§7/§8 if desired.
