# EoF Item Catalog (itemdex core) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Pull every Echoes-of-Faydwer-era item from the Daybreak Census API and persist a faithful, untranslated CSV catalog (by category + a cross-class Max Health list) that doubles as a local cache.

**Architecture:** A throttled stdlib HTTP client pages the `item` collection (server-side pruned to Varsoon items under the level-70 cap), an EoF classifier keeps items whose Varsoon first-discovery falls in the unlock window, and a CSV writer emits wide-format category files. A data-source layer reads existing CSVs by default and only re-queries Census on `--refresh`.

**Tech Stack:** Go 1.26, stdlib only (`net/http`, `encoding/json`, `regexp`, `encoding/csv`, `testing`, `net/http/httptest`). Module `github.com/amdrake93/eq2-eof-itemdex`.

This is **Plan 1 of 2**. Plan 2 (Assassin DPS model & BiS) builds on the item types and classifier defined here.

---

## File Structure

| File | Responsibility |
|---|---|
| `go.mod` | Module definition |
| `internal/census/client.go` | Throttled HTTP client: build Census URLs, ≤10 req/min, 429 backoff |
| `internal/census/item.go` | Item structs + `item_list` decoding |
| `internal/classify/eof.go` | Varsoon (world 614) EoF-window classifier |
| `internal/catalog/category.go` | Slot→category mapping + armor-type derivation |
| `internal/catalog/csv.go` | Wide-format CSV writer (union of stat columns) + load-back |
| `internal/extract/extract.go` | Paged, pruned EoF item extraction over the client |
| `internal/source/source.go` | Data-source layer: CSV cache vs fresh Census pull |
| `cmd/itemdex/main.go` | CLI entrypoint + flags |

Test files sit beside each (`*_test.go`).

---

## Task 0: Module init

**Files:**
- Create: `go.mod`

- [ ] **Step 1: Initialize the module**

Run: `cd ~/repos/eq2-eof-itemdex && go mod init github.com/amdrake93/eq2-eof-itemdex`
Expected: creates `go.mod` containing `module github.com/amdrake93/eq2-eof-itemdex` and `go 1.26`.

- [ ] **Step 2: Verify it builds (empty)**

Run: `go build ./...`
Expected: no output, exit 0.

- [ ] **Step 3: Commit**

```bash
git add go.mod
git commit -m "chore: go module init"
```

---

## Task 1: Throttled Census client

**Files:**
- Create: `internal/census/client.go`
- Test: `internal/census/client_test.go`

- [ ] **Step 1: Write the failing test**

```go
package census

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGetBuildsURLAndReturnsBody(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New("s:example")
	c.BaseURL = srv.URL
	c.MinInterval = 0

	body, err := c.Get(context.Background(), "get", "item", "c:limit=1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !strings.Contains(string(body), `"ok":true`) {
		t.Fatalf("body = %s", body)
	}
	if gotPath != "/s:example/get/eq2/item/" {
		t.Fatalf("path = %s", gotPath)
	}
	if gotQuery != "c:limit=1" {
		t.Fatalf("query = %s", gotQuery)
	}
}

func TestGetRetriesOn429(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New("s:example")
	c.BaseURL = srv.URL
	c.MinInterval = 0
	c.Backoff = time.Millisecond

	if _, err := c.Get(context.Background(), "get", "item", ""); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls (1 retry), got %d", calls)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/census/ -run TestGet -v`
Expected: FAIL — `undefined: New`.

- [ ] **Step 3: Write minimal implementation**

```go
package census

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Client is a throttled Census API client. The public s:example service ID is
// limited to 10 requests/min per IP, so MinInterval defaults to 6s.
type Client struct {
	BaseURL     string
	SID         string
	HTTP        *http.Client
	MinInterval time.Duration
	Backoff     time.Duration

	mu   sync.Mutex
	last time.Time
}

func New(sid string) *Client {
	return &Client{
		BaseURL:     "https://census.daybreakgames.com",
		SID:         sid,
		HTTP:        &http.Client{Timeout: 30 * time.Second},
		MinInterval: 6 * time.Second,
		Backoff:     30 * time.Second,
	}
}

func (c *Client) throttle() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.MinInterval > 0 {
		if wait := c.MinInterval - time.Since(c.last); wait > 0 {
			time.Sleep(wait)
		}
	}
	c.last = time.Now()
}

// Get performs GET {BaseURL}/{SID}/{verb}/eq2/{collection}/?{query}.
// On HTTP 429 it backs off once and retries.
func (c *Client) Get(ctx context.Context, verb, collection, query string) ([]byte, error) {
	url := fmt.Sprintf("%s/%s/%s/eq2/%s/?%s", c.BaseURL, c.SID, verb, collection, query)
	for attempt := 0; attempt < 2; attempt++ {
		c.throttle()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/census/ -run TestGet -v`
Expected: PASS (both).

- [ ] **Step 5: Commit**

