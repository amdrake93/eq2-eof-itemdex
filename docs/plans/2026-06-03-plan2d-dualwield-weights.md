# Plan 2d — Dual-Wield Auto Model & Realistic Weight Derivation

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans. Steps use checkbox (`- [ ]`).

**Goal:** Make the auto-attack model dual-wield and derive stat weights against **real weapon damage** (Soulfire main-hand + a representative fabled off-hand), so the multiplicative auto-stats (haste / DPS-mod / flurry) get fair weights and the auto-vs-CA balance is realistic.

**Architecture:** Add `AutoDPSDual` (sum of both weapons, equal — the off-hand penalty is "no weapon-multiplier benefit," a stat we don't model, so main/off are equal here). Generalize `DeriveWeights` to take a DPS closure (decouples weights from the weapon/CA specifics and sets up Plan 2e's per-set iteration). The `weights` command builds the closure from the **actual Soulfire + a fabled off-hand pulled from `bis.db`**.

**Tech Stack:** Go 1.26, pure. Reuses `internal/model`, `internal/spell`, `internal/store`/`bis.db`. Module `github.com/amdrake93/eq2-eof-itemdex`.

This is **Plan 2d**; **Plan 2e** (final) adds item scoring + the per-slot top-3-Fabled/Legendary report + locked-items + per-set weight iteration. Decisions captured this session:
- **Multiplicative auto-stats demand real weapon damage in the derivation** — a 100-avg placeholder unfairly crushed haste/dps-mod/flurry vs CAs hitting thousands.
- **Dual-wield = main + off, equal** (off-hand penalty is a weapon-multiplier stat we don't model → omitted from both).
- **Weapons are solved:** main = Soulfire (Mythical), off = a generic fabled 4s 1H — so the weapon base is **pinned** for derivation; only armor/jewelry will iterate (Plan 2e).

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/model/dps.go` (modify) | add `AutoDPSDual(sb, main, off Weapon)` |
| `internal/model/weights.go` (modify) | `DeriveWeights(base, dps func(StatBlock) float64)` — closure form |
| `cmd/weights/main.go` (modify) | build dual-wield + CA DPS closure from real Soulfire + fabled off-hand in `bis.db`; re-derive |

---

## Task 1: Dual-wield auto-attack

**Files:**
- Modify: `internal/model/dps.go`
- Test: `internal/model/dps_test.go`

- [ ] **Step 1: failing test** (append to `dps_test.go`)

```go
func TestAutoDPSDual(t *testing.T) {
	main := Weapon{AvgDamage: 100, DelaySecs: 2.0} // 50 dps
	off := Weapon{AvgDamage: 60, DelaySecs: 3.0}   // 20 dps
	// No stats: 50 + 20 = 70.
	approx(t, 70.0, AutoDPSDual(StatBlock{}, main, off))
	// Equal to summing the two single-weapon calls.
	approx(t, AutoDPS(StatBlock{Haste: 50}, main)+AutoDPS(StatBlock{Haste: 50}, off),
		AutoDPSDual(StatBlock{Haste: 50}, main, off))
}
```

- [ ] **Step 2: run, verify FAIL**: `go test ./internal/model/ -run TestAutoDPSDual -v` → undefined.

- [ ] **Step 3: implement** (add to `dps.go`)

```go
// AutoDPSDual is dual-wield auto-attack: both weapons swing on their own delay.
// Main and off-hand are treated equally — the off-hand's only penalty in EoF is
// not benefiting from the weapon-multiplier stat, which this model doesn't track,
// so it nets out the same for relative comparison.
func AutoDPSDual(sb StatBlock, main, off Weapon) float64 {
	return AutoDPS(sb, main) + AutoDPS(sb, off)
}
```

- [ ] **Step 4: run, verify PASS**: `go test ./internal/model/ -v` → PASS.
- [ ] **Step 5: commit**

```bash
git add internal/model/dps.go internal/model/dps_test.go
git commit -m "feat: dual-wield auto-attack (main + off-hand)"
```

---

## Task 2: Generalize `DeriveWeights` to a DPS closure

**Files:**
- Modify: `internal/model/weights.go`
- Test: `internal/model/weights_test.go`

`DeriveWeights` currently hardcodes `TotalDPS(base, w, cas)` (single weapon). Generalize it to take a `func(StatBlock) float64` so the caller binds the full loadout (dual-wield weapons + CAs) — and so Plan 2e can re-bind it per candidate set during iteration.

- [ ] **Step 1: update the test** (replace the existing `TestDeriveWeights` and `TestDPSModWeightZeroAtCap` to the closure form)

```go
func TestDeriveWeights(t *testing.T) {
	w := Weapon{AvgDamage: 100, DelaySecs: 2.0}
	cas := []spell.CombatArt{{Name: "X", MinDamage: 800, MaxDamage: 1200, RecastSecs: 10}}
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, w) + CADPS(sb, cas) }
	weights := DeriveWeights(StatBlock{}, dps)
	for _, k := range WeightStats {
		_, ok := weights[k]
		require.True(t, ok, k)
	}
	require.Greater(t, weights["critchance"], 0.0)
	require.Greater(t, weights["dpsmod"], 0.0)
}

