package catalog

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"
)

// EffectStat is a single effect-derived modifier keyed to an item.
type EffectStat struct {
	ItemID int
	Stat   string
	Value  float64
}

// ItemProc is a triggered item proc keyed to an item.
type ItemProc struct {
	ItemID    int
	Trigger   string
	PerMinute float64
	DmgType   string
	MinDmg    float64
	MaxDmg    float64
	Raw       string
}

var effectStatHeader = []string{"item_id", "stat", "value"}

// WriteEffectStatsCSV writes rows to w in CSV format with header item_id,stat,value.
func WriteEffectStatsCSV(w io.Writer, rows []EffectStat) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(effectStatHeader); err != nil {
		return err
	}
	for _, r := range rows {
		rec := []string{
			strconv.Itoa(r.ItemID),
			r.Stat,
			strconv.FormatFloat(r.Value, 'g', -1, 64),
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// ReadEffectStatsCSV reads a WriteEffectStatsCSV stream back into a slice.
func ReadEffectStatsCSV(r io.Reader) ([]EffectStat, error) {
	cr := csv.NewReader(r)
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) <= 1 {
		return nil, nil
	}
	out := make([]EffectStat, 0, len(rows)-1)
	for _, row := range rows[1:] {
		if len(row) < 3 {
			continue
		}
		id, err := strconv.Atoi(row[0])
		if err != nil {
			return nil, fmt.Errorf("effect_stats: bad item_id %q: %w", row[0], err)
		}
		v, err := strconv.ParseFloat(row[2], 64)
		if err != nil {
			return nil, fmt.Errorf("effect_stats: bad value %q: %w", row[2], err)
		}
		out = append(out, EffectStat{ItemID: id, Stat: row[1], Value: v})
	}
	return out, nil
}

var itemProcHeader = []string{"item_id", "trigger", "per_minute", "dmg_type", "min_dmg", "max_dmg", "raw"}

// WriteProcsCSV writes rows to w in CSV format with header item_id,trigger,per_minute,dmg_type,min_dmg,max_dmg,raw.
func WriteProcsCSV(w io.Writer, rows []ItemProc) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(itemProcHeader); err != nil {
		return err
	}
	for _, r := range rows {
		rec := []string{
			strconv.Itoa(r.ItemID),
			r.Trigger,
			strconv.FormatFloat(r.PerMinute, 'g', -1, 64),
			r.DmgType,
			strconv.FormatFloat(r.MinDmg, 'g', -1, 64),
			strconv.FormatFloat(r.MaxDmg, 'g', -1, 64),
			r.Raw,
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// ReadProcsCSV reads a WriteProcsCSV stream back into a slice.
func ReadProcsCSV(r io.Reader) ([]ItemProc, error) {
	cr := csv.NewReader(r)
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) <= 1 {
		return nil, nil
	}
	out := make([]ItemProc, 0, len(rows)-1)
	for _, row := range rows[1:] {
		if len(row) < 7 {
			continue
		}
		id, err := strconv.Atoi(row[0])
		if err != nil {
			return nil, fmt.Errorf("procs: bad item_id %q: %w", row[0], err)
		}
		perMin, err := strconv.ParseFloat(row[2], 64)
		if err != nil {
			return nil, fmt.Errorf("procs: bad per_minute %q: %w", row[2], err)
		}
		minDmg, err := strconv.ParseFloat(row[4], 64)
		if err != nil {
			return nil, fmt.Errorf("procs: bad min_dmg %q: %w", row[4], err)
		}
		maxDmg, err := strconv.ParseFloat(row[5], 64)
		if err != nil {
			return nil, fmt.Errorf("procs: bad max_dmg %q: %w", row[5], err)
		}
		out = append(out, ItemProc{
			ItemID:    id,
			Trigger:   row[1],
			PerMinute: perMin,
			DmgType:   row[3],
			MinDmg:    minDmg,
			MaxDmg:    maxDmg,
			Raw:       row[6],
		})
	}
	return out, nil
}

var auditCSVHeader = []string{"item_id", "kind", "detail", "description"}

// WriteAuditCSV writes the audit report to w in CSV format (round-trippable with
// ReadAuditCSV), sorted by item ID for determinism.
func WriteAuditCSV(w io.Writer, report map[int][]AuditLine) error {
	ids := make([]int, 0, len(report))
	for id := range report {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	cw := csv.NewWriter(w)
	if err := cw.Write(auditCSVHeader); err != nil {
		return err
	}
	for _, id := range ids {
		for _, line := range report[id] {
			rec := []string{strconv.Itoa(id), line.Kind, line.Detail, line.Description}
			if err := cw.Write(rec); err != nil {
				return err
			}
		}
	}
	cw.Flush()
	return cw.Error()
}

// ReadAuditCSV reads a WriteAuditCSV stream back into a report grouped by item ID.
func ReadAuditCSV(r io.Reader) (map[int][]AuditLine, error) {
	cr := csv.NewReader(r)
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) <= 1 {
		return map[int][]AuditLine{}, nil
	}
	out := map[int][]AuditLine{}
	for _, row := range rows[1:] {
		if len(row) < 4 {
			continue
		}
		id, err := strconv.Atoi(row[0])
		if err != nil {
			return nil, fmt.Errorf("audit: bad item_id %q: %w", row[0], err)
		}
		out[id] = append(out[id], AuditLine{Kind: row[1], Detail: row[2], Description: row[3]})
	}
	return out, nil
}

// WriteAuditReport writes a markdown audit report to w.
// Rows are sorted by item ID for determinism.
func WriteAuditReport(w io.Writer, report map[int][]AuditLine) error {
	ids := make([]int, 0, len(report))
	for id := range report {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	if _, err := fmt.Fprintln(w, "# Effect Parser Audit Report"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "| item_id | kind | detail | wording |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "| --- | --- | --- | --- |"); err != nil {
		return err
	}
	for _, id := range ids {
		for _, line := range report[id] {
			_, err := fmt.Fprintf(w, "| %d | %s | %s | %s |\n", id, line.Kind, line.Detail, line.Description)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