```bash
git add internal/census/client.go internal/census/client_test.go
git commit -m "feat: throttled census http client"
```

---

## Task 2: Item types + decoding

**Files:**
- Create: `internal/census/item.go`
- Test: `internal/census/item_test.go`

- [ ] **Step 1: Write the failing test**

```go
package census

import "testing"

const sampleItemList = `{"item_list":[{
  "id":4202408049,"displayname":"Soulfire Hammer","tier":"FABLED","itemlevel":70,
  "gamelink":"040000...","slot_list":[{"id":0,"name":"Primary"}],
  "typeinfo":{"name":"weapon","skilltype":"crushing","delay":4.0,"damagerating":75.9,
    "minbasedamage":76,"maxbasedamage":228,
    "classes":{"assassin":{"displayname":"Assassin","id":15,"level":70}}},
  "modifiers":{"strength":{"displayname":"str","type":"attribute","value":39}},
  "_extended":{"discovered":{"timestamp":1183075200,
    "world_list":[{"timestamp":1183075200,"id":108,"charid":1},{"timestamp":1686700800,"id":614,"charid":2}]}}
}],"returned":1}`

func TestDecodeItems(t *testing.T) {
	items, err := DecodeItems([]byte(sampleItemList))
	if err != nil {
		t.Fatalf("DecodeItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items", len(items))
	}
	it := items[0]
	if it.DisplayName != "Soulfire Hammer" || it.ItemLevel != 70 {
		t.Fatalf("bad item: %+v", it)
	}
	if it.Slots[0].Name != "Primary" {
		t.Fatalf("bad slot: %+v", it.Slots)
	}
	if _, ok := it.TypeInfo.Classes["assassin"]; !ok {
		t.Fatalf("missing assassin class")
	}
	if it.Modifiers["strength"].Value != 39 {
		t.Fatalf("bad modifier: %+v", it.Modifiers)
	}
	var has614 bool
	for _, w := range it.Extended.Discovered.Worlds {
		if w.ID == 614 {
			has614 = true
		}
	}
	if !has614 {
		t.Fatalf("missing world 614 entry")
	}
}

func TestDecodeItemsError(t *testing.T) {
	if _, err := DecodeItems([]byte(`{"errorCode":"SERVER_ERROR"}`)); err == nil {
		t.Fatalf("expected error on SERVER_ERROR payload")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/census/ -run TestDecode -v`
Expected: FAIL — `undefined: DecodeItems`.

- [ ] **Step 3: Write minimal implementation**

```go
package census

import (
	"encoding/json"
	"fmt"
)

type Slot struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Modifier struct {
	DisplayName string  `json:"displayname"`
	Type        string  `json:"type"`
	Value       float64 `json:"value"`
}

type ClassReq struct {
	DisplayName string `json:"displayname"`
	ID          int    `json:"id"`
	Level       int    `json:"level"`
}

type TypeInfo struct {
	Name          string              `json:"name"`
	SkillType     string              `json:"skilltype"`
	KnowledgeDesc string              `json:"knowledgedesc"`
	Classes       map[string]ClassReq `json:"classes"`
	// Weapon-only fields (zero for non-weapons).
	DamageRating  float64 `json:"damagerating"`
	Delay         float64 `json:"delay"`
	MinBaseDamage float64 `json:"minbasedamage"`
	MaxBaseDamage float64 `json:"maxbasedamage"`
}

type WorldDiscovery struct {
	Timestamp float64 `json:"timestamp"`
	ID        int     `json:"id"`
	CharID    int64   `json:"charid"`
}

type Discovered struct {
	Timestamp float64          `json:"timestamp"`
	Worlds    []WorldDiscovery `json:"world_list"`
}

type Extended struct {
	Discovered Discovered `json:"discovered"`
}

type Item struct {
	ID          int64               `json:"id"`
	DisplayName string              `json:"displayname"`
	Tier        string              `json:"tier"`
	ItemLevel   int                 `json:"itemlevel"`
	GameLink    string              `json:"gamelink"`
	Slots       []Slot              `json:"slot_list"`
	TypeInfo    TypeInfo            `json:"typeinfo"`
	Modifiers   map[string]Modifier `json:"modifiers"`
	Extended    Extended            `json:"_extended"`
}

type itemListResponse struct {
	Items     []Item `json:"item_list"`
	Returned  int    `json:"returned"`
	ErrorCode string `json:"errorCode"`
	Error     string `json:"error"`
}

// DecodeItems parses a Census item_list payload, surfacing API error envelopes.
func DecodeItems(body []byte) ([]Item, error) {
	var r itemListResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	if r.ErrorCode != "" || r.Error != "" {
		return nil, fmt.Errorf("census error: %s%s", r.ErrorCode, r.Error)
	}
	return r.Items, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/census/ -run TestDecode -v`
Expected: PASS (both).

- [ ] **Step 5: Commit**

