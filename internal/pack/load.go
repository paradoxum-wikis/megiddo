package pack

import (
	"os"
	"path/filepath"
	"strings"
)

func LoadFile(path string) (*Pack, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	p, err := DecodePackLoose(data)
	if err != nil {
		return nil, err
	}
	absolutizeFilePaths(p, filepath.Dir(path))
	return p, nil
}

func absolutizeFilePaths(p *Pack, srcDir string) {
	if p == nil || srcDir == "" {
		return
	}
	for i := range p.Replacements {
		fp := strings.TrimSpace(p.Replacements[i].ReplaceWithFile)
		if fp == "" || filepath.IsAbs(fp) {
			continue
		}
		p.Replacements[i].ReplaceWithFile = filepath.Clean(filepath.Join(srcDir, fp))
	}
}
