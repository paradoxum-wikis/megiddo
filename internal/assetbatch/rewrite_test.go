package assetbatch

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"megiddo/internal/localserve"
	"megiddo/internal/megiddo"
	"megiddo/internal/replacement"
	"megiddo/internal/texpacklookup"
)

func encodeCRPLForSlot(slot int) string {
	fidelity := base64.StdEncoding.EncodeToString([]byte{byte(slot & 0x3f)})
	arrJSON, err := json.Marshal([]map[string]string{{"fidelity": fidelity}})
	if err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(arrJSON)
}

func TestRewriteBatchJSON_replaceWithZeroClearsAssetID(t *testing.T) {
	m := replacement.NewMap()
	m.Swap(map[replacement.Key]replacement.Entry{
		{AssetID: 500, SlotIndex: -1}: replacement.RemoveEntry(),
	})
	body := []byte(`[{"assetId":500,"assetTypeId":"1"}]`)
	res, err := RewriteBatchJSON(body, m, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed {
		t.Fatal("expected swap to 0")
	}
	var items []map[string]any
	if err := json.Unmarshal(res.Body, &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || toInt(t, items[0]["assetId"]) != 0 {
		t.Fatalf("got %+v", items)
	}
}

func TestRewriteBatchJSON_plainAssetSlotNegative(t *testing.T) {
	m := replacement.NewMap()
	m.Swap(map[replacement.Key]replacement.Entry{
		{AssetID: 500, SlotIndex: -1}: replacement.IDEntry(900),
	})
	body := []byte(`[{"assetId":500}]`)
	res, err := RewriteBatchJSON(body, m, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed {
		t.Fatal("expected swap")
	}
	var items []map[string]any
	if err := json.Unmarshal(res.Body, &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || toInt(t, items[0]["assetId"]) != 900 {
		t.Fatalf("got %+v", items)
	}
}

func TestRewriteBatchJSON_texturePackBySlot(t *testing.T) {
	crpl := encodeCRPLForSlot(2)
	arr := []map[string]any{
		{
			"assetTypeId":                       json.Number("63"),
			"assetId":                           json.Number("100"),
			"contentRepresentationPriorityList": crpl,
		},
	}
	body, err := json.Marshal(arr)
	if err != nil {
		t.Fatal(err)
	}
	m := replacement.NewMap()
	m.Swap(map[replacement.Key]replacement.Entry{
		{AssetID: 100, SlotIndex: 2}: replacement.IDEntry(222),
		{AssetID: 100, SlotIndex: 0}: replacement.IDEntry(111),
	})
	res, err := RewriteBatchJSON(body, m, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed {
		t.Fatal("expected swap")
	}
	var items []map[string]any
	if err := json.Unmarshal(res.Body, &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || toInt(t, items[0]["assetId"]) != 222 {
		t.Fatalf("expected slot-specific hit, got %+v", items)
	}
}

func TestRewriteBatchJSON_texturePackSlot32FallsBackTo0(t *testing.T) {
	crpl := encodeCRPLForSlot(32)
	arr := []map[string]any{
		{
			"assetTypeId":                       json.Number("63"),
			"assetId":                           json.Number("100"),
			"contentRepresentationPriorityList": crpl,
		},
	}
	body, err := json.Marshal(arr)
	if err != nil {
		t.Fatal(err)
	}
	m := replacement.NewMap()
	m.Swap(map[replacement.Key]replacement.Entry{
		{AssetID: 100, SlotIndex: 0}: replacement.IDEntry(222),
	})
	res, err := RewriteBatchJSON(body, m, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed {
		t.Fatal("expected swap via slot fallback")
	}
	var items []map[string]any
	if err := json.Unmarshal(res.Body, &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || toInt(t, items[0]["assetId"]) != 222 {
		t.Fatalf("expected fallback hit, got %+v", items)
	}
}

func TestRewriteBatchJSON_fileEntryTracks(t *testing.T) {
	crpl := encodeCRPLForSlot(0)
	arr := []map[string]any{
		{
			"assetTypeId":                       json.Number("63"),
			"assetId":                           json.Number("777"),
			"contentRepresentationPriorityList": crpl,
		},
	}
	body, err := json.Marshal(arr)
	if err != nil {
		t.Fatal(err)
	}
	m := replacement.NewMap()
	m.Swap(map[replacement.Key]replacement.Entry{
		{AssetID: 777, SlotIndex: -1}: {FilePath: `C:\fake\file.ktx2`},
	})
	res, err := RewriteBatchJSON(body, m, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed {
		t.Fatal("file entries must not rewrite request body")
	}
	if len(res.Files) != 1 || res.Files[0].Index != 0 {
		t.Fatalf("expected single file mark at index 0, got %+v", res.Files)
	}
	if res.Files[0].Token != localserve.Token(777, -1) {
		t.Fatalf("unexpected token %q", res.Files[0].Token)
	}
}

func TestRewriteBatchJSON_texpackReverseLookupIDSwap(t *testing.T) {
	const parentID = int64(79196098860673)
	const subAssetID = int64(129324924637058)
	const replacementID = int64(999)

	crpl := encodeCRPLForSlot(0)
	arr := []map[string]any{
		{
			"assetTypeId":                       json.Number("63"),
			"assetId":                           json.Number("79196098860673"),
			"contentRepresentationPriorityList": crpl,
		},
	}
	body, err := json.Marshal(arr)
	if err != nil {
		t.Fatal(err)
	}

	m := replacement.NewMap()
	m.Swap(map[replacement.Key]replacement.Entry{
		{AssetID: subAssetID, SlotIndex: -1}: replacement.IDEntry(replacementID),
	})

	tpl := texpacklookup.New()
	xml := `<roblox><color>129324924637058</color></roblox>`
	tpl.Learn(parentID, []byte(xml))

	res, err := RewriteBatchJSON(body, m, tpl, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed {
		t.Fatal("expected swap via reverse lookup")
	}
	var items []map[string]any
	if err := json.Unmarshal(res.Body, &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || toInt(t, items[0]["assetId"]) != replacementID {
		t.Fatalf("expected replacement %d via reverse lookup, got %+v", replacementID, items)
	}
}

func TestRewriteBatchJSON_texpackReverseLookupFileTrack(t *testing.T) {
	const parentID = int64(79196098860673)
	const subAssetID = int64(129324924637058)

	crpl := encodeCRPLForSlot(0)
	arr := []map[string]any{
		{
			"assetTypeId":                       json.Number("63"),
			"assetId":                           json.Number("79196098860673"),
			"contentRepresentationPriorityList": crpl,
		},
	}
	body, err := json.Marshal(arr)
	if err != nil {
		t.Fatal(err)
	}

	m := replacement.NewMap()
	m.Swap(map[replacement.Key]replacement.Entry{
		{AssetID: subAssetID, SlotIndex: -1}: {FilePath: `C:\fake\color.ktx2`},
	})

	tpl := texpacklookup.New()
	tpl.Learn(parentID, []byte(`<roblox><color>129324924637058</color></roblox>`))

	res, err := RewriteBatchJSON(body, m, tpl, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed {
		t.Fatal("file entries must not rewrite request body")
	}
	if len(res.Files) != 1 {
		t.Fatalf("expected 1 file mark, got %+v", res.Files)
	}
	wantToken := localserve.Token(subAssetID, -1)
	if res.Files[0].Token != wantToken {
		t.Fatalf("token: got %q want %q", res.Files[0].Token, wantToken)
	}
}

func TestRewriteBatchResponseJSON_patchesLocation(t *testing.T) {
	body := []byte(`[{"assetId":777,"location":"https://orig.example/abc"},{"assetId":222,"location":"https://orig.example/def"}]`)
	out, changed, err := RewriteBatchResponseJSON(body, []FileMark{{Index: 0, Token: "777-0"}})
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected change")
	}
	var items []map[string]any
	if err := json.Unmarshal(out, &items); err != nil {
		t.Fatal(err)
	}
	want := "https://" + megiddo.FtsHost + megiddo.LocalServePathPrefix + "777-0"
	if items[0]["location"] != want {
		t.Fatalf("got location %v want %v", items[0]["location"], want)
	}
	if items[1]["location"] != "https://orig.example/def" {
		t.Fatalf("untracked item must stay intact, got %v", items[1]["location"])
	}
}

func TestMaybeDecompress_identityTooLarge(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	raw := bytes.Repeat([]byte{'x'}, MaxDecodedBatchBody+1)
	_, err := maybeDecompress(raw, http.Header{})
	if err == nil {
		t.Fatal("expected ErrBatchBodyTooLarge")
	}
	if !errors.Is(err, ErrBatchBodyTooLarge) {
		t.Fatalf("got %v", err)
	}
}

func TestMaybeDecompress_gzipDecodesBeyondLimit(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	plain := bytes.Repeat([]byte{'z'}, MaxDecodedBatchBody+20)
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write(plain); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	h := http.Header{}
	h.Set("Content-Encoding", "gzip")
	_, err := maybeDecompress(buf.Bytes(), h)
	if err == nil {
		t.Fatal("expected ErrBatchBodyTooLarge")
	}
	if !errors.Is(err, ErrBatchBodyTooLarge) {
		t.Fatalf("got %v", err)
	}
}

func TestRewriteMITMBatch_propagatesErrBatchTooLarge(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	plain := bytes.Repeat([]byte{'z'}, MaxDecodedBatchBody+5)
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write(plain); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	h := http.Header{}
	h.Set("Content-Encoding", "gzip")
	m := replacement.NewMap()
	m.Swap(map[replacement.Key]replacement.Entry{
		{AssetID: 1, SlotIndex: -1}: replacement.IDEntry(2),
	})

	_, err := RewriteMITMBatch(buf.Bytes(), h, m, nil, nil)
	if err == nil {
		t.Fatal("expected ErrBatchBodyTooLarge")
	}
	if !errors.Is(err, ErrBatchBodyTooLarge) {
		t.Fatalf("got %v", err)
	}
}

func toInt(tb testing.TB, v any) int64 {
	tb.Helper()
	switch x := v.(type) {
	case json.Number:
		n, err := x.Int64()
		if err != nil {
			tb.Fatal(err)
		}
		return n
	case float64:
		return int64(x)
	case int64:
		return x
	default:
		tb.Fatalf("unexpected %T %+v", v, v)
		return 0
	}
}
