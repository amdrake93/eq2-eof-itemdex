# Character Gear Import Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Import the live equipped loadout for one character from Census into an inspectable loadout file, then let `bis` run sims from that real gear set.

**Architecture:** Approach A (loadout file). A new `itemdex import` subcommand performs the single network step — it fetches the character's `equipmentslot_list`, resolves each kept slot's stats (item base + filled-adornment stats, fetching anything uncataloged), and writes `characters/<name>-loadout.toml`. A new `bis --loadout <file>` builds a `Set` from that file: optimizable slots seed/lock the equipped item and re-optimize against the catalog; fixed slots and adornments contribute stats but are never swapped. Authoritative behavior: `docs/SPEC.md` Planned subsections §3/§4/§6/§7/§8/§16. Rationale + live feasibility probes: `docs/plans/2026-06-24-character-gear-import-design.md`.

**Tech Stack:** Go 1.26 (module `github.com/amdrake93/eq2-eof-itemdex`), BurntSushi/toml, testify/require, httptest for Census mocking, modernc/SQLite (`:memory:` in tests). Build: `go build ./...`. Test: `go test ./...`.

---

## File Structure

| File | Responsibility | New/Modify |
|---|---|---|
| `internal/census/character.go` | `Character`/`EquipmentSlot`/`EquippedItem`/`Adornment` types; `DecodeCharacter`; `FetchCharacter`; `FetchItemsByIDs` | Create |
| `internal/census/character_test.go` | decode + fetch tests (httptest) | Create |
| `internal/charconfig/charconfig.go` | add `CensusName`/`World` to `Character` | Modify |
| `internal/loadout/loadout.go` | `File`/`SlotEntry` types; `Write`/`Read` (TOML); slot classification (`SkipSlot`, `mapping`) | Create |
| `internal/loadout/loadout_test.go` | round-trip + classification tests | Create |
| `internal/loadout/resolve.go` | `Resolve(ctx, c, char, catalog, adornCache)` → `(File, fetched []census.Item, adorns []census.Item, unresolved []string)` | Create |
| `internal/loadout/resolve_test.go` | resolution tests (catalog hit, fetch path, adornment sum, skip slots) | Create |
| `internal/catalog/adornments.go` | `data/adornments.csv` read/write (`Adornment` row type) | Create |
| `internal/catalog/adornments_test.go` | adornment CSV round-trip | Create |
| `cmd/itemdex/main.go` | `import` subcommand dispatch + `runImport` | Modify |
| `internal/bis/loadoutset.go` | `SetFromLoadout(file, profile, lo, autoMult, fightLen)` → `*Set` + `optimizableSlots` | Create |
| `internal/bis/loadoutset_test.go` | set-from-loadout tests | Create |
| `cmd/bis/main.go` | `--loadout` flag + import-mode report path | Modify |

**Parallelizable groups for subagent dispatch:** Tasks 1–2 (census), Task 3 (config), Task 4 (adornment CSV), Task 5 (slot mapping) are independent and can run concurrently. Task 6 (loadout file types) depends on nothing but is small. Task 7 (resolve) depends on 1,4,5. Task 8 (itemdex import) depends on 1,3,6,7. Tasks 9–10 (bis --loadout) depend on 6 and the bis engine. Task 11 is final spec/doc sync.

---

## Task 1: Census character types + decode

**Files:**
- Create: `internal/census/character.go`
- Test: `internal/census/character_test.go`

- [ ] **Step 1: Write the failing test**

```go
package census

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeCharacterEquipment(t *testing.T) {
	body := []byte(`{"character_list":[{
		"displayname":"Biffels (Wuoshi)",
		"type":{"class":"Assassin","level":70},
		"last_update":1782258823.5,
		"equipmentslot_list":[
			{"name":"cloak","item":{"id":264598753,"adornment_list":[{"color":"white"},{"id":111,"color":"orange"}]}},
			{"name":"food","item":{"id":461060541,"adornment_list":[]}},
			{"name":"mount_armor","item":{"adornment_list":[]}}
		]
	}],"returned":1}`)

	ch, err := DecodeCharacter(body)
	require.NoError(t, err)
	require.Equal(t, "Biffels (Wuoshi)", ch.DisplayName)
	require.Equal(t, "Assassin", ch.Type.Class)
	require.Equal(t, 70, ch.Type.Level)
	require.InDelta(t, 1782258823.5, ch.LastUpdate, 1e-3)
	require.Len(t, ch.EquipmentSlots, 3)

	cloak := ch.EquipmentSlots[0]
	require.Equal(t, "cloak", cloak.Name)
	require.Equal(t, int64(264598753), cloak.Item.ID)
	// only the filled socket (id 111) is a real adornment; color-only is an empty socket
	require.Equal(t, []int64{111}, cloak.Item.FilledAdornmentIDs())
}

func TestDecodeCharacterErrorEnvelope(t *testing.T) {
	_, err := DecodeCharacter([]byte(`{"errorCode":"SERVER_ERROR"}`))
	require.Error(t, err)
}

func TestDecodeCharacterNotFound(t *testing.T) {
	_, err := DecodeCharacter([]byte(`{"character_list":[],"returned":0}`))
	require.ErrorIs(t, err, ErrCharacterNotFound)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/census/ -run TestDecodeCharacter -v`
Expected: FAIL — `undefined: DecodeCharacter` / `ErrCharacterNotFound`.

- [ ] **Step 3: Write minimal implementation**