```bash
git add internal/census/item.go internal/census/item_test.go
git commit -m "feat: census item types and decoder"
```

---

## Task 3: EoF Varsoon classifier

**Files:**
- Create: `internal/classify/eof.go`
- Test: `internal/classify/eof_test.go`

- [ ] **Step 1: Write the failing test**

Calibration timestamps (UTC): KoS Qeynos Claymore 2022-12-13, EoF White Oak Acorn 2023-04-11, EoF Soulfire 2023-06-14, RoK Tuft 2023-08-08.

```go
package classify

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
)

func itemWithVarsoon(unix float64) census.Item {
	return census.Item{Extended: census.Extended{Discovered: census.Discovered{
		Worlds: []census.WorldDiscovery{
			{ID: 104, Timestamp: 1},     // Antonia Bayle, irrelevant
			{ID: 614, Timestamp: unix},  // Varsoon
		}}}}
}

func TestIsEoF(t *testing.T) {
	cases := []struct {
		name string
		unix float64
		want bool
	}{
		{"KoS Qeynos Claymore 2022-12-13", 1670889600, false},
		{"EoF White Oak Acorn 2023-04-11", 1681171200, true},
		{"EoF Soulfire 2023-06-14", 1686700800, true},
		{"RoK Tuft 2023-08-08 (ceiling, exclusive)", 1691452800, false},
	}
	for _, c := range cases {
		if got := IsEoF(itemWithVarsoon(c.unix)); got != c.want {
			t.Errorf("%s: IsEoF = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestIsEoFNoVarsoonEntry(t *testing.T) {
	it := census.Item{Extended: census.Extended{Discovered: census.Discovered{
		Worlds: []census.WorldDiscovery{{ID: 108, Timestamp: 1183075200}}}}}
	if IsEoF(it) {
		t.Fatalf("item never on Varsoon must not classify as EoF")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/classify/ -v`
Expected: FAIL — `undefined: IsEoF`.

- [ ] **Step 3: Write minimal implementation**

```go
package classify

import (
	"time"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
)

const VarsoonWorldID = 614

// EoF unlock window on Varsoon (content-gated by expansion-exclusive collectables):
// lower bound = White Oak Acorn (EoF-exclusive); upper bound = Tuft of Dark Brown
// Brute Fur (RoK-exclusive), exclusive.
var (
	EoFStart = time.Date(2023, 4, 11, 0, 0, 0, 0, time.UTC)
	EoFEnd   = time.Date(2023, 8, 8, 0, 0, 0, 0, time.UTC)
	// KoS window (lower bound bracketed, not collectable-pinned) — used by the
	// optional max-life KoS extension in Plan 1's source layer.
	KoSStart = time.Date(2022, 12, 11, 0, 0, 0, 0, time.UTC)
	KoSEnd   = EoFStart
)

// VarsoonDiscovery returns the first-discovery time on Varsoon (world 614), if any.
func VarsoonDiscovery(it census.Item) (time.Time, bool) {
	for _, w := range it.Extended.Discovered.Worlds {
		if w.ID == VarsoonWorldID {
			return time.Unix(int64(w.Timestamp), 0).UTC(), true
		}
	}
	return time.Time{}, false
}

func inWindow(t time.Time, start, end time.Time) bool {
	return !t.Before(start) && t.Before(end)
}

// IsEoF reports whether the item's Varsoon first-discovery falls in the EoF window.
func IsEoF(it census.Item) bool {
	t, ok := VarsoonDiscovery(it)
	return ok && inWindow(t, EoFStart, EoFEnd)
}

// IsKoS reports whether the item's Varsoon first-discovery falls in the KoS window.
func IsKoS(it census.Item) bool {
	t, ok := VarsoonDiscovery(it)
	return ok && inWindow(t, KoSStart, KoSEnd)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/classify/ -v`
Expected: PASS (both).

- [ ] **Step 5: Commit**

```bash
git add internal/classify/eof.go internal/classify/eof_test.go
git commit -m "feat: varsoon eof/kos discovery-window classifier"
```

---

## Task 4: Slot category + armor-type mapping

**Files:**
- Create: `internal/catalog/category.go`
- Test: `internal/catalog/category_test.go`

First confirm the live `skilltype` strings (only `heavyarmor`→Plate is verified so far).

- [ ] **Step 1: Confirm armor skilltype values from live Census**

Run:
```bash
curl -s "https://census.daybreakgames.com/s:example/get/eq2/item/?type=Armor&c:limit=8&c:show=displayname,typeinfo.skilltype,typeinfo.knowledgedesc" | python3 -m json.tool
```
Record the distinct `skilltype` → `knowledgedesc` pairs (expect e.g. `heavyarmor`→"Plate Armor", and the cloth/leather/chain equivalents). Use the observed strings in Step 3's map; if a value is unseen, the code falls back to the raw skilltype.

- [ ] **Step 2: Write the failing test**

