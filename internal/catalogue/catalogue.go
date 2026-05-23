package catalogue

import (
	"encoding/json"
)

const supportedFormatVersion = 1

type Snapshot struct {
	FormatVersion         int              `json:"format_version"`
	Name                  string           `json:"name"`
	Description           string           `json:"description"`
	ReplacementsCatalogue []CatalogueEntry `json:"replacements_catalogue"`
}

type RbxmProps struct {
	DumpLabel    string `json:"dump_label,omitempty"`
	InstancePath string `json:"instance_path,omitempty"`
	ClassName    string `json:"class_name,omitempty"`
	Prop         string `json:"prop,omitempty"`
}

type CatalogueEntry struct {
	Label     string     `json:"label"`
	Character any        `json:"character"`
	Model     string     `json:"model,omitempty"`
	Role      string     `json:"role"`
	AssetType string     `json:"asset_type"`
	Slot      *int       `json:"slot,omitempty"`
	TargetID  int64      `json:"target_id"`
	Notes     string     `json:"notes,omitempty"`
	RbxmProps *RbxmProps `json:"rbxm_props,omitempty"`
}

func Parse(data []byte) (*Snapshot, error) {
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}
