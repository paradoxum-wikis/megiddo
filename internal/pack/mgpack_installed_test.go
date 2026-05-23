package pack

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteInstalledManifest_relPaths(t *testing.T) {
	root := t.TempDir()
	profileID := ProfileID("Test", "me")
	profileDir := filepath.Join(root, profileID)
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatal(err)
	}
	asset := filepath.Join(profileDir, "body.ktx2")
	if err := os.WriteFile(asset, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &Pack{
		FormatVersion: 1,
		Name:          "Test",
		Author:        "me",
		Replacements: []Replacement{
			{
				Label:           "row",
				TargetID:        1,
				ReplaceWithFile: asset,
			},
		},
	}
	if err := WriteInstalledManifest(profileDir, p); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadInstalled(root, profileID)
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.Replacements[0].ReplaceWithFile; got != asset {
		t.Fatalf("got %q want %q", got, asset)
	}
}