```go
package catalog

import "testing"

func TestCategoryForSlot(t *testing.T) {
	cases := map[string]string{
		"Primary": "weapons", "Secondary": "weapons", "Ranged": "weapons",
		"Head": "armor", "Chest": "armor", "Forearms": "armor", "Feet": "armor",
		"Neck": "jewelry-charms", "Ears": "jewelry-charms", "Ring": "jewelry-charms",
		"Charm": "jewelry-charms", "Waist": "jewelry-charms", "Cloak": "jewelry-charms",
		"Mount": "other",
	}
	for slot, want := range cases {
		if got := CategoryForSlot(slot); got != want {
			t.Errorf("CategoryForSlot(%q) = %q, want %q", slot, got, want)
		}
	}
}

func TestArmorType(t *testing.T) {
	cases := map[string]string{
		"heavyarmor": "Plate", "mediumarmor": "Chain",
		"leatherarmor": "Leather", "clotharmor": "Cloth",
		"crushing": "", // weapon skilltype -> not armor
	}
	for skill, want := range cases {
		if got := ArmorType(skill); got != want {
			t.Errorf("ArmorType(%q) = %q, want %q", skill, got, want)
		}
	}
}
```

- [ ] **Step 3: Write minimal implementation**

Replace the armor-type keys with the strings confirmed in Step 1 if they differ.

```go
package catalog

import "strings"

var slotCategory = map[string]string{
	"primary": "weapons", "secondary": "weapons", "ranged": "weapons",
	"head": "armor", "shoulders": "armor", "chest": "armor", "forearms": "armor",
	"hands": "armor", "legs": "armor", "feet": "armor",
	"neck": "jewelry-charms", "ears": "jewelry-charms", "ear": "jewelry-charms",
	"wrist": "jewelry-charms", "wrists": "jewelry-charms", "ring": "jewelry-charms",
	"finger": "jewelry-charms", "charm": "jewelry-charms", "waist": "jewelry-charms",
	"belt": "jewelry-charms", "cloak": "jewelry-charms", "cloak/back": "jewelry-charms",
}

// CategoryForSlot maps a slot name to a catalog category. Unmapped slots go to
// "other" so nothing is silently dropped.
func CategoryForSlot(slot string) string {
	if c, ok := slotCategory[strings.ToLower(slot)]; ok {
		return c
	}
	return "other"
}

var armorSkillType = map[string]string{
	"clotharmor":   "Cloth",
	"leatherarmor": "Leather",
	"mediumarmor":  "Chain",
	"heavyarmor":   "Plate",
}

// ArmorType maps a Census typeinfo.skilltype to Cloth/Leather/Chain/Plate.
// Returns "" for non-armor skill types.
func ArmorType(skillType string) string {
	return armorSkillType[strings.ToLower(skillType)]
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/catalog/ -run "TestCategory|TestArmorType" -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/catalog/category.go internal/catalog/category_test.go
git commit -m "feat: slot-category and armor-type mapping"
```

---

## Task 5: Wide-format CSV writer + load-back

**Files:**
- Create: `internal/catalog/csv.go`
- Test: `internal/catalog/csv_test.go`

- [ ] **Step 1: Write the failing test**

```go
package catalog

import (
	"bytes"
	"strings"
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
)

func TestWriteCSVUnionColumns(t *testing.T) {
	items := []census.Item{
		{ID: 1, DisplayName: "Sword", Tier: "FABLED", ItemLevel: 70, GameLink: "lnkA",
			Slots:     []census.Slot{{Name: "Primary"}},
			TypeInfo:  census.TypeInfo{Name: "weapon", SkillType: "slashing", MinBaseDamage: 10, MaxBaseDamage: 20, Delay: 3, DamageRating: 50, Classes: map[string]census.ClassReq{"assassin": {}}},
			Modifiers: map[string]census.Modifier{"strength": {Value: 40}},
		},
		{ID: 2, DisplayName: "Cap", Tier: "LEGENDARY", ItemLevel: 68, GameLink: "lnkB",
			Slots:     []census.Slot{{Name: "Head"}},
			TypeInfo:  census.TypeInfo{Name: "armor", SkillType: "heavyarmor", Classes: map[string]census.ClassReq{"guardian": {}}},
			Modifiers: map[string]census.Modifier{"stamina": {Value: 55}, "maxhpperc": {Value: 6}},
		},
	}

	var buf bytes.Buffer
	if err := WriteCSV(&buf, items); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}
	out := buf.String()
	header := strings.SplitN(out, "\n", 2)[0]
	// Fixed columns present.
	for _, col := range []string{"name", "slot", "tier", "itemlevel", "armor_type", "classes", "gamelink", "id"} {
		if !strings.Contains(header, col) {
			t.Errorf("header missing %q: %s", col, header)
		}
	}
	// Union of stat columns from BOTH items.
	for _, col := range []string{"strength", "stamina", "maxhpperc"} {
		if !strings.Contains(header, col) {
			t.Errorf("header missing stat %q: %s", col, header)
		}
	}
	// armor_type populated only for the armor item.
	if !strings.Contains(out, "Plate") {
		t.Errorf("expected Plate armor_type in output:\n%s", out)
	}
}

func TestRoundTrip(t *testing.T) {
	items := []census.Item{
		{ID: 1, DisplayName: "Sword", Tier: "FABLED", ItemLevel: 70,
			Slots:     []census.Slot{{Name: "Primary"}},
			TypeInfo:  census.TypeInfo{Name: "weapon", SkillType: "slashing", MinBaseDamage: 10, MaxBaseDamage: 20, Delay: 3, DamageRating: 50, Classes: map[string]census.ClassReq{"assassin": {}}},
			Modifiers: map[string]census.Modifier{"strength": {Value: 40}},
		},
	}
	var buf bytes.Buffer
	if err := WriteCSV(&buf, items); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}
	got, err := ReadCSV(&buf)
	if err != nil {
		t.Fatalf("ReadCSV: %v", err)
	}
	if len(got) != 1 || got[0].DisplayName != "Sword" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got[0].Modifiers["strength"].Value != 40 {
		t.Fatalf("lost modifier: %+v", got[0].Modifiers)
	}
	if _, ok := got[0].TypeInfo.Classes["assassin"]; !ok {
		t.Fatalf("lost class eligibility: %+v", got[0].TypeInfo.Classes)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/catalog/ -run "TestWriteCSV|TestRoundTrip" -v`
