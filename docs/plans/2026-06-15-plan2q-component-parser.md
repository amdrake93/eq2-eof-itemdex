# Multi-Component Ability Parser (Increment A) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Parse each combat art's census `effect_list` into typed damage components (DirectHit / DoT / Termination / TriggerProc / RateProc) and persist them, so the sim (Increment B) can sum per-component damage.

**Architecture:** Census exposes parent/child structure via an `indentation` field and effect duration via a structured `duration` object. We add those decode fields, define a `Component` type, write a parser that walks `effect_list` into components, populate `CombatArt.Components` + `DurationSecs` at pull time, and round-trip them through a new `combat_art_components` table. The existing damage model is untouched — `MinDamage/MaxDamage` stay populated as before; the sim ignores `Components` until Increment B. This is the parser/data layer only: **no sim, abmod, or rotation changes** (spec §3.1, those are Increment B).

**Tech Stack:** Go, `regexp`/`strconv`, modernc.org/sqlite (pure-Go), testify.

**Spec:** `docs/design-plan2.md` §3.1 "Multi-component abilities". Branch: `dot-component-parser`.

**Ground-truth fixtures** (real census strings, used verbatim in tests):
- **Gushing Wound** (Termination + DirectHit + DoT-instant): `Applies Untreated Bleeding on termination.`(ind0) / `Inflicts 6 - 10 piercing damage on target.`(ind1) / `Inflicts 0 - 1 melee damage on target`(ind0) / `Inflicts 1 - 2 piercing damage on target instantly and every 4 seconds.`(ind0); `duration.max_sec_tenths=240`.
- **Impale**: `Inflicts 73 - 122 piercing damage on target`(ind0) / `Inflicts 20 - 33 piercing damage on target instantly and every 4 seconds.`(ind0).
- **Quick Strike** (DoT periodic-only, single value): `Inflicts 8 - 13 melee damage on target`(ind0) / `Inflicts 2 slashing damage on target every 4 seconds.`(ind0).
- **Death Mark IV** (TriggerProc, damage at ind2): `When damaged with a melee weapon this spell has a 5% chance to cast Marked on target.  Lasts for 36.0 seconds.`(ind0) / `When damaged with a melee weapon this spell will cast Agonizing Pain on target.`(ind1) / `Inflicts 295 - 492 piercing damage on target`(ind2) / `Grants a total of 5 triggers of the spell.`(ind2) / `Grants a total of 1 trigger of the spell.`(ind1).
- **Whirling Blades IV** (RateProc): `On a melee hit this spell may cast Swipe on target of attack.  Triggers about 2.0 times per minute.`(ind0) / `Inflicts 252 - 421 melee damage on target`(ind1).
- **Apply Poison** (RateProc casting a DoT): `On a melee hit this spell may cast Assassin's Hemotoxin on target of attack.  Lasts for 24.0 seconds.  Triggers about 3.0 times per minute.`(ind0) / `Inflicts 217 poison damage on target instantly and every 4 seconds.`(ind1).

---

## File Structure

- `internal/spell/spell.go` — **modify**: add `Indentation` to `Effect`, add `Duration` struct + `Duration` field to `Spell`.
- `internal/spell/component.go` — **create**: `ComponentKind` enum + `Component` struct (data model).
- `internal/spell/parse.go` — **modify**: add `parseDamageLine` (single-line classifier) + `ParseComponents` (full walker) + regexes; keep existing `ParseDamage`.
- `internal/spell/pull.go` — **modify**: add `DurationSecs` + `Components` to `CombatArt`; populate them in `FilterCombatArts`.
- `internal/store/store.go` — **modify**: extend `combat_arts` schema (`duration_secs`), add `combat_art_components` table, persist components in `LoadCombatArts`, add `CombatArts()` loader, route `LoadLoadout` through it.
- Tests: `internal/spell/spell_test.go`, `internal/spell/parse_test.go`, `internal/spell/pull_test.go`, `internal/store/store_test.go` (or existing `combatarts_test.go`).

---

### Task 1: Census decode — indentation + duration

**Files:**
- Modify: `internal/spell/spell.go`
- Test: `internal/spell/spell_test.go` (create if absent)

- [ ] **Step 1: Write the failing test**

In `internal/spell/spell_test.go`:

