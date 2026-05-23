package catalogue

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMerged_caducus(t *testing.T) {
	root := filepath.Join("..", "..", "assets")
	snap, err := LoadMerged(os.DirFS(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.ReplacementsCatalogue) < 30 {
		t.Fatalf("snapshot: rows=%d", len(snap.ReplacementsCatalogue))
	}
	var defaultBody, nightmareBody int64
	for _, row := range snap.ReplacementsCatalogue {
		if row.Label != "Body Mesh" {
			continue
		}
		switch row.Model {
		case "Default":
			defaultBody = row.TargetID
		case "Nightmare":
			nightmareBody = row.TargetID
		}
	}
	if defaultBody != 139700074774589 {
		t.Fatalf("default body mesh: got %d want 139700074774589", defaultBody)
	}
	if nightmareBody != 127661319862542 {
		t.Fatalf("nightmare body mesh: got %d want 127661319862542", nightmareBody)
	}
}