Expected: FAIL — `undefined: WriteCSV`.

- [ ] **Step 3: Write minimal implementation**

The CSV must round-trip everything the model (Plan 2) needs: stats, weapon fields, armor_type, classes, slot. `classes` is serialized as a `|`-joined sorted list; weapon fields and slot reconstruct `TypeInfo`/`Slots`.

```go
package catalog

import (
	"encoding/csv"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
)

var fixedCols = []string{
	"id", "name", "slot", "tier", "itemlevel", "armor_type", "classes",
	"weapon_min_dmg", "weapon_max_dmg", "delay", "damage_rating", "gamelink",
}

func slotNames(it census.Item) string {
	names := make([]string, 0, len(it.Slots))
	for _, s := range it.Slots {
		names = append(names, s.Name)
	}
	return strings.Join(names, "|")
}

func classNames(it census.Item) string {
	names := make([]string, 0, len(it.TypeInfo.Classes))
	for k := range it.TypeInfo.Classes {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, "|")
}

func statKeyUnion(items []census.Item) []string {
	set := map[string]struct{}{}
	for _, it := range items {
		for k := range it.Modifiers {
			set[k] = struct{}{}
		}
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func f(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// WriteCSV emits items in wide format: fixed columns + the union of all stat keys.
func WriteCSV(w io.Writer, items []census.Item) error {
	statCols := statKeyUnion(items)
	cw := csv.NewWriter(w)
	if err := cw.Write(append(append([]string{}, fixedCols...), statCols...)); err != nil {
		return err
	}
	for _, it := range items {
		row := []string{
			strconv.FormatInt(it.ID, 10),
			it.DisplayName,
			slotNames(it),
			it.Tier,
			strconv.Itoa(it.ItemLevel),
			ArmorType(it.TypeInfo.SkillType),
			classNames(it),
			f(it.TypeInfo.MinBaseDamage),
			f(it.TypeInfo.MaxBaseDamage),
			f(it.TypeInfo.Delay),
			f(it.TypeInfo.DamageRating),
			it.GameLink,
		}
		for _, k := range statCols {
			if m, ok := it.Modifiers[k]; ok {
				row = append(row, f(m.Value))
			} else {
				row = append(row, "")
			}
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func atof(s string) float64 { v, _ := strconv.ParseFloat(s, 64); return v }

// ReadCSV reconstructs items from a WriteCSV stream (cache load-back).
func ReadCSV(r io.Reader) ([]census.Item, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	rows, err := cr.ReadAll()
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	header := rows[0]
	idx := map[string]int{}
	for i, h := range header {
		idx[h] = i
	}
	statCols := header[len(fixedCols):]
	var items []census.Item
	for _, row := range rows[1:] {
		id, _ := strconv.ParseInt(row[idx["id"]], 10, 64)
		lvl, _ := strconv.Atoi(row[idx["itemlevel"]])
		it := census.Item{
			ID:          id,
			DisplayName: row[idx["name"]],
			Tier:        row[idx["tier"]],
			ItemLevel:   lvl,
			GameLink:    row[idx["gamelink"]],
			TypeInfo: census.TypeInfo{
				MinBaseDamage: atof(row[idx["weapon_min_dmg"]]),
				MaxBaseDamage: atof(row[idx["weapon_max_dmg"]]),
				Delay:         atof(row[idx["delay"]]),
				DamageRating:  atof(row[idx["damage_rating"]]),
				Classes:       map[string]census.ClassReq{},
			},
			Modifiers: map[string]census.Modifier{},
		}
		for _, name := range strings.Split(row[idx["slot"]], "|") {
			if name != "" {
				it.Slots = append(it.Slots, census.Slot{Name: name})
			}
		}
		for _, c := range strings.Split(row[idx["classes"]], "|") {
			if c != "" {
				it.TypeInfo.Classes[c] = census.ClassReq{}
			}
		}
		for _, k := range statCols {
			if cell := row[idx[k]]; cell != "" {
				it.Modifiers[k] = census.Modifier{Value: atof(cell)}
			}
		}
		items = append(items, it)
	}
	return items, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/catalog/ -v`