```go
package census

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrCharacterNotFound means the census character query returned zero rows.
var ErrCharacterNotFound = errors.New("census: character not found")

// Character is the subset of an eq2 character record the gear import needs.
type Character struct {
	DisplayName    string
	Type           CharType
	LastUpdate     float64
	EquipmentSlots []EquipmentSlot
}

type CharType struct {
	Class string `json:"class"`
	Level int    `json:"level"`
}

// EquipmentSlot is one entry of equipmentslot_list (the census slot name + item).
type EquipmentSlot struct {
	Name string       `json:"name"`
	Item EquippedItem `json:"item"`
}

// EquippedItem is the item in a slot, with its adornment sockets.
type EquippedItem struct {
	ID         int64       `json:"id"`
	Adornments []Adornment `json:"adornment_list"`
}

// Adornment is one socket: a filled socket has ID != 0; an empty one is color-only.
type Adornment struct {
	ID    int64  `json:"id"`
	Color string `json:"color"`
}

// FilledAdornmentIDs returns the ids of filled sockets (skips empty color-only sockets).
func (e EquippedItem) FilledAdornmentIDs() []int64 {
	var out []int64
	for _, a := range e.Adornments {
		if a.ID != 0 {
			out = append(out, a.ID)
		}
	}
	return out
}

type charType struct {
	Class FlexString `json:"class"`
	Level int        `json:"level"`
}

type characterRaw struct {
	DisplayName    FlexString      `json:"displayname"`
	Type           charType        `json:"type"`
	LastUpdate     float64         `json:"last_update"`
	EquipmentSlots []EquipmentSlot `json:"equipmentslot_list"`
}

type characterListResponse struct {
	Characters []characterRaw `json:"character_list"`
	Returned   int            `json:"returned"`
	ErrorCode  string         `json:"errorCode"`
	Error      string         `json:"error"`
}

// DecodeCharacter parses a census character_list payload for the first character.
func DecodeCharacter(body []byte) (Character, error) {
	var r characterListResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return Character{}, err
	}
	if r.ErrorCode != "" || r.Error != "" {
		return Character{}, fmt.Errorf("census error: %s%s", r.ErrorCode, r.Error)
	}
	if len(r.Characters) == 0 {
		return Character{}, ErrCharacterNotFound
	}
	c := r.Characters[0]
	return Character{
		DisplayName:    string(c.DisplayName),
		Type:           CharType{Class: string(c.Type.Class), Level: c.Type.Level},
		LastUpdate:     c.LastUpdate,
		EquipmentSlots: c.EquipmentSlots,
	}, nil
}
```

> Note: `FlexString` already exists in the package (`census.FlexString`, SPEC §3). Confirm `displayname`/`class` decode through it; if `class` is always a plain string in census, a plain `string` field is fine — keep `FlexString` to be safe against census's flexible typing.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/census/ -run TestDecodeCharacter -v`
Expected: PASS (all three).

- [ ] **Step 5: Commit**

```bash
git add internal/census/character.go internal/census/character_test.go
git commit -m "feat(census): character equipment decode types"
```

---

## Task 2: Census fetch helpers (character + items by id)

**Files:**
- Modify: `internal/census/character.go`
- Test: `internal/census/character_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestFetchCharacterBuildsQuery(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"character_list":[{"displayname":"Biffels (Wuoshi)","type":{"class":"Assassin","level":70},"equipmentslot_list":[]}],"returned":1}`))
	}))
	defer srv.Close()

	c := New("s:example")
	c.BaseURL = srv.URL
	c.Limiter = rate.NewLimiter(rate.Inf, 1)

	ch, err := FetchCharacter(context.Background(), c, "Biffels", 618)
	require.NoError(t, err)
	require.Equal(t, "Biffels (Wuoshi)", ch.DisplayName)
	require.Contains(t, gotQuery, "name.first_lower=biffels")
	require.Contains(t, gotQuery, "locationdata.worldid=618")
	require.Contains(t, gotQuery, "c%3Ashow=") // c:show= url-encoded
}

func TestFetchItemsByIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Contains(t, r.URL.RawQuery, "id=101%2C102") // id=101,102 url-encoded
		_, _ = w.Write([]byte(`{"item_list":[{"id":101,"displayname":"A"},{"id":102,"displayname":"B"}]}`))
	}))
	defer srv.Close()

	c := New("s:example")
	c.BaseURL = srv.URL
	c.Limiter = rate.NewLimiter(rate.Inf, 1)

	items, err := FetchItemsByIDs(context.Background(), c, []int64{101, 102})
	require.NoError(t, err)
	require.Len(t, items, 2)
}
```

Add imports to the test file: `context`, `net/http`, `net/http/httptest`, `golang.org/x/time/rate`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/census/ -run "TestFetchCharacter|TestFetchItemsByIDs" -v`
Expected: FAIL — `undefined: FetchCharacter` / `FetchItemsByIDs`.

- [ ] **Step 3: Write minimal implementation** (append to `internal/census/character.go`)

```go
import (
	"context"
	"net/url"
	"strconv"
	"strings"
)

const characterShowFields = "displayname,type.class,type.level,last_update,equipmentslot_list"

// FetchCharacter queries one character by name + world from the eq2 character collection.
func FetchCharacter(ctx context.Context, c *Client, censusName string, world int) (Character, error) {
	q := url.Values{}
	q.Set("name.first_lower", strings.ToLower(censusName))
	q.Set("locationdata.worldid", strconv.Itoa(world))
	q.Set("c:limit", "1")
	q.Set("c:show", characterShowFields)
	body, err := c.Get(ctx, "get", "character", q.Encode())
	if err != nil {
		return Character{}, err
	}
	return DecodeCharacter(body)
}

// FetchItemsByIDs pulls full item records (incl. modifiers + effect_list) for the
// given ids in a single request. Used for items/adornments absent from the catalog.
func FetchItemsByIDs(ctx context.Context, c *Client, ids []int64) ([]Item, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = strconv.FormatInt(id, 10)
	}
	q := url.Values{}
	q.Set("id", strings.Join(strs, ","))
	q.Set("c:limit", strconv.Itoa(len(ids)))
	q.Set("c:show", itemShowFields)
	body, err := c.Get(ctx, "get", "item", q.Encode())
	if err != nil {
		return nil, err
	}
	return DecodeItems(body)
}
```

> `itemShowFields` — reuse the existing constant the item pull uses (the recon shows the item catalog c:show set including `modifiers`, `typeinfo`, `effect_list`, `slot_list`). If it is unexported in another file, either export it or define a local `const itemShowFields = "id,displayname,tier,itemlevel,slot_list,typeinfo,modifiers,effect_list"`. Verify against `internal/extract` / `internal/source` for the canonical field list and match it so fetched items carry the same fields as cataloged ones.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/census/ -run "TestFetchCharacter|TestFetchItemsByIDs" -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/census/character.go internal/census/character_test.go
git commit -m "feat(census): FetchCharacter + FetchItemsByIDs"
```

---

## Task 3: Config `census_name` + `world`

**Files:**
- Modify: `internal/charconfig/charconfig.go` (the `Character` struct)
- Test: `internal/charconfig/charconfig_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestLoadCensusFields(t *testing.T) {
	p := writeConfig(t, `
[character]
name = "Alex"
class = "assassin"
art_tier = "expert"
census_name = "Biffels"
world = 618
[contexts.solo]
mainstat = 156
`)
	cfg, err := Load(p)
	require.NoError(t, err)
	require.Equal(t, "Biffels", cfg.Character.CensusName)
	require.Equal(t, 618, cfg.Character.World)
}