```go
package spell

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeSpellsParsesIndentationAndDuration(t *testing.T) {
	body := []byte(`{"spell_list":[{
	  "name":"Gushing Wound","level":2,"tier_name":"Expert","type":"arts","beneficial":0,
	  "recast_secs":30.0,"cast_secs_hundredths":50,
	  "duration":{"max_sec_tenths":240,"min_sec_tenths":240,"does_not_expire":0},
	  "classes":{"assassin":{"id":40,"level":2}},
	  "effect_list":[
	    {"description":"Applies Untreated Bleeding on termination.","indentation":0},
	    {"description":"Inflicts 6 - 10 piercing damage on target.","indentation":1},
	    {"description":"Inflicts 0 - 1 melee damage on target","indentation":0}
	  ]}],"returned":1}`)
	spells, err := DecodeSpells(body)
	require.NoError(t, err)
	require.Len(t, spells, 1)
	require.Equal(t, 240, spells[0].Duration.MaxSecTenths)
	require.Equal(t, 1, spells[0].Effects[1].Indentation)
	require.Equal(t, 0, spells[0].Effects[2].Indentation)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/spell/ -run TestDecodeSpellsParsesIndentationAndDuration -v`
Expected: FAIL — `Effect` has no `Indentation` field / `Spell` has no `Duration` field (compile error).

- [ ] **Step 3: Add the decode fields**

In `internal/spell/spell.go`, replace the `Effect` struct and add a `Duration` struct + field on `Spell`:

```go
type Effect struct {
	Description string `json:"description"`
	Indentation int    `json:"indentation"`
}

// Duration is the census effect-duration object; seconds = max_sec_tenths / 10.
type Duration struct {
	MaxSecTenths  int `json:"max_sec_tenths"`
	MinSecTenths  int `json:"min_sec_tenths"`
	DoesNotExpire int `json:"does_not_expire"`
}
```

Add to the `Spell` struct (after `RecastSecs`):

```go
	Duration           Duration            `json:"duration"`
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/spell/ -run TestDecodeSpellsParsesIndentationAndDuration -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/spell/spell.go internal/spell/spell_test.go
git commit -m "Census decode: indentation + duration fields"
```

---

### Task 2: Component data model

**Files:**
- Create: `internal/spell/component.go`
- Modify: `internal/spell/pull.go` (CombatArt fields)

No standalone test — a pure data struct has no behavior; it's exercised by Task 4+. This task is a compile-only definition.

- [ ] **Step 1: Create the component type**

`internal/spell/component.go`:

```go
package spell

// ComponentKind classifies how a damage component is delivered.
type ComponentKind int

const (
	DirectHit   ComponentKind = iota // single instant hit
	DoT                              // damage over time (periodic, optionally with an instant tick)
	Termination                      // fires once when a DoT/effect duration expires
	TriggerProc                      // cast on an event a fixed number of times
	RateProc                         // cast on an event at an approximate rate per minute
)

// Component is one parsed damage line of an ability. An ability's total damage
// is the sum over its components (each simmed with the per-component equation in
// Increment B). Fields beyond the kind's own are zero-valued.
type Component struct {
	Kind           ComponentKind
	DamageType     string  // melee, piercing, ranged, slashing, poison, ...
	MinDamage      float64
	MaxDamage      float64
	IntervalSecs   float64 // DoT (or proc-cast DoT): tick interval in seconds
	HasInstant     bool    // DoT: "instantly and every" (true) vs bare "every" (false)
	AoE            bool    // "targets in Area of Effect"
	TriggeredSpell string  // Termination/Proc: name of the applied/cast spell
	Triggers       int     // TriggerProc: total trigger count
	PerMinute      float64 // RateProc: approximate procs per minute
}
```

- [ ] **Step 2: Add fields to CombatArt**

In `internal/spell/pull.go`, add to the `CombatArt` struct (after `PotencyAdd`):

```go
	DurationSecs       float64     // effect duration in seconds (0 if none); census duration.max_sec_tenths/10
	Components         []Component // parsed damage components (Increment A; the sim consumes these in Increment B)
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./...`
Expected: builds clean.

- [ ] **Step 4: Commit**

```bash
git add internal/spell/component.go internal/spell/pull.go
git commit -m "Component data model + CombatArt.Components/DurationSecs fields"
```

---

### Task 3: Single damage-line classifier

**Files:**
- Modify: `internal/spell/parse.go`
- Test: `internal/spell/parse_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/spell/parse_test.go`:

```go
func TestParseDamageLine(t *testing.T) {
	cases := []struct {
		desc string
		want damageLine
		ok   bool
	}{
		{"Inflicts 73 - 122 piercing damage on target", damageLine{min: 73, max: 122, dmgType: "piercing"}, true},
		{"Inflicts 0 - 1 melee damage on target", damageLine{min: 0, max: 1, dmgType: "melee"}, true},
		{"Inflicts 1 - 2 piercing damage on target instantly and every 4 seconds.", damageLine{min: 1, max: 2, dmgType: "piercing", periodic: true, hasInstant: true, intervalSecs: 4}, true},
		{"Inflicts 2 slashing damage on target every 4 seconds.", damageLine{min: 2, max: 2, dmgType: "slashing", periodic: true, intervalSecs: 4}, true},
		{"Inflicts 217 poison damage on target instantly and every 4 seconds.", damageLine{min: 217, max: 217, dmgType: "poison", periodic: true, hasInstant: true, intervalSecs: 4}, true},
		{"Inflicts 252 - 421 melee damage on targets in Area of Effect", damageLine{min: 252, max: 421, dmgType: "melee", aoe: true}, true},
		{"Applies Untreated Bleeding on termination.", damageLine{}, false},
		{"Increases Haste of caster by 30.6.", damageLine{}, false},
	}
	for _, c := range cases {
		got, ok := parseDamageLine(c.desc)
		require.Equal(t, c.ok, ok, c.desc)
		if c.ok {
			require.Equal(t, c.want, got, c.desc)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/spell/ -run TestParseDamageLine -v`
Expected: FAIL — `parseDamageLine` / `damageLine` undefined.

- [ ] **Step 3: Implement the classifier**

Add to `internal/spell/parse.go` (keep the existing `damageRe`, `toFloat`, `ParseDamage`):

```go
// dmgLineRe matches an "Inflicts ..." damage description: amount (single or
// range), damage type, target scope, and an optional periodic clause.
var dmgLineRe = regexp.MustCompile(
	`^Inflicts\s+([\d,]+)(?:\s*-\s*([\d,]+))?\s+(\w+)\s+damage\s+on\s+(target|targets in Area of Effect)(?:\s+(instantly and every|every)\s+([\d.]+)\s+seconds)?`)

// damageLine is a parsed "Inflicts ..." description before kind/context resolution.
type damageLine struct {
	min, max     float64
	dmgType      string
	aoe          bool
	periodic     bool    // has an "every N seconds" clause
	hasInstant   bool    // "instantly and every" (vs bare "every")
	intervalSecs float64
}

// parseDamageLine extracts a damageLine from one effect description. ok is false
// for non-damage lines (buffs, termination/proc descriptors, conditions).
func parseDamageLine(desc string) (damageLine, bool) {
	m := dmgLineRe.FindStringSubmatch(desc)
	if m == nil {
		return damageLine{}, false
	}
	dl := damageLine{
		min:     toFloat(m[1]),
		dmgType: m[3],
		aoe:     strings.HasPrefix(m[4], "targets"),
	}
	if m[2] != "" {
		dl.max = toFloat(m[2])
	} else {
		dl.max = dl.min
	}
	if m[5] != "" {
		dl.periodic = true
		dl.hasInstant = m[5] == "instantly and every"
		dl.intervalSecs = toFloat(m[6])
	}
	return dl, true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/spell/ -run TestParseDamageLine -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/spell/parse.go internal/spell/parse_test.go
git commit -m "Single damage-line classifier (parseDamageLine)"
```

---

### Task 4: ParseComponents — DirectHit + DoT (standalone, ind 0)

**Files:**
- Modify: `internal/spell/parse.go`
- Test: `internal/spell/parse_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/spell/parse_test.go`:

```go
func TestParseComponents_DirectHitAndDoT(t *testing.T) {
	// Impale: a DirectHit + a DoT-with-instant (both ind 0).
	impale := []Effect{
		{Description: "Inflicts 73 - 122 piercing damage on target", Indentation: 0},
		{Description: "Inflicts 20 - 33 piercing damage on target instantly and every 4 seconds.", Indentation: 0},
	}
	comps := ParseComponents(impale, 24.0)
	require.Len(t, comps, 2)
	require.Equal(t, DirectHit, comps[0].Kind)
	require.Equal(t, 122.0, comps[0].MaxDamage)
	require.Equal(t, DoT, comps[1].Kind)
	require.True(t, comps[1].HasInstant)
	require.Equal(t, 4.0, comps[1].IntervalSecs)

	// Quick Strike: DirectHit + periodic-only DoT (single value, no instant).
	quick := []Effect{
		{Description: "Inflicts 8 - 13 melee damage on target", Indentation: 0},
		{Description: "Inflicts 2 slashing damage on target every 4 seconds.", Indentation: 0},
	}
	comps = ParseComponents(quick, 12.0)
	require.Len(t, comps, 2)
	require.Equal(t, DirectHit, comps[0].Kind)
	require.Equal(t, DoT, comps[1].Kind)
	require.False(t, comps[1].HasInstant)
	require.Equal(t, 2.0, comps[1].MinDamage)
	require.Equal(t, 2.0, comps[1].MaxDamage)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/spell/ -run TestParseComponents_DirectHitAndDoT -v`
Expected: FAIL — `ParseComponents` undefined.

