package census

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

// Client is a throttled Census API client. The public s:example service ID is
// limited to 10 requests/min per IP.
type Client struct {
	BaseURL string
	SID     string
	HTTP    *http.Client
	Limiter *rate.Limiter
	Backoff time.Duration
}

func New(sid string) *Client {
	return &Client{
		BaseURL: "https://census.daybreakgames.com",
		SID:     sid,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
		Limiter: rate.NewLimiter(rate.Every(6*time.Second), 1),
		Backoff: 30 * time.Second,
	}
}

// Get performs GET {BaseURL}/{SID}/{verb}/eq2/{collection}/?{query}.
// It retries once on HTTP 429 or on a network timeout, backing off before retry.
func (c *Client) Get(ctx context.Context, verb, collection, query string) ([]byte, error) {
	url := fmt.Sprintf("%s/%s/%s/eq2/%s/?%s", c.BaseURL, c.SID, verb, collection, query)
	for attempt := 0; attempt < 2; attempt++ {
		if err := c.Limiter.Wait(ctx); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			// Network-level error (timeout, connection reset, etc.) — backoff and retry.
			if attempt == 0 {
				time.Sleep(c.Backoff)
				continue
			}
			return nil, fmt.Errorf("census %s: %w", collection, err)
		}
		body, readErr := io.ReadAll(resp.Body)
		if closeErr := resp.Body.Close(); closeErr != nil && readErr == nil {
			readErr = closeErr
		}
		if err = readErr; err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			time.Sleep(c.Backoff)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("census %s: status %d: %s", collection, resp.StatusCode, body)
		}
		return body, nil
	}
	return nil, fmt.Errorf("census %s: rate limited after retry", collection)
}
