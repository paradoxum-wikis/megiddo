package pack

import (
	"bytes"
	"fmt"
)

func trimBOMPrefix(b []byte) []byte {
	return bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf})
}

func DecodePackLoose(blob []byte) (*Pack, error) {
	raw := bytes.TrimSpace(trimBOMPrefix(blob))
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty pack payload")
	}
	if p, err := ParseJSON(raw); err == nil {
		return p, nil
	}
	obj := extractBalancedJSONObject(string(raw))
	if obj == nil {
		return nil, fmt.Errorf("no json object containing pack fields")
	}
	return ParseJSON(obj)
}

func extractBalancedJSONObject(data string) []byte {
	start := -1
	for i := 0; i < len(data); i++ {
		if data[i] == '{' {
			start = i
			break
		}
	}
	if start < 0 {
		return nil
	}
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(data); i++ {
		c := data[i]
		if inStr {
			if esc {
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return []byte(data[start : i+1])
			}
		case '"':
			inStr = true
		}
	}
	return nil
}