Expected: PASS (all).

- [ ] **Step 5: Commit**

```bash
git add internal/catalog/csv.go internal/catalog/csv_test.go
git commit -m "feat: wide-format csv writer with cache round-trip"
```

---

## Task 6: Category + max-life splitting

**Files:**
- Modify: `internal/catalog/csv.go`
- Test: `internal/catalog/split_test.go`

- [ ] **Step 1: Write the failing test**

```go
package catalog

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
)

func TestSplitByCategory(t *testing.T) {
	items := []census.Item{
		{ID: 1, Slots: []census.Slot{{Name: "Primary"}}},
		{ID: 2, Slots: []census.Slot{{Name: "Head"}}},
		{ID: 3, Slots: []census.Slot{{Name: "Ring"}}},
	}
	groups := SplitByCategory(items)
	if len(groups["weapons"]) != 1 || len(groups["armor"]) != 1 || len(groups["jewelry-charms"]) != 1 {
		t.Fatalf("bad split: %+v", groups)
	}
}

func TestWithMaxLife(t *testing.T) {
	items := []census.Item{
		{ID: 1, Modifiers: map[string]census.Modifier{"maxhpperc": {Value: 5}}},
		{ID: 2, Modifiers: map[string]census.Modifier{"strength": {Value: 5}}},
		{ID: 3, Modifiers: map[string]census.Modifier{"health": {Value: 100}}},
	}
	got := WithMaxLife(items)
	if len(got) != 2 {
		t.Fatalf("expected 2 max-life items, got %d", len(got))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/catalog/ -run "TestSplitByCategory|TestWithMaxLife" -v`
Expected: FAIL — `undefined: SplitByCategory`.

- [ ] **Step 3: Write minimal implementation (append to csv.go)**

```go
// maxLifeKeys are the Census modifier keys that represent maximum health.
var maxLifeKeys = []string{"maxhpperc", "health", "hp", "maxhealth"}

// SplitByCategory groups items by their first slot's category.
func SplitByCategory(items []census.Item) map[string][]census.Item {
	groups := map[string][]census.Item{}
	for _, it := range items {
		slot := ""
		if len(it.Slots) > 0 {
			slot = it.Slots[0].Name
		}
		cat := CategoryForSlot(slot)
		groups[cat] = append(groups[cat], it)
	}
	return groups
}

// WithMaxLife returns items carrying any maximum-health modifier (any class).
func WithMaxLife(items []census.Item) []census.Item {
	var out []census.Item
	for _, it := range items {
		for _, k := range maxLifeKeys {
			if _, ok := it.Modifiers[k]; ok {
				out = append(out, it)
				break
			}
		}
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/catalog/ -v`
Expected: PASS (all).

- [ ] **Step 5: Commit**

```bash
git add internal/catalog/csv.go internal/catalog/split_test.go
git commit -m "feat: category split and max-life filter"
```

---

## Task 7: Paged, pruned EoF extraction

**Files:**
- Create: `internal/extract/extract.go`
- Test: `internal/extract/extract_test.go`

Server-side prune: `_extended.discovered.world_list.id=614` + `itemlevel=<72`. Page with `c:limit`/`c:start`. Stop when a page returns fewer than the page size.

- [ ] **Step 1: Write the failing test**

```go
package extract

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
)

func TestExtractPagesUntilShort(t *testing.T) {
	// Page 0: full page of 2 (one in-window EoF, one KoS out-of-window).
	// Page 1: 1 item (short) -> stop.
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
		if start == "" || start == "0" {
			fmt.Fprint(w, pages[0])
		} else {
			fmt.Fprint(w, pages[1])
		}
	}))
	defer srv.Close()

	c := census.New("s:example")
	c.BaseURL = srv.URL
	c.MinInterval = 0

	items, err := AllEoF(context.Background(), c, 2)
	if err != nil {
		t.Fatalf("AllEoF: %v", err)
	}
	// Items 1 and 3 are in-window EoF; item 2 (KoS) is filtered out.
	if len(items) != 2 {
		t.Fatalf("expected 2 EoF items, got %d: %+v", len(items), items)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/extract/ -v`
