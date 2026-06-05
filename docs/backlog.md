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
