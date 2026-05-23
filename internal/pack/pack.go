package pack

import (
	"encoding/json"
	"fmt"
)

const supportedFormatVersion = 1

type Pack struct {
	FormatVersion int           `json:"format_version"`
	Name          string        `json:"name"`
	Author        string        `json:"author"`
	Version       string        `json:"version"`
	Description   string        `json:"description"`
	Replacements  []Replacement `json:"replacements"`
}

type Replacement struct {
	Label           string `json:"label"`
	Character       any    `json:"character"`
	Role            string `json:"role"`
	AssetType       string `json:"asset_type"`
	Slot            *int   `json:"slot"`
	TargetID        int64  `json:"target_id"`
	ReplaceWith     int64  `json:"replace_with,omitempty"`
	ReplaceWithFile string `json:"replace_with_file,omitempty"`
}

func ParseJSON(data []byte) (*Pack, error) {
	var p Pack
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	if p.FormatVersion != supportedFormatVersion {
		return nil, fmt.Errorf("unsupported format_version=%d (want %d)", p.FormatVersion, supportedFormatVersion)
	}
	return &p, nil
}
