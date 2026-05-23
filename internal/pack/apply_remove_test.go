package pack

import (
	"strings"
	"testing"

	"megiddo/internal/replacement"
)

func TestReplacementTable_clear(t *testing.T) {
	tab, err := (&Pack{Replacements: []Replacement{{
		Label: "x", AssetType: "texture", TargetID: 123, ReplaceWith: 0,
	}}}).ReplacementTable()
	if err != nil {
		t.Fatal(err)
	}
	if e := tab[replacement.Key{AssetID: 123, SlotIndex: -1}]; !e.IsRemove() {
		t.Fatalf("got %+v", e)
	}
}

func TestReplacementTable_clearTexpack(t *testing.T) {
	slot := 0
	tab, err := (&Pack{Replacements: []Replacement{{
		Label: "x", AssetType: "texturepack", Slot: &slot, TargetID: 456, ReplaceWith: 0,
	}}}).ReplacementTable()
	if err != nil {
		t.Fatal(err)
	}
	e := tab[replacement.Key{AssetID: 456, SlotIndex: -1}]
	if !e.IsFile() || !strings.HasSuffix(e.FilePath, "blank.ktx2") {
		t.Fatalf("got %+v", e)
	}
}