func TestDPSModWeightZeroAtCap(t *testing.T) {
	w := Weapon{AvgDamage: 100, DelaySecs: 2.0}
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, w) }
	weights := DeriveWeights(StatBlock{DPSMod: 200}, dps)
	require.InDelta(t, 0.0, weights["dpsmod"], 1e-6)
}
```

- [ ] **Step 2: run, verify FAIL**: `go test ./internal/model/ -run TestDerive -v` → signature mismatch (DeriveWeights takes (base, Weapon, cas), test now passes (base, func)).

- [ ] **Step 3: change `DeriveWeights` signature in `weights.go`**

```go
// DeriveWeights returns the marginal DPS per +1 unit of each stat at the given
// baseline, where dps computes total DPS for a stat block (caller binds the
// loadout: dual-wield weapons + combat arts). Saturated stats yield ~0.
func DeriveWeights(base StatBlock, dps func(StatBlock) float64) map[string]float64 {
	d0 := dps(base)
	out := make(map[string]float64, len(WeightStats))
	for _, s := range WeightStats {
		out[s] = (dps(bump(base, s, epsilon)) - d0) / epsilon
	}
	return out
}
```
(`bump`, `WeightStats`, `epsilon` unchanged. The `spell` import in `weights.go` may now be unused — remove it if so; the test still imports `spell` for its closures.)

- [ ] **Step 4: run, verify PASS**: `go test ./internal/model/ -v` → PASS.
- [ ] **Step 5: `make lint`; commit**

```bash
git add internal/model/weights.go internal/model/weights_test.go
git commit -m "refactor: DeriveWeights takes a DPS closure (enables dual-wield + iteration)"
```

---

## Task 3: Real-weapon dual-wield weight derivation in the command

**Files:**
- Modify: `cmd/weights/main.go`

Build the DPS closure from **actual weapon damage**: Soulfire (Mythical) main-hand + a representative fabled 4s 1H off-hand, both from `bis.db`. This is the fix — multiplicative auto-stats now scale a realistic auto-attack base.

- [ ] **Step 1: add weapon loading to `cmd/weights/main.go`**

Add a helper that reads a weapon row into `model.Weapon` (`AvgDamage = (min+max)/2`, `DelaySecs = delay`):
```go
func loadWeapon(db *sql.DB, query string, args ...any) (model.Weapon, string, error) {
	var name string
	var mn, mx, delay float64
	err := db.QueryRow(query, args...).Scan(&name, &mn, &mx, &delay)
	if err != nil {
		return model.Weapon{}, "", err
	}
	return model.Weapon{AvgDamage: (mn + mx) / 2, DelaySecs: delay}, name, nil
}
```
Then in `main`, after opening the DB (refactor `loadCAs` to share the `*sql.DB`, or open once):
```go
	// Main-hand: the Assassin's Soulfire (Mythical), 1H.
	main, mainName, err := loadWeapon(db,
		`SELECT name, weapon_min_dmg, weapon_max_dmg, delay FROM items
		 WHERE name LIKE 'Soulfire%' AND wieldstyle='One-Handed' AND classes LIKE '%assassin%'
		 ORDER BY weapon_max_dmg DESC LIMIT 1`)
	if err != nil { /* fmt.Fprintln(os.Stderr, ...); os.Exit(1) */ }

	// Off-hand: a representative fabled 4s 1H (median-ish; any fabled 1H is in the same band).
	off, offName, err := loadWeapon(db,
		`SELECT name, weapon_min_dmg, weapon_max_dmg, delay FROM items
		 WHERE tier='FABLED' AND wieldstyle='One-Handed' AND classes LIKE '%assassin%'
		   AND skill IN ('piercing','slashing') AND delay BETWEEN 3.5 AND 4.5
		 ORDER BY weapon_max_dmg DESC LIMIT 1 OFFSET 10`) // ~representative, not the absolute top
	if err != nil { /* handle */ }
	fmt.Printf("main-hand: %s (avg %.0f / %.1fs)   off-hand: %s (avg %.0f / %.1fs)\n",
		mainName, main.AvgDamage, main.DelaySecs, offName, off.AvgDamage, off.DelaySecs)
