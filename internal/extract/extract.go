package extract

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/amdrake93/eq2-eof-itemdex/internal/classify"
)

const showFields = "displayname,id,tier,itemlevel,gamelink,slot_list,typeinfo,modifiers,_extended.discovered.world_list"

// AllEoF pages the full Census item set (server-side pruned to Varsoon items
// under the level-70 cap), keeping only items in the EoF discovery window.
func AllEoF(ctx context.Context, c *census.Client, pageSize int) ([]census.Item, error) {
	return collect(ctx, c, pageSize, classify.IsEoF)
}

// AllKoS is the optional KoS extension for the max-life list.
func AllKoS(ctx context.Context, c *census.Client, pageSize int) ([]census.Item, error) {
	return collect(ctx, c, pageSize, classify.IsKoS)
}

func collect(ctx context.Context, c *census.Client, pageSize int, keep func(census.Item) bool) ([]census.Item, error) {
	var out []census.Item
	for start := 0; ; start += pageSize {
		query := fmt.Sprintf(
			"_extended.discovered.world_list.id=614&itemlevel=<72&c:show=%s&c:limit=%d&c:start=%d",
			showFields, pageSize, start)
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
