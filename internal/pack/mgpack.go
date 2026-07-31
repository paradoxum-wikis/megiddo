package pack

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const MgpackManifestName = "manifest.json"

type InstalledSummary struct {
	ProfileID string `json:"profile_id"`
	Name      string `json:"name"`
	Author    string `json:"author"`
	Version   string `json:"version"`
}

func ProfileID(name, author string) string {
	n := slugProfilePart(name)
	if n == "" {
		n = "pack"
	}
	if a := slugProfilePart(author); a != "" {
		return n + "_" + a
	}
	return n
}

func slugProfilePart(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == ' ', r == '-', r == '_':
			if !prevDash && b.Len() > 0 {
				b.WriteByte('_')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "_")
}

func WriteMgpack(w io.Writer, p *Pack) (err error) {
	if p == nil {
		return fmt.Errorf("nil pack")
	}
	zw := zip.NewWriter(w)
	defer func() {
		if cerr := zw.Close(); err == nil {
			err = cerr
		}
	}()

	manifest := *p
	manifest.Replacements = slices.Clone(p.Replacements)

	used := make(map[string]int, len(p.Replacements))
	for i, r := range manifest.Replacements {
		fp := strings.TrimSpace(r.ReplaceWithFile)
		if fp == "" {
			continue
		}
		data, err := os.ReadFile(fp)
		if err != nil {
			return fmt.Errorf("row %q: read %s: %w", r.Label, fp, err)
		}
		arc := uniqueAssetArcName(r.Label, fp, used)
		fh, err := zw.Create(arc)
		if err != nil {
			return err
		}
		if _, err := fh.Write(data); err != nil {
			return err
		}
		manifest.Replacements[i].ReplaceWithFile = arc
	}

	mb, err := json.MarshalIndent(&manifest, "", "\t")
	if err != nil {
		return err
	}
	mw, err := zw.Create(MgpackManifestName)
	if err != nil {
		return err
	}
	_, err = mw.Write(mb)
	return err
}

func uniqueAssetArcName(label, srcPath string, used map[string]int) string {
	ext := strings.ToLower(filepath.Ext(srcPath))
	base := slugProfilePart(label)
	if base == "" {
		base = "asset"
	}
	for i := 0; ; i++ {
		var cand string
		if i == 0 {
			cand = base + ext
		} else {
			cand = fmt.Sprintf("%s_%d%s", base, i, ext)
		}
		if strings.EqualFold(cand, MgpackManifestName) {
			continue
		}
		if _, ok := used[cand]; ok {
			continue
		}
		used[cand] = 1
		return cand
	}
}

func IsZipBytes(b []byte) bool {
	return len(b) >= 4 && b[0] == 'P' && b[1] == 'K' && (b[2] == 3 || b[2] == 5 || b[2] == 7) && (b[3] == 4 || b[3] == 6 || b[3] == 8)
}

func PeekProfileID(mgpackPath string) (string, *Pack, error) {
	zr, err := zip.OpenReader(mgpackPath)
	if err != nil {
		return "", nil, fmt.Errorf("open mgpack: %w", err)
	}
	defer zr.Close()
	return peekProfileID(readManifestFromZipFiles(zr.File))
}

func PeekProfileIDFromBytes(raw []byte) (string, *Pack, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return "", nil, fmt.Errorf("open mgpack bytes: %w", err)
	}
	return peekProfileID(readManifestFromZipFiles(zr.File))
}

func peekProfileID(p *Pack, err error) (string, *Pack, error) {
	if err != nil {
		return "", nil, err
	}
	if strings.TrimSpace(p.Name) == "" {
		return "", nil, fmt.Errorf("mgpack manifest missing pack name")
	}
	return ProfileID(p.Name, p.Author), p, nil
}

