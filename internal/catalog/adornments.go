package catalog

import (
	"encoding/csv"
	"io"
	"sort"
	"strconv"
)

// Adornment is one cataloged adornment: id, name, and its census-keyed stat grants.
type Adornment struct {
	ID    int64
	Name  string
	Stats map[string]float64
}

// MergeAdornments appends incoming adornments whose id is not already present in
// existing, returning the merged slice and the count newly added. Existing rows
// are kept as-is (incoming duplicates do not overwrite).
func MergeAdornments(existing, incoming []Adornment) (merged []Adornment, added int) {
	known := make(map[int64]bool, len(existing))
	for _, a := range existing {
		known[a.ID] = true
	}

	merged = append(merged, existing...)
	for _, a := range incoming {
		if known[a.ID] {
			continue
		}
		known[a.ID] = true
		merged = append(merged, a)
		added++
	}
	return merged, added
}

// WriteAdornmentsCSV writes a wide CSV: fixed (id,name) + sorted union of stat keys.
func WriteAdornmentsCSV(w io.Writer, rows []Adornment) error {
	statKeys := map[string]bool{}
	for _, r := range rows {
		for k := range r.Stats {
			statKeys[k] = true
		}
	}
	var keys []string
	for k := range statKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	cw := csv.NewWriter(w)
	header := append([]string{"id", "name"}, keys...)
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, r := range rows {
		rec := []string{strconv.FormatInt(r.ID, 10), r.Name}
		for _, k := range keys {
			if v, ok := r.Stats[k]; ok {
				rec = append(rec, strconv.FormatFloat(v, 'g', -1, 64))
			} else {
				rec = append(rec, "")
			}
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// ReadAdornmentsCSV reverses WriteAdornmentsCSV.
func ReadAdornmentsCSV(r io.Reader) ([]Adornment, error) {
	cr := csv.NewReader(r)
	recs, err := cr.ReadAll()
	if err != nil || len(recs) == 0 {
		return nil, err
	}
	header := recs[0]
	var out []Adornment
	for _, rec := range recs[1:] {
		id, _ := strconv.ParseInt(rec[0], 10, 64)
		a := Adornment{ID: id, Name: rec[1], Stats: map[string]float64{}}
		for i := 2; i < len(header); i++ {
			if rec[i] == "" {
				continue
			}
			a.Stats[header[i]] = atof(rec[i])
		}
		out = append(out, a)
	}
	return out, nil
}
