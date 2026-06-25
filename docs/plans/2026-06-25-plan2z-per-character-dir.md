# Per-Character Directory Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Scope every character's files to one directory `characters/<census_name lowercased>/` — committed `config.toml` plus generated `loadout.toml`, `upgrade-report.md`, `bis-report.md` — so multiple characters coexist without clobbering.

**Architecture:** Move the committed config into a per-character dir (`characters/alex.toml` → `characters/biffels/config.toml`). All generated outputs **co-locate with the config's directory**: each command derives its output dir from the `--character`/`--loadout` file path via `filepath.Dir`, so nothing re-derives a name. Update flag defaults, `.gitignore`, and the charconfig tests.

**Tech Stack:** Go 1.26 (module `github.com/amdrake93/eq2-eof-itemdex`), testify/require. Build `go build ./...`; test `go test ./...`. Branch `character-gear-import-spec`. Spec: `docs/SPEC.md` §6 "Per-character directory".

---

## File Structure / touchpoints

| File:line | Change |
|---|---|
| `characters/alex.toml` | `git mv` → `characters/biffels/config.toml` |
| `.gitignore:25-27` | per-character generated patterns |
| `internal/charconfig/charconfig_test.go:29,164` | load `../../characters/biffels/config.toml` |
| `cmd/itemdex/main.go:144,228` | `--character` default; `outPath = filepath.Dir(config)/loadout.toml` |
| `cmd/bis/main.go:178,181,120,293` | `--character` default; `--out` default `""` + computed co-located path |
| `cmd/weights/main.go:51` | `--character` default |

Run sequentially. Task 1 leaves build/tests green (the config tests load the new path; flag defaults are unused by tests). Tasks 2–3 are independent cmd path changes. Task 4 verifies e2e.

---

## Task 1: Migrate config + gitignore + tests

**Files:**
- Move: `characters/alex.toml` → `characters/biffels/config.toml`
- Modify: `.gitignore`, `internal/charconfig/charconfig_test.go`

- [ ] **Step 1: Move the committed config**

```bash
cd /Users/Alex/repos/eq2-eof-itemdex
mkdir -p characters/biffels
git mv characters/alex.toml characters/biffels/config.toml
```

- [ ] **Step 2: Remove stale local generated artifacts** (untracked/gitignored under old patterns — regenerated later in new locations)

```bash
rm -f characters/biffels-loadout.toml loadout-report.md bis-report.md
```

- [ ] **Step 3: Update `.gitignore`** — replace the three old lines (currently):
```
/characters/*-loadout.toml
/loadout-report.md
/bis-report.md
```
with:
```
/characters/*/loadout.toml
/characters/*/upgrade-report.md
/characters/*/bis-report.md
```
(`config.toml` inside `characters/<name>/` is NOT matched → stays committed.)

- [ ] **Step 4: Update the charconfig test paths**

In `internal/charconfig/charconfig_test.go`, both occurrences of:
```go
	cfg, err := Load("../../characters/alex.toml")
```
become:
```go
	cfg, err := Load("../../characters/biffels/config.toml")
```
(Lines ~29 and ~164. If the enclosing test is named `TestLoadCommittedAlexConfig`, rename it to `TestLoadCommittedConfig` for accuracy — update the `func` name only, no body change.)

- [ ] **Step 5: Verify**

Run: `go build ./...` and `go test ./internal/charconfig/ -v` (the committed-config test now loads the new path → PASS), then full `go test ./...`.
Expected: all pass. (Confirm `git status` shows `characters/biffels/config.toml` staged as a rename and no stray untracked generated files.)

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: move character config to characters/<name>/config.toml; per-character gitignore"
```

---

## Task 2: `itemdex import` writes into the config's directory

**Files:**
- Modify: `cmd/itemdex/main.go`

- [ ] **Step 1: Update the `--character` default** (line ~144 in `runImport`)

```go
	character := fs.String("character", "characters/biffels/config.toml", "character config TOML")
```

- [ ] **Step 2: Co-locate the loadout output with the config**

Replace (line ~228):
```go
	outPath := filepath.Join("characters", strings.ToLower(cfg.Character.CensusName)+"-loadout.toml")
```
with:
```go
	outDir := filepath.Dir(*character)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "error creating output dir:", err)
		os.Exit(1)
	}
	outPath := filepath.Join(outDir, "loadout.toml")
```
(`filepath` and `os` are already imported. The loadout now lands next to the config that produced it — the per-character dir.)

- [ ] **Step 3: Drop a now-unused import if needed**

If `strings` is no longer referenced anywhere in `cmd/itemdex/main.go` after Step 2, remove it from the import block. Run `go build ./...` — a `"strings" imported and not used` error tells you to remove it; if the build is clean, `strings` is still used elsewhere (leave it).

- [ ] **Step 4: Build + vet**

Run: `go build ./...` (green), `go vet ./...` (clean), `go test ./...` (all pass — no behavior tested at cmd level, library unchanged).

- [ ] **Step 5: Commit**

```bash
git add cmd/itemdex/main.go
git commit -m "feat(itemdex): write loadout.toml into the config's per-character directory"
```

---

## Task 3: `bis` + `weights` defaults; report co-location

**Files:**
- Modify: `cmd/bis/main.go`, `cmd/weights/main.go`

- [ ] **Step 1: Update `--character` defaults**

`cmd/bis/main.go` line ~181:
```go
	character := flag.String("character", "characters/biffels/config.toml", "character config (TOML)")
