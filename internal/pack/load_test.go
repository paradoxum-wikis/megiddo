package pack

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFile_resolvesRelativeReplaceWithFile(t *testing.T) {
	dir := t.TempDir()
	asset := filepath.Join(dir, "tex.ktx2")
	if err := os.WriteFile(asset, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	pj := filepath.Join(dir, "pack.json")
	body := `{
		"format_version": 1,
		"replacements": [
			{
				"label": "tex",
				"role": "alter_ego",
				"asset_type": "texturepack",
				"slot": 0,
				"target_id": 42,
				"replace_with_file": "tex.ktx2"
			}
		]
	}`
	if err := os.WriteFile(pj, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := LoadFile(pj)
	if err != nil {
		t.Fatal(err)
	}
	got := p.Replacements[0].ReplaceWithFile
	want := filepath.Clean(asset)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
