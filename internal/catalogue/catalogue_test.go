package catalogue

import (
	"testing"
)

func TestParse_catalogueRows(t *testing.T) {
	const raw = `{
	  "format_version": 1,
	  "name": "t",
	  "description": "",
	  "replacements_catalogue": [
	    {
	      "label": "banner",
	      "character": null,
	      "role": "ui",
	      "asset_type": "image",
	      "target_id": 555,
	      "rbxm_props": { "dump_label": "Lobby", "class_name": "ImageLabel", "prop": "Image" }
	    }
	  ]
	}`
	s, err := Parse([]byte(raw))
	if err != nil || len(s.ReplacementsCatalogue) != 1 {
		t.Fatalf("parse: %v %+v", err, s)
	}
	r0 := s.ReplacementsCatalogue[0]
	if r0.TargetID != 555 || r0.Slot != nil {
		t.Fatal("row mismatch")
	}
	if r0.RbxmProps == nil || r0.RbxmProps.ClassName != "ImageLabel" {
		t.Fatal("rbxm props missing")
	}
}
