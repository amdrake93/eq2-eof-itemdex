# Plan 2o — Auto-Attack Weapon-Damage Equation + Class-Data System

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make auto-attack damage match the game — add the AGI multiplier (already on CAs) and the Assassin's innate ×2.0 class auto-attack multiplier to `AutoDPS`, with the class multiplier sourced from a new uniform-schema `classes/<class>.toml`.

**Architecture:** A class-data loader (`charconfig.LoadClass`) reads `classes/<class>.toml` (strict — `auto_attack_multiplier` required). In the model, `AutoDPS` gains the AGI factor internally (via `sb.MainStat`, so a zero-MainStat block is unchanged); the class multiplier is threaded as an explicit param through the auto entry points (`AutoDPSDual`/`TotalDPS`/`TotalDPSDual`/`ItemDelta`) and carried on the bis `Set` — NOT a `StatBlock` field, because a multiplier's 0.0 zero-value would zero out auto damage everywhere `StatBlock{}` is used.

**Tech Stack:** Go 1.26, BurntSushi/toml (already a dep), testify.

**Spec:** `docs/design-plan2.md` §3.1 (auto-attack weapon-damage equation) + §4 (class-data system), committed this session.

**Calibrated values (from `/weaponstat` + 3 gear states; the model must reproduce these):** innate `auto_attack_multiplier = 2.0`; `MainStatEffect(625)=51.74`, `MainStatEffect(983)=64.06` (committed table samples); `HasteDpsModEffect(73.2)=51`. Blood Fire census max 290 → actual 882 at (AGI 625/dpsmod 0/×2.0) and → 1442 at (AGI 983/dpsmod stat 73.2→51%/×2.0).

---

### Task 1: Class-data loader + `classes/assassin.toml`

**Files:**
- Create: `classes/assassin.toml`
- Modify: `internal/charconfig/charconfig.go` (append)
- Test: `internal/charconfig/charconfig_test.go` (append)

- [ ] **Step 1: Create `classes/assassin.toml`**

```toml
# Class-intrinsic measured constants (uniform schema across classes; see
# docs/design-plan2.md §4 + backlog §10). Every class file defines the same
# fields — a missing field is a hard error (the sim is incomplete without it).
auto_attack_multiplier = 2.0  # innate Assassin auto-attack multiplier (measured /weaponstat 2026-06-13; Enchanter ≈0.7 for reference)
```

- [ ] **Step 2: Write the failing tests** (append to `internal/charconfig/charconfig_test.go`)

```go
func TestLoadClassAssassin(t *testing.T) {
	cd, err := LoadClass("../../classes", "assassin")
	require.NoError(t, err)
	require.InDelta(t, 2.0, cd.AutoAttackMultiplier, 1e-9)
}

func TestLoadClassMissingFile(t *testing.T) {
	_, err := LoadClass("../../classes", "wizard")
	require.Error(t, err)
}

func TestLoadClassRejectsMissingMultiplier(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "x.toml"), []byte("# empty\n"), 0o644))
	_, err := LoadClass(dir, "x")
	require.ErrorContains(t, err, "auto_attack_multiplier")
}

func TestLoadClassRejectsUnknownKey(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "x.toml"),
		[]byte("auto_attack_multiplier = 2.0\nbogus = 1\n"), 0o644))
	_, err := LoadClass(dir, "x")
	require.ErrorContains(t, err, "bogus")
}
```

(`os` and `path/filepath` are already imported in this test file from the existing `writeConfig` helper — confirm; add if missing.)

- [ ] **Step 3: Run to verify failure**

Run: `go test ./internal/charconfig/ -run TestLoadClass -v`
Expected: FAIL — `undefined: LoadClass`.

- [ ] **Step 4: Implement** (append to `internal/charconfig/charconfig.go`)

```go
// ClassData holds class-intrinsic constants (docs/design-plan2.md §4): values
// identical for every character of a class but differing between classes.
// Uniform schema — every classes/<class>.toml defines the same fields.
type ClassData struct {
	AutoAttackMultiplier float64 `toml:"auto_attack_multiplier"`
}

// LoadClass reads classes/<class>.toml. Strict: unknown keys and a missing or
// non-positive auto_attack_multiplier are errors — the sim is incomplete
// without these class constants.
func LoadClass(dir, class string) (ClassData, error) {
	path := filepath.Join(dir, class+".toml")
	var cd ClassData
	md, err := toml.DecodeFile(path, &cd)
	if err != nil {
		return ClassData{}, err
	}
	if undec := md.Undecoded(); len(undec) > 0 {
		return ClassData{}, fmt.Errorf("%s: unknown class keys: %v", path, undec)
	}
	if cd.AutoAttackMultiplier <= 0 {
		return ClassData{}, fmt.Errorf("%s: auto_attack_multiplier must be > 0 (got %v)", path, cd.AutoAttackMultiplier)
	}
	return cd, nil
}
```

