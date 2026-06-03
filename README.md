# eq2-eof-itemdex

EverQuest 2 — Echoes of Faydwer item index & Assassin best-in-slot analysis, built on the Daybreak Census API for a Varsoon-style Time-Locked Expansion (TLE) server.

## What it does

- **Comprehensive item catalog** — pulls every EoF-era item (all classes) from Census and persists faithful, *untranslated* CSVs by category (weapons / armor / jewelry-charms), plus a cross-class **Max Health** list for tanks.
- **Assassin best-in-slot** — a relative DPS model (auto-attack + combat arts) derives per-stat weights at a buffed raid baseline and ranks gear per slot for an EoF Assassin.
- **CSV-first** — the catalogs double as a local cache; runs read from them by default and only re-query Census with `--refresh`.

## How EoF items are identified

Census has no expansion field, so items are classified by their **first-discovery timestamp on Varsoon (world 614)** falling within the EoF unlock window `[2023-04-11, 2023-08-08)` — content-gated on both ends by expansion-exclusive collectables. See the spec for the full method and validation.

## Usage

```bash
go run ./cmd/itemdex            # load items (CSV cache if present, else a fresh Census pull)
go run ./cmd/itemdex --refresh  # force a fresh Census pull, rewriting data/*.csv
```

The catalog is **committed in [`data/`](data/)** — clone or pull the repo to use it directly, no run required. Current snapshot (6,629 EoF-era items, all classes):

| file | items | contents |
|---|---|---|
| `data/weapons.csv` | 669 | primary / secondary / ranged |
| `data/armor.csv` | 1,616 | head…feet, with an `armor_type` column (Cloth / Leather / Chain / Plate) |
| `data/jewelry-charms.csv` | 1,212 | neck / ears / wrists / rings / charms / waist / cloak |
| `data/other.csv` | 3,132 | everything else with an EoF-window Varsoon discovery |
| `data/maxlife.csv` | 173 | every EoF item with Max Health, any class (tank reference) |

Stats are exactly as Census reports them — **no translations**.

> **`--refresh` note:** the public `s:example` ID has a small per-session request quota, so a full refresh (~63 pages) runs across several quota windows and resumes automatically (offset tracked in `data/.census_next_offset`). It takes a while; normal runs use the committed cache and need no network.

## Status

Plan 1 (item catalog) is **built** and the `data/` CSVs are populated. Plan 2 (Assassin DPS model & BiS) is next. Spec: [`docs/design.md`](docs/design.md); plans: [`docs/plans/`](docs/plans/).

## Stack

Go 1.26 — `net/http` + `golang.org/x/time/rate` (throttled client), `encoding/json`, `encoding/csv`, `regexp`; `stretchr/testify` for tests; `golangci-lint` + `Makefile`.

## Data source

Daybreak Census API (`census.daybreakgames.com`), namespace `eq2`, public `s:example` service ID (small per-session request quota; the pull resumes across sessions).