- [ ] **Step 3: Implement the standalone walker**

Add to `internal/spell/parse.go`:

```go
// ParseComponents extracts the typed damage components of an ability from its
// effect_list. durationSecs is the art's effect duration (census
// duration.max_sec_tenths/10). Parsing only — the sim consumes Components in
// Increment B. (Termination and proc nesting are added in later tasks.)
func ParseComponents(effects []Effect, durationSecs float64) []Component {
	var comps []Component
	for _, e := range effects {
		if e.Indentation != 0 {
			continue // indented children handled with their parents (later tasks)
		}
		dl, ok := parseDamageLine(e.Description)
		if !ok {
			continue
		}
		comps = append(comps, standaloneComponent(dl))
	}
	return comps
}

func standaloneComponent(dl damageLine) Component {
	c := Component{
		DamageType: dl.dmgType,
		MinDamage:  dl.min,
		MaxDamage:  dl.max,
		AoE:        dl.aoe,
	}
	if dl.periodic {
		c.Kind = DoT
		c.IntervalSecs = dl.intervalSecs
		c.HasInstant = dl.hasInstant
	} else {
		c.Kind = DirectHit
	}
	return c
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/spell/ -run TestParseComponents_DirectHitAndDoT -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/spell/parse.go internal/spell/parse_test.go
git commit -m "ParseComponents: standalone DirectHit + DoT components"
```

---

### Task 5: ParseComponents — Termination (indented child of "on termination")

**Files:**
- Modify: `internal/spell/parse.go`
- Test: `internal/spell/parse_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/spell/parse_test.go`:

```go
func TestParseComponents_Termination(t *testing.T) {
	// Gushing Wound: termination (+inlined detonate child) + DirectHit + DoT.
	gushing := []Effect{
		{Description: "Applies Untreated Bleeding on termination.", Indentation: 0},
		{Description: "Inflicts 6 - 10 piercing damage on target.", Indentation: 1},
		{Description: "Inflicts 0 - 1 melee damage on target", Indentation: 0},
		{Description: "Inflicts 1 - 2 piercing damage on target instantly and every 4 seconds.", Indentation: 0},
	}
	comps := ParseComponents(gushing, 24.0)
	require.Len(t, comps, 3)
	require.Equal(t, Termination, comps[0].Kind)
	require.Equal(t, "Untreated Bleeding", comps[0].TriggeredSpell)
	require.Equal(t, 10.0, comps[0].MaxDamage)
	require.Equal(t, DirectHit, comps[1].Kind)
	require.Equal(t, "melee", comps[1].DamageType)
	require.Equal(t, DoT, comps[2].Kind)
	require.True(t, comps[2].HasInstant)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/spell/ -run TestParseComponents_Termination -v`
Expected: FAIL — the ind-1 detonate child is skipped, so only 2 components are returned.

- [ ] **Step 3: Extend the walker for termination**

In `internal/spell/parse.go`, add the regex and replace `ParseComponents` with the indentation-tracking version:

```go
var terminationRe = regexp.MustCompile(`^Applies (.+?) on termination`)

// ParseComponents extracts the typed damage components of an ability from its
// effect_list. durationSecs is the art's effect duration (census
// duration.max_sec_tenths/10). Parsing only — the sim consumes Components in
// Increment B. Indented damage lines are resolved against their parent line
// (the entry at indentation-1): a child of an "Applies <Spell> on termination"
// line is the termination/detonate damage.
func ParseComponents(effects []Effect, durationSecs float64) []Component {
	var comps []Component
	parent := map[int]string{} // indentation -> last description seen at that level
	for _, e := range effects {
		parent[e.Indentation] = e.Description
		dl, ok := parseDamageLine(e.Description)
		if !ok {
			continue
		}
		if e.Indentation == 0 {
			comps = append(comps, standaloneComponent(dl))
			continue
		}
		pd := parent[e.Indentation-1]
		if terminationRe.MatchString(pd) {
			comps = append(comps, terminationComponent(dl, pd))
		}
	}
	return comps
}

func terminationComponent(dl damageLine, parentDesc string) Component {
	c := Component{
		Kind:       Termination,
		DamageType: dl.dmgType,
		MinDamage:  dl.min,
		MaxDamage:  dl.max,
		AoE:        dl.aoe,
	}
	if m := terminationRe.FindStringSubmatch(parentDesc); m != nil {
		c.TriggeredSpell = m[1]
	}
	return c
}
```

- [ ] **Step 4: Run both ParseComponents tests to verify they pass**

Run: `go test ./internal/spell/ -run TestParseComponents -v`
Expected: PASS (both `_DirectHitAndDoT` and `_Termination`).

- [ ] **Step 5: Commit**

