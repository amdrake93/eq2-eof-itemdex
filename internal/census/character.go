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