```
`cmd/weights/main.go` line ~51:
```go
	character := flag.String("character", "characters/biffels/config.toml", "character config (TOML)")
```

- [ ] **Step 2: Make `--out` default empty + co-locate** in `cmd/bis/main.go`

Change the flag (line ~178):
```go
	out := flag.String("out", "", "report output path (default: <config-or-loadout-dir>/{bis-report,upgrade-report}.md)")
```
Ensure `"path/filepath"` is in the import block (add it if missing).

In the `--loadout` branch (where it currently calls `runLoadoutReport(... *loadoutPath, *out, *topN, *fight)`), compute the report path first:
```go
	if *loadoutPath != "" {
		reportPath := *out
		if reportPath == "" {
			reportPath = filepath.Join(filepath.Dir(*loadoutPath), "upgrade-report.md")
		}
		runLoadoutReport(classData, lo, raid, items, *loadoutPath, reportPath, *topN, *fight)
		return
	}
```
(Match the existing argument order of `runLoadoutReport`; only the `*out` argument becomes `reportPath`.)

For the from-scratch write (line ~293, currently `os.WriteFile(*out, []byte(bis.Render(reports, *fight)), 0o644)`), compute and use the co-located default. Replace that write block with:
```go
	reportPath := *out
	if reportPath == "" {
		reportPath = filepath.Join(filepath.Dir(*character), "bis-report.md")
	}
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "bis: create report dir:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(reportPath, []byte(bis.Render(reports, *fight)), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write report:", err)
		os.Exit(1)
	}
```
If a stdout summary line afterward references `*out` (e.g. `fmt.Printf("wrote %s ...", *out, ...)`), change it to `reportPath`.

- [ ] **Step 3: Co-locate the loadout report dir** — in `runLoadoutReport`, before its `os.WriteFile(out, ...)` (line ~120), add a `MkdirAll` guard so a custom `--out` dir is created:
```go
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "bis: create report dir:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(out, []byte(md), 0o644); err != nil {
```
(`out` is the `reportPath` passed from main; it already defaults to `<loadout-dir>/upgrade-report.md`.)

- [ ] **Step 4: Build + vet + test**

Run: `go build ./...` (green), `go vet ./...` (clean), `go test ./...` (all pass).

- [ ] **Step 5: Commit**

```bash
git add cmd/bis/main.go cmd/weights/main.go
git commit -m "feat(bis): co-locate reports in the per-character directory; default --character to config.toml"
```

---

## Task 4: End-to-end verification

**Files:** none (verification only).

- [ ] **Step 1: Run the full gear-import loop with the new paths**

```bash
cd /Users/Alex/repos/eq2-eof-itemdex
go run ./cmd/itemdex import --character characters/biffels/config.toml --out data
go run ./cmd/bis --loadout characters/biffels/loadout.toml
go run ./cmd/bis --character characters/biffels/config.toml
ls -la characters/biffels/
```
Expected `ls`: `config.toml` (committed), `loadout.toml`, `upgrade-report.md`, `bis-report.md` — all in `characters/biffels/`. Nothing written to the repo root. The stdout lines from each command should report paths under `characters/biffels/`.

- [ ] **Step 2: Confirm gitignore + suite**

Run: `git status --short` (only `characters/biffels/config.toml` is tracked; `loadout.toml`/`upgrade-report.md`/`bis-report.md` do not appear — they're ignored). Then `go test ./...`, `go build ./...`, `go vet ./...` — all green.

- [ ] **Step 3: No commit** (Task 4 is verification; generated files are gitignored). If a fix was needed, commit it with a `fix(...)` message.

---

## Self-Review

**Spec coverage (§6 Per-character directory + §4/§7/§8 path updates):**
- `characters/<census_name>/` holds config + generated outputs → Task 1 (config move) + Tasks 2–3 (outputs co-located) ✓
- Generated files gitignored, config committed → Task 1 Step 3 ✓
- Path rule: outputs derive from `--character`/`--loadout` dir → Task 2 (`filepath.Dir(*character)`), Task 3 (`filepath.Dir(*loadoutPath)` / `filepath.Dir(*character)`) ✓
- `loadout.toml` / `upgrade-report.md` / `bis-report.md` names → Tasks 2, 3 ✓
- `--out` still overrides → Task 3 keeps the explicit-`--out` path ✓
- Flag defaults point at `characters/biffels/config.toml` → Tasks 2, 3 ✓

**Placeholder scan:** none — concrete code/commands throughout.

**Type/consistency:** `runLoadoutReport` signature unchanged (still `(..., out string, topN int, fight float64)`); Task 3 passes the computed `reportPath` as `out`. No new types. The only removed symbol is the `strings.ToLower(census_name)` loadout-name derivation (Task 2), and the `strings` import is dropped only if unused (guarded by the build error in Task 2 Step 3).

**Migration safety:** Task 1 makes the rename + test-path update atomically (build/tests green after), so the committed config is never missing relative to the tests that load it. Flag defaults are updated in Tasks 2–3 before the Task-4 e2e exercises them.
