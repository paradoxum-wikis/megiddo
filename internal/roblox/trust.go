package roblox

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"megiddo/internal/megiddo"
)

const PlayerEXE = "RobloxPlayerBeta.exe"
const StudioEXE = "RobloxStudioBeta.exe"

func DiscoverInstallRoots(extra ...string) []string {
	seen := map[string]struct{}{}
	out := []string{}

	for _, rootAbs := range extra {
		ap, err := filepath.Abs(strings.TrimSpace(rootAbs))
		if err != nil || ap == "" {
			continue
		}
		if hasRobloxRelease(ap, PlayerEXE, StudioEXE) {
			if _, ok := seen[strings.ToLower(ap)]; !ok {
				seen[strings.ToLower(ap)] = struct{}{}
				out = append(out, ap)
			}
		}
	}

	for _, cand := range staticScanRoots() {
		found := scanDirDepthLimit(cand, 2)
		for _, path := range found {
			lp := strings.ToLower(path)
			if _, exists := seen[lp]; exists {
				continue
			}
			seen[lp] = struct{}{}
			out = append(out, path)
		}
	}
	return dedupePreserve(out)
}

func dedupePreserve(in []string) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, s := range in {
		ls := strings.ToLower(s)
		if _, ok := seen[ls]; ok {
			continue
		}
		seen[ls] = struct{}{}
		out = append(out, s)
	}
	return out
}

func scanDirDepthLimit(root string, maxDepth int) []string {
	var matches []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || !d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		if rel != "." && strings.Count(rel, string(filepath.Separator)) > maxDepth {
			return filepath.SkipDir
		}
		if hasRobloxRelease(path, PlayerEXE, StudioEXE) {
			matches = append(matches, path)
		}
		return nil
	})
	return matches
}

func hasRobloxRelease(dir string, exes ...string) bool {
	for _, exe := range exes {
		if fi, err := os.Stat(filepath.Join(dir, exe)); err == nil && !fi.IsDir() {
			return true
		}
	}
	return false
}

func staticScanRoots() []string {
	var out []string
	if la := os.Getenv("LOCALAPPDATA"); la != "" {
		out = append(out, filepath.Join(la, "Roblox", "Versions"))
	}
	if pf86 := os.Getenv("ProgramFiles(x86)"); pf86 != "" {
		out = append(out, filepath.Join(pf86, "Roblox", "Versions"))
	}
	out = append(out, `C:\XboxGames\Roblox`)

	dirOnly := []string{}
	for _, p := range out {
		ap := filepath.Clean(p)
		if st, err := os.Stat(ap); err == nil && st.IsDir() {
			dirOnly = append(dirOnly, ap)
		}
	}
	return dirOnly
}

func UpsertTrustBundle(installRoots []string, caPEM []byte) ([]string, error) {
	canonicalPEM := strings.Trim(strings.ReplaceAll(string(caPEM), "\r\n", "\n"), " \t\n")
	if canonicalPEM == "" {
		return nil, errors.New("empty CA PEM bundle")
	}
	if !strings.HasSuffix(canonicalPEM, "\n") {
		canonicalPEM += "\n"
	}

	log := []string{}
	for _, root := range dedupePreserve(installRoots) {
		target := filepath.Join(root, "ssl", "cacert.pem")
		changed, err := mergeCACertBundle(target, canonicalPEM)
		if err != nil {
			log = append(log, fmt.Sprintf("%s: %v", filepath.Base(root), err))
			continue
		}
		if changed {
			log = append(log, filepath.Base(root)+": patched ssl/cacert.pem")
			continue
		}
		log = append(log, filepath.Base(root)+": already patched")
	}
	return log, nil
}

type TrustBundleState struct {
	Root    string `json:"root"`
	Patched bool   `json:"patched"`
	Exists  bool   `json:"exists"`
	Error   string `json:"error,omitempty"`
}

func TrustBundleStatus(installRoots []string) []TrustBundleState {
	out := make([]TrustBundleState, 0, len(installRoots))
	for _, root := range dedupePreserve(installRoots) {
		target := filepath.Join(root, "ssl", "cacert.pem")
		state := TrustBundleState{Root: root}
		raw, err := os.ReadFile(target)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				out = append(out, state)
				continue
			}
			state.Error = err.Error()
			out = append(out, state)
			continue
		}
		state.Exists = true
		state.Patched = hasMegiddoCA(string(raw))
		out = append(out, state)
	}
	return out
}

func RemoveTrustBundle(installRoots []string) ([]string, error) {
	log := []string{}
	for _, root := range dedupePreserve(installRoots) {
		target := filepath.Join(root, "ssl", "cacert.pem")
		changed, err := stripBundle(target)
		if err != nil {
			log = append(log, fmt.Sprintf("%s: %v", filepath.Base(root), err))
			continue
		}
		if changed {
			log = append(log, filepath.Base(root)+": unpatched ssl/cacert.pem")
		} else {
			log = append(log, filepath.Base(root)+": already unpatched")
		}
	}
	return log, nil
}

func mergeCACertBundle(path, canonicalCA string) (bool, error) {
	var prior string
	raw, readErr := os.ReadFile(path)
	if readErr == nil {
		prior = normalizePEM(string(raw))
	} else if !errors.Is(readErr, fs.ErrNotExist) {
		return false, readErr
	}

	desired := buildMergedBundle(
		strings.TrimSuffix(prior, "\n"),
		strings.TrimSpace(strings.TrimSuffix(canonicalCA, "\n")),
	)
	if normalizePEM(prior) == normalizePEM(desired) {
		return false, nil
	}
	return true, writeAtomicString(path, desired)
}

func stripBundle(path string) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	prior := normalizePEM(string(raw))
	desired := normalizePEM(stripMegiddoCACerts(prior))
	if prior == desired {
		return false, nil
	}
	return true, writeAtomicString(path, desired)
}

func writeAtomicString(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmpDir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(tmpDir, ".megiddo_ca_*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func buildMergedBundle(previous, canonical string) string {
	without := strings.TrimSuffix(stripMegiddoCACerts(previous), "\n")
	canonical = strings.TrimSpace(canonical)
	if without == "" {
		return canonical + "\n"
	}
	return without + "\n" + canonical + "\n"
}

func normalizePEM(s string) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\r\n", "\n")
	if s == "" {
		return ""
	}
	return s + "\n"
}

func stripMegiddoCACerts(bundle string) string {
	rest := []byte(bundle)
	out := ""
	for len(rest) > 0 {
		var blk *pem.Block
		blk, rest = pem.Decode(rest)
		if blk == nil {
			break
		}
		crt, err := x509.ParseCertificate(blk.Bytes)
		if err == nil && strings.EqualFold(crt.Subject.CommonName, megiddo.CanaryCommonName) {
			continue
		}
		out += string(pem.EncodeToMemory(blk))
	}
	return out
}

func hasMegiddoCA(bundle string) bool {
	normalized := normalizePEM(bundle)
	stripped := normalizePEM(stripMegiddoCACerts(bundle))
	return normalized != stripped
}