```
(If the Soulfire query returns no row — e.g. its `wieldstyle`/`skill` differs — relax it to `name LIKE 'Soulfire%' AND classes LIKE '%assassin%'` and report what it found. If the off-hand `OFFSET 10` overshoots a small result set, drop the OFFSET. Report the actual weapons chosen.)

- [ ] **Step 2: build the dual-wield + CA closure per baseline and derive**

Replace the single-weapon `ref`/`DeriveWeights(b.sb, ref, cas)` with:
```go
	for _, b := range []struct {
		name string
		sb   model.StatBlock
	}{{"SOLO", baseline.Solo}, {"RAID", baseline.Raid}} {
		dps := func(sb model.StatBlock) float64 {
			return model.AutoDPSDual(sb, main, off) + model.CADPS(sb, cas)
		}
		fmt.Printf("\n== %s baseline weights (dual-wield real weapons, %d arts) ==\n", b.name, len(cas))
		ws := model.DeriveWeights(b.sb, dps)
		// ... existing sort + print ...
	}
```

- [ ] **Step 3: build + lint**: `make build && make lint` → exit 0.

- [ ] **Step 4: run and CAPTURE OUTPUT**:

Run: `go run ./cmd/weights` (bis.db exists; else `go run ./cmd/builddb` first).
PASTE the full output: the chosen main/off weapons + both baselines' weight tables. **This is the re-validation milestone** — vs Plan 2c's weights, expect haste/MA/flurry to rise substantially (real weapon damage × dual-wield), the auto-vs-CA gap to narrow toward something realistic, reuse still strong. Sanity-read against experience.

- [ ] **Step 5: commit**

```bash
git add cmd/weights/main.go
git commit -m "feat: derive weights from real dual-wield weapons (Soulfire + fabled off-hand)"
```

---

## Self-Review

**Coverage of this session's decisions:**
- Dual-wield auto (main+off, equal; off-hand penalty omitted as un-modeled) → Task 1.
- Real weapon damage in derivation (the multiplicative-fairness fix) → Task 3.
- DPS-closure generalization (sets up Plan 2e iteration) → Task 2.
- **Deferred to Plan 2e:** item scoring (`Σ weight×stat` + breakdowns), `scores` table, per-slot top-3-Fabled/Legendary report, locked-items re-model, per-candidate-set weight iteration.

**Placeholder scan:** none — the weapon-selection SQL has explicit fallbacks (relax Soulfire filter / drop OFFSET) and a "report what you chose" instruction, not a hand-wave. The off-hand "representative fabled 1H" is a deliberate, documented choice (weapons are solved; any fabled 1H is in-band).

**Type consistency:** `model.Weapon{AvgDamage,DelaySecs}` built consistently from `(min+max)/2`,`delay`. `AutoDPSDual(sb, main, off)` defined in Task 1, used in Task 3. `DeriveWeights(base, func(StatBlock) float64)` new signature (Task 2) matches the Task 3 call. `CADPS(sb, cas)` unchanged. `WeightStats`/`bump`/`epsilon` untouched.

**Note (weight magnitudes):** with real weapon damage, absolute weight numbers will be larger/different from 2c — that's expected and fine; only the *relative* balance matters. The numbers aren't comparable across weapon setups (which is exactly why a placeholder was wrong).
