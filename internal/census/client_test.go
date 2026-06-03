package census

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

func TestGetBuildsURLAndReturnsBody(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New("s:example")
	c.BaseURL = srv.URL
	c.Limiter = rate.NewLimiter(rate.Inf, 1)

	body, err := c.Get(context.Background(), "get", "item", "c:limit=1")
	require.NoError(t, err)
	require.Contains(t, string(body), `"ok":true`)
	require.Equal(t, "/s:example/get/eq2/item/", gotPath)
	require.Equal(t, "c:limit=1", gotQuery)
}

func TestGetRetriesOn429(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New("s:example")
	c.BaseURL = srv.URL
	c.Limiter = rate.NewLimiter(rate.Inf, 1)
	c.Backoff = time.Millisecond

	_, err := c.Get(context.Background(), "get", "item", "")
	require.NoError(t, err)
	require.Equal(t, 2, calls)
}
