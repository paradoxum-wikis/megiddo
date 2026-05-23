package pack

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"megiddo/internal/replacement"
)

func TestValidateAndSwap_nilPack(t *testing.T) {
	var p *Pack
	err := p.ValidateAndSwap(context.Background(), replacement.NewMap())
	if err == nil || err.Error() != "nil pack" {
		t.Fatalf("expected nil pack error, got %v", err)
	}
}

func TestApplyValidated_nilPack(t *testing.T) {
	var p *Pack
	_, err := p.ApplyValidated(context.Background())
	if err == nil || err.Error() != "nil pack" {
		t.Fatalf("expected nil pack error, got %v", err)
	}
}

func TestReplacementTable_fileEntry(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "tex.ktx2")
	if err := os.WriteFile(fp, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	slot := 0
	p := &Pack{
		Replacements: []Replacement{{
			Label:           "tex",
			AssetType:       "texturepack",
			Slot:            &slot,
			TargetID:        42,
			ReplaceWithFile: fp,
		}},
	}
	tab, err := p.ReplacementTable()
	if err != nil {
		t.Fatal(err)
	}
	got := tab[replacement.Key{AssetID: 42, SlotIndex: -1}]
	if got.FilePath != fp {
		t.Fatalf("expected file entry %q, got %+v", fp, got)
	}
}

func TestReplacementTable_relativeFileRejected(t *testing.T) {
	slot := 0
	p := &Pack{
		Replacements: []Replacement{{
			Label:           "tex",
			AssetType:       "texturepack",
			Slot:            &slot,
			TargetID:        42,
			ReplaceWithFile: "assets/tex.ktx2",
		}},
	}
	if _, err := p.ReplacementTable(); err == nil {
		t.Fatal("expected error for relative path")
	}
}

func TestReplacementTable_missingFile(t *testing.T) {
	p := &Pack{
		Replacements: []Replacement{{
			Label:           "missing",
			AssetType:       "image",
			TargetID:        7,
			ReplaceWithFile: filepath.Join(t.TempDir(), "nope.png"),
		}},
	}
	if _, err := p.ReplacementTable(); err == nil {
		t.Fatal("expected stat error for missing file")
	}
}

func TestReplacementTable_texturepackFileMustBeKTX2(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "tex.png")
	if err := os.WriteFile(fp, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	slot := 0
	p := &Pack{
		Replacements: []Replacement{{
			Label:           "tex",
			AssetType:       "texturepack",
			Slot:            &slot,
			TargetID:        42,
			ReplaceWithFile: fp,
		}},
	}
	if _, err := p.ReplacementTable(); err == nil {
		t.Fatal("expected extension validation error")
	}
}

func TestReplacementTable_meshReplaceWithZeroAllowed(t *testing.T) {
	p := &Pack{
		Replacements: []Replacement{{
			Label:       "Ears Ant_. Mesh",
			AssetType:   "mesh",
			TargetID:    121117322453337,
			ReplaceWith: 0,
		}},
	}
	tab, err := p.ReplacementTable()
	if err != nil {
		t.Fatal(err)
	}
	got := tab[replacement.Key{AssetID: 121117322453337, SlotIndex: -1}]
	if !got.IsRemove() {
		t.Fatalf("expected remove entry for mesh, got %+v", got)
	}
}

func TestReplacementTable_texturepackReplaceWithZeroBlankKTX(t *testing.T) {
	slot := 0
	p := &Pack{
		Replacements: []Replacement{{
			Label:       "Coat_ ColorMap",
			AssetType:   "texturepack",
			Slot:        &slot,
			TargetID:    83377379455927,
			ReplaceWith: 0,
		}},
	}
	tab, err := p.ReplacementTable()
	if err != nil {
		t.Fatal(err)
	}
	got := tab[replacement.Key{AssetID: 83377379455927, SlotIndex: -1}]
	if !got.IsFile() || got.FilePath == "" {
		t.Fatalf("expected blank ktx file entry, got %+v", got)
	}
}

func TestReplacementTable_conflictingTexturePackSlot(t *testing.T) {
	slot := 1
	p := &Pack{
		Replacements: []Replacement{
			{
				Label:       "first",
				AssetType:   "texturepack",
				Slot:        &slot,
				TargetID:    123,
				ReplaceWith: 456,
			},
			{
				Label:       "second",
				AssetType:   "TexturePack",
				Slot:        &slot,
				TargetID:    123,
				ReplaceWith: 789,
			},
		},
	}
	_, err := p.ReplacementTable()
	if err == nil {
		t.Fatal("expected conflicting replacements error")
	}
}
