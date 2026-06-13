# Backlog / Future Ideas

Deferred work, not yet planned. Captured so it isn't lost. Each should become a
real spec/plan when picked up.

## 1. Seed the starting set from a real character (census pull)

Pull the user's character from the EQ2 census, read currently-equipped gear, and
use that as the starting baseline `StatBlock` + equipped set instead of the
synthetic baseline.

Then the existing machinery answers two questions directly:
- `ConvergedWeights` → "what stat is good with my *current* setup"
- `CandidateDelta` / `BuildSet` → "biggest single upgrade vs my *actual* gear"
  (the highest-ΔDPS swap against the real equipped item, per slot)

Plumbs into the seam we already have: the model always starts from a baseline +
a set of equipped items and derives everything from that state. This only changes
*where* the initial state comes from — no model changes, just a new front end
that builds the initial `Set`.

## 2. Lore-aware multi-slot doubling (remove the forced-distinct assumption)

**This is a correctness fix, not a feature.** Today `pickBest` (build.go) forces
DISTINCT items per multi-slot (Ear/Finger/Wrist/Charm) via the `used` map. That
silently asserts "never run two of the same item" — a *declared* assumption, the
opposite of this project's derive-don't-declare principle. The numbers should
decide whether doubling an item wins; the builder must not pre-forbid it.

Correct behavior: a multi-slot may repeat an item when the math favors it,
blocked **only** for lore-equip items (equip ≤1). The greedy in `pickBest`
already evaluates each addition in-context (`[X,X]` vs `[X,Y]`), so caps and
within-slot interactions stay exact once the distinct constraint is lifted.

Needs the lore flag, which we don't pull yet — that's the only blocker:
- `extract.showFields` += `flags`; add `Flags{ LoreEquip }` to `census.Item`.
- `store`: a `lore_equip` column; `ScorableItem.LoreEquip`; load/write it.
- `build.go` `pickBest`: only mark an item `used` when it's lore-equip; non-lore
  items become eligible to repeat.
- Rebuild the DB.

Scope note: only **lore-equip** matters for our items (equip ≤1, may carry more).
Plain LORE and lore-group / equipment-set lore are not needed per current
understanding. The user sanity-checks any doubled pick the model produces.

## 3. Manual supplement for scaling low-level arts

Some damaging arts are **Apprentice-tier, very low level, and scale with caster
level** — so census files them at their base level with base-level damage, and
our Expert-tier + level-57 pull misses them entirely. Known ones:

```
Hilt Strike            Apprentice  L6   recast 20.0  cast 0.5  base 17-21    dmg@70 = ?
Strike of Consistency  Apprentice  L0   recast 12.0  cast 0.5  base 2        dmg@70 = ?
```

Recast/cast/beneficial come straight from census and are correct (they don't
scale); only the **level-70 damage** must be hand-entered (in-game examine at 70,
since census only has the base value). Implement as a small `manualArts` list
appended after `FilterCombatArts`. Blocked on the damage numbers.

Impact: fills rotation idle and nudges absolute CADPS. Does **not** change the
stat-weight ordering — verified that adding 3 ranged arts (a bigger change) left
the ranking identical and moved magnitudes <10%.

## 4. Stealth-grant rotation modeling

Mechanic (confirmed): **stealth breaks on any CA cast** — so every stealth-required
art needs its own fresh grant immediately before it (one grant, one attack). Auto-
attack does not break stealth. The 36s "Shroud" does not survive a CA cast, so it's
irrelevant to sustained DPS.

Model: stealth arts pair as `[grant -> attack]` (two slots), spending a granter
from a shared cooldown pool. Granters: **Masked Strike V** (10s, deals 585 —
preferred, already in pool), **Stalk V** (30s, 0 dmg — would add as a non-damaging
granter). **Smoke Bomb** (180s) is likely post-EoF (census only has L100/110) and
negligible as a grant engine — leave out unless confirmed EoF-legal on Varsoon.

Effect: granter cooldowns cap stealth attacks at ~80/fight, spent on the highest-
value stealth arts (Assassinate, Mortal Blade, Eviscerate, Jugular, Massacre,
Stealth Assault), which **collapses Ambush from ~61 to ~15** — fixing the current
rotation's impossible Ambush spam.

