package pack

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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
	a := slugProfilePart(author)
	if n == "" {
		n = "pack"
	}
	if a == "" {
		return n
	}
	return n + "_" + a
}

func slugProfilePart(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
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
	out := strings.Trim(b.String(), "_")
	return out
}

func WriteMgpack(w io.Writer, p *Pack) error {
	if p == nil {
		return fmt.Errorf("nil pack")
	}
	zw := zip.NewWriter(w)
	defer zw.Close()

	manifest := *p
	manifest.Replacements = make([]Replacement, len(p.Replacements))
	copy(manifest.Replacements, p.Replacements)

	used := map[string]int{}
	for i, r := range p.Replacements {
		fp := strings.TrimSpace(r.ReplaceWithFile)
		if fp == "" {
			manifest.Replacements[i] = r
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
		nr := r
		nr.ReplaceWithFile = arc
		manifest.Replacements[i] = nr
	}

	mb, err := json.MarshalIndent(&manifest, "", "\t")
	if err != nil {
		return err
	}
	mw, err := zw.Create(MgpackManifestName)
	if err != nil {
		return err
	}
	if _, err := mw.Write(mb); err != nil {
		return err
	}
	return zw.Close()
}

func uniqueAssetArcName(label, srcPath string, used map[string]int) string {
	ext := strings.ToLower(filepath.Ext(srcPath))
	base := slugProfilePart(label)
	if base == "" {
		base = "asset"
	}
	name := base + ext
	if strings.EqualFold(name, MgpackManifestName) {
		name = base + "_file" + ext
	}
	if n, ok := used[name]; ok {
		used[name] = n + 1
		name = fmt.Sprintf("%s_%d%s", base, n+1, ext)
	} else {
		used[name] = 1
	}
	return name
}

func IsZipBytes(b []byte) bool {
	return len(b) >= 4 && b[0] == 'P' && b[1] == 'K' && (b[2] == 3 || b[2] == 5 || b[2] == 7) && (b[3] == 4 || b[3] == 6 || b[3] == 8)
}

func PeekProfileID(mgpackPath string) (string, *Pack, error) {
	p, err := readManifestFromZip(mgpackPath)
	if err != nil {
		return "", nil, err
	}
	if strings.TrimSpace(p.Name) == "" {
		return "", nil, fmt.Errorf("mgpack manifest missing pack name")
	}
	return ProfileID(p.Name, p.Author), p, nil
}

func PeekProfileIDFromBytes(raw []byte) (string, *Pack, error) {
	p, err := readManifestFromZipBytes(raw)
	if err != nil {
		return "", nil, err
	}
	if strings.TrimSpace(p.Name) == "" {
		return "", nil, fmt.Errorf("mgpack manifest missing pack name")
	}
	return ProfileID(p.Name, p.Author), p, nil
}

func InstallMgpackFromBytes(raw []byte, packsRoot string, replace bool) (*Pack, error) {
	if !IsZipBytes(raw) {
		return nil, fmt.Errorf("not a zip mgpack archive")
	}
	profileID, _, err := PeekProfileIDFromBytes(raw)
	if err != nil {
		return nil, err
	}
	f, err := os.CreateTemp("", "megiddo-*.mgpack")
	if err != nil {
		return nil, err
	}
	path := f.Name()
	defer os.Remove(path)
	if _, err := f.Write(raw); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	if _, err := InstallMgpack(path, packsRoot, profileID, replace); err != nil {
		return nil, err
	}
	return LoadInstalled(packsRoot, profileID)
}

func readManifestFromZip(mgpackPath string) (*Pack, error) {
	zr, err := zip.OpenReader(mgpackPath)
	if err != nil {
		return nil, fmt.Errorf("open mgpack: %w", err)
	}
	defer zr.Close()
	return readManifestFromZipFiles(zr.File)
}

func readManifestFromZipBytes(raw []byte) (*Pack, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("open mgpack bytes: %w", err)
	}
	return readManifestFromZipFiles(zr.File)
}

func readManifestFromZipFiles(files []*zip.File) (*Pack, error) {
	var raw []byte
	for _, f := range files {
		if filepath.ToSlash(f.Name) != MgpackManifestName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		raw, err = io.ReadAll(io.LimitReader(rc, maxPackDownloadBytes+1))
		rc.Close()
		if err != nil {
			return nil, err
		}
		break
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("mgpack missing %s", MgpackManifestName)
	}
	if len(raw) > maxPackDownloadBytes {
		return nil, fmt.Errorf("mgpack manifest exceeds byte limit (%d)", maxPackDownloadBytes)
	}
	return ParseJSON(raw)
}

func InstallMgpack(mgpackPath, packsRoot string, profileID string, replace bool) (string, error) {
	dest := filepath.Join(packsRoot, profileID)
	if st, err := os.Stat(dest); err == nil && st.IsDir() {
		if !replace {
			return dest, fmt.Errorf("pack profile %q already installed at %s", profileID, dest)
		}
		if err := os.RemoveAll(dest); err != nil {
			return dest, err
		}
	} else if err != nil && !os.IsNotExist(err) {
		return dest, err
	}
	if err := os.MkdirAll(packsRoot, 0o755); err != nil {
		return dest, err
	}
	if err := extractMgpack(mgpackPath, dest); err != nil {
		return dest, err
	}
	return dest, nil
}

func extractMgpack(mgpackPath, destDir string) error {
	zr, err := zip.OpenReader(mgpackPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	for _, f := range zr.File {
		name := filepath.ToSlash(f.Name)
		if strings.Contains(name, "..") || strings.Contains(name, "/") {
			continue
		}
		target := filepath.Join(destDir, filepath.FromSlash(name))
		if f.FileInfo().IsDir() || strings.HasSuffix(name, "/") {
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
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func LoadInstalled(packsRoot, profileID string) (*Pack, error) {
	manifest := filepath.Join(packsRoot, profileID, MgpackManifestName)
	return LoadFile(manifest)
}

func WriteInstalledManifest(profileDir string, p *Pack) error {
	if p == nil {
		return fmt.Errorf("nil pack")
	}
	profileDir = filepath.Clean(profileDir)
	manifest := *p
	manifest.Replacements = make([]Replacement, len(p.Replacements))
	copy(manifest.Replacements, p.Replacements)
	for i, r := range manifest.Replacements {
		fp := strings.TrimSpace(r.ReplaceWithFile)
		if fp == "" {
			continue
		}
		clean := filepath.Clean(fp)
		rel, err := filepath.Rel(profileDir, clean)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			manifest.Replacements[i].ReplaceWithFile = filepath.ToSlash(rel)
		}
	}
	data, err := json.MarshalIndent(&manifest, "", "\t")
	if err != nil {
		return err
	}
	path := filepath.Join(profileDir, MgpackManifestName)
	return os.WriteFile(path, data, 0o644)
}

func ListInstalled(packsRoot string) ([]InstalledSummary, error) {
	entries, err := os.ReadDir(packsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []InstalledSummary
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		manifest := filepath.Join(packsRoot, ent.Name(), MgpackManifestName)
		data, err := os.ReadFile(manifest)
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
	sort.Slice(out, func(i, j int) bool { return out[i].ProfileID < out[j].ProfileID })
	return out, nil
}
