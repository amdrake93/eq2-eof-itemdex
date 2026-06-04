# Plan 2i — Combat-Art Rotation Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the combat-art rotation reflect what an EoF assassin actually presses: drop buffs (Whirling Blades / Blade Flurry et al. are stances that proc damage, not nukes), drop ranged-weapon arts (the "Shot" line — using them costs melee auto-attacks), and pace each cast by its **real** cast time instead of a flat 0.5s.

**Architecture:** Two code changes on branch `plan2e-scoring-report` (not yet merged): (1) `internal/spell` filters the pulled arts — skip `beneficial != 0` (buffs) and skip ranged arts (effect_list says "If weapon equipped in Ranged"); (2) `internal/model` rotation paces each cast by the art's `cast_secs_hundredths` + recovery, not a uniform constant. Then re-pull (`cmd/builddb`) → re-derive (`cmd/bis`). Stealth/positional arts (Assassinate, Eviscerate, Mortal Blade…) are KEPT — the assassin maintains stealth and stays behind the boss.

**Tech Stack:** Go 1.26, `modernc.org/sqlite`, `stretchr/testify`.

**Why:** A diagnostic showed the rotation was 63% "Whirling Blades" (a `recast 0` buff whose Swipe proc has a damage line `ParseDamage` mistook for a nuke). The pull keeps everything `type=arts` with a parseable damage line, so it swept in stance buffs and ranged abilities. And the sim treats every cast as 0.5s though real casts run 0.25–2.0s. All three distort CADPS and every CA-stat weight.

---

### Task 1: Filter buffs and ranged arts from the pull

**Files:**
- Modify: `internal/spell/pull.go`
- Test: `internal/spell/pull_test.go` *(new)*

**Context:** `AssassinCombatArts` currently keeps any spell with a parseable damage line. `Spell` has `Beneficial int` and `Effects []Effect` (each `Effect.Description string`). Read `internal/spell/pull.go` and `internal/spell/spell.go` first. Extract a testable filter so the census call stays in `AssassinCombatArts` but the keep-logic is unit-testable.

- [ ] **Step 1: Write the failing test** (`internal/spell/pull_test.go`):

```go
package spell

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func sp(name string, beneficial int, recast float64, cast int, effects ...string) Spell {
	effs := make([]Effect, len(effects))
	for i, e := range effects {
		effs[i] = Effect{Description: e}
	}
	return Spell{Name: name, Beneficial: beneficial, RecastSecs: recast, CastSecsHundredths: cast, Effects: effs}
}

func TestFilterCombatArts(t *testing.T) {
	spells := []Spell{
		sp("Eviscerate V", 0, 60, 50, "Inflicts 1133 - 1889 melee damage on target", "Must be flanking or behind"),
		sp("Whirling Blades IV", 1, 0, 50, "Increases Slashing of caster by 40.9", "Inflicts 252 - 421 melee damage on target"), // buff
		sp("Spine Shot IV", 0, 60, 150, "Inflicts 830 - 1383 ranged damage on target", "If weapon equipped in Ranged"),           // ranged
		sp("Caltrops", 0, 20, 50, "Decreases Speed of target"),                                                                    // no damage line
		sp("Assassinate II", 0, 300, 50, "Inflicts 7754 - 12924 melee damage on target", "You must be sneaking to use this ability."),
	}

	arts := FilterCombatArts(spells)

	names := map[string]bool{}
	for _, a := range arts {
		names[a.Name] = true
	}
	require.True(t, names["Eviscerate V"], "melee damaging art kept")
	require.True(t, names["Assassinate II"], "sneaking art kept")
	require.False(t, names["Whirling Blades IV"], "beneficial buff dropped")
	require.False(t, names["Spine Shot IV"], "ranged art dropped")
	require.False(t, names["Caltrops"], "non-damaging art dropped")
	require.Len(t, arts, 2)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/spell/ -run TestFilterCombatArts -v`
Expected: FAIL — `undefined: FilterCombatArts`.

- [ ] **Step 3: Implement** in `internal/spell/pull.go`. Add a `strings` import, the ranged helper, and `FilterCombatArts`; rewrite `AssassinCombatArts` to delegate:

```go
// isRanged reports whether an art requires a ranged weapon (and thus a minimum
// range that pulls the assassin out of melee, costing auto-attacks).
func isRanged(effects []Effect) bool {
	for _, e := range effects {
		if strings.Contains(e.Description, "If weapon equipped in Ranged") {
			return true
		}
	}
	return false
}

// FilterCombatArts keeps only the arts an assassin presses in a melee rotation:
// damaging (parseable damage line), not a buff (beneficial == 0), and not ranged.
func FilterCombatArts(spells []Spell) []CombatArt {
	var arts []CombatArt
	for _, s := range spells {
		if s.Beneficial != 0 {
			continue // buffs/stances (e.g. Whirling Blades) — not press-in-rotation
		}
		if isRanged(s.Effects) {
			continue // ranged shots cost melee auto-attacks; excluded from the melee rotation
		}
		min, max, ok := ParseDamage(effectStrings(s.Effects))
		if !ok {
			continue
		}
		arts = append(arts, CombatArt{
			Name:               s.Name,
			Level:              s.Level,
			MinDamage:          min,
			MaxDamage:          max,
			RecastSecs:         s.RecastSecs,
			CastSecsHundredths: s.CastSecsHundredths,
		})
	}
	return arts
}
```

Then replace the loop in `AssassinCombatArts` so it returns `FilterCombatArts(spells)`:
```go
	spells, err := DecodeSpells(body)
	if err != nil {
		return nil, err
	}
	return FilterCombatArts(spells), nil
```
(Delete the old inline `for _, s := range spells { ParseDamage... }` loop — its logic now lives in `FilterCombatArts`.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/spell/ -run TestFilterCombatArts -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
go test ./internal/spell/ && make lint
git add internal/spell/pull.go internal/spell/pull_test.go
git commit -m "Filter buffs and ranged arts from the combat-art pull"
```

---

### Task 2: Pace the rotation by each art's real cast time

**Files:**
- Modify: `internal/model/rotation.go`
- Modify: `internal/model/dps.go` (the `CADPS` caller)
- Test: `internal/model/rotation_test.go` (or wherever `RotationCADPS` is tested) + `internal/model/timeline_test.go`

**Context:** `RotationCADPS(sb, cas, durationSecs, slotSecs)` advances time by a uniform `slotSecs` per cast. Real cast times vary 0.25–2.0s (`CombatArt.CastSecsHundredths`). Change it to advance by **that art's cast time + recovery**. `constants.CARecoverySecs` (0.25) is the recovery; `constants.CACastTimeSecs` (0.5) becomes the fallback when an art's cast time is missing.

Read `internal/model/rotation.go` and `internal/model/dps.go` first. The current sim:
```go
func RotationCADPS(sb StatBlock, cas []spell.CombatArt, durationSecs, slotSecs float64) float64 {
	if durationSecs <= 0 || slotSecs <= 0 || len(cas) == 0 {
		return 0
	}
	eff := make([]float64, len(cas))
	rec := make([]float64, len(cas))
	avail := make([]float64, len(cas))
	for i, ca := range cas {
		eff[i] = CAEffectiveDamage(sb, ca)
		rec[i] = effRecast(sb, ca)
	}
	var total, t float64
	for t < durationSecs {
		best, bestDmg := -1, -1.0
		for i := range cas {
			if avail[i] <= t && eff[i] > bestDmg {
				best, bestDmg = i, eff[i]
			}
		}
		if best < 0 {
			soonest := math.Inf(1)
			for i := range cas {
				if avail[i] < soonest {
					soonest = avail[i]
				}
			}
			if math.IsInf(soonest, 1) || soonest <= t {
				break
			}
			t = soonest
			continue
		}
		total += bestDmg
		avail[best] = t + rec[best]
		t += slotSecs
	}
	return total / durationSecs
}
```

- [ ] **Step 1: Write the failing test.** Add to the rotation test file (e.g. `internal/model/rotation_test.go`):

```go
func TestRotationPerArtCastTime(t *testing.T) {
	// Two always-up arts (recast 0), equal damage, different cast times.
	// The 0.5s-cast art occupies 0.75s/cast; the 2.0s-cast art occupies 2.25s/cast,
	// so over a fixed window the slow art fires far fewer times. With equal damage,
	// a rotation of only the slow art yields lower CADPS than only the fast art.
	fast := []spell.CombatArt{{Name: "Fast", MinDamage: 100, MaxDamage: 100, RecastSecs: 0, CastSecsHundredths: 50}}
	slow := []spell.CombatArt{{Name: "Slow", MinDamage: 100, MaxDamage: 100, RecastSecs: 0, CastSecsHundredths: 200}}

	fastDPS := RotationCADPS(StatBlock{}, fast, 600, 0.25)
	slowDPS := RotationCADPS(StatBlock{}, slow, 600, 0.25)
	require.Greater(t, fastDPS, slowDPS, "the slower-cast art should yield less CADPS")

	// fast: slot 0.5+0.25=0.75 → 800 casts → 80000/600 ≈ 133.3
	require.InDelta(t, 100.0/0.75, fastDPS, 1e-6)
	// slow: slot 2.0+0.25=2.25 → ~266 casts → ≈ 44.4
	require.InDelta(t, 100.0/2.25, slowDPS, 1e-6)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run TestRotationPerArtCastTime -v`
Expected: FAIL — the 4th arg is currently the full slot (it would need to be 0.75/2.25 by hand); with `0.25` passed as `slotSecs` the current code advances only 0.25s/cast, so both DPS values are equal/wrong.

- [ ] **Step 3: Implement.** Change the 4th parameter to `recoverySecs` and advance by each art's cast time + recovery. Replace the function:

```go
// RotationCADPS simulates the priority rotation. recoverySecs is the post-cast
// recovery added to each art's own cast time to size its timeline slot; an art
// with no recorded cast time falls back to constants.CACastTimeSecs. Auto-attack
// runs in parallel (modeled separately), so casting does not displace it.
func RotationCADPS(sb StatBlock, cas []spell.CombatArt, durationSecs, recoverySecs float64) float64 {
	if durationSecs <= 0 || len(cas) == 0 {
		return 0
	}
	eff := make([]float64, len(cas))
	rec := make([]float64, len(cas))
	slot := make([]float64, len(cas))
	avail := make([]float64, len(cas))
	for i, ca := range cas {
		eff[i] = CAEffectiveDamage(sb, ca)
		rec[i] = effRecast(sb, ca)
		castSecs := float64(ca.CastSecsHundredths) / 100
		if castSecs <= 0 {
			castSecs = constants.CACastTimeSecs
		}
		slot[i] = castSecs + recoverySecs
	}
	var total, t float64
	for t < durationSecs {
		best, bestDmg := -1, -1.0
		for i := range cas {
			if avail[i] <= t && eff[i] > bestDmg {
				best, bestDmg = i, eff[i]
			}
		}
		if best < 0 {
			soonest := math.Inf(1)
			for i := range cas {
				if avail[i] < soonest {
					soonest = avail[i]
				}
			}
			if math.IsInf(soonest, 1) || soonest <= t {
				break
			}
			t = soonest
			continue
		}
		total += bestDmg
		avail[best] = t + rec[best]
		t += slot[best]
	}
	return total / durationSecs
}
```

- [ ] **Step 4: Update the `CADPS` caller** in `internal/model/dps.go`. It currently passes `constants.CACastTimeSecs + constants.CARecoverySecs` as the slot; now pass just the recovery:

```go
// CADPS is the simulated combat-art DPS over a standard fight (priority rotation).
// Each cast occupies its own cast time + recovery before the next cast.
func CADPS(sb StatBlock, cas []spell.CombatArt) float64 {
	return RotationCADPS(sb, cas, constants.FightDurationSecs, constants.CARecoverySecs)
}
```
Then `grep -rn "RotationCADPS" internal/` and fix any other caller/test that passes a combined slot as the 4th arg — they should pass a recovery value now. In particular `internal/model/timeline_test.go` (the `RotationCADPS`/`RotationCADPS wrapper` tests) and any test using a fixed slot: update them so the 4th arg is the recovery, and recompute expected values using the test arts' `CastSecsHundredths` (set `CastSecsHundredths` explicitly in those test arts so the slot is deterministic).

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/model/ -v`
Expected: PASS (the new test + all updated existing rotation/timeline tests).

- [ ] **Step 6: Commit**

```bash
go test ./internal/model/ && go build ./... && make lint
git add internal/model/rotation.go internal/model/dps.go internal/model/rotation_test.go internal/model/timeline_test.go
git commit -m "Pace the CA rotation by each art's real cast time"
```

---

### Task 3: Re-pull, rebuild, re-derive, validate

**Files:** none (operational) — manual run + capture

- [ ] **Step 1: Rebuild the db with the cleaned art list**

Run:
```bash
go build ./... && go vet ./... && go test ./... && make lint
go run ./cmd/builddb     # re-pulls combat arts → now buff/ranged-filtered
```
Expected: all green; builddb reports the gear count and a **lower combat-art count** than before (buffs + ranged dropped). Note the new art count.

- [ ] **Step 2: Confirm the cleaned art list**

Run:
```bash
sqlite3 -column bis.db "SELECT COUNT(*) FROM combat_arts;"
sqlite3 -column bis.db "SELECT name FROM combat_arts WHERE name LIKE '%Whirling%' OR name LIKE '%Blade Flurry%' OR name LIKE '%Head Shot%' OR name LIKE '%Spine Shot%' OR name LIKE '%Back Shot%' OR name LIKE '%Deadly Shot%' OR name LIKE '%Open Shot%';"
```
Expected: the second query returns **no rows** — buffs and ranged shots are gone.

- [ ] **Step 3: Re-derive and re-validate the report**

Run: `go run ./cmd/bis --db bis.db --out bis-report.md`
Then capture for review:
- the RAID converged weight table (`awk '/^## RAID/{f=1} /^### /{f=0} f' bis-report.md`)
- whether the Secondary/Primary picks still look right (no regression from the model change)

- [ ] **Step 4: Report the new rotation's cast count** (the open validation question)

The cast count can't be read from the report directly. Note for the controller: re-run the temporary `RotationCastLog` diagnostic (a throwaway that mirrors `RotationCADPS` with per-art slots and counts casts per art) at the RAID baseline, and report total casts + the per-art breakdown — so the user can gut-check whether ~N casts in 600s is a believable assassin rotation now that Whirling Blades and the ranged shots are gone. Do NOT commit the throwaway.

- [ ] **Step 5: Commit nothing** (operational task). The data artifact `bis.db` is gitignored; the report is regenerated.

---

## Self-Review

**1. Spec coverage:**
- Drop buffs (`beneficial != 0`) → Task 1 (`FilterCombatArts`). ✔
- Drop ranged ("If weapon equipped in Ranged") → Task 1 (`isRanged`). ✔
- Per-art cast times → Task 2 (slot = `CastSecsHundredths/100 + recovery`, fallback `CACastTimeSecs`). ✔
- Keep stealth/positional arts → not filtered (only beneficial + ranged are). ✔
- Rebuild + re-derive + validate cast count → Task 3. ✔

**2. Placeholder scan:** No TBD/TODO; code steps complete; Task 3 is operational/validation by design.

**3. Type consistency:** `FilterCombatArts([]Spell) []CombatArt` and `isRanged([]Effect) bool` use existing types (`Spell.Beneficial`, `Spell.Effects`, `CombatArt` fields). `effectStrings` already exists in pull.go. `RotationCADPS`'s 4th param changes meaning (slot → recovery); `CADPS` updated to pass `constants.CARecoverySecs`; Task 2 Step 4 sweeps all other callers/tests. `CACastTimeSecs` retained as the missing-cast-time fallback.

**Known consequence (intended):** dropping the ranged Shots removes Head Shot (1708) and Spine Shot (1383) — two former top arts — and per-art cast times make slow arts (Massacre 1.5s, Improvised Weapon 1.0s) occupy more timeline. CADPS and every CA-stat weight will shift; the user re-validates. This is the correction, not a regression.