func TestLoadCensusFieldsOptional(t *testing.T) {
	// existing configs without census_name/world still load
	p := writeConfig(t, `
[character]
name = "Alex"
class = "assassin"
art_tier = "expert"
[contexts.solo]
mainstat = 156
`)
	cfg, err := Load(p)
	require.NoError(t, err)
	require.Equal(t, "", cfg.Character.CensusName)
	require.Equal(t, 0, cfg.Character.World)
}
```

(`writeConfig` helper already exists in this test file per recon.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/charconfig/ -run TestLoadCensusFields -v`
Expected: FAIL — `cfg.Character.CensusName undefined` (and strict-mode "unknown config keys: census_name, world" because the keys aren't yet on the struct).

- [ ] **Step 3: Write minimal implementation**

Modify the `Character` struct:

```go
type Character struct {
	Name       string `toml:"name"`
	Class      string `toml:"class"`
	ArtTier    string `toml:"art_tier"`
	CensusName string `toml:"census_name"` // in-game character name for gear import (§4)
	World      int    `toml:"world"`       // census worldid for gear import (Wuoshi = 618)
}
```

No validation change: both fields are optional for non-import flows. (Import enforces presence in Task 8.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/charconfig/ -run TestLoadCensusFields -v`
Expected: PASS. Also run `go test ./internal/charconfig/` to confirm the committed-config test still passes.

- [ ] **Step 5: Commit**

```bash
git add internal/charconfig/charconfig.go internal/charconfig/charconfig_test.go
git commit -m "feat(charconfig): optional census_name + world fields"
```

---

## Task 4: Adornment catalog CSV (`data/adornments.csv`)

**Files:**
- Create: `internal/catalog/adornments.go`
- Test: `internal/catalog/adornments_test.go`

- [ ] **Step 1: Write the failing test**

```go
package catalog

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdornmentCSVRoundTrip(t *testing.T) {
	rows := []Adornment{
		{ID: 111, Name: "Deadly Adornment", Stats: map[string]float64{"critchance": 2}},
		{ID: 222, Name: "Adornment of Haste", Stats: map[string]float64{"attackspeed": 3, "flurry": 1}},
	}
	var buf bytes.Buffer
	require.NoError(t, WriteAdornmentsCSV(&buf, rows))

	got, err := ReadAdornmentsCSV(&buf)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, int64(111), got[0].ID)
	require.Equal(t, "Deadly Adornment", got[0].Name)
	require.InDelta(t, 2, got[0].Stats["critchance"], 1e-9)
	require.InDelta(t, 3, got[1].Stats["attackspeed"], 1e-9)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/catalog/ -run TestAdornmentCSVRoundTrip -v`
Expected: FAIL — `undefined: Adornment / WriteAdornmentsCSV / ReadAdornmentsCSV`.

- [ ] **Step 3: Write minimal implementation**

```go
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
			v, _ := strconv.ParseFloat(rec[i], 64)
			a.Stats[header[i]] = v
		}
		out = append(out, a)
	}
	return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/catalog/ -run TestAdornmentCSVRoundTrip -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/catalog/adornments.go internal/catalog/adornments_test.go