Expected: FAIL — `undefined: AllEoF`.

- [ ] **Step 3: Write minimal implementation**

```go
package extract

import (
	"context"
	"fmt"

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
		if len(page) < pageSize {
			break
		}
	}
	return out, nil
}
```

> Note: if Step-1 verification (Task 8) shows the server-side `world_list.id=614` filter is unsupported, drop that clause from `query` and rely on the client-side `keep` predicate plus the `itemlevel` ceiling. The function signature is unchanged.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/extract/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/extract/extract.go internal/extract/extract_test.go
git commit -m "feat: paged pruned eof/kos item extraction"
```

---

## Task 8: Data-source layer (cache vs fresh pull)

**Files:**
- Create: `internal/source/source.go`
- Test: `internal/source/source_test.go`

- [ ] **Step 1: Write the failing test**

```go
package source

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/catalog"
	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
)

func TestLoadFromCacheWhenPresent(t *testing.T) {
	dir := t.TempDir()
	// Seed a weapons.csv so the loader finds a cache.
	items := []census.Item{{ID: 1, DisplayName: "Sword", Slots: []census.Slot{{Name: "Primary"}},
		TypeInfo: census.TypeInfo{Classes: map[string]census.ClassReq{}}, Modifiers: map[string]census.Modifier{}}}
	f, _ := os.Create(filepath.Join(dir, "weapons.csv"))
	if err := catalog.WriteCSV(f, items); err != nil {
		t.Fatal(err)
	}
	f.Close()

	got, err := LoadCache(dir)
	if err != nil {
		t.Fatalf("LoadCache: %v", err)
	}
	if len(got) != 1 || got[0].DisplayName != "Sword" {
		t.Fatalf("cache load mismatch: %+v", got)
	}
}