Add `"path/filepath"` to `charconfig.go`'s imports if not present.

- [ ] **Step 5: Run to verify pass**

Run: `go test ./internal/charconfig/ -v` then `make lint`
Expected: all PASS, lint clean.

- [ ] **Step 6: Commit**

```bash
git add classes/assassin.toml internal/charconfig/charconfig.go internal/charconfig/charconfig_test.go
git commit -m "Class data: classes/assassin.toml + LoadClass loader (strict auto_attack_multiplier)"
```

---

### Task 2: Auto-attack equation + wiring (model + bis + cmd — ONE commit, compile boundary)

Changing the model signatures breaks `internal/bis` and `cmd` until they're updated, so this is a single commit.

**Files:**
- Modify: `internal/model/dps.go`, `internal/model/itemdelta.go`
- Modify: `internal/bis/set.go`, `internal/bis/report.go`, `internal/bis/build.go`
- Modify: `cmd/weights/main.go`, `cmd/bis/main.go`
- Test: `internal/model/dps_test.go`, `internal/model/itemdelta_test.go`, `internal/model/timeline_test.go`, `internal/bis/set_test.go`, `internal/bis/build_test.go`

#### Model

- [ ] **Step 1: Update model tests first**

In `internal/model/dps_test.go`:
- `TestAutoDPS` — unchanged (AutoDPS keeps its 2-arg signature; MainStat 0 → AGI factor 1.0).
- `TestAutoDPSDual`, `TestAutoDPSDualNoOffhandUnpenalized` — add a `, 1.0` class-mult arg to every `AutoDPSDual(...)` call; expected values unchanged (1.0 is identity).
- Add a new test asserting the class multiplier and AGI both apply:

```go
func TestAutoDPSClassMultAndAGI(t *testing.T) {
	main := Weapon{AvgDamage: 100, DelaySecs: 2.0}
	off := Weapon{AvgDamage: 60, DelaySecs: 3.0}
	// classMult 2.0 doubles the auto sum vs 1.0.
	base := AutoDPSDual(StatBlock{}, main, off, 1.0)
	require.InDelta(t, 2.0*base, AutoDPSDual(StatBlock{}, main, off, 2.0), 1e-9)
	// AGI scales a single weapon: MainStat 625 → +51.74% per swing.
	require.InDelta(t, 1.5174, AutoDPS(StatBlock{MainStat: 625}, main)/AutoDPS(StatBlock{}, main), 1e-3)
}

func TestAutoWeaponMultiplierCalibration(t *testing.T) {
	// /weaponstat 2026-06-13, Blood Fire census max 290.
	// dps-mod 0, AGI MainStat 625 (curve→51.74%), classMult 2.0 → actual max 882.
	m0 := AutoWeaponMultiplier(StatBlock{MainStat: 625, DPSMod: 0}, 2.0)
	require.InDelta(t, 882.0, 290*m0, 6) // 290×1.5174×1.0×2.0 = 880.1
	// dps-mod stat 73.2 (curve→51%), AGI MainStat 983 (→64.06%), classMult 2.0 → actual max 1442.
	m1 := AutoWeaponMultiplier(StatBlock{MainStat: 983, DPSMod: 73.2}, 2.0)
	require.InDelta(t, 1442.0, 290*m1, 8) // 290×1.6406×1.51×2.0 = 1436.7
}
```

In `internal/model/itemdelta_test.go`: add a `, 1.0` class-mult arg to every `ItemDelta(...)` call; expected values/inequalities unchanged.