```bash
git add internal/spell/parse.go internal/spell/parse_test.go
git commit -m "ParseComponents: Termination components (indented detonate child)"
```

---

### Task 6: ParseComponents — TriggerProc + RateProc

**Files:**
- Modify: `internal/spell/parse.go`
- Test: `internal/spell/parse_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/spell/parse_test.go`:

```go
func TestParseComponents_Procs(t *testing.T) {
	// Death Mark IV: TriggerProc — damage + count at ind 2; outer "Grants 1
	// trigger" at ind 1 (for Marked, which has no inlined damage) must be ignored.
	deathMark := []Effect{
		{Description: "When damaged with a melee weapon this spell has a 5% chance to cast Marked on target.  Lasts for 36.0 seconds.", Indentation: 0},
		{Description: "When damaged with a melee weapon this spell will cast Agonizing Pain on target.", Indentation: 1},
		{Description: "Inflicts 295 - 492 piercing damage on target", Indentation: 2},
		{Description: "Grants a total of 5 triggers of the spell.", Indentation: 2},
		{Description: "Grants a total of 1 trigger of the spell.", Indentation: 1},
	}
	comps := ParseComponents(deathMark, 72.0)
	require.Len(t, comps, 1)
	require.Equal(t, TriggerProc, comps[0].Kind)
	require.Equal(t, "Agonizing Pain", comps[0].TriggeredSpell)
	require.Equal(t, 492.0, comps[0].MaxDamage)
	require.Equal(t, 5, comps[0].Triggers)

	// Whirling Blades IV: RateProc — a hit cast ~2/min.
	whirling := []Effect{
		{Description: "On a melee hit this spell may cast Swipe on target of attack.  Triggers about 2.0 times per minute.", Indentation: 0},
		{Description: "Inflicts 252 - 421 melee damage on target", Indentation: 1},
	}
	comps = ParseComponents(whirling, 0)
	require.Len(t, comps, 1)
	require.Equal(t, RateProc, comps[0].Kind)
	require.Equal(t, "Swipe", comps[0].TriggeredSpell)
	require.Equal(t, 2.0, comps[0].PerMinute)

	// Apply Poison: RateProc that casts a DoT (interval/instant captured).
	applyPoison := []Effect{
		{Description: "On a melee hit this spell may cast Assassin's Hemotoxin on target of attack.  Lasts for 24.0 seconds.  Triggers about 3.0 times per minute.", Indentation: 0},
		{Description: "Inflicts 217 poison damage on target instantly and every 4 seconds.", Indentation: 1},
	}
	comps = ParseComponents(applyPoison, 0)
	require.Len(t, comps, 1)
	require.Equal(t, RateProc, comps[0].Kind)
	require.Equal(t, 3.0, comps[0].PerMinute)
	require.Equal(t, 4.0, comps[0].IntervalSecs)
	require.True(t, comps[0].HasInstant)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/spell/ -run TestParseComponents_Procs -v`
Expected: FAIL — proc parents aren't recognized, so 0 components returned.

- [ ] **Step 3: Extend the walker for procs**

In `internal/spell/parse.go`, add the regexes and helpers, and extend `ParseComponents`'s indented-line handling:

```go
var (
	castSpellRe = regexp.MustCompile(`(?:may|will) cast (.+?) on target`)
	perMinuteRe = regexp.MustCompile(`Triggers about ([\d.]+) times per minute`)
	triggersRe  = regexp.MustCompile(`Grants a total of (\d+) trigger`)
)
```

Replace `ParseComponents` with this version (adds proc handling + trigger-count attachment by indentation):

```go
func ParseComponents(effects []Effect, durationSecs float64) []Component {
	var comps []Component
	parent := map[int]string{} // indentation -> last description seen at that level
	procIdx := map[int]int{}   // indentation -> index in comps of a proc component at that level
	for _, e := range effects {
		parent[e.Indentation] = e.Description

		if m := triggersRe.FindStringSubmatch(e.Description); m != nil {
			if idx, ok := procIdx[e.Indentation]; ok {
				comps[idx].Triggers, _ = strconv.Atoi(m[1])
			}
			continue
		}

		dl, ok := parseDamageLine(e.Description)
		if !ok {
			continue
		}
		if e.Indentation == 0 {
			comps = append(comps, standaloneComponent(dl))
			continue
		}

		pd := parent[e.Indentation-1]
		switch {
		case terminationRe.MatchString(pd):
			comps = append(comps, terminationComponent(dl, pd))
		case castSpellRe.MatchString(pd) && perMinuteRe.MatchString(pd):
			comps = append(comps, procComponent(dl, pd, RateProc))
			procIdx[e.Indentation] = len(comps) - 1
		case castSpellRe.MatchString(pd):
			comps = append(comps, procComponent(dl, pd, TriggerProc))
			procIdx[e.Indentation] = len(comps) - 1
		}
	}
	return comps
}

func procComponent(dl damageLine, parentDesc string, kind ComponentKind) Component {
	c := Component{
		Kind:       kind,
		DamageType: dl.dmgType,
		MinDamage:  dl.min,
		MaxDamage:  dl.max,
		AoE:        dl.aoe,
	}
	if dl.periodic { // a proc that casts a DoT (e.g. Hemotoxin)
		c.IntervalSecs = dl.intervalSecs
		c.HasInstant = dl.hasInstant
	}
	if m := castSpellRe.FindStringSubmatch(parentDesc); m != nil {
		c.TriggeredSpell = m[1]
	}
	if kind == RateProc {
		if m := perMinuteRe.FindStringSubmatch(parentDesc); m != nil {
			c.PerMinute = toFloat(m[1])
		}
	}
	return c
}
```

