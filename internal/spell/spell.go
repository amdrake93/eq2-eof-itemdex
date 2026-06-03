package spell

import (
	"encoding/json"
	"fmt"
)

type Effect struct {
	Description string `json:"description"`
}

type ClassReq struct {
	DisplayName string `json:"displayname"`
	ID          int    `json:"id"`
	Level       int    `json:"level"`
}

// Spell is the subset of the Census spell record this project needs.
type Spell struct {
	Name               string              `json:"name"`
	NameLower          string              `json:"name_lower"`
	Level              int                 `json:"level"`
	TierName           string              `json:"tier_name"`
	Type               string              `json:"type"`
	Beneficial         int                 `json:"beneficial"`
	CastSecsHundredths int                 `json:"cast_secs_hundredths"`
	RecastSecs         float64             `json:"recast_secs"`
	Classes            map[string]ClassReq `json:"classes"`
	Effects            []Effect            `json:"effect_list"`
}

type spellListResponse struct {
	Spells    []Spell `json:"spell_list"`
	Returned  int     `json:"returned"`
	ErrorCode string  `json:"errorCode"`
	Error     string  `json:"error"`
}

// DecodeSpells parses a Census spell_list payload, surfacing API error envelopes.
func DecodeSpells(body []byte) ([]Spell, error) {
	var r spellListResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	if r.ErrorCode != "" || r.Error != "" {
		return nil, fmt.Errorf("census error: %s%s", r.ErrorCode, r.Error)
	}
	return r.Spells, nil
}
