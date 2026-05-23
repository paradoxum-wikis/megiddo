package catalogue

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

type Index struct {
	FormatVersion int            `json:"format_version"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Characters    []CharacterRef `json:"characters"`
}

type CharacterRef struct {
	ID   string `json:"id"`
	File string `json:"file"`
}

type CharacterCatalog struct {
	Character             string           `json:"character"`
	Model                 string           `json:"model,omitempty"`
	Description           string           `json:"description,omitempty"`
	ReplacementsCatalogue []CatalogueEntry `json:"replacements_catalogue"`
}

func LoadMerged(root fs.FS) (*Snapshot, error) {
	indexData, err := fs.ReadFile(root, "index.json")
	if err != nil {
		return nil, fmt.Errorf("read catalogue index: %w", err)
	}
	var idx Index
	if err := json.Unmarshal(indexData, &idx); err != nil {
		return nil, fmt.Errorf("parse catalogue index: %w", err)
	}
	if idx.FormatVersion != supportedFormatVersion {
		return nil, fmt.Errorf("unsupported catalogue format_version=%d (want %d)", idx.FormatVersion, supportedFormatVersion)
	}
	out := &Snapshot{
		FormatVersion:         idx.FormatVersion,
		Name:                  idx.Name,
		Description:           idx.Description,
		ReplacementsCatalogue: nil,
	}
	for _, ch := range idx.Characters {
		if ch.File == "" {
			continue
		}
		file := ch.File
		if !strings.HasSuffix(strings.ToLower(file), ".json") {
			file += ".json"
		}
		raw, err := fs.ReadFile(root, file)
		if err != nil {
			return nil, fmt.Errorf("read character catalogue %q: %w", file, err)
		}
		var cc CharacterCatalog
		if err := json.Unmarshal(raw, &cc); err != nil {
			return nil, fmt.Errorf("parse character catalogue %q: %w", file, err)
		}
		model := strings.TrimSpace(cc.Model)
		if model == "" {
			model = "Default"
		}
		for i := range cc.ReplacementsCatalogue {
			if strings.TrimSpace(cc.ReplacementsCatalogue[i].Model) == "" {
				cc.ReplacementsCatalogue[i].Model = model
			}
		}
		out.ReplacementsCatalogue = append(out.ReplacementsCatalogue, cc.ReplacementsCatalogue...)
	}
	return out, nil
}

func LoadMergedDir(embedRoot fs.FS, dir string) (*Snapshot, error) {
	sub, err := fs.Sub(embedRoot, path.Clean(dir))
	if err != nil {
		return nil, err
	}
	return LoadMerged(sub)
}
