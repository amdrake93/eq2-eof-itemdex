package extract

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/amdrake93/eq2-eof-itemdex/internal/classify"
)

const maxItemLevel = 72

// ErrQuotaExceeded is returned (wrapped) when Census cuts off a session due to
// the s:example service-ID quota. The caller may treat partial results as valid
// and re-run with --refresh to collect more pages.
var ErrQuotaExceeded = errors.New("census quota exceeded")

// AllEoF pages the Census item set starting from offset 0, server-side pruned
// to Varsoon items under the level-70 cap and discovered within the EoF window,
// then refined client-side by classify.IsEoF.
func AllEoF(ctx context.Context, c *census.Client, pageSize int) ([]census.Item, error) {
	return AllEoFFrom(ctx, c, pageSize, 0)
}

// AllEoFFrom is like AllEoF but begins paging at the given Census item offset,
// enabling incremental pulls when s:example quota cuts a session short.
func AllEoFFrom(ctx context.Context, c *census.Client, pageSize, startOffset int) ([]census.Item, error) {
	return collect(ctx, c, pageSize, startOffset, classify.EoFStart, classify.EoFEnd, classify.IsEoF)
}

// AllKoS is the optional KoS extension for the max-life list.
func AllKoS(ctx context.Context, c *census.Client, pageSize int) ([]census.Item, error) {
	return collect(ctx, c, pageSize, 0, classify.KoSStart, classify.KoSEnd, classify.IsKoS)
}

// PartialError carries the items collected before Census cut off the session,
// plus the Census page offset at which the session ended. Callers can resume
// from NextOffset on the next --refresh run to accumulate more pages.
type PartialError struct {
	Items      []census.Item
	NextOffset int
	Cause      error
}

func (e *PartialError) Error() string {
	return fmt.Sprintf("%v (partial: %d items, next offset: %d)", e.Cause, len(e.Items), e.NextOffset)
}

func (e *PartialError) Unwrap() error { return e.Cause }

// isQuotaError reports whether an error is the s:example session-quota cutoff
// rather than a real API failure. When the quota is exhausted, Census returns
// {"error":"Missing Service ID. A valid Service ID is required for continued
// api use..."}, which DecodeItems surfaces as a "census error: Missing Service
// ID" message — so we match on that sentinel substring.
func isQuotaError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Missing Service ID")
}

// collect pages all items matching the server-side prune (Varsoon world 614, a
// discovery-timestamp window, and an item-level ceiling), keeping those that
// pass the client-side keep predicate. The timestamp window is a recall-safe
// pre-filter: Census array matching can't bind id==614 and the timestamp range
// to the same element, so this window is intentionally loose (it never drops a
// true match) and keep does the precise per-item classification. Operators are
// URL-encoded (%3E=>, %3C=<) because the live API requires it.
//
// If Census returns a quota error mid-run, collect returns a *PartialError
// wrapping ErrQuotaExceeded along with the items collected so far and the next
// page offset so the caller can resume in a future session.
func collect(ctx context.Context, c *census.Client, pageSize, startOffset int, windowStart, windowEnd time.Time, keep func(census.Item) bool) ([]census.Item, error) {
	var out []census.Item
	for start := startOffset; ; start += pageSize {
		query := fmt.Sprintf(
			"_extended.discovered.world_list.id=%d"+
				"&_extended.discovered.world_list.timestamp=%%3E%d"+
				"&_extended.discovered.world_list.timestamp=%%3C%d"+
				"&itemlevel=%%3C%d&c:show=%s&c:limit=%d&c:start=%d",
			classify.VarsoonWorldID, windowStart.Unix()-1, windowEnd.Unix(), maxItemLevel, census.ItemShowFields, pageSize, start)
		body, err := c.Get(ctx, "get", "item", query)
		if err != nil {
			if isQuotaError(err) {
				return nil, &PartialError{Items: out, NextOffset: start, Cause: ErrQuotaExceeded}
			}
			return nil, err
		}
		page, err := census.DecodeItems(body)
		if err != nil {
			if isQuotaError(err) {
				return nil, &PartialError{Items: out, NextOffset: start, Cause: ErrQuotaExceeded}
			}
			return nil, err
		}
		for _, it := range page {
			if keep(it) {
				out = append(out, it)
			}
		}
		slog.Info("fetched census page", "start", start, "got", len(page), "kept", len(out))
		if len(page) < pageSize {
			break
		}
	}
	return out, nil
}