Add `"strconv"` to the import block if not already present (it is, via `toFloat`).

- [ ] **Step 4: Run all spell parser tests to verify they pass**

Run: `go test ./internal/spell/ -run TestParseComponents -v`
Expected: PASS (`_DirectHitAndDoT`, `_Termination`, `_Procs`).

- [ ] **Step 5: Commit**

```bash
git add internal/spell/parse.go internal/spell/parse_test.go
git commit -m "ParseComponents: TriggerProc + RateProc with trigger-count attachment"
```

---

### Task 7: Populate components at pull time

**Files:**
- Modify: `internal/spell/pull.go`
- Test: `internal/spell/pull_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/spell/pull_test.go`:

```go
func TestFilterCombatArtsPopulatesComponents(t *testing.T) {
	gushing := Spell{
		Name:       "Gushing Wound",
		Level:      66,
		Beneficial: 0,
		RecastSecs: 30,
		Duration:   Duration{MaxSecTenths: 240},
		Effects: []Effect{
			{Description: "Applies Untreated Bleeding on termination.", Indentation: 0},
			{Description: "Inflicts 6 - 10 piercing damage on target.", Indentation: 1},
			{Description: "Inflicts 0 - 1 melee damage on target", Indentation: 0},
			{Description: "Inflicts 1 - 2 piercing damage on target instantly and every 4 seconds.", Indentation: 0},
		},
	}
	arts := FilterCombatArts([]Spell{gushing})
	require.Len(t, arts, 1)
	require.Equal(t, 24.0, arts[0].DurationSecs)
	require.Len(t, arts[0].Components, 3)
	require.Equal(t, Termination, arts[0].Components[0].Kind)
	require.Equal(t, DirectHit, arts[0].Components[1].Kind)
	require.Equal(t, DoT, arts[0].Components[2].Kind)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/spell/ -run TestFilterCombatArtsPopulatesComponents -v`
Expected: FAIL — `DurationSecs` is 0 and `Components` is nil (not yet populated).

- [ ] **Step 3: Populate components in FilterCombatArts**

In `internal/spell/pull.go`, inside `FilterCombatArts`, replace the `arts = append(...)` block with:

```go
		durationSecs := float64(s.Duration.MaxSecTenths) / 10
		arts = append(arts, CombatArt{
			Name:               s.Name,
			Level:              s.Level,
			MinDamage:          min,
			MaxDamage:          max,
			RecastSecs:         s.RecastSecs,
			CastSecsHundredths: s.CastSecsHundredths,
			DurationSecs:       durationSecs,
			Components:         ParseComponents(s.Effects, durationSecs),
		})
```

- [ ] **Step 4: Run the spell package tests**

