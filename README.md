# eq2-eof-itemdex

EverQuest 2 — Echoes of Faydwer item index & Assassin best-in-slot analysis, built on the Daybreak Census API for a Varsoon-style Time-Locked Expansion (TLE) server.

## What it does

- **Comprehensive item catalog** — pulls every EoF-era item (all classes) from Census and persists faithful, *untranslated* CSVs by category (weapons / armor / jewelry-charms), plus a cross-class **Max Health** list for tanks.
- **Assassin best-in-slot** — a relative DPS model (auto-attack + combat arts) derives per-stat weights at a buffed raid baseline and ranks gear per slot for an EoF Assassin.
- **CSV-first** — the catalogs double as a local cache; runs read from them by default and only re-query Census with `--refresh`.

## How EoF items are identified

Census has no expansion field, so items are classified by their **first-discovery timestamp on Varsoon (world 614)** falling within the EoF unlock window `[2023-04-11, 2023-08-08)` — content-gated on both ends by expansion-exclusive collectables. See the spec for the full method and validation.

## Status

Design phase. Full design spec: [`docs/design.md`](docs/design.md).

## Stack

Go (stdlib `net/http`, `encoding/json`, `regexp`).

## Data source

Daybreak Census API (`census.daybreakgames.com`), namespace `eq2`, public `s:example` service ID (throttled to 10 requests/min).
