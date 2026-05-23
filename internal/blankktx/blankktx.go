package blankktx

import (
	"image"
	"os"
	"path/filepath"
	"sync"

	"megiddo/internal/ktx2encode"
	"megiddo/internal/paths"
)

var once = sync.OnceValues(func() (string, error) {
	base, err := paths.LocalAppMegiddo()
	if err != nil {
		return "", err
	}
	fp := filepath.Join(base, "assets", "blank.ktx2")
	if err := os.MkdirAll(filepath.Dir(fp), 0o755); err != nil {
		return "", err
	}
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	return fp, os.WriteFile(fp, ktx2encode.FromImage(img), 0o644)
})

func Ensure() (string, error) {
	return once()
}
