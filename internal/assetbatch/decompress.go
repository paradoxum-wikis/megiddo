package assetbatch

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"

	"github.com/klauspost/compress/zstd"
)

const (
	zstdMagic0 = 0x28
	zstdMagic1 = 0xb5
	zstdMagic2 = 0x2f
	zstdMagic3 = 0xfd
)

func wantsGzip(ce string, raw []byte) bool {
	return strings.Contains(strings.ToLower(ce), "gzip") || looksGzip(raw)
}

func wantsZSTD(ce string, raw []byte) bool {
	return strings.Contains(strings.ToLower(ce), "zstd") || looksZSTD(raw)
}

func maybeDecompress(raw []byte, hdr http.Header) ([]byte, error) {
	if len(raw) == 0 {
		return raw, nil
	}
	ce := strings.ToLower(hdr.Get("Content-Encoding"))
	switch {
	case wantsGzip(ce, raw):
		r, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		upper := int64(MaxDecodedBatchBody + 1)
		out, err := io.ReadAll(io.LimitReader(r, upper))
		if err != nil {
			return nil, err
		}
		if len(out) > MaxDecodedBatchBody {
			return nil, ErrBatchBodyTooLarge
		}
		return out, nil
	case wantsZSTD(ce, raw):
		zdec, err := zstd.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		defer zdec.Close()
		upper := int64(MaxDecodedBatchBody + 1)
		out, err := io.ReadAll(io.LimitReader(zdec, upper))
		if err != nil {
			return nil, err
		}
		if len(out) > MaxDecodedBatchBody {
			return nil, ErrBatchBodyTooLarge
		}
		return out, nil
	default:
		if len(raw) > MaxDecodedBatchBody {
			return nil, ErrBatchBodyTooLarge
		}
		return raw, nil
	}
}

func looksGzip(b []byte) bool {
	return len(b) >= 2 && b[0] == 0x1f && b[1] == 0x8b
}

func looksZSTD(b []byte) bool {
	return len(b) >= 4 && b[0] == zstdMagic0 && b[1] == zstdMagic1 && b[2] == zstdMagic2 && b[3] == zstdMagic3
}
