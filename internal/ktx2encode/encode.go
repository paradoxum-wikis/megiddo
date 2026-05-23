package ktx2encode

import (
	"encoding/binary"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
)

var ktx2Identifier = [12]byte{
	0xAB, 0x4B, 0x54, 0x58, 0x20, 0x32, 0x30, 0xBB, 0x0D, 0x0A, 0x1A, 0x0A,
}

var dfdRGBA8 = [92]byte{
	0x5C, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00,
	0x02, 0x00, 0x58, 0x00,
	0x01, 0x01, 0x01, 0x00,
	0x00, 0x00, 0x00, 0x00,
	0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x07, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFF, 0x00, 0x00, 0x00,
	0x08, 0x00, 0x07, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFF, 0x00, 0x00, 0x00,
	0x10, 0x00, 0x07, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFF, 0x00, 0x00, 0x00,
	0x18, 0x00, 0x07, 0x0F, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFF, 0x00, 0x00, 0x00,
}

func FromImageFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	return FromImage(img), nil
}

func FromImage(img image.Image) []byte {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	pixels := make([]byte, w*h*4)
	off := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, ba, a := img.At(x, y).RGBA()
			pixels[off+0] = uint8(r >> 8)
			pixels[off+1] = uint8(g >> 8)
			pixels[off+2] = uint8(ba >> 8)
			pixels[off+3] = uint8(a >> 8)
			off += 4
		}
	}
	return buildKTX2(uint32(w), uint32(h), pixels)
}

func buildKTX2(width, height uint32, pixels []byte) []byte {
	const (
		szHeader     = 36
		szIndex      = 32
		szLevelIndex = 24
		szDFD        = 92
	)
	dataOffset := uint64(len(ktx2Identifier) + szHeader + szIndex + szLevelIndex + szDFD)
	dataLen := uint64(len(pixels))

	buf := make([]byte, 0, int(dataOffset)+len(pixels))
	le := binary.LittleEndian

	buf = append(buf, ktx2Identifier[:]...)

	hdr := make([]byte, szHeader)
	le.PutUint32(hdr[0:], 37)
	le.PutUint32(hdr[4:], 1)
	le.PutUint32(hdr[8:], width)
	le.PutUint32(hdr[12:], height)
	le.PutUint32(hdr[16:], 0)
	le.PutUint32(hdr[20:], 0)
	le.PutUint32(hdr[24:], 1)
	le.PutUint32(hdr[28:], 1)
	le.PutUint32(hdr[32:], 0)
	buf = append(buf, hdr...)

	dfdOffset := uint32(len(ktx2Identifier) + szHeader + szIndex + szLevelIndex)
	idx := make([]byte, szIndex)
	le.PutUint32(idx[0:], dfdOffset)
	le.PutUint32(idx[4:], szDFD)
	le.PutUint32(idx[8:], dfdOffset+szDFD)
	le.PutUint32(idx[12:], 0)
	le.PutUint64(idx[16:], 0)
	le.PutUint64(idx[24:], 0)
	buf = append(buf, idx...)

	lvl := make([]byte, szLevelIndex)
	le.PutUint64(lvl[0:], dataOffset)
	le.PutUint64(lvl[8:], dataLen)
	le.PutUint64(lvl[16:], dataLen)
	buf = append(buf, lvl...)

	buf = append(buf, dfdRGBA8[:]...)
	buf = append(buf, pixels...)
	return buf
}

func IsSupported(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg":
		return true
	}
	return false
}
