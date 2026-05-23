package pack

import "testing"

func TestMergePacks_LaterOverrides(t *testing.T) {
	slot := 0
	a := &Pack{
		FormatVersion: 1,
		Name:          "A",
		Replacements: []Replacement{
			{Label: "a", AssetType: "texturepack", Slot: &slot, TargetID: 123, ReplaceWith: 111},
		},
	}
	b := &Pack{
		FormatVersion: 1,
		Name:          "B",
		Replacements: []Replacement{
			{Label: "b", AssetType: "texturepack", Slot: &slot, TargetID: 123, ReplaceWith: 222},
		},
	}
	m, err := MergePacks([]*Pack{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Replacements) != 1 {
		t.Fatalf("rows=%d", len(m.Replacements))
	}
	if m.Replacements[0].ReplaceWith != 222 {
		t.Fatalf("want later override got %d", m.Replacements[0].ReplaceWith)
	}
}