Requires: pull `RequiresStealth` ("must be sneaking / in stealth") and
`GrantsStealth` ("grants stealth to caster") flags from the effect_list; add Stalk;
add stealth state + granter scheduling to the rotation sim. Tests + DB rebuild.

Realism/cast-log accuracy only — verified it does NOT change the stat-weight
ordering (roughly damage-neutral reshuffle). Pick up only if we want a believable
rotation *playthrough*, not for stat weights.

### The real optimization target: boundary-cast retention

Grant-delays accumulate. A delayed cast starts its next cooldown later, shifting
every subsequent cast. So the binding effect isn't the per-cast 10s — it's whether
the accumulated drift pushes the *last* cast of a long-CD ability past the fight
window, costing a whole cast. Worst on **Mortal Blade** (7 casts @ 88s — the last
lands near ~590s with little slack); Assassinate (5 casts @ 147s) is more boundary-
luck. This is the same discrete cliff as reuse, on the granter timeline.

Consequence: optimal grant scheduling wants **look-ahead** — reserve a granter for
an incoming big stealth hit instead of burning it on a small one. BUT the payoff is
small here: Masked Strike (10s) cycles ~10x faster than the hits worth reserving for
(88-147s), so greedy delays a big hit by at most ~10s and look-ahead recovers <~1
cast. Not worth building for stat weights. (And the boundary loss is partly an
artifact of fixing the fight at exactly 600s — over variable lengths it averages to
a fractional expected cast.)

This is the **third** reason reuse is undervalued by the table: reuse shrinks the
granter recasts -> less drift -> more likely to fit that last boundary cast.

### Concealment — a periodic burst granter (not a stance)