git commit -m "feat(catalog): adornments.csv read/write"
```

---

## Task 5: Slot classification (character slot → keep/skip)

**Files:**
- Create: `internal/loadout/loadout.go` (classification half)
- Test: `internal/loadout/loadout_test.go`

- [ ] **Step 1: Write the failing test**

```go
package loadout

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSkipSlot(t *testing.T) {
	skip := []string{"food", "drink", "mount_adornment", "mount_armor"}
	for _, s := range skip {
		require.True(t, SkipSlot(s), "expected %q skipped", s)
	}
	keep := []string{"primary", "secondary", "head", "cloak", "left_ring", "ears2", "ranged", "ammo", "activate1", "event_slot", "waist"}
	for _, s := range keep {
		require.False(t, SkipSlot(s), "expected %q kept", s)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/loadout/ -run TestSkipSlot -v`
Expected: FAIL — `undefined: SkipSlot` (package does not yet exist).

- [ ] **Step 3: Write minimal implementation**

```go
// Package loadout reads/writes imported-character loadout files and resolves
// equipped items + adornments to stat blocks for the bis sim (SPEC §4, §7).
package loadout

// skippedCharSlots are census character-equipment slots whose stats are NOT
// counted on import: food/drink change ad hoc; mount slots aren't player gear.
var skippedCharSlots = map[string]bool{
	"food":            true,
	"drink":           true,
	"mount_adornment": true,
	"mount_armor":     true,
}

// SkipSlot reports whether a census character slot name is excluded from import.
func SkipSlot(charSlot string) bool { return skippedCharSlots[charSlot] }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/loadout/ -run TestSkipSlot -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/loadout/loadout.go internal/loadout/loadout_test.go
git commit -m "feat(loadout): character slot skip classification"
```

---

## Task 6: Loadout file type + TOML round-trip

**Files:**
- Modify: `internal/loadout/loadout.go`
- Test: `internal/loadout/loadout_test.go`

- [ ] **Step 1: Write the failing test**

```go
import (
	"path/filepath"

	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
)

func TestFileRoundTrip(t *testing.T) {
	f := File{
		CharacterName: "Biffels (Wuoshi)",
		LastUpdate:    1782258823.5,
		Slots: []SlotEntry{
			{CatalogSlot: "Back", CharSlot: "cloak", ItemID: 264598753, Name: "Cloak of Flames",
				Optimizable: true, Stats: model.StatBlock{Haste: 25}},
			{CatalogSlot: "Charm", CharSlot: "activate1", ItemID: 4135486725, Name: "Clicky",
				Optimizable: false, Stats: model.StatBlock{CritChance: 3}},
			{CatalogSlot: "Primary", CharSlot: "primary", ItemID: 1606057721, Name: "Sabre",
				Optimizable: true, WeaponMin: 80, WeaponMax: 160, WeaponDelay: 3.0,
				Stats: model.StatBlock{Potency: 5}},
		},
		Unresolved: []string{"event_slot:3649577502"},
	}
	path := filepath.Join(t.TempDir(), "biffels-loadout.toml")
	require.NoError(t, Write(path, f))

	got, err := Read(path)
	require.NoError(t, err)
	require.Equal(t, f.CharacterName, got.CharacterName)
	require.InDelta(t, f.LastUpdate, got.LastUpdate, 1e-3)
	require.Len(t, got.Slots, 3)
	require.Equal(t, "Back", got.Slots[0].CatalogSlot)
	require.InDelta(t, 25, got.Slots[0].Stats.Haste, 1e-9)
	require.True(t, got.Slots[0].Optimizable)
	require.False(t, got.Slots[1].Optimizable)
	require.InDelta(t, 3.0, got.Slots[2].WeaponDelay, 1e-9)
	require.Equal(t, []string{"event_slot:3649577502"}, got.Unresolved)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/loadout/ -run TestFileRoundTrip -v`
Expected: FAIL — `undefined: File / SlotEntry / Write / Read`.

- [ ] **Step 3: Write minimal implementation** (append to `internal/loadout/loadout.go`)

```go
import (
	"os"

	"github.com/BurntSushi/toml"
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
)

// File is the on-disk imported loadout (characters/<name>-loadout.toml).
type File struct {
	CharacterName string      `toml:"character_name"`
	LastUpdate    float64     `toml:"last_update"`
	Slots         []SlotEntry `toml:"slots"`
	Unresolved    []string    `toml:"unresolved"` // "charSlot:itemID" census could not resolve
}

// SlotEntry is one equipped slot's resolved data (item base + filled adornments).
type SlotEntry struct {
	CatalogSlot string          `toml:"catalog_slot"` // item's census slot_list name; the Set Equipped key
	CharSlot    string          `toml:"char_slot"`    // census character-equipment slot name
	ItemID      int64           `toml:"item_id"`
	Name        string          `toml:"name"`
	Optimizable bool            `toml:"optimizable"`
	WeaponMin   float64         `toml:"weapon_min,omitempty"`
	WeaponMax   float64         `toml:"weapon_max,omitempty"`
	WeaponDelay float64         `toml:"weapon_delay,omitempty"`
	Stats       model.StatBlock `toml:"stats"`
}

// Write serializes the loadout file as TOML.
func Write(path string, f File) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	return toml.NewEncoder(out).Encode(f)
}

// Read parses a loadout file.
func Read(path string) (File, error) {
	var f File
	_, err := toml.DecodeFile(path, &f)
	return f, err
}
```

> `model.StatBlock` fields are exported and plain float64, so BurntSushi encodes them as a `[slots.stats]` sub-table keyed by field name (Haste, Potency, …). The round-trip test confirms this works without custom TOML tags. If the encoder rejects a slice-of-struct-with-submap layout, fall back to encoding `Stats` as `map[string]float64` (census keys) on `SlotEntry` and converting via `StatBlock.AddModifiers` at read time — but try the struct form first.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/loadout/ -run TestFileRoundTrip -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/loadout/loadout.go internal/loadout/loadout_test.go
git commit -m "feat(loadout): loadout file TOML round-trip"
```

---

## Task 7: Resolve equipment → loadout file

**Files:**
- Create: `internal/loadout/resolve.go`
- Test: `internal/loadout/resolve_test.go`

**Design:** `Resolve` is a pure function (no network): the caller passes a `Character`, a catalog lookup (`func(id int64) (census.Item, bool)`), an adornment-stat lookup (`func(id int64) (map[string]float64, bool)`), and an `optimizable` predicate (`func(catalogSlot string) bool`). `Resolve` returns the `File` plus the lists of item ids / adornment ids it could NOT find (so the command layer can fetch them and re-resolve). This keeps Census IO in the command (Task 8) and makes resolution fully unit-testable.

- [ ] **Step 1: Write the failing test**

```go
package loadout

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
)

func itemStats(mods map[string]float64) census.Item {
	m := map[string]census.Modifier{}
	for k, v := range mods {
		m[k] = census.Modifier{Value: v}
	}
	return census.Item{Modifiers: m}
}

func TestResolveSumsItemAndAdornments(t *testing.T) {
	ch := census.Character{
		DisplayName: "Biffels (Wuoshi)",
		LastUpdate:  123,
		EquipmentSlots: []census.EquipmentSlot{
			{Name: "cloak", Item: census.EquippedItem{ID: 264598753,
				Adornments: []census.Adornment{{Color: "white"}, {ID: 111}}}},
			{Name: "food", Item: census.EquippedItem{ID: 461060541}},
			{Name: "mount_armor", Item: census.EquippedItem{}},
		},
	}
	catalog := func(id int64) (census.Item, bool) {
		if id == 264598753 {
			it := itemStats(map[string]float64{"attackspeed": 25})
			it.ID = id
			it.DisplayName = "Cloak of Flames"
			it.Slots = []census.Slot{{Name: "Back"}}
			return it, true
		}
		return census.Item{}, false
	}
	adorn := func(id int64) (map[string]float64, bool) {
		if id == 111 {
			return map[string]float64{"critchance": 2}, true
		}
		return nil, false
	}
	optimizable := func(catalogSlot string) bool { return catalogSlot == "Back" }

	f, missItems, missAdorns := Resolve(ch, catalog, adorn, optimizable)

	require.Empty(t, missItems)
	require.Empty(t, missAdorns)
	require.Equal(t, "Biffels (Wuoshi)", f.CharacterName)
	require.Len(t, f.Slots, 1, "food + mount_armor skipped")
	cloak := f.Slots[0]
	require.Equal(t, "Back", cloak.CatalogSlot)
	require.Equal(t, "cloak", cloak.CharSlot)
	require.True(t, cloak.Optimizable)
	require.InDelta(t, 25, cloak.Stats.Haste, 1e-9)       // item base
	require.InDelta(t, 2, cloak.Stats.CritChance, 1e-9)   // + adornment
}

func TestResolveReportsMissing(t *testing.T) {
	ch := census.Character{EquipmentSlots: []census.EquipmentSlot{
		{Name: "head", Item: census.EquippedItem{ID: 999, Adornments: []census.Adornment{{ID: 888}}}},
	}}
	none := func(id int64) (census.Item, bool) { return census.Item{}, false }
	noneA := func(id int64) (map[string]float64, bool) { return nil, false }
	f, missItems, missAdorns := Resolve(ch, none, noneA, func(string) bool { return true })

	require.Equal(t, []int64{999}, missItems)
	require.Equal(t, []int64{888}, missAdorns)
	require.Empty(t, f.Slots, "unresolved item produces no slot entry")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/loadout/ -run TestResolve -v`
Expected: FAIL — `undefined: Resolve`.

- [ ] **Step 3: Write minimal implementation**

```go
package loadout

import (
	"fmt"

	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
)

// Resolve turns a fetched Character into a loadout File. It is pure: catalogLookup
// returns a cataloged census.Item by id; adornLookup returns an adornment's
// census-keyed stat grants by id; optimizable decides whether a catalog slot is a
// swap candidate. Ids not found are returned in missItems/missAdorns so the caller
// can fetch them and re-run Resolve. Skipped slots (food/drink/mount) and empty
// item sockets (ID 0) are omitted.
func Resolve(
	ch census.Character,
	catalogLookup func(int64) (census.Item, bool),
	adornLookup func(int64) (map[string]float64, bool),
	optimizable func(catalogSlot string) bool,
) (f File, missItems []int64, missAdorns []int64) {
	f.CharacterName = ch.DisplayName
	f.LastUpdate = ch.LastUpdate

	for _, slot := range ch.EquipmentSlots {
		if SkipSlot(slot.Name) || slot.Item.ID == 0 {
			continue
		}
		it, ok := catalogLookup(slot.Item.ID)
		if !ok {
			missItems = append(missItems, slot.Item.ID)
		}

		// adornment ids — record misses regardless of whether the item resolved
		adornStats := map[string]float64{}
		for _, aid := range slot.Item.FilledAdornmentIDs() {
			as, ok := adornLookup(aid)
			if !ok {
				missAdorns = append(missAdorns, aid)
				continue
			}
			for k, v := range as {
				adornStats[k] += v
			}
		}

		if !ok {
			continue // can't build a slot entry without the item; caller re-resolves
		}

		catalogSlot := ""
		if len(it.Slots) > 0 {
			catalogSlot = it.Slots[0].Name
		}
		var sb model.StatBlock
		mods := map[string]float64{}
		for k, m := range it.Modifiers {
			mods[k] += m.Value
		}
		for k, v := range adornStats {
			mods[k] += v
		}
		sb.AddModifiers(mods)

		f.Slots = append(f.Slots, SlotEntry{
			CatalogSlot: catalogSlot,
			CharSlot:    slot.Name,
			ItemID:      slot.Item.ID,
			Name:        string(it.DisplayName),
			Optimizable: optimizable(catalogSlot),
			WeaponMin:   it.TypeInfo.MinBaseDamage,
			WeaponMax:   it.TypeInfo.MaxBaseDamage,
			WeaponDelay: it.TypeInfo.Delay,
			Stats:       sb,
		})
	}
	return f, missItems, missAdorns
}

// MarkUnresolved records ids the caller still could not fetch after one re-resolve,
// so they surface in the file rather than being silently dropped.
func (f *File) MarkUnresolved(label string, ids []int64) {
	for _, id := range ids {
		f.Unresolved = append(f.Unresolved, fmt.Sprintf("%s:%d", label, id))
	}
}
```

> The static-grant stats on an item/adornment that come from `effect_list` (not the `modifiers` block) are handled at fetch time in Task 8: when the command fetches an uncataloged item or any adornment, it runs `catalog.ParseEffects(it.EffectList)` and merges those keys into the `modifiers` map it hands to the lookup. `Resolve` itself only reads `it.Modifiers`, so the command pre-folds effect grants into `Modifiers` (or into the adornment stat map) before calling `Resolve`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/loadout/ -run TestResolve -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/loadout/resolve.go internal/loadout/resolve_test.go
git commit -m "feat(loadout): pure Resolve (equipment -> loadout file)"
```

---

## Task 8: `itemdex import` command

**Files:**
- Modify: `cmd/itemdex/main.go`

**Behavior:** `itemdex import [--character …] [--out data] [--sid …]`. Loads the config (requires `census_name` + `world`), fetches the character, resolves against the existing catalog + `data/adornments.csv`; fetches missing items/adornments from census (running `ParseEffects` to fold effect-grant stats into modifiers); appends newly-fetched items to the item catalog CSVs and new adornments to `data/adornments.csv`; re-resolves; writes `characters/<census_name lowercased>-loadout.toml`; prints a `builddb` reminder if anything was appended; lists any still-unresolved ids.

- [ ] **Step 1: Write the failing test** — none at `cmd` level (no cmd tests in repo per recon). Instead add a focused unit test for the one piece of non-trivial logic that lives in a testable spot: the `optimizable(catalogSlot)` predicate. Put it in `internal/bis` where the candidate slot set is known (Task 9 covers it), and keep `runImport` as thin orchestration. Mark this step done by confirming `runImport` only wires already-tested functions.

- [ ] **Step 2: Add subcommand dispatch** at the top of `main()` in `cmd/itemdex/main.go`, before the existing `flag.Parse()` block:

```go
func main() {
	if len(os.Args) > 1 && os.Args[1] == "import" {
		runImport(os.Args[2:])
		return
	}
	// ... existing flag parsing + effects/normal modes unchanged ...
}
```

- [ ] **Step 3: Implement `runImport`** (new function in `cmd/itemdex/main.go`):

```go
func runImport(argv []string) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	character := fs.String("character", "characters/alex.toml", "character config (TOML)")
	dir := fs.String("out", "data", "catalog dir (appends newly-fetched items/adornments)")
	sid := fs.String("sid", "s:example", "Census service ID")
	_ = fs.Parse(argv)

	cfg, err := charconfig.Load(*character)
	if err != nil {
		fmt.Fprintln(os.Stderr, "import: config:", err)
		os.Exit(1)
	}
	if cfg.Character.CensusName == "" || cfg.Character.World == 0 {
		fmt.Fprintln(os.Stderr, "import: config needs [character] census_name and world")
		os.Exit(1)
	}

	c := census.New(*sid)
	ctx := context.Background()

	ch, err := census.FetchCharacter(ctx, c, cfg.Character.CensusName, cfg.Character.World)
	if err != nil {
		fmt.Fprintln(os.Stderr, "import: fetch character:", err)
		os.Exit(1)
	}

	// Catalog lookups seeded from the CSV cache + adornments.csv.
	cat := loadCatalogIndex(*dir)          // map[int64]census.Item from source.LoadCache
	adorns := loadAdornmentIndex(*dir)     // map[int64]map[string]float64 from adornments.csv
	optimizable := bis.OptimizableSlot     // predicate over catalog slot name (Task 9)

	catLookup := func(id int64) (census.Item, bool) { it, ok := cat[id]; return it, ok }
	adornLookup := func(id int64) (map[string]float64, bool) { s, ok := adorns[id]; return s, ok }

	f, missItems, missAdorns := loadout.Resolve(ch, catLookup, adornLookup, optimizable)

	// Fetch misses once, fold effect grants, append to catalog, re-resolve.
	var appendedItems, appendedAdorns int
	if len(missItems) > 0 {
		fetched, err := census.FetchItemsByIDs(ctx, c, missItems)
		if err != nil {
			fmt.Fprintln(os.Stderr, "import: fetch items:", err)
			os.Exit(1)
		}
		for _, it := range fetched {
			foldEffectGrants(&it)            // merge ParseEffects stat grants into it.Modifiers
			cat[it.ID] = it
		}
		appendedItems = appendItemsToCatalog(*dir, fetched) // append to weapons/armor/jewelry CSVs by category
	}
	if len(missAdorns) > 0 {
		fetched, err := census.FetchItemsByIDs(ctx, c, missAdorns)
		if err != nil {
			fmt.Fprintln(os.Stderr, "import: fetch adornments:", err)
			os.Exit(1)
		}
		var newRows []catalog.Adornment
		for _, it := range fetched {
			stats := adornStats(it)          // modifiers + ParseEffects grants -> census-key map
			adorns[it.ID] = stats
			newRows = append(newRows, catalog.Adornment{ID: it.ID, Name: string(it.DisplayName), Stats: stats})
		}
		appendedAdorns = appendAdornments(*dir, newRows)
	}

	// Re-resolve with the now-fuller indexes; anything still missing is unresolved.
	f, missItems, missAdorns = loadout.Resolve(ch, catLookup, adornLookup, optimizable)
	f.MarkUnresolved("item", missItems)
	f.MarkUnresolved("adornment", missAdorns)

	outPath := filepath.Join("characters", strings.ToLower(cfg.Character.CensusName)+"-loadout.toml")
	if err := loadout.Write(outPath, f); err != nil {
		fmt.Fprintln(os.Stderr, "import: write loadout:", err)
		os.Exit(1)
	}

	fmt.Printf("wrote %s (%d slots; %d unresolved)\n", outPath, len(f.Slots), len(f.Unresolved))
	if appendedItems+appendedAdorns > 0 {
		fmt.Printf("appended %d items and %d adornments to %s/ — run `builddb` before `bis`\n",
			appendedItems, appendedAdorns, *dir)
	}
}
```

Helper functions to implement in the same file (each thin, wrapping tested pieces):
- `loadCatalogIndex(dir)` → call `source.LoadCache(dir)`, index by `it.ID`.
- `loadAdornmentIndex(dir)` → read `data/adornments.csv` via `catalog.ReadAdornmentsCSV`, index `id → Stats`.
- `foldEffectGrants(it *census.Item)` → `stats, _, _ := catalog.ParseEffects(it.EffectList)`; for each `k,v` ensure `it.Modifiers[k]` includes `v` (additive; create `Modifier{Value: existing+v}`).
- `adornStats(it census.Item)` → start from `it.Modifiers` values, add `ParseEffects` grants; return `map[string]float64`.
- `appendItemsToCatalog(dir, items)` → group by category (reuse the same categorization `source.FreshPull` uses for weapons/armor/jewelry), append rows to the matching CSV via `catalog.WriteCSV` semantics (read existing, union, rewrite), return count appended. Reuse existing `source` helpers if one already rewrites a category file; otherwise add a small `source.AppendItems(dir, items)`.
- `appendAdornments(dir, rows)` → read existing `adornments.csv`, merge by id, rewrite via `catalog.WriteAdornmentsCSV`, return count of new ids.

- [ ] **Step 4: Build + manual smoke**

Run: `go build ./...`
Expected: compiles.

Manual (network — optional, requires `census_name`/`world` in a config): set `census_name = "Biffels"`, `world = 618` in `characters/alex.toml`, then:
```bash
go run ./cmd/itemdex import --character characters/alex.toml --out data
```
Expected: writes `characters/biffels-loadout.toml` with ~21 slots, cloak = Cloak of Flames (id 264598753) with `Haste` in its stats, and a `builddb` reminder if items were appended.

- [ ] **Step 5: Commit**

```bash
git add cmd/itemdex/main.go
git commit -m "feat(itemdex): import subcommand -> loadout file"
```

---

## Task 9: `OptimizableSlot` predicate + `SetFromLoadout`

**Files:**
- Create: `internal/bis/loadoutset.go`
- Test: `internal/bis/loadoutset_test.go`

- [ ] **Step 1: Write the failing test**

```go
package bis

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/amdrake93/eq2-eof-itemdex/internal/loadout"
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
)

