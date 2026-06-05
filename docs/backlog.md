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
