package pack

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWikiAPIWarning_unrecognizedParam(t *testing.T) {
	raw := `{"warnings":{"main":{"*":"Unrecognized parameter: oldids."}},"batchcomplete":""}`
	var root map[string]any
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		t.Fatal(err)
	}
	msg := wikiAPIWarning(root)
	if msg == "" || !strings.Contains(msg, "oldids") {
		t.Fatalf("got %q", msg)
	}
}

func TestExtractFirstCommentURL(t *testing.T) {
	content := "<!--https://bin.t7ru.link/b10a8914bac7-->\n{\"format_version\":1}"
	u, ok := extractFirstCommentURL(content)
	if !ok || u != "https://bin.t7ru.link/b10a8914bac7" {
		t.Fatalf("got %q ok=%v", u, ok)
	}
}

func TestIsZipBytes(t *testing.T) {
	if !IsZipBytes([]byte{0x50, 0x4b, 0x03, 0x04}) {
		t.Fatal("expected zip magic")
	}
	if IsZipBytes([]byte(`{"format_version":1}`)) {
		t.Fatal("json should not be zip")
	}
}

func TestWikiRevisionMarkdown_sampleAPI(t *testing.T) {
	raw := `
{
 "query": {
   "pages": {
     "42": {
       "pageid": 42,
       "revisions": [
         {
           "slots": {
             "main": {
               "contentmodel": "wikitext",
               "*": "{\"format_version\":1,\"replacements\":[]}"
             }
           }
         }
       ]
     }
   }
 }
}`
	s, err := wikiRevisionMarkdown([]byte(strings.TrimSpace(raw)))
	if err != nil {
		t.Fatal(err)
	}
	p, err := DecodePackLoose([]byte(s))
	if err != nil || p.FormatVersion != 1 {
		t.Fatalf("%v %+v", err, p)
	}
}
