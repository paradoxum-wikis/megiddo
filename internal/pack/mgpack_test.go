package pack

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileID(t *testing.T) {
	if got := ProfileID("Shadow Pack", "Alice"); got != "Shadow_Pack_Alice" {
		t.Fatalf("got %q", got)
	}
	if got := ProfileID("Solo", ""); got != "Solo" {
		t.Fatalf("got %q", got)
	}
}

func TestWriteMgpack_roundtrip(t *testing.T) {
	dir := t.TempDir()
	asset := filepath.Join(dir, "tex.ktx2")
	if err := os.WriteFile(asset, []byte("ktx2data"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &Pack{
		FormatVersion: 1,
		Name:          "Test Pack",
		Author:        "Tester",
		Replacements: []Replacement{
			{
				Label:           "row",
				AssetType:       "texturepack",
				TargetID:        1,
				Slot:            intPtr(0),
				ReplaceWithFile: asset,
			},
		},
	}
	var buf bytes.Buffer
	if err := WriteMgpack(&buf, p); err != nil {
		t.Fatal(err)
	}
	mgpack := filepath.Join(dir, "test.mgpack")
	if err := os.WriteFile(mgpack, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	id, peek, err := PeekProfileID(mgpack)
	if err != nil || id != "Test_Pack_Tester" || peek.Name != "Test Pack" {
		t.Fatalf("peek: id=%q err=%v peek=%+v", id, err, peek)
	}
	packsRoot := filepath.Join(dir, "packs")
	installDir, err := InstallMgpack(mgpack, packsRoot, id, true)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadInstalled(packsRoot, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Replacements) != 1 {
		t.Fatalf("rows=%d", len(loaded.Replacements))
	}
	fp := loaded.Replacements[0].ReplaceWithFile
	if _, err := os.Stat(fp); err != nil {
		t.Fatalf("asset path %s: %v", fp, err)
	}
	rel, err := filepath.Rel(installDir, fp)
	if err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("want under %s got %s", installDir, fp)
	}

	zr, err := zip.OpenReader(mgpack)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	if len(names) < 2 {
		t.Fatalf("zip entries: %v", names)
	}
}

func intPtr(n int) *int { return &n }
