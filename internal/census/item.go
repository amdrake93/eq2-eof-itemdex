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
	DamageRating  float64             `json:"damagerating"`
	Delay         float64             `json:"delay"`
	MinBaseDamage float64             `json:"minbasedamage"`
	MaxBaseDamage float64             `json:"maxbasedamage"`
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
