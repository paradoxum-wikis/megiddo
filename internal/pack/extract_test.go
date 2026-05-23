package pack

import (
	"testing"
)

func TestDecodePackLoose_embeddedLeadingNoise(t *testing.T) {
	blob := []byte(`
Some chatter before JSON
{"format_version": 1,
 "replacements":[]
}`)
	p, err := DecodePackLoose(blob)
	if err != nil || p.FormatVersion != 1 {
		t.Fatalf("%v %+v", err, p)
	}
}
