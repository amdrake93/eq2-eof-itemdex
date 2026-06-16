# Manual Scaling Arts (Backlog §9) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add the two low-level-learned combat arts the census pull misses (they're below the L57 filter), using their tooltip-recovered level-70 bases.

**Architecture:** The recovered bases are calibration constants, so they live in Go (`internal/spell`, like the curve tables live in `internal/model`) as a `manualArts` list of `CombatArt`s each with a single DirectHit component. `builddb` appends `spell.ManualArts()` after the census pull so they persist to the DB and flow through the model unchanged. No census-component overrides for in-pool arts — census (raw) is trusted there (the Gushing Wound melee match validated census VI as the true base; the ~7–11% gaps on other components are opposite-direction read noise — spec §3.1 base-source note).

**Tech Stack:** Go, testify. Builds on Increment B (`CAEffectiveDamage` already sums per-component, so a DirectHit-only manual art scores correctly).

**Recovered bases (backlog §9, attribute-divided, census-equivalent):**
- **Hilt Strike** — `262–315`, recast 20s, cast 0.5s.
- **Strike of Consistency** — `199` flat, recast 12s, cast 0.5s.

---

### Task 1: Define manualArts + ManualArts() accessor

**Files:**
- Create: `internal/spell/manual.go`
- Test: `internal/spell/manual_test.go`

- [ ] **Step 1: Write the failing test**

`internal/spell/manual_test.go`:

```go
package spell

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManualArts(t *testing.T) {
	byName := map[string]CombatArt{}
	for _, a := range ManualArts() {
		byName[a.Name] = a
	}

	hs, ok := byName["Hilt Strike"]
	require.True(t, ok)
	require.Equal(t, 20.0, hs.RecastSecs)
	require.Equal(t, 50, hs.CastSecsHundredths)
	require.Len(t, hs.Components, 1)
	require.Equal(t, DirectHit, hs.Components[0].Kind)
	require.Equal(t, 262.0, hs.Components[0].MinDamage)
	require.Equal(t, 315.0, hs.Components[0].MaxDamage)

	soc, ok := byName["Strike of Consistency"]
	require.True(t, ok)
	require.Equal(t, 12.0, soc.RecastSecs)
	require.Equal(t, 199.0, soc.Components[0].MinDamage)
	require.Equal(t, 199.0, soc.Components[0].MaxDamage)

	// Returns a copy — mutating the result must not corrupt the package data.
	ManualArts()[0].Components[0].MinDamage = -1
	require.Equal(t, 262.0, ManualArts()[0].Components[0].MinDamage)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/spell/ -run TestManualArts -v`
Expected: FAIL — `ManualArts` undefined.

- [ ] **Step 3: Implement**

`internal/spell/manual.go`:

```go
package spell

// manualArts are level-70 combat arts the census pull misses because they are
// learned below the level-57 floor (census reports their low-level damage, not
// the level-70 effective base). Bases recovered via tooltip calibration
// (attribute-divided → census-equivalent; backlog §9, spec §3.1 "Component
// bases"). Each is a single DirectHit; recast/cast are the census values.
var manualArts = []CombatArt{
	{
		Name: "Hilt Strike", Level: 70, RecastSecs: 20, CastSecsHundredths: 50,
		MinDamage: 262, MaxDamage: 315,
		Components: []Component{{Kind: DirectHit, DamageType: "melee", MinDamage: 262, MaxDamage: 315}},
	},
	{
		Name: "Strike of Consistency", Level: 70, RecastSecs: 12, CastSecsHundredths: 50,
		MinDamage: 199, MaxDamage: 199,
		Components: []Component{{Kind: DirectHit, DamageType: "melee", MinDamage: 199, MaxDamage: 199}},
	},
}

// ManualArts returns a deep copy of the recovered low-level-learned arts to
// append after the census pull (see backlog §9). A copy keeps callers from
// mutating the package-level constants.
func ManualArts() []CombatArt {
	out := make([]CombatArt, len(manualArts))
	for i, a := range manualArts {
		a.Components = append([]Component(nil), a.Components...)
		out[i] = a
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/spell/ -run TestManualArts -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/spell/manual.go internal/spell/manual_test.go
git commit -m "Manual scaling arts (§9): Hilt Strike + Strike of Consistency recovered L70 bases"
```

---

### Task 2: Append manual arts in builddb + smoke

**Files:**
- Modify: `cmd/builddb/main.go`

- [ ] **Step 1: Wire in the manual arts**

In `cmd/builddb/main.go`, right after the `spell.AssassinCombatArts` block (before `os.Remove`), append:

```go
	arts = append(arts, spell.ManualArts()...)
```

- [ ] **Step 2: Build**

Run: `go build ./...`
Expected: builds clean.

- [ ] **Step 3: Rebuild the DB and confirm the two arts landed**

Run:
```bash
go run ./cmd/builddb -data data -db bis.db -sid s:example 2>&1 | tail -1
```
Expected: art count rises by 2 vs the prior build (e.g. 19 → 21).

- [ ] **Step 4: Confirm they score (bis report runs and lists them)**

Run: `go run ./cmd/bis 2>&1 | tail -2`
Expected: report regenerates, no error.

- [ ] **Step 5: Commit**

```bash
git add cmd/builddb/main.go
git commit -m "builddb: append manual scaling arts to the combat-art pool (§9)"
```

---

## Self-Review

**Spec coverage:** backlog §9 (add Hilt Strike + Strike of Consistency with recovered L70 bases, recast/cast from census) → Tasks 1–2. ✅ No census-component overrides (decided against — census trusted, spec §3.1 base-source note). ✅
**Placeholder scan:** none.
**Type consistency:** `ManualArts() []CombatArt`; `Component{Kind: DirectHit, ...}` matches Increment A's `component.go`; `CombatArt` fields (`Name`, `Level`, `RecastSecs`, `CastSecsHundredths`, `MinDamage`, `MaxDamage`, `Components`) match `pull.go`.