Run: `go test ./internal/spell/ -v`
Expected: PASS — the new test plus the unchanged `TestAssassinCombatArts`/`TestFilterCombatArts` (which don't assert on the new fields).

- [ ] **Step 5: Commit**

```bash
git add internal/spell/pull.go internal/spell/pull_test.go
git commit -m "FilterCombatArts: populate Components + DurationSecs"
```

---

### Task 8: Persist + load components through the DB

**Files:**
- Modify: `internal/store/store.go`
- Test: `internal/store/store_test.go` (create) or extend `internal/store/combatarts_test.go`

- [ ] **Step 1: Write the failing round-trip test**

Add to `internal/store/store_test.go` (create the file with this package/imports if needed):

```go
package store

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	"github.com/stretchr/testify/require"
)

func TestCombatArtsRoundTripComponents(t *testing.T) {
	db, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	require.NoError(t, db.Init())

	art := spell.CombatArt{
		Name: "Gushing Wound", Level: 66, RecastSecs: 30, DurationSecs: 24,
		Components: []spell.Component{
			{Kind: spell.Termination, DamageType: "piercing", MinDamage: 6, MaxDamage: 10, TriggeredSpell: "Untreated Bleeding"},
			{Kind: spell.DirectHit, DamageType: "melee", MinDamage: 0, MaxDamage: 1},
			{Kind: spell.DoT, DamageType: "piercing", MinDamage: 1, MaxDamage: 2, IntervalSecs: 4, HasInstant: true},
		},
	}
	require.NoError(t, db.LoadCombatArts([]spell.CombatArt{art}))

	got, err := db.CombatArts()
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, 24.0, got[0].DurationSecs)
	require.Len(t, got[0].Components, 3)
	require.Equal(t, spell.Termination, got[0].Components[0].Kind)
	require.Equal(t, "Untreated Bleeding", got[0].Components[0].TriggeredSpell)
	require.Equal(t, spell.DoT, got[0].Components[2].Kind)
	require.True(t, got[0].Components[2].HasInstant)
	require.Equal(t, 4.0, got[0].Components[2].IntervalSecs)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestCombatArtsRoundTripComponents -v`
Expected: FAIL — `db.CombatArts` undefined; `duration_secs`/`combat_art_components` not in schema.

- [ ] **Step 3: Extend the schema**

In `internal/store/store.go`, in the `schema` const, replace the `combat_arts` table and add the components table:

```sql
CREATE TABLE IF NOT EXISTS combat_arts (
  name TEXT PRIMARY KEY, level INTEGER, min_dmg REAL, max_dmg REAL,
  recast_secs REAL, cast_secs_hundredths INTEGER, duration_secs REAL
);
CREATE TABLE IF NOT EXISTS combat_art_components (
  art_name TEXT, idx INTEGER, kind INTEGER, dmg_type TEXT,
  min_dmg REAL, max_dmg REAL, interval_secs REAL, has_instant INTEGER,
  aoe INTEGER, triggered_spell TEXT, triggers INTEGER, per_minute REAL,
  PRIMARY KEY (art_name, idx)
);
```

- [ ] **Step 4: Persist duration + components in LoadCombatArts**

In `internal/store/store.go`, replace the insert loop body of `LoadCombatArts` with:

```go
	for _, a := range arts {
		if _, err = tx.Exec(
			`INSERT OR REPLACE INTO combat_arts (name,level,min_dmg,max_dmg,recast_secs,cast_secs_hundredths,duration_secs)
			 VALUES (?,?,?,?,?,?,?)`,
			a.Name, a.Level, a.MinDamage, a.MaxDamage, a.RecastSecs, a.CastSecsHundredths, a.DurationSecs,
		); err != nil {
			return err
		}
		if _, err = tx.Exec(`DELETE FROM combat_art_components WHERE art_name = ?`, a.Name); err != nil {
			return err
		}
		for i, c := range a.Components {
			if _, err = tx.Exec(
				`INSERT INTO combat_art_components
				 (art_name,idx,kind,dmg_type,min_dmg,max_dmg,interval_secs,has_instant,aoe,triggered_spell,triggers,per_minute)
				 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
				a.Name, i, int(c.Kind), c.DamageType, c.MinDamage, c.MaxDamage,
				c.IntervalSecs, boolToInt(c.HasInstant), boolToInt(c.AoE), c.TriggeredSpell, c.Triggers, c.PerMinute,
			); err != nil {
				return err
			}
		}
	}
