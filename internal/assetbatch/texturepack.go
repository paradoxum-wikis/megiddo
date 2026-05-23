package assetbatch

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

const texturePackAssetTypeID = 63

// slot lives in base64-json crpl -> fidelity blob, low 6 bits of first byte
func SlotFromRepresentationField(crpl string) (slot int, ok bool) {
	if crpl == "" {
		return 0, false
	}
	raw, err := base64.StdEncoding.DecodeString(crpl)
	if err != nil || len(raw) == 0 {
		return 0, false
	}
	var arr []map[string]any
	if json.Unmarshal(raw, &arr) != nil || len(arr) == 0 {
		return 0, false
	}
	fidB64, _ := arr[0]["fidelity"].(string)
	if fidB64 == "" {
		return 0, false
	}
	fb, err := base64.StdEncoding.DecodeString(fidB64)
	if err != nil || len(fb) == 0 {
		return 0, false
	}
	return int(fb[0] & 0x3f), true
}

func assetTypeLikelyTexturePack(obj map[string]any) bool {
	if id, ok := int64FromJSON(obj["assetTypeId"]); ok && id == texturePackAssetTypeID {
		return true
	}
	s, ok := obj["assetType"].(string)
	if !ok || s == "" {
		return false
	}
	return strings.EqualFold(s, "TexturePack")
}
