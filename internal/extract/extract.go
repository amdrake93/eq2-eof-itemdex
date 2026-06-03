package extract

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/amdrake93/eq2-eof-itemdex/internal/classify"
)

const (
	showFields   = "displayname,id,tier,itemlevel,gamelink,slot_list,typeinfo,modifiers,_extended.discovered.world_list"
	maxItemLevel = 72
)

// AllEoF pages the Census item set, server-side pruned to Varsoon items under
// the level-70 cap and discovered within the EoF window, then refined
// client-side by classify.IsEoF.
func AllEoF(ctx context.Context, c *census.Client, pageSize int) ([]census.Item, error) {
	return collect(ctx, c, pageSize, classify.EoFStart, classify.EoFEnd, classify.IsEoF)
}

// AllKoS is the optional KoS extension for the max-life list.
func AllKoS(ctx context.Context, c *census.Client, pageSize int) ([]census.Item, error) {
	return collect(ctx, c, pageSize, classify.KoSStart, classify.KoSEnd, classify.IsKoS)
}

// collect pages all items matching the server-side prune (Varsoon world 614, a
// discovery-timestamp window, and an item-level ceiling), keeping those that
// pass the client-side keep predicate. The timestamp window is a recall-safe
// pre-filter: Census array matching can't bind id==614 and the timestamp range
// to the same element, so this window is intentionally loose (it never drops a
// true match) and keep does the precise per-item classification. Operators are
// URL-encoded (%3E=>, %3C=<) because the live API requires it.
func collect(ctx context.Context, c *census.Client, pageSize int, windowStart, windowEnd time.Time, keep func(census.Item) bool) ([]census.Item, error) {
	var out []census.Item
	for start := 0; ; start += pageSize {
		query := fmt.Sprintf(
			"_extended.discovered.world_list.id=%d"+
				"&_extended.discovered.world_list.timestamp=%%3E%d"+
				"&_extended.discovered.world_list.timestamp=%%3C%d"+
				"&itemlevel=%%3C%d&c:show=%s&c:limit=%d&c:start=%d",
			classify.VarsoonWorldID, windowStart.Unix()-1, windowEnd.Unix(), maxItemLevel, showFields, pageSize, start)
		body, err := c.Get(ctx, "get", "item", query)
		if err != nil {
			return nil, err
		}
		page, err := census.DecodeItems(body)
		if err != nil {
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
