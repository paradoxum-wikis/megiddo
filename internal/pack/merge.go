package pack

import (
	"fmt"
	"strings"

	"megiddo/internal/replacement"
)

func MergePacks(packs []*Pack) (*Pack, error) {
	if len(packs) == 0 {
		return nil, fmt.Errorf("no packs to merge")
	}
	var out Pack
	out.FormatVersion = supportedFormatVersion

	parts := make([]string, 0, len(packs))
	lastByKey := map[replacement.Key]int{}
	for i, p := range packs {
		if p == nil {
			continue
		}
		if i == 0 {
			out.Author = p.Author
			out.Version = p.Version
			out.Description = p.Description
		}
		n := strings.TrimSpace(p.Name)
		if n == "" {
			n = fmt.Sprintf("pack_%d", i+1)
		}
		parts = append(parts, n)
		for _, r := range p.Replacements {
			k, err := replacementKeyForRow(r)
			if err != nil {
				return nil, err
			}
			if idx, ok := lastByKey[k]; ok {
				out.Replacements[idx] = r // later pack wins
				continue
			}
			lastByKey[k] = len(out.Replacements)
			out.Replacements = append(out.Replacements, r)
		}
	}
	if len(parts) == 1 {
		out.Name = parts[0]
	} else {
		out.Name = "Merged: " + strings.Join(parts, " + ")
	}
	return &out, nil
}

func replacementKeyForRow(r Replacement) (replacement.Key, error) {
	at := strings.ToLower(strings.TrimSpace(r.AssetType))
	switch at {
	case "texturepack":
		if r.Slot == nil {
			return replacement.Key{}, fmt.Errorf("texturepack %q (%d) missing slot column", r.Label, r.TargetID)
		}
		if *r.Slot < 0 {
			return replacement.Key{}, fmt.Errorf("texturepack slot may not be negative (asset %d)", r.TargetID)
		}
		if r.TargetID == 0 {
			return replacement.Key{}, fmt.Errorf("texturepack %q: target_id must be non-zero", r.Label)
		}
		return replacement.Key{AssetID: r.TargetID, SlotIndex: -1}, nil
	default:
		return replacement.Key{AssetID: r.TargetID, SlotIndex: -1}, nil
	}
}