func TestCacheExists(t *testing.T) {
	dir := t.TempDir()
	if CacheExists(dir) {
		t.Fatal("empty dir should not be a cache")
	}
	os.Create(filepath.Join(dir, "weapons.csv"))
	if !CacheExists(dir) {
		t.Fatal("dir with weapons.csv should be a cache")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/source/ -v`
Expected: FAIL — `undefined: LoadCache`.

- [ ] **Step 3: Write minimal implementation**

```go
package source

import (
	"context"
	"os"
	"path/filepath"

	"github.com/amdrake93/eq2-eof-itemdex/internal/catalog"
	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/amdrake93/eq2-eof-itemdex/internal/extract"
)

var categoryFiles = []string{"weapons.csv", "armor.csv", "jewelry-charms.csv", "other.csv"}

// CacheExists reports whether at least one category CSV is present in dir.
func CacheExists(dir string) bool {
	for _, name := range categoryFiles {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

// LoadCache reconstructs the full item set by reading every category CSV in dir.
func LoadCache(dir string) ([]census.Item, error) {
	var all []census.Item
	for _, name := range categoryFiles {
		path := filepath.Join(dir, name)
		f, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		items, err := catalog.ReadCSV(f)
		f.Close()
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

// FreshPull queries Census for the full EoF item set and writes the category +
// max-life CSVs into dir, returning the items.
func FreshPull(ctx context.Context, c *census.Client, dir string, pageSize int) ([]census.Item, error) {
	items, err := extract.AllEoF(ctx, c, pageSize)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	for cat, group := range catalog.SplitByCategory(items) {
		if err := writeFile(filepath.Join(dir, cat+".csv"), group); err != nil {
			return nil, err
		}
	}
	if err := writeFile(filepath.Join(dir, "maxlife.csv"), catalog.WithMaxLife(items)); err != nil {
		return nil, err
	}
	return items, nil
}

func writeFile(path string, items []census.Item) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return catalog.WriteCSV(f, items)
}

// Load returns items from the CSV cache when present and refresh is false;
// otherwise it does a fresh Census pull (which rewrites the cache).
func Load(ctx context.Context, c *census.Client, dir string, refresh bool, pageSize int) ([]census.Item, error) {
	if !refresh && CacheExists(dir) {
		return LoadCache(dir)
	}
	return FreshPull(ctx, c, dir, pageSize)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/source/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/source/source.go internal/source/source_test.go
git commit -m "feat: csv-cache-first data source layer"
```

---

## Task 9: CLI entrypoint

**Files:**
- Create: `cmd/itemdex/main.go`

- [ ] **Step 1: Write the entrypoint**

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/amdrake93/eq2-eof-itemdex/internal/source"
)

func main() {
	var (
		dir      = flag.String("out", "data", "directory for CSV catalog (also the cache)")
		refresh  = flag.Bool("refresh", false, "force a fresh Census pull (rewrites CSVs)")
		sid      = flag.String("sid", "s:example", "Census service ID")
		pageSize = flag.Int("page", 100, "items per Census request")
	)
	flag.Parse()

	c := census.New(*sid)
	items, err := source.Load(context.Background(), c, *dir, *refresh, *pageSize)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	src := "cache"
	if *refresh || !source.CacheExists(*dir) {
		src = "Census"
	}
	fmt.Printf("loaded %d EoF items from %s -> %s/\n", len(items), src, *dir)
}
```

- [ ] **Step 2: Build**

Run: `go build ./...`
Expected: exit 0.

- [ ] **Step 3: Verify the server-side filter, then do a real first pull**

First confirm the nested filter is accepted (per Task 7 note):
```bash
curl -s "https://census.daybreakgames.com/s:example/get/eq2/item/?_extended.discovered.world_list.id=614&itemlevel=<72&c:limit=2&c:show=displayname,itemlevel" | python3 -m json.tool
```
Expected: a small `item_list` (filter works) — **not** `{"errorCode":...}`. If it errors, edit `extract.go` to drop the `world_list.id=614` clause (Task 7 note) before continuing.

Then:
Run: `go run ./cmd/itemdex --refresh`
Expected: `loaded N EoF items from Census -> data/` with N in the hundreds–thousands; `data/weapons.csv`, `armor.csv`, `jewelry-charms.csv`, `maxlife.csv` created. (This is throttled at 10 req/min, so a few thousand items over ~100/page ≈ several minutes.)

- [ ] **Step 4: Verify cache-first on second run**

Run: `go run ./cmd/itemdex`
Expected: `loaded N EoF items from cache -> data/` (fast, no network).

- [ ] **Step 5: Spot-check the data**

Run: `head -3 data/armor.csv` and confirm an `armor_type` column populated with Plate/Chain/Leather/Cloth, and that a known EoF weapon (e.g. Grinning Dirk of Horror) appears in `weapons.csv`:
```bash
grep -i "grinning dirk" data/weapons.csv
```

- [ ] **Step 6: Commit (code only — decide on data/ separately)**

```bash
git add cmd/itemdex/main.go
git commit -m "feat: itemdex CLI with csv-cache-first loading"
```

---

## Task 10: README usage + data decision

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add a Usage section to README.md**

```markdown
## Usage

```bash
go run ./cmd/itemdex            # load items (CSV cache if present, else Census pull)
go run ./cmd/itemdex --refresh  # force a fresh Census pull, rewriting data/*.csv
```

Outputs `data/weapons.csv`, `data/armor.csv`, `data/jewelry-charms.csv`, and `data/maxlife.csv`
(every EoF item with Max Health, any class). Stats are exactly as Census reports them — no translations.
```

- [ ] **Step 2: Decide whether to track `data/*.csv`**

If sharing the catalogs via GitHub (recommended — that's the point), `git add data/*.csv`. Otherwise uncomment the `/data/*.csv` line in `.gitignore`.

- [ ] **Step 3: Commit + push**

```bash
git add README.md data/  # include data/ only if sharing the catalogs
git commit -m "docs: usage; publish EoF item catalog CSVs"
git push
```

---

## Self-Review

**Spec coverage (spec §§ → tasks):**
- §2 Census API / throttle → Task 1; item schema → Task 2.
- §2.1 translation rule (CSVs untranslated) → Tasks 5/6 store raw values; no translation applied anywhere in Plan 1 ✓ (translation is Plan 2's model concern only).
- §3 classifier (world 614, window, 5 calibration items) → Task 3.
- §8 extraction (verify nested filter, prune, page) → Tasks 7 & 9 Step 3.
- §8 data-source modes (CSV-first, `--refresh`) → Task 8, Task 9.
- §10 catalog (comprehensive all-class, category split, columns incl. armor_type/classes/gamelink, max-life cross-cut, KoS extension hook, cache dual-duty) → Tasks 4/5/6/8; KoS via `extract.AllKoS` + `classify.IsKoS`.
- §11 validation (calibration items, Grinning Dirk anchor) → Task 3 tests, Task 9 Step 5.
- **Out of scope for Plan 1 (→ Plan 2):** spell/CA pipeline (§6), DPS model & weights (§4), BiS output (§9), buff baseline (§5). The KoS-extension *wiring* exists (`AllKoS`); invoking it from the CLI is deferred to when needed.

**Placeholder scan:** none — armor-type unknowns are resolved by a real probe (Task 4 Step 1) with a raw-string fallback; the server-side-filter uncertainty has an explicit verify-then-branch step (Task 9 Step 3 + Task 7 note).

**Type consistency:** `census.Item`, `TypeInfo.Classes`, `Modifier.Value`, `WorldDiscovery.ID` used identically across classify/catalog/extract/source. `WriteCSV`/`ReadCSV`, `SplitByCategory`/`WithMaxLife`, `AllEoF`/`AllKoS`, `Load`/`LoadCache`/`CacheExists`/`FreshPull` signatures match their call sites. `fixedCols` order matches both writer and reader (reader slices `header[len(fixedCols):]` for stat columns).
