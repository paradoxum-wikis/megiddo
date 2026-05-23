package localserve

import (
	"testing"

	"megiddo/internal/megiddo"
	"megiddo/internal/replacement"
)

func TestTokenRoundTrip(t *testing.T) {
	tok := Token(123, 2)
	if tok != "123-2" {
		t.Fatalf("got %q", tok)
	}
	got, ok := TokenFromPath(megiddo.LocalServePathPrefix + tok)
	if !ok || got != tok {
		t.Fatalf("got %q ok=%v", got, ok)
	}
}

func TestTokenSlotNegativeFlatten(t *testing.T) {
	if Token(99, -1) != "99" {
		t.Fatal("slot=-1 must collapse")
	}
	tok, ok := TokenFromPath(megiddo.LocalServePathPrefix + "99")
	if !ok || tok != "99" {
		t.Fatalf("got %q ok=%v", tok, ok)
	}
}

func TestTokenFromPathRejectsBadInput(t *testing.T) {
	cases := []string{
		"/not-megiddo",
		megiddo.LocalServePathPrefix,
		megiddo.LocalServePathPrefix + "abc",
		megiddo.LocalServePathPrefix + "123-x",
	}
	for _, p := range cases {
		if _, ok := TokenFromPath(p); ok {
			t.Fatalf("expected reject for %q", p)
		}
	}
}

func TestSwapAndResolve(t *testing.T) {
	s := New()
	s.Swap(map[replacement.Key]replacement.Entry{
		{AssetID: 7, SlotIndex: -1}: {FilePath: `C:\file.ktx2`},
		{AssetID: 8, SlotIndex: 1}:  {FilePath: `C:\other.png`},
		{AssetID: 9, SlotIndex: -1}: replacement.IDEntry(123), // id-only should not register
	})
	if fp, ok := s.Resolve(megiddo.LocalServePathPrefix + "7"); !ok || fp != `C:\file.ktx2` {
		t.Fatalf("got %q ok=%v", fp, ok)
	}
	if fp, ok := s.Resolve(megiddo.LocalServePathPrefix + "8-1"); !ok || fp != `C:\other.png` {
		t.Fatalf("got %q ok=%v", fp, ok)
	}
	if _, ok := s.Resolve(megiddo.LocalServePathPrefix + "9"); ok {
		t.Fatal("id-only entries must not appear in local store")
	}
	s.Clear()
	if _, ok := s.Resolve(megiddo.LocalServePathPrefix + "7"); ok {
		t.Fatal("clear must wipe")
	}
}

func TestContentTypeFor(t *testing.T) {
	cases := map[string]string{
		"foo.ktx2": "image/ktx2",
		"foo.PNG":  "image/png",
		"foo.OGG":  "audio/ogg",
		"foo.mesh": "application/octet-stream",
		"foo":      "application/octet-stream",
	}
	for in, want := range cases {
		if got := ContentTypeFor(in); got != want {
			t.Fatalf("%q -> %q want %q", in, got, want)
		}
	}
}