func TestOptimizableSlot(t *testing.T) {
	for _, s := range []string{"Head", "Chest", "Finger", "Ear", "Wrist", "Back", "Waist", "Primary", "Secondary"} {
		require.True(t, OptimizableSlot(s), s)
	}
	for _, s := range []string{"Charm", "Ranged", "Ammo", "Food"} {
		require.False(t, OptimizableSlot(s), s)
	}
}

func TestSetFromLoadoutCountsFixedStats(t *testing.T) {
	f := loadout.File{
		Slots: []loadout.SlotEntry{
			{CatalogSlot: "Back", ItemID: 1, Name: "Cloak", Optimizable: true, Stats: model.StatBlock{Haste: 25}},
			{CatalogSlot: "Charm", ItemID: 2, Name: "Clicky", Optimizable: false, Stats: model.StatBlock{CritChance: 3}},
		},
	}
	lo := store.Loadout{Main: model.Weapon{AvgDamage: 160, DelaySecs: 4}}
	set, optimizable := SetFromLoadout(f, model.StatBlock{}, lo, 1.0, 600)

	// both items' stats are in the equipped set
	require.Contains(t, set.Equipped, "Back")
	require.Contains(t, set.Equipped, "Charm")
	// only the optimizable catalog slots are returned for re-optimization
	require.Equal(t, map[string]bool{"Back": true}, optimizable)
	require.Greater(t, set.DPS(), 0.0)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bis/ -run "TestOptimizableSlot|TestSetFromLoadout" -v`
Expected: FAIL — `undefined: OptimizableSlot / SetFromLoadout`.

- [ ] **Step 3: Write minimal implementation**

```go
package bis

import (
	"github.com/amdrake93/eq2-eof-itemdex/internal/loadout"
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
)

// optimizableCatalogSlots are the catalog (census item slot_list) slot names that
// have a candidate pool — the slots bis can suggest upgrades for. Ranged/Ammo/
// Charm/event carry stats on import but are never swap candidates (SPEC §7, §16).
var optimizableCatalogSlots = map[string]bool{
	"Primary": true, "Secondary": true, "Head": true, "Chest": true,
	"Shoulder": true, "Shoulders": true, "Forearms": true, "Hand": true, "Hands": true,
	"Leg": true, "Legs": true, "Foot": true, "Feet": true, "Finger": true,
	"Ear": true, "Wrist": true, "Neck": true, "Back": true, "Waist": true,
}

// OptimizableSlot reports whether a catalog slot is a bis swap candidate.
func OptimizableSlot(catalogSlot string) bool { return optimizableCatalogSlots[catalogSlot] }

// SetFromLoadout builds a Set with every kept slot's stats equipped (fixed +
// optimizable alike), and returns the set of catalog slots eligible for
// re-optimization. The Primary/Secondary weapon entries override lo.Main / the
// off-hand weapon when present.
func SetFromLoadout(f loadout.File, profile model.StatBlock, lo store.Loadout, autoMult, fightLen float64) (*Set, map[string]bool) {
	set := NewSet(profile, lo, autoMult, fightLen)
	optimizable := map[string]bool{}
	for _, e := range f.Slots {
		it := store.ScorableItem{
			ID:    int(e.ItemID),
			Name:  e.Name,
			Slot:  e.CatalogSlot,
			Stats: e.Stats,
		}
		if e.WeaponDelay > 0 {
			it.WeaponMin = e.WeaponMin
			it.WeaponMax = e.WeaponMax
			it.WeaponAvg = (e.WeaponMin + e.WeaponMax) / 2
			it.WeaponDelay = e.WeaponDelay
		}
		set.Equipped[e.CatalogSlot] = append(set.Equipped[e.CatalogSlot], it)
		if e.Optimizable {
			optimizable[e.CatalogSlot] = true
		}
		if e.CatalogSlot == "Primary" && it.IsWeapon() {
			set.Main = model.Weapon{AvgDamage: it.WeaponAvg, MinDamage: it.WeaponMin, MaxDamage: it.WeaponMax, DelaySecs: it.WeaponDelay}
		}
	}
	return set, optimizable
}
```

> Verify against `internal/bis/set.go` how `Set.DPS()` reads the off-hand (`offWeapon()`): the imported `Secondary` weapon must land in `Equipped["Secondary"]` for `offWeapon()` to pick it up. The recon shows `offWeapon()`/`restOff` derive from `Equipped[offHandSlot]`, so appending the Secondary ScorableItem (with weapon fields set) is sufficient — confirm and add an assertion if `DPS()` should reflect the imported off-hand delay.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bis/ -run "TestOptimizableSlot|TestSetFromLoadout" -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/bis/loadoutset.go internal/bis/loadoutset_test.go
git commit -m "feat(bis): SetFromLoadout + OptimizableSlot"
```

---

## Task 10: `bis --loadout` report path

**Files:**
- Modify: `cmd/bis/main.go`

**Behavior:** add `--loadout ""` flag. When set, instead of the from-scratch tier runs, build a Set from the loadout file and emit the four uses: (1) current-set DPS, (2) per-optimizable-slot best alternative ΔDPS (ranked), (3) seed-optimization (`BuildSet` seeded from the imported set), (4) the absolute DPS for parse validation. Reuse existing report rendering where possible.

- [ ] **Step 1: Add the flag** (in the flag block of `cmd/bis/main.go`):

```go
loadoutPath := flag.String("loadout", "", "imported loadout file to sim from (itemdex import output)")
```

- [ ] **Step 2: Branch into import-mode** after config/DB/loadout load, before the tier loop:

```go
if *loadoutPath != "" {
	runLoadoutReport(db, cfg, classData, lo, raid, items, *loadoutPath, *out, *topN, *fight)
	return
}
```

- [ ] **Step 3: Implement `runLoadoutReport`** (new function in `cmd/bis/main.go`):

```go
func runLoadoutReport(db *store.DB, cfg charconfig.Config, classData charconfig.ClassData, lo store.Loadout,
	profile model.StatBlock, items []store.ScorableItem, loadoutPath, out string, topN int, fight float64) {

	f, err := loadout.Read(loadoutPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bis: read loadout:", err)
		os.Exit(1)
	}

	set, optimizable := bis.SetFromLoadout(f, profile, lo, classData.AutoAttackMultiplier, fight)

	// (1) + (4) current-set / validation absolute DPS
	current := set.DPS()

	// (2) what to upgrade next: best catalog alternative per optimizable slot
	bySlot := bis.SlotCandidates(items, func(it store.ScorableItem) bool { return !bis.IsAvatar(it) && !bis.IsHunters(it) && !bis.Curated(it) })
	type upgrade struct {
		Slot    string
		Best    string
		DeltaTo float64
	}
	var upgrades []upgrade
	for slot := range optimizable {
		var bestName string
		var bestDelta float64
		for _, cand := range bySlot[slot] {
			d := set.CandidateDelta(slot, cand) // delta vs the currently-equipped set
			if d > bestDelta {
				bestDelta, bestName = d, cand.Name
			}
		}
		if bestName != "" {
			upgrades = append(upgrades, upgrade{Slot: slot, Best: bestName, DeltaTo: bestDelta})
		}
	}
	sort.Slice(upgrades, func(i, j int) bool { return upgrades[i].DeltaTo > upgrades[j].DeltaTo })

	// (3) seed optimization from the imported set: lock fixed slots, optimize the rest
	locked := map[string][]store.ScorableItem{}
	for slot, eq := range set.Equipped {
		if !optimizable[slot] {
			locked[slot] = eq
		}
	}
	seeded := bis.BuildSet(profile, lo, bySlot, locked, maxBuildPasses, classData.AutoAttackMultiplier, fight)

	// Render a focused markdown report.
	md := renderLoadoutReport(f, current, seeded.DPS(), upgrades, topN)
	if err := os.WriteFile(out, []byte(md), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "bis: write report:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (current set %.0f DPS, %d unresolved imports)\n", out, current, len(f.Unresolved))
	if len(f.Unresolved) > 0 {
		fmt.Printf("unresolved (stats not counted): %v\n", f.Unresolved)
	}
}
```

`renderLoadoutReport(f, current, seededDPS, upgrades, topN)` builds a small markdown string: a header with `f.CharacterName` and `last_update`, the current-set DPS, a ranked "what to upgrade next" table (`Slot | currently equipped | best alternative | +ΔDPS`), the seeded-optimization DPS and its delta over current, and an "unresolved imports" footnote if `f.Unresolved` is non-empty. Keep it self-contained (don't force it through the tier `bis.Render` shape).

> The exact `runLoadoutReport` parameter list must match how `main()` already holds these values (recon: `cfg`, `classData`, `lo`, `raid`/`solo` blocks, `items`, `maxBuildPasses`). Thread the same variables `main()` already computed; do not recompute config/DB.

- [ ] **Step 4: Build + manual smoke**

Run: `go build ./...`
Expected: compiles.

Manual (after Task 8 produced a loadout file and a `builddb` run if items were appended):
```bash
go run ./cmd/bis --loadout characters/biffels-loadout.toml --out loadout-report.md
```
Expected: prints current-set DPS, writes a report with the upgrade table; Cloak of Flames present in the imported set.

- [ ] **Step 5: Commit**

```bash
git add cmd/bis/main.go
git commit -m "feat(bis): --loadout import-mode report (score/upgrade/seed/validate)"
```

---

## Task 11: Spec + design-doc sync, full test pass

**Files:**
- Modify: `docs/SPEC.md` (flip the relevant "(Planned)" tags to implemented where code now exists; align command name to `itemdex import`)
- Modify: `docs/plans/2026-06-24-character-gear-import-design.md` (status → implemented)

- [ ] **Step 1: Run the full suite + build**

Run: `go test ./... && go build ./...`
Expected: all packages `ok`, build clean.

- [ ] **Step 2: Update SPEC.md** — for each subsection that now matches shipped code (§3 import query, §4 adornment catalog + loadout file, §6 config fields, §7 imported loadout, §8 commands), remove the "(Planned)"/"Not yet implemented" markers and update any signature names to match the final code. Leave §16 deferrals (adornment optimization, food/drink) as-is — those remain not built.

- [ ] **Step 3: Update the design doc status** line to `Implemented YYYY-MM-DD (plan 2w)`.

- [ ] **Step 4: Commit**

```bash
git add docs/SPEC.md docs/plans/2026-06-24-character-gear-import-design.md
git commit -m "docs: sync SPEC + design doc for gear import (plan 2w)"
```

---

## Self-Review

**Spec coverage check (against SPEC §3/§4/§6/§7/§8/§16 Planned subsections):**
- §3 character import query → Tasks 1–2 ✓
- §4 adornment catalog (`data/adornments.csv`) → Task 4 ✓; loadout file artifact → Task 6 ✓; fetch-on-demand + append + unresolved reporting → Tasks 7–8 ✓
- §6 `census_name`/`world` config → Task 3 ✓
- §7 imported loadout Set, optimizable/fixed/skip, weapon override, four uses → Tasks 5, 9, 10 ✓
- §8 `itemdex import` + `bis --loadout` → Tasks 8, 10 ✓
- §16 deferrals (adornment optimization, food/drink) → enforced by skip-list (Task 5) + optimizable predicate (Task 9); documented, not built ✓

**Placeholder scan:** No "TBD"/"handle edge cases" left. Two flagged verification notes (Task 6 TOML struct encoding; Task 9 off-hand `offWeapon()` wiring) are explicit "confirm against existing code and add assertion" steps with a concrete fallback, not open placeholders.

**Type consistency:** `census.Character`/`EquipmentSlot`/`EquippedItem`/`Adornment`/`FilledAdornmentIDs` (T1) reused in T2/T7. `loadout.File`/`SlotEntry`/`Resolve`/`Write`/`Read`/`SkipSlot`/`MarkUnresolved` (T5–7) reused in T8–10. `catalog.Adornment`/`WriteAdornmentsCSV`/`ReadAdornmentsCSV` (T4) reused in T8. `bis.OptimizableSlot`/`SetFromLoadout` (T9) reused in T8/T10. `store.ScorableItem` fields (ID,Name,Slot,Stats,WeaponMin/Max/Avg/Delay) and `store.Loadout{Main,Arts}` per recon. `model.StatBlock.AddModifiers` per recon.

**Known integration risks to verify during execution (called out inline):** (a) BurntSushi encoding of `[]SlotEntry` with embedded `model.StatBlock` sub-table — fallback to `map[string]float64` on SlotEntry; (b) `Set.offWeapon()` picking up the imported Secondary; (c) `itemShowFields` constant location; (d) category append helper may need a small `source.AppendItems`.