```

Add this helper near the top of `internal/store/store.go` (after the imports):

```go
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
```

- [ ] **Step 5: Add the CombatArts loader and route LoadLoadout through it**

In `internal/store/store.go`, add:

```go
// CombatArts loads every combat art with its parsed damage components attached.
func (d *DB) CombatArts() ([]spell.CombatArt, error) {
	rows, err := d.db.Query(`SELECT name, level, min_dmg, max_dmg, recast_secs, cast_secs_hundredths, duration_secs FROM combat_arts`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var arts []spell.CombatArt
	for rows.Next() {
		var a spell.CombatArt
		if err := rows.Scan(&a.Name, &a.Level, &a.MinDamage, &a.MaxDamage, &a.RecastSecs, &a.CastSecsHundredths, &a.DurationSecs); err != nil {
			return nil, err
		}
		arts = append(arts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	comps, err := d.loadComponents()
	if err != nil {
		return nil, err
	}
	for i := range arts {
		arts[i].Components = comps[arts[i].Name]
	}
	return arts, nil
}

func (d *DB) loadComponents() (map[string][]spell.Component, error) {
	rows, err := d.db.Query(
		`SELECT art_name, kind, dmg_type, min_dmg, max_dmg, interval_secs, has_instant, aoe, triggered_spell, triggers, per_minute
		 FROM combat_art_components ORDER BY art_name, idx`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string][]spell.Component{}
	for rows.Next() {
		var (
			name        string
			kind        int
			hasInstant  int
			aoe         int
			c           spell.Component
		)
		if err := rows.Scan(&name, &kind, &c.DamageType, &c.MinDamage, &c.MaxDamage,
			&c.IntervalSecs, &hasInstant, &aoe, &c.TriggeredSpell, &c.Triggers, &c.PerMinute); err != nil {
			return nil, err
		}
		c.Kind = spell.ComponentKind(kind)
		c.HasInstant = hasInstant != 0
		c.AoE = aoe != 0
		out[name] = append(out[name], c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
```

Then in `LoadLoadout`, replace the inline `combat_arts` query/scan block (from `rows, err := d.db.Query(`SELECT name, min_dmg...` through the `rows.Err()` check) with:

```go
	arts, err := d.CombatArts()
	if err != nil {
		return Loadout{}, err
	}
	return Loadout{Main: main, MainName: mainName, Arts: spell.HighestRanks(arts)}, nil
```

- [ ] **Step 6: Run the round-trip test**

Run: `go test ./internal/store/ -run TestCombatArtsRoundTripComponents -v`
Expected: PASS.

- [ ] **Step 7: Run the full store + spell suites**

Run: `go test ./internal/store/ ./internal/spell/ -v`
Expected: PASS (existing `LoadLoadout`/combatarts tests still green through the refactor).

- [ ] **Step 8: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go
git commit -m "Persist + load combat-art components (combat_art_components table; CombatArts loader)"
```

---

### Task 9: Full-suite green + live builddb smoke

**Files:** none (verification only).

- [ ] **Step 1: Run the entire test suite**

Run: `go test ./...`
Expected: all packages PASS. (No production damage/sim code changed, so `internal/model` and `internal/bis` results are unchanged — `Components` is populated but unread until Increment B.)

- [ ] **Step 2: Rebuild the DB from live census (smoke)**

Run: `go run ./cmd/builddb -data data -db bis.db -sid s:example`
Expected: prints `built bis.db: <N> gear items, <M> combat arts` with no error. (Confirms the new schema + component inserts work against real census payloads. `s:example` is rate-limited to 10 req/min but the single arts pull fits.)

- [ ] **Step 3: Verify components landed for a known multi-component art**

Run:
```bash
go run ./cmd/itemdex --sql "SELECT art_name, kind, dmg_type, min_dmg, max_dmg, has_instant, triggered_spell FROM combat_art_components WHERE art_name LIKE 'Gushing Wound%' ORDER BY idx"
```
If `cmd/itemdex` has no `--sql` passthrough, instead add a throwaway check via the store in a scratch `main` or skip — the Task 8 round-trip test is the authoritative proof. Expected (when run): three rows for Gushing Wound — a Termination (`Untreated Bleeding`), a DirectHit (melee), and a DoT (piercing, has_instant=1).

- [ ] **Step 4: Final commit (if any uncommitted verification artifacts)**

```bash
git status   # expect clean; bis.db is gitignored
```

---

## Self-Review

**Spec coverage** (§3.1 "Multi-component abilities"):
- Component taxonomy DirectHit/DoT/Termination/TriggerProc/RateProc → Tasks 2–6. ✅
- Parse from census via `indentation` + `duration` → Tasks 1, 5, 6. ✅
- Inlined termination/proc child damage (no sub-pull) → Tasks 5, 6. ✅
- DoT with-instant vs periodic-only (single value) → Tasks 3, 4. ✅
- `MinDamage/MaxDamage` and existing model untouched → Task 2/7 keep them; no `internal/model` changes. ✅
- Persistence so Increment B can consume → Task 8. ✅
- **Out of scope (Increment B), correctly absent here:** per-component damage sim, abmod-to-DirectHit rule, `max(effRecast,duration)` rotation, level-70 base recovery for low-level-learned arts, scoring of TriggerProc/RateProc.

**Placeholder scan:** Task 9 Step 3 notes a fallback if `cmd/itemdex` lacks a `--sql` flag (the round-trip test is the real proof) — that's a genuine conditional, not a placeholder. No TBD/TODO elsewhere.

**Type consistency:** `Component`/`ComponentKind` fields (`Kind`, `DamageType`, `MinDamage`, `MaxDamage`, `IntervalSecs`, `HasInstant`, `AoE`, `TriggeredSpell`, `Triggers`, `PerMinute`) are defined in Task 2 and used identically in Tasks 4–8. `ParseComponents(effects []Effect, durationSecs float64) []Component` signature is stable across Tasks 4→5→6→7. `DB.CombatArts()`/`loadComponents()` names consistent in Task 8.