func InstallMgpackFromBytes(raw []byte, packsRoot string, replace bool) (*Pack, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("not a zip mgpack archive: %w", err)
	}
	p, err := readManifestFromZipFiles(zr.File)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.Name) == "" {
		return nil, fmt.Errorf("mgpack manifest missing pack name")
	}
	profileID := ProfileID(p.Name, p.Author)
	dest := filepath.Join(packsRoot, profileID)

	if err := prepareDest(dest, replace, profileID); err != nil {
		return nil, err
	}
	if err := extractMgpackFiles(zr.File, dest); err != nil {
		return nil, err
	}
	return LoadInstalled(packsRoot, profileID)
}

func prepareDest(dest string, replace bool, profileID string) error {
	_, statErr := os.Stat(dest)
	switch {
	case statErr == nil:
		if !replace {
			return fmt.Errorf("pack profile %q already installed at %s", profileID, dest)
		}
		if err := os.RemoveAll(dest); err != nil {
			return err
		}
	case errors.Is(statErr, fs.ErrNotExist):
	default:
		return statErr
	}
	return os.MkdirAll(dest, 0o755)
}

func readManifestFromZipFiles(files []*zip.File) (*Pack, error) {
	for _, f := range files {
		if f.Name != MgpackManifestName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		raw, err := io.ReadAll(io.LimitReader(rc, maxPackDownloadBytes+1))
		rc.Close()
		if err != nil {
			return nil, err
		}
		if len(raw) == 0 {
			return nil, fmt.Errorf("mgpack manifest %s is empty", MgpackManifestName)
		}
		if len(raw) > maxPackDownloadBytes {
			return nil, fmt.Errorf("mgpack manifest exceeds byte limit (%d)", maxPackDownloadBytes)
		}
		return ParseJSON(raw)
	}
	return nil, fmt.Errorf("mgpack missing %s", MgpackManifestName)
}

func InstallMgpack(mgpackPath, packsRoot, profileID string, replace bool) (string, error) {
	dest := filepath.Join(packsRoot, profileID)
	if err := prepareDest(dest, replace, profileID); err != nil {
		return dest, err
	}
	zr, err := zip.OpenReader(mgpackPath)
	if err != nil {
		return dest, err
	}
	defer zr.Close()
	if err := extractMgpackFiles(zr.File, dest); err != nil {
		return dest, err
	}
	return dest, nil
}

func extractMgpackFiles(files []*zip.File, destDir string) error {
	for _, f := range files {
		name := f.Name
		if !filepath.IsLocal(name) {
			continue
		}
		target := filepath.Join(destDir, filepath.FromSlash(name))
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		rc.Close()
		out.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

func LoadInstalled(packsRoot, profileID string) (*Pack, error) {
	return LoadFile(filepath.Join(packsRoot, profileID, MgpackManifestName))
}

func WriteInstalledManifest(profileDir string, p *Pack) error {
	profileDir = filepath.Clean(profileDir)
	manifest := *p
	manifest.Replacements = slices.Clone(p.Replacements)
	for i, r := range manifest.Replacements {
		fp := strings.TrimSpace(r.ReplaceWithFile)
		if fp == "" {
			continue
		}
		rel, err := filepath.Rel(profileDir, filepath.Clean(fp))
		if err != nil || !filepath.IsLocal(rel) {
			continue
		}
		manifest.Replacements[i].ReplaceWithFile = filepath.ToSlash(rel)
	}
	data, err := json.MarshalIndent(&manifest, "", "\t")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(profileDir, MgpackManifestName), data, 0o644)
}

func ListInstalled(packsRoot string) ([]InstalledSummary, error) {
	entries, err := os.ReadDir(packsRoot)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []InstalledSummary
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(packsRoot, ent.Name(), MgpackManifestName))
		if err != nil {
			continue
		}
		p, err := ParseJSON(data)
		if err != nil {
			continue
		}
		out = append(out, InstalledSummary{
			ProfileID: ent.Name(),
			Name:      p.Name,
			Author:    p.Author,
			Version:   p.Version,
		})
	}
	slices.SortFunc(out, func(a, b InstalledSummary) int {
		return strings.Compare(a.ProfileID, b.ProfileID)
	})
	return out, nil
}
