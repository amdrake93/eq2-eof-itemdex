package extract

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/time/rate"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
)

func rateInf() *rate.Limiter { return rate.NewLimiter(rate.Inf, 1) }

func TestExtractPagesUntilShort(t *testing.T) {
	pages := []string{
		`{"item_list":[
		  {"id":1,"itemlevel":70,"_extended":{"discovered":{"world_list":[{"id":614,"timestamp":1686700800}]}}},
		  {"id":2,"itemlevel":69,"_extended":{"discovered":{"world_list":[{"id":614,"timestamp":1670889600}]}}}
		],"returned":2}`,
		`{"item_list":[
		  {"id":3,"itemlevel":70,"_extended":{"discovered":{"world_list":[{"id":614,"timestamp":1681171200}]}}}
		],"returned":1}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := r.URL.Query().Get("c:start")
		var body string
		if start == "" || start == "0" {
			body = pages[0]
		} else {
			body = pages[1]
		}
		if _, err := fmt.Fprint(w, body); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := census.New("s:example")
	c.BaseURL = srv.URL
	// disable throttle for the test:
	c.Limiter = rateInf()

	items, err := AllEoF(context.Background(), c, 2)
	if err != nil {
		t.Fatalf("AllEoF: %v", err)
	}
	// Items 1 and 3 are in-window EoF; item 2 (KoS) is filtered out.
	if len(items) != 2 {
		t.Fatalf("expected 2 EoF items, got %d: %+v", len(items), items)
	}
}
