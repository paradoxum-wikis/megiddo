package assetbatch

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"megiddo/internal/megiddo"
)

func RewriteBatchResponseJSON(body []byte, files []FileMark) ([]byte, bool, error) {
	if len(files) == 0 {
		return body, false, nil
	}
	var items []any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&items); err != nil {
		return nil, false, fmt.Errorf("batch resp json: %w", err)
	}
	changed := false
	for _, mark := range files {
		if mark.Index < 0 || mark.Index >= len(items) {
			continue
		}
		obj, ok := items[mark.Index].(map[string]any)
		if !ok {
			continue
		}
		obj["location"] = "https://" + megiddo.FtsHost + megiddo.LocalServePathPrefix + mark.Token
		delete(obj, "errors") // noisuh
		items[mark.Index] = obj
		changed = true
	}
	if !changed {
		return body, false, nil
	}
	out, err := json.Marshal(items)
	if err != nil {
		return nil, false, err
	}
	return out, true, nil
}

func RewriteMITMBatchResponse(raw []byte, hdr http.Header, files []FileMark, log Logger) ([]byte, bool, error) {
	if len(files) == 0 {
		return raw, false, nil
	}
	plain, err := maybeDecompress(raw, hdr)
	if err != nil {
		if errors.Is(err, ErrBatchBodyTooLarge) {
			return nil, false, err
		}
		return raw, false, nil
	}
	out, changed, err := RewriteBatchResponseJSON(plain, files)
	if err != nil {
		return raw, false, err
	}
	if !changed {
		return raw, false, nil
	}
	if log != nil {
		log("megiddo: batch response patched (%d local locations)", len(files))
	}
	return out, true, nil
}

func IsBatchResponseJSON(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(ct))
	if ct == "" {
		return true // roblox sometimes omits content-type on batch responses idfk
	}
	return strings.Contains(ct, "json")
}