Concealment (L55 assassin, **EoF-legal** — real Master/Grandmaster tiers, unlike
Smoke Bomb's modern-only entries): beneficial buff, instant cast, **60s recast,
7.0s duration**, also drops hate (good for DPS). While active, every combat hit
re-grants stealth ("On a combat hit -> Combat Stealth -> Shroud 36s -> Grants
stealth"). So for ~7s every 60s (~12% uptime) you chain stealth arts granter-free;
align the window with several stealth arts being off cooldown to dump them.

It **partially** relieves granter scarcity (~12% of the fight), it does NOT remove
it — the bottleneck and the reuse channel still apply the other ~88%. Model it as a
periodic 7s/60s window where stealth-required arts don't consume a granter. (Earlier
mistaken read: it is NOT a maintained permanent-stealth stance.)

## 5. Launch-day checklist (server not out yet)

Both of these wait on the new TLE server going live:

- **Re-export the gear cache.** Gear comes from the local CSV cache
  (`source.LoadCache` in builddb), NOT a live census pull — only combat arts hit
  census live. So gear is only as fresh as the last cache export. On launch (and
  as items get discovered on the new server), re-export the cache to pick up new
  item versions. Census won't publish a version until it's been discovered in-game,
  so early-server data will be sparse and fill in over time.
- **Character-pull seeding** (item #1) — needs a character to exist first.

## 6. Mid-content item availability (temporal phasing)

Not all EoF items are obtainable at the expansion's start. On the Varsoon TLE
server (as in original EoF), some items/zones unlock **partway through the
expansion** via later content patches — so the available item pool *grows* over
the EoF window; it is not static at launch.

The model currently treats every EoF-era item as available simultaneously, so
**early-expansion BiS lists will recommend gear that doesn't exist yet** (an item
that drops from a mid-content zone shows up in a launch-day BiS).

To solve: tag each item with a **content-availability phase** (e.g. launch /
mid-EoF / late-EoF) and let the report filter or segment by "what's obtainable at
point X in the timeline." This is a **different axis** from the existing
rarity/source tiers (pre-raid / raid / best-of-best) — temporal availability, not
gear quality — and the two compose (e.g. "raid BiS available at launch" vs "raid
BiS once mid-content unlocks").

Open questions for pick-up:
- Source of phase data: census discovery timestamps on the live TLE server? a
  curated zone -> phase mapping? the original EoF content-patch schedule?
- Granularity: just launch-vs-later, or finer (per content patch / per zone)?
- Gated on knowing the TLE server's unlock cadence (server not live yet — §5).
  Discovery timestamps only become meaningful once the server is up and items
  start being found, so this naturally co-develops with the launch-day re-pull.

## 7. cmd/weights main-hand pick diverges from the bis pipeline

`cmd/weights/main.go` selects the main-hand by max weapon damage among
`Soulfire%` rows, which lands on "Soulfire Gladius"; the bis pipeline (and spec
§4) pin the Soulfire **Sabre**. The weights printout is therefore computed on a
slightly different loadout than the report. Cosmetic for weight *ordering*, but
align the query (or share the loadout-loading code with cmd/bis) when next
touching cmd/weights. (Surfaced by the 2026-06 curve-refit final review.)

## 8. Primary-attribute CA damage scaling — RESOLVED 2026-06-12 except the ⚠ mystery (spec §3.1/§12)

Measured and spec'd: AGI multiplies CA damage via a 13-reading interpolated
curve (cap 1100 -> 65%); potency pool = displayed + per-art AA riders + a
calibrated hidden bonus; ability-mod cap disproven. REMAINING: the ~23.4-point
naked/AA-less hidden potency bonus (the ⚠ §12 mystery — hunt experiments listed
there) and the auto-attack question. Original discovery notes below.

### Original discovery notes (2026-06-12)

Tooltip calibration (Eviscerate V Expert, potency 57.7%, ability mod 738) shows
combat-art tooltips carry a base multiplier of ~2.99 where potency alone predicts
1.577 — an extra **~×1.9 on CA base damage** from primary attributes — **AGI specifically**
(user-confirmed; STR explicitly does NOT affect scout damage per its tooltip).
Census files items' "+N primary attributes" under the `strength` key (3,248
items), which grants AGI point-for-point to a scout; ~70 explicit single-stat
`agility`/`wisdom`/`intelligence` items exist and are data-suspicious (separate
fixing task). Overturns the spec's "attributes excluded" assumption (§11).

Impact when modeled: CADPS ~2× understated today → CA-stat weights (potency,
reuse, ability-mod) undervalued relative to auto stats; ability-mod cap binds
later on bigger bases; STR becomes rankable.

Data protocol (haste-curve playbook): confirm the driving attribute by gear
swap, then sweep (attribute value → Eviscerate tooltip max) over a wide range,
potency/ability-mod held or re-read per point; fit attribute → multiplier curve
(form/cap unknown). Then spec amendment → plan.

### Mystery-hunt research log

**2026-06-13 — weapon-skill scaling hypothesis (user lead): DISFAVORED, not eliminated.**
EQ2's documented skill→damage mechanic is OVER-cap range compression (skill above
the 5×level cap raises an art's *minimum* damage toward its max); weapon skill is
otherwise contested accuracy. This fails our mystery's signature twice: (1) our
mystery is a clean multiplier scaling min AND max identically (measured equal across
Quick Strike / Ambush / Eviscerate) — compression would change each art's min/max
ratio; (2) it's an over-cap effect, but a naked L70 is AT cap (350), not over it,
yet the 23.4% was present naked. Spell skills (Disruption/Subjugation/Ordination)
are resist-reduction in EQ2, not damage — the "skill scales spell damage" memory is
EQ1's Destruction-family mechanic, not carried into EQ2. Our own data also shows the
mystery pools ADDITIVELY with displayed potency, which looks like undisplayed flat
potency from an innate/class source, not a skill-damage effect.
CAVEAT: EoF-era deep-dives (eq2wire.com) were inaccessible (403); EoF TLE mechanics
may differ from the 2010+ docs read. One unverified snippet ("Piercing +15% base
CA/spell damage") distrusted (= the AA rider value, no primary source).
DISCRIMINATING TEST: weapon skill is on the char sheet, rises with use. If skill
scaled CA damage, a below-350 Piercing char shows lower tooltips and capping raises
them. Record naked Piercing/Slashing value — if 350 with the 23.4% present, at-cap
is the census baseline and skill can't be the extra source.

## 9. Manual scaling arts — recovered level-70 bases (unblocks §3)

Via the same calibration: **Hilt Strike base ≈ 262–315**, **Strike of
Consistency ≈ 199 flat** at level 70 (census files 17–21 / 2 at their base
levels). Attribute-independent (modifiers divided out), so valid as
census-equivalent bases under the current model. Implement per §3: manualArts
appended after the census pull; recast/cast from census are correct (20s/0.5s
and 12s/0.5s).

## 10. Class-intrinsic data system (expand for character import)

Started 2026-06-13: `classes/<class>.toml` (uniform strict schema, keyed by the
character config's `class` field) holds class-intrinsic measured constants. v1
ships only `auto_attack_multiplier` (Assassin 2.0; Enchanter ≈0.7 for reference).
As character-import (§1) nears, move the remaining class-intrinsic values in —
all "same field, different value per class," all strict-required (the sim is
incomplete without them):

- **`census_class_id`** (Assassin = 40) — currently hardcoded in `spell/pull.go`;
  the CA census pull can't fetch another class's arts without it. Moving it makes
  `builddb`/`spell.AssassinCombatArts` class-parameterized.
- **`primary_attribute`** (scout = agi; fighter = str, mage = int, priest = wis) —
  unused today (census files all "+N primary attributes" under the `strength` key,
  which routes to MainStat regardless), so deferred until the sim branches on it
  or another class needs a different mainstat curve.
- **NOT a class field:** `minDamageArtLevel` (57) is better understood as
  *derived* than intrinsic — abilities tend to re-rank roughly every ~13 levels,
  so `maxLevel − 13` (70 − 13 = 57) is a good **starting heuristic** keyed off the
  character's level cap. NOT a concrete rule (re-rank cadence isn't guaranteed
  uniform); fine as a default when bringing up new classes, refine if one proves
  otherwise.

### Candidate class-intrinsic "magical multipliers" — compare across classes

Two innate multipliers were measured for the Assassin and have no explanation
beyond "it's intrinsic." Both are prime suspects for being **class-specific** —
when we bring up a second class, measure its equivalents and compare; if they
differ by class, they belong in the class file:

- **Auto-attack innate ×2.0** — RESOLVED and housed: it's `auto_attack_multiplier`
  in `classes/assassin.toml` (this is the model for the others).
- **⚠ CA-side potency-pool innate (~23.4 points)** — the §12 "potency-pool
  mystery": ~23.4 hidden potency-pool points that survive naked/AA-less/buff-less
  (≈ +15% to CA damage at raid gear). Currently parked in the per-character config
  as `potency_bonus` (calibrated), NOT yet proven class-intrinsic — but it smells
  exactly like the auto ×2.0's CA-side twin. **Do not lose this.** When a second
  class is measured, check whether its naked CA potency-pool innate differs; if so,
  move it from `[stats]` into the class file. (User flagged 2026-06-13 as a likely
  "magical Assassin multiplier" worth tracking.)

Also watch: the mainstat (AGI) curve itself was measured for the Assassin — if a
different class's primary-stat→damage curve differs, it too becomes class data.

## 11. Residual rotation non-monotonicity (dominance inversions in scoring)

Fight-length smoothing (plan 2p) reduced the discrete-sim cast-boundary
quantization (~18% variance drop; fixed the Wrist "reuse-stick ranked #1 but not
picked" case) but did NOT eliminate it. The priority sim's CADPS is still
**non-monotone in reuse/cast-speed at fine resolution** — increasing reuse can
locally lower CADPS by ~one mid-art cast, because shifting cast availability
re-orders the greedy lattice and can push a cast just past a sample length.
Increasing the smoothing sample count K does NOT fix it (intrinsic to the
discrete sim, not a sampling-resolution issue).

Consequence: **strict-dominance inversions in the ΔDPS scores** — an item that is
≥ another on every weighted stat can score lower. Found 2026-06-13 at the default
fight=600: pre-raid Hands *Blood Drenched Wraps* (171.0) out-scores the
strictly-dominant *Carmine Gloves* (142.2) by ~29; raid Wrist *Butler's Cuffs*
(161.0) over *Unholy Manacle* (159.5) by 1.5. ~37 such inversions across
slot/baseline groups, magnitudes ~1.5–29 DPS (≈ one cast of a mid-size art).

These are display/ranking artifacts on near-tied items; the converged BiS picks
(via full resim in pickBest) are a local optimum regardless. But it violates the
dominance invariant and should be decided at spec level. Options to weigh:
- **Accept + document** (cheapest; inversions are small, only flip near-ties).
- **Smoother CADPS** — model expected *fractional* casts analytically instead of
  the discrete greedy sim (removes quantization at the source; bigger rewrite).
- **Re-add weight-side band-aids** — note these only smoothed the *displayed
  weights*, NOT the resim ΔDPS scores where the inversions live, so they would
  NOT fix this; the fix has to be in CADPS/the sim itself.
Route through brainstorm before implementing.
