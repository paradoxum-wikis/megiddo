package assetbatch

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"megiddo/internal/localserve"
	"megiddo/internal/replacement"
	"megiddo/internal/texpacklookup"
)

const BatchPathSubstring = "/v1/assets/batch"

func BatchRequestPath(path string) bool {
	return strings.Contains(path, BatchPathSubstring)
}

func int64FromJSON(val any) (int64, bool) {
	switch t := val.(type) {
	case json.Number:
		i, err := t.Int64()
		return i, err == nil
	case float64:
		return int64(t), true
	case int64:
		return t, true
	case int:
		return int64(t), true
	default:
		return 0, false
	}
}

func stringFromJSON(val any) (string, bool) {
	switch t := val.(type) {
	case string:
		return t, true
	default:
		return "", false
	}
}

type Logger func(format string, args ...any)

type FileMark struct {
	Index int
	Token string
}

type RewriteResult struct {
	Body    []byte
	Changed bool
	Files   []FileMark
}

func RewriteMITMBatch(raw []byte, hdr http.Header, rep *replacement.Map, tpl *texpacklookup.Store, log Logger) (RewriteResult, error) {
	if rep == nil {
		return RewriteResult{Body: raw}, nil
	}
	plain, err := maybeDecompress(raw, hdr)
	if err != nil {
		if errors.Is(err, ErrBatchBodyTooLarge) {
			return RewriteResult{}, err
		}
		return RewriteResult{Body: raw}, nil
	}
	if tpl != nil {
		tpl.ObserveAuth(hdr.Get("Cookie"), hdr.Get("Roblox-Place-Id"))
	}
	res, err := RewriteBatchJSON(plain, rep, tpl, log)
	if err != nil {
		return RewriteResult{Body: raw}, err
	}
	if !res.Changed {
		if len(res.Files) > 0 && log != nil {
			log("megiddo: batch matched %d local file replacement(s)", len(res.Files))
		} else if log != nil {
			activeKeys := rep.Len()
			log("megiddo: batch passthrough (active_keys=%d no hit); sample keys: %s", activeKeys, summarizeBatchKeys(plain))
		}
		return RewriteResult{Body: raw, Files: res.Files}, nil
	}
	if log != nil {
		log("megiddo: batch body rewritten (%d bytes -> %d bytes, %d local)", len(raw), len(res.Body), len(res.Files))
	}
	return res, nil
}

func RewriteBatchJSON(body []byte, rep *replacement.Map, tpl *texpacklookup.Store, log Logger) (RewriteResult, error) {
	if rep == nil {
		return RewriteResult{Body: body}, nil
	}
	var items []any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&items); err != nil {
		return RewriteResult{}, fmt.Errorf("batch json: %w", err)
	}
	changed := false
	var files []FileMark
	for i := range items {
		obj, ok := items[i].(map[string]any)
		if !ok {
			continue
		}
		aidVal, has := obj["assetId"]
		if !has {
			continue
		}
		aid, ok := int64FromJSON(aidVal)
		if !ok {
			continue
		}
		var slotPtr *int
		isTP := assetTypeLikelyTexturePack(obj)
		if isTP {
			crpl, _ := stringFromJSON(obj["contentRepresentationPriorityList"])
			if slot, ok := SlotFromRepresentationField(crpl); ok {
				slotPtr = new(slot)
			}
			if tpl != nil {
				tpl.DiscoverAsync(aid, log)
			}
		}

		entry, hit := rep.Lookup(aid, slotPtr)

		effectiveSubID := int64(0)
		if !hit && tpl != nil && isTP && slotPtr != nil {
			if subID, ok := tpl.ReverseLookup(aid, *slotPtr); ok {
				effectiveSubID = subID
				entry, hit = rep.Lookup(subID, nil)
				if hit && log != nil {
					log("texpacklookup: reverse hit: parent %d slot %d -> sub %d", aid, *slotPtr, subID)
				}
			}
		}

		if !hit {
			continue
		}
		switch {
		case entry.IsFile():
			tok := localserve.Token(aid, -1)
			if effectiveSubID != 0 {
				tok = localserve.Token(effectiveSubID, -1)
			}
			files = append(files, FileMark{Index: i, Token: tok})
		case entry.IsRemove():
			obj["assetId"] = json.Number("0")
			items[i] = obj
			changed = true
		case entry.IsID():
			obj["assetId"] = entry.AssetID
			items[i] = obj
			changed = true
		}
	}
	if !changed && len(files) == 0 {
		return RewriteResult{Body: body}, nil
	}
	if !changed {
		return RewriteResult{Body: body, Files: files}, nil
	}
	out, err := json.Marshal(items)
	if err != nil {
		return RewriteResult{}, err
	}
	return RewriteResult{Body: out, Changed: true, Files: files}, nil
}

func summarizeBatchKeys(body []byte) string {
	var items []any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&items); err != nil || len(items) == 0 {
		return "(unavailable)"
	}
	sample := make([]string, 0, 5)
	for i := range items {
		if len(sample) >= 5 {
			break
		}
		obj, ok := items[i].(map[string]any)
		if !ok {
			continue
		}
		aid, ok := int64FromJSON(obj["assetId"])
		if !ok {
			continue
		}
		if assetTypeLikelyTexturePack(obj) {
			crpl, _ := stringFromJSON(obj["contentRepresentationPriorityList"])
			if slot, ok := SlotFromRepresentationField(crpl); ok {
				sample = append(sample, fmt.Sprintf("%d:%d", aid, slot))
				continue
			}
		}
		sample = append(sample, fmt.Sprintf("%d", aid))
	}
	if len(sample) == 0 {
		return "(none)"
	}
	return strings.Join(sample, ", ")
}

func PrepareBatchRequestHeader(hdr http.Header, newLen int) {
	hdr.Del("Content-Encoding")
	hdr.Del("Transfer-Encoding")
	hdr.Set("Content-Length", fmt.Sprintf("%d", newLen))
}
