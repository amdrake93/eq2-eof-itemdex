package census

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
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

type charTypeRaw struct {
	Class FlexString `json:"class"`
	Level int        `json:"level"`
}

type characterRaw struct {
	DisplayName    FlexString      `json:"displayname"`
	Type           charTypeRaw     `json:"type"`
	LastUpdate     float64         `json:"last_update"`
	EquipmentSlots []EquipmentSlot `json:"equipmentslot_list"`
}

type characterListResponse struct {
	Characters []characterRaw `json:"character_list"`
	Returned   int            `json:"returned"`
	ErrorCode  string         `json:"errorCode"`
	Error      string         `json:"error"`
}

const characterShowFields = "displayname,type.class,type.level,last_update,equipmentslot_list"

// ItemShowFields is the c:show field set for a full item fetch — the single source
// of truth shared by the catalog paging in internal/extract and FetchItemsByIDs, so
// items pulled either way carry identical fields.
const ItemShowFields = "displayname,id,tier,itemlevel,gamelink,slot_list,typeinfo,modifiers,effect_list,_extended.discovered.world_list"

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
	q.Set("c:show", ItemShowFields)
	body, err := c.Get(ctx, "get", "item", q.Encode())
	if err != nil {
		return nil, err
	}
	return DecodeItems(body)
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
