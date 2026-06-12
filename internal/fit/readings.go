// Package fit derives the shared haste/dps-mod conversion curve from in-game
// readings (data/curve-readings.csv). Shown effects are UI-floored integers, so
// each reading's least-squares target is shown+0.5 — the midpoint of its floor
// interval [shown, shown+1).
package fit

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
)

// Reading is one in-game observation: a raw stat total and the effect % the UI
// displayed for it.
type Reading struct {
	Stat   string  // "haste" or "dpsmod"
	Raw    float64 // raw stat total shown by the UI
	Effect float64 // shown effect % (floored integer)
	Era    string  // "varsoon" (original server) or "live" (current TLE)
}

// FitTarget is the reading's least-squares target: the midpoint of [shown, shown+1).
func (r Reading) FitTarget() float64 { return r.Effect + 0.5 }

// LoadReadings parses the readings CSV (header: stat,raw,effect,era).
func LoadReadings(path string) ([]Reading, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("%s: no readings", path)
	}

	out := make([]Reading, 0, len(rows)-1)
	for _, row := range rows[1:] {
		raw, err := strconv.ParseFloat(row[1], 64)
		if err != nil {
			return nil, fmt.Errorf("raw %q: %w", row[1], err)
		}
		effect, err := strconv.ParseFloat(row[2], 64)
		if err != nil {
			return nil, fmt.Errorf("effect %q: %w", row[2], err)
		}
		out = append(out, Reading{Stat: row[0], Raw: raw, Effect: effect, Era: row[3]})
	}
	return out, nil
}

// Filter returns the readings for one stat ("haste" or "dpsmod").
func Filter(rs []Reading, stat string) []Reading {
	out := make([]Reading, 0, len(rs))
	for _, r := range rs {
		if r.Stat == stat {
			out = append(out, r)
		}
	}
	return out
}