In `internal/model/timeline_test.go`: if it calls any changed signature (`TotalDPS`/`TotalDPSDual`/`AutoDPSDual`/`ItemDelta`), add `, 1.0`. (It mainly exercises `RotationCADPS`/`CADPS`, which are unchanged — update only what the compiler flags.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/model/ 2>&1 | head`
Expected: compile errors (`AutoWeaponMultiplier` undefined; arg-count mismatches) — the expected red state.

- [ ] **Step 3: Implement model changes**

`internal/model/dps.go` — add the AGI/dps-mod helper, fold it into `AutoDPS`, and thread the class multiplier:

```go
// autoDamageMult is the per-swing damage multiplier from the wielder's stats:
// main-stat (AGI, same curve as CAs) × dps-mod. (The class auto multiplier is
// applied separately at the AutoDPSDual/TotalDPS boundary.)
func autoDamageMult(sb StatBlock) float64 {
	return (1 + MainStatEffect(sb.MainStat)/100) * dpsModFactor(sb)
}

// AutoWeaponMultiplier is the full multiplier on census-base per-swing damage:
// main-stat × dps-mod × class auto multiplier. Calibration target for /weaponstat.
func AutoWeaponMultiplier(sb StatBlock, classAutoMult float64) float64 {
	return autoDamageMult(sb) * classAutoMult
}
```

Change `AutoDPS` to use `autoDamageMult` (replaces the bare `dpsModFactor(sb)` and adds AGI):

```go
// AutoDPS models sustained auto-attack damage per second for one weapon. AGI and
// dps-mod scale per-swing damage (autoDamageMult); the class auto multiplier is
// applied by the caller (AutoDPSDual/TotalDPS).
func AutoDPS(sb StatBlock, w Weapon) float64 {
	if w.DelaySecs <= 0 {
		return 0
	}
	swings := w.AvgDamage / effDelay(sb, w)
	return swings * (1 + MultiAttackEffect(sb.MultiAttack)/100) * autoDamageMult(sb) * critFactor(sb) * flurryFactor(sb)
}
```

Thread `classAutoMult` through the auto entry points:

```go
func AutoDPSDual(sb StatBlock, main, off Weapon, classAutoMult float64) float64 {
	if off.DelaySecs > 0 {
		main.DelaySecs *= constants.DualWieldDelayPenalty
		off.DelaySecs *= constants.DualWieldDelayPenalty
	}
	return classAutoMult * (AutoDPS(sb, main) + AutoDPS(sb, off))
}

func TotalDPS(sb StatBlock, w Weapon, cas []spell.CombatArt, classAutoMult float64) float64 {
	return classAutoMult*AutoDPS(sb, w) + CADPS(sb, cas)
}

func TotalDPSDual(sb StatBlock, main, off Weapon, cas []spell.CombatArt, classAutoMult float64) float64 {
	return AutoDPSDual(sb, main, off, classAutoMult) + CADPS(sb, cas)
}
```

Keep the existing `AutoDPSDual` doc comment (off-hand detection) and the `TotalDPSDual` dual-wield-context comment; update them to mention the class multiplier where natural.

`internal/model/itemdelta.go` — add the param and pass it through both `TotalDPSDual` calls:

```go
func ItemDelta(restBase StatBlock, main, restOff Weapon, arts []spell.CombatArt, itemStats StatBlock, newOff *Weapon, classAutoMult float64) float64 {
	before := TotalDPSDual(restBase, main, restOff, arts, classAutoMult)
	off := restOff
	if newOff != nil {
		off = *newOff
	}
	after := TotalDPSDual(restBase.Add(itemStats), main, off, arts, classAutoMult)
	return after - before
}
```

- [ ] **Step 4: Run model tests**

Run: `go test ./internal/model/ -v`
Expected: all PASS (incl. the two new tests). `internal/bis` and `cmd` do NOT compile yet — that's expected; fix them next in this same commit.

#### bis

- [ ] **Step 5: Update bis tests**

In `internal/bis/set_test.go` and `internal/bis/build_test.go`: every `NewSet(profile, lo)` becomes `NewSet(profile, lo, 1.0)` (1.0 = no class multiplier, preserving existing expected values). `TestSetDPSAndCandidateDelta`'s `40.0` expectation stays (single-wield, no off-hand, classMult 1.0 → unchanged).

Add one test that the Set carries the multiplier into DPS:

```go
func TestSetAppliesClassAutoMult(t *testing.T) {
	lo := store.Loadout{Main: model.Weapon{AvgDamage: 160, DelaySecs: 4}}
	base := NewSet(model.StatBlock{}, lo, 1.0).DPS()
	scaled := NewSet(model.StatBlock{}, lo, 2.0).DPS()
	require.InDelta(t, 2.0*base, scaled, 1e-9) // no CAs/arts in fixture → DPS is pure auto
}
```

- [ ] **Step 6: Implement bis changes**

`internal/bis/set.go`:

```go
type Set struct {
	Profile  model.StatBlock
	Main     model.Weapon
	Arts     []spell.CombatArt
	AutoMult float64 // class-intrinsic auto-attack multiplier (classes/<class>.toml)
	Equipped map[string][]store.ScorableItem
}

func NewSet(profile model.StatBlock, lo store.Loadout, autoMult float64) *Set {
	return &Set{Profile: profile, Main: lo.Main, Arts: lo.Arts, AutoMult: autoMult, Equipped: map[string][]store.ScorableItem{}}
}
```

`Set.DPS()` → `model.TotalDPSDual(s.restBase(""), s.Main, s.offWeapon(), s.Arts, s.AutoMult)`.

`Set.CandidateDelta` → both `model.ItemDelta(...)` calls gain `, s.AutoMult` as the final arg.

`internal/bis/report.go` `ConvergedWeights` → the inner `dps` closure: `return model.TotalDPSDual(sb, set.Main, off, set.Arts, set.AutoMult)`.

`internal/bis/build.go:74` → `NewSet(profile, lo, autoMult)`. `BuildSet` must receive `autoMult`: add an `autoMult float64` param to `BuildSet`'s signature and pass it to `NewSet`. (Check `BuildSet`'s callers — `cmd/bis` — and update in Step 7.)

- [ ] **Step 7: Update cmd wiring**

`cmd/bis/main.go`: after loading the character config, load the class data and thread the multiplier:

```go
classData, err := charconfig.LoadClass("classes", cfg.Character.Class)
if err != nil {
	fmt.Fprintln(os.Stderr, "load class data:", err)
	os.Exit(1)
}
```

Pass `classData.AutoAttackMultiplier` into every `bis.BuildSet(...)` call (the three tiers + the locked-items block).

`cmd/weights/main.go`: same `charconfig.LoadClass("classes", cfg.Character.Class)` load, and the `dps` closure becomes:

```go
dps := func(sb model.StatBlock) float64 {
	return model.TotalDPSDual(sb, mainWeapon, offWeapon, cas, classData.AutoAttackMultiplier)
}
```

- [ ] **Step 8: Full gates**

Run: `go test ./... -count=1 && make lint && go build ./...`
Expected: all packages PASS, lint clean, build clean.

- [ ] **Step 9: Commit**

```bash
git add internal/model/ internal/bis/ cmd/weights/ cmd/bis/
git commit -m "Auto-attack: AGI + class auto-multiplier (×2.0) in AutoDPS; class data wired through Set/cmds"
```

---

### Task 3: Regenerate report + confirm the predicted weight swing

**Files:** none (regenerated `bis-report.md` stays untracked)

- [ ] **Step 1: Regenerate**

Run: `go run ./cmd/weights` then `go run ./cmd/bis`
Expected: both succeed; the run prints the loaded class (or no error). `bis-report.md` rewritten.

- [ ] **Step 2: Confirm the prediction**

Adding the ×2.0 innate + AGI roughly triples modeled auto damage, so auto-side stat weights should jump **up** (reversing and exceeding the dual-wield penalty's reduction), and CA-side stats fall **relatively**. Paste the three converged weight tables in the summary and state whether haste/multiattack/flurry/dps-mod rose sharply vs. the pre-change values:
- pre-change haste: 1.12 / 1.41 / 1.41 (PRE-RAID/RAID/BoB)
- pre-change multiattack: 0.72 / 1.11 / 1.11
- pre-change flurry: 3.45 / 5.29 / 5.29
- pre-change dpsmod: 0.66 / 0.49 / 0.49

A clear jump in those confirms the auto term grew as designed. No commit (report untracked).

---

## Self-review notes

- **Spec coverage:** class loader + `classes/assassin.toml` strict ✓ (T1); AGI on auto via `sb.MainStat` ✓ (T2 `autoDamageMult`); class mult NOT in StatBlock, threaded + on Set ✓ (T2); `AutoWeaponMultiplier` calibration helper + test pinned to `/weaponstat` ✓ (T2); dps-mod unchanged / potency not on auto ✓ (untouched); report regen + weight-shift check ✓ (T3).
- **Compile boundary:** model signature changes (`AutoDPSDual`/`TotalDPS`/`TotalDPSDual`/`ItemDelta` gain `classAutoMult`; `BuildSet`/`NewSet` gain `autoMult`) break bis+cmd until updated — all in the Task 2 commit. `AutoDPS` keeps its 2-arg signature (AGI internal), so its many test callers in `weights_test.go` are untouched.
- **Type consistency:** `ClassData.AutoAttackMultiplier`, `LoadClass(dir, class)`, `autoDamageMult(sb)`, `AutoWeaponMultiplier(sb, classAutoMult)`, `Set.AutoMult`, `NewSet(profile, lo, autoMult)`, `BuildSet(..., autoMult)` — names consistent across tasks.
- **Calibration arithmetic verified:** `MainStatEffect(625)=51.74` & `(983)=64.06` (table samples); `HasteDpsModEffect(73.2)=floor(51.76)=51`; `290×1.5174×2.0=880.1≈882` (tol 6); `290×1.6406×1.51×2.0=1436.7≈1442` (tol 8).
- **YAGNI:** class file holds only `auto_attack_multiplier` (the one measured-and-used value); `census_class_id`/`primary_attribute` deferred to the import pass (backlog §10). `TotalDPS` gains the param for signature consistency though it has no production caller yet.
