package localserve

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"megiddo/internal/megiddo"
	"megiddo/internal/replacement"
)

type Store struct {
	mu    sync.RWMutex
	files map[string]string
}

func New() *Store { return &Store{files: map[string]string{}} }

func (s *Store) Swap(entries map[replacement.Key]replacement.Entry) {
	next := make(map[string]string, len(entries))
	for k, e := range entries {
		if !e.IsFile() {
			continue
		}
		next[Token(k.AssetID, k.SlotIndex)] = e.FilePath
	}
	s.mu.Lock()
	s.files = next
	s.mu.Unlock()
}

func (s *Store) Clear() {
	s.mu.Lock()
	s.files = map[string]string{}
	s.mu.Unlock()
}

func (s *Store) Resolve(path string) (string, bool) {
	tok, ok := TokenFromPath(path)
	if !ok {
		return "", false
	}
	s.mu.RLock()
	fp, ok := s.files[tok]
	s.mu.RUnlock()
	return fp, ok
}

func Token(assetID int64, slot int) string {
	if slot < 0 {
		return fmt.Sprintf("%d", assetID)
	}
	return fmt.Sprintf("%d-%d", assetID, slot)
}

func URLFor(assetID int64, slot int) string {
	return "https://" + megiddo.FtsHost + megiddo.LocalServePathPrefix + Token(assetID, slot)
}

func TokenFromPath(path string) (string, bool) {
	if !strings.HasPrefix(path, megiddo.LocalServePathPrefix) {
		return "", false
	}
	tok := strings.TrimPrefix(path, megiddo.LocalServePathPrefix)
	if i := strings.IndexAny(tok, "?#"); i >= 0 {
		tok = tok[:i]
	}
	tok = strings.Trim(tok, "/")
	if tok == "" {
		return "", false
	}
	if !validToken(tok) {
		return "", false
	}
	return tok, true
}

func validToken(tok string) bool {
	parts := strings.SplitN(tok, "-", 2)
	if _, err := strconv.ParseInt(parts[0], 10, 64); err != nil {
		return false
	}
	if len(parts) == 2 {
		if _, err := strconv.Atoi(parts[1]); err != nil {
			return false
		}
	}
	return true
}

func ContentTypeFor(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ktx2":
		return "image/ktx2"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".ogg":
		return "audio/ogg"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	}
	return "application/octet-stream"
}
