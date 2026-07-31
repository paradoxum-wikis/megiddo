package main

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"megiddo/internal/catalogue"
	"megiddo/internal/certmgr"
	"megiddo/internal/ktx2encode"
	"megiddo/internal/localserve"
	skpack "megiddo/internal/pack"
	"megiddo/internal/paths"
	"megiddo/internal/replacement"
	"megiddo/internal/roblox"
	"megiddo/internal/runner"
	"megiddo/internal/texpacklookup"
	"megiddo/internal/winutil"
)

type App struct {
	startupCtx      context.Context
	lifecycleCancel context.CancelFunc

	rep   *replacement.Map
	files *localserve.Store
	tpl   *texpacklookup.Store

	catalog *catalogue.Snapshot

	logMu sync.Mutex
	logs  []string

	active atomic.Bool
	errMu  sync.RWMutex
	runErr error
}

func NewApp() (*App, error) {
	snap, err := catalogue.LoadMergedDir(bundledCatalogueFS, "assets")
	if err != nil {
		return nil, fmt.Errorf("catalogue snapshot: %w", err)
	}
	return &App{
		rep:     replacement.NewMap(),
		files:   localserve.New(),
		tpl:     texpacklookup.New(),
		catalog: snap,
	}, nil
}

func (a *App) startup(ctx context.Context) {
	a.startupCtx = ctx

	lCtx, cancel := context.WithCancel(context.Background())
	a.lifecycleCancel = cancel
	go a.runProxy(lCtx)
}

func (a *App) shutdown(_ context.Context) {
	if a.lifecycleCancel != nil {
		a.lifecycleCancel()
	}
}

func (a *App) runProxy(ctx context.Context) {
	a.active.Store(true)
	defer a.active.Store(false)

	sink := func(pat string, args ...any) { a.writeLog(pat, args...) }
	if err := runner.Lifecycle(ctx, sink, a.rep, a.files, a.tpl); err != nil && !errors.Is(err, context.Canceled) {
		a.errMu.Lock()
		a.runErr = err
		a.errMu.Unlock()
		a.writeLog("lifecycle: fatal - %v", err)
	}
}

func (a *App) writeLog(pat string, args ...any) {
	line := "[" + time.Now().Format("15:04:05") + "] "
	line += fmt.Sprintf(pat, args...)

	a.logMu.Lock()
	defer a.logMu.Unlock()

	a.logs = append(a.logs, line)
	const capLog = 400
	if len(a.logs) > capLog {
		a.logs = append([]string(nil), a.logs[len(a.logs)-capLog:]...)
	}
}

type ProxyStatus struct {
	Active        bool   `json:"active"`
	Elevated      bool   `json:"elevated"`
	LastLifecycle string `json:"lastLifecycle,omitempty"`
}

type CertPatchStatus struct {
	Total     int      `json:"total"`
	Patched   int      `json:"patched"`
	Unpatched int      `json:"unpatched"`
	Details   []string `json:"details"`
}

type packStats struct {
	Rows          int
	IDRows        int
	FileRows      int
	TextureRows   int
	UniqueKeys    int
	DuplicateKeys int
}

func summarizePack(p *skpack.Pack) packStats {
	if p == nil {
		return packStats{}
	}
	stats := packStats{Rows: len(p.Replacements)}
	keys := make(map[replacement.Key]int, len(p.Replacements))
	for _, r := range p.Replacements {
		if strings.TrimSpace(r.ReplaceWithFile) != "" {
			stats.FileRows++
		} else if r.ReplaceWith > 0 {
			stats.IDRows++
		}
		at := strings.ToLower(strings.TrimSpace(r.AssetType))
		k := replacement.Key{AssetID: r.TargetID, SlotIndex: -1}
		if at == "texturepack" {
			stats.TextureRows++
			if r.Slot != nil {
				k.SlotIndex = *r.Slot
			}
		}
		keys[k]++
	}
	stats.UniqueKeys = len(keys)
	for _, n := range keys {
		if n > 1 {
			stats.DuplicateKeys++
		}
	}
	return stats
}

func (a *App) GetProxyStatus() ProxyStatus {
	a.errMu.RLock()
	defer a.errMu.RUnlock()
	msg := ""
	if a.runErr != nil {
		msg = a.runErr.Error()
	}
	return ProxyStatus{
		Active:        a.active.Load(),
		Elevated:      winutil.ProcessIsElevated(),
		LastLifecycle: msg,
	}
}

func (a *App) GetLogs() []string {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	out := make([]string, len(a.logs))
	copy(out, a.logs)
	return out
}

func (a *App) GetCatalogue() *catalogue.Snapshot {
	return a.catalog
}

func (a *App) LoadPackFromURL(rawURL string) (*skpack.Pack, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("bad url")
	}
	a.writeLog("pack: load from url (%s)", rawURL)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	packsDir, err := paths.PacksDir()
	if err != nil {
		a.writeLog("pack: load from url failed: %v", err)
		return nil, err
	}

	p, zipBytes, profileID, err := skpack.FetchPackFromURL(ctx, rawURL)
	if err != nil {
		a.writeLog("pack: load from url failed: %v", err)
		return nil, err
	}
	if len(zipBytes) > 0 {
		if err := a.ensurePackProfileSlot(packsDir, profileID); err != nil {
			if errors.Is(err, errPackInstallCancelled) {
				a.writeLog("pack: url mgpack install cancelled (profile conflict)")
				return nil, nil
			}
			a.writeLog("pack: load from url failed: %v", err)
			return nil, err
		}
		p, err = skpack.InstallMgpackFromBytes(zipBytes, packsDir, true)
		if err != nil {
			a.writeLog("pack: load from url failed: %v", err)
			return nil, err
		}
		a.writeLog("pack: url mgpack installed (profile=%q)", profileID)
	}

	st := summarizePack(p)
	a.writeLog("pack: load from url ok (rows=%d id=%d file=%d texturepack=%d unique_keys=%d dup_keys=%d)", st.Rows, st.IDRows, st.FileRows, st.TextureRows, st.UniqueKeys, st.DuplicateKeys)
	return p, nil
}

type InstalledPackInfo struct {
	ProfileID string `json:"profile_id"`
	Name      string `json:"name"`
	Author    string `json:"author"`
	Version   string `json:"version"`
}

func (a *App) LoadMgpackFromFile() (*skpack.Pack, error) {
	fp, err := runtime.OpenFileDialog(a.startupCtx, runtime.OpenDialogOptions{
		Title: "Megiddo pack",
		Filters: []runtime.FileFilter{
			{DisplayName: "Megiddo pack (*.mgpack)", Pattern: "*.mgpack"},
			{DisplayName: "All files", Pattern: "*"},
		},
	})
	if err != nil {
		return nil, err
	}
	fp = strings.TrimSpace(fp)
	if fp == "" {
		a.writeLog("pack: mgpack import cancelled")
		return nil, nil
	}
	return a.importMgpackFile(fp)
}

func (a *App) ExportMgpack(p skpack.Pack) (string, error) {
	if strings.TrimSpace(p.Name) == "" {
		return "", fmt.Errorf("pack name is required to export an mgpack")
	}
	st := summarizePack(&p)
	profileID := skpack.ProfileID(p.Name, p.Author)
	a.writeLog("pack: mgpack export requested (profile=%q name=%q rows=%d file=%d)", profileID, p.Name, st.Rows, st.FileRows)

	packsDir, err := paths.PacksDir()
	if err != nil {
		return "", err
	}
	if err := a.ensurePackProfileSlot(packsDir, profileID); err != nil {
		if errors.Is(err, errPackInstallCancelled) {
			a.writeLog("pack: mgpack export cancelled (profile conflict)")
			return "", nil
		}
		return "", err
	}

	fp, err := runtime.SaveFileDialog(a.startupCtx, runtime.SaveDialogOptions{
		DefaultFilename: profileID + ".mgpack",
		Title:           "Export Megiddo pack",
		Filters: []runtime.FileFilter{
			{DisplayName: "Megiddo pack (*.mgpack)", Pattern: "*.mgpack"},
			{DisplayName: "All files", Pattern: "*"},
		},
	})
	if err != nil {
		a.writeLog("pack: mgpack export dialog failed: %v", err)
		return "", err
	}
	fp = strings.TrimSpace(fp)
	if fp == "" {
		a.writeLog("pack: mgpack export cancelled")
		return "", nil
	}
	f, err := os.Create(fp)
	if err != nil {
		return "", err
	}
	if err := skpack.WriteMgpack(f, &p); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if _, err := skpack.InstallMgpack(fp, packsDir, profileID, true); err != nil {
		a.writeLog("pack: mgpack installed to profile failed: %v", err)
		return fp, err
	}
	a.writeLog("pack: mgpack exported %s and installed profile %q", fp, profileID)
	return fp, nil
}

func (a *App) ListInstalledMgpacks() ([]InstalledPackInfo, error) {
	packsDir, err := paths.PacksDir()
	if err != nil {
		return nil, err
	}
	list, err := skpack.ListInstalled(packsDir)
	if err != nil {
		return nil, err
	}
	out := make([]InstalledPackInfo, len(list))
	for i, s := range list {
		out[i] = InstalledPackInfo{
			ProfileID: s.ProfileID,
			Name:      s.Name,
			Author:    s.Author,
			Version:   s.Version,
		}
	}
	return out, nil
}

func (a *App) LoadInstalledMgpack(profileID string) (*skpack.Pack, error) {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return nil, fmt.Errorf("empty profile id")
	}
	packsDir, err := paths.PacksDir()
	if err != nil {
		return nil, err
	}
	a.writeLog("pack: load installed profile %q", profileID)
	p, err := skpack.LoadInstalled(packsDir, profileID)
	if err != nil {
		a.writeLog("pack: load installed profile failed: %v", err)
		return nil, err
	}
	st := summarizePack(p)
	a.writeLog("pack: load installed profile ok (rows=%d file=%d)", st.Rows, st.FileRows)
	return p, nil
}

func (a *App) SaveInstalledMgpack(profileID string, p skpack.Pack) error {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return fmt.Errorf("empty profile id")
	}
	packsDir, err := paths.PacksDir()
	if err != nil {
		return err
	}
	dest := filepath.Join(packsDir, profileID)
	if st, err := os.Stat(dest); err != nil || !st.IsDir() {
		return fmt.Errorf("profile %q is not installed", profileID)
	}
	a.writeLog("pack: save installed profile %q (name=%q rows=%d)", profileID, p.Name, len(p.Replacements))
	if err := skpack.WriteInstalledManifest(dest, &p); err != nil {
		a.writeLog("pack: save installed profile failed: %v", err)
		return err
	}
	a.writeLog("pack: save installed profile ok")
	return nil
}

func (a *App) DeleteInstalledMgpack(profileID string) error {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return fmt.Errorf("empty profile id")
	}
	packsDir, err := paths.PacksDir()
	if err != nil {
		return err
	}
	target := filepath.Join(packsDir, profileID)
	if _, err := os.Stat(target); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	a.writeLog("pack: deleted installed profile %q", profileID)
	return nil
}

func (a *App) MergeInstalledMgpacks(profileIDs []string) (*skpack.Pack, error) {
	packs, err := a.loadInstalledProfiles(profileIDs)
	if err != nil {
		return nil, err
	}
	merged, err := skpack.MergePacks(packs)
	if err != nil {
		return nil, err
	}
	st := summarizePack(merged)
	a.writeLog("pack: merged profiles (%d) -> rows=%d unique_keys=%d dup_keys=%d", len(profileIDs), st.Rows, st.UniqueKeys, st.DuplicateKeys)
	return merged, nil
}

var errPackInstallCancelled = errors.New("pack install cancelled")

func (a *App) importMgpackFile(mgpackPath string) (*skpack.Pack, error) {
	a.writeLog("pack: import mgpack (%s)", mgpackPath)
	profileID, _, err := skpack.PeekProfileID(mgpackPath)
	if err != nil {
		a.writeLog("pack: mgpack import failed: %v", err)
		return nil, err
	}
	packsDir, err := paths.PacksDir()
	if err != nil {
		return nil, err
	}
	if err := a.ensurePackProfileSlot(packsDir, profileID); err != nil {
		if errors.Is(err, errPackInstallCancelled) {
			a.writeLog("pack: mgpack import cancelled (profile conflict)")
			return nil, nil
		}
		return nil, err
	}
	if _, err := skpack.InstallMgpack(mgpackPath, packsDir, profileID, true); err != nil {
		a.writeLog("pack: mgpack install failed: %v", err)
		return nil, err
	}
	p, err := skpack.LoadInstalled(packsDir, profileID)
	if err != nil {
		return nil, err
	}
	st := summarizePack(p)
	a.writeLog("pack: mgpack import ok (profile=%q rows=%d file=%d)", profileID, st.Rows, st.FileRows)
	return p, nil
}

func (a *App) ensurePackProfileSlot(packsDir, profileID string) error {
	dest := filepath.Join(packsDir, profileID)
	if _, err := os.Stat(dest); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	replace, err := a.confirmReplaceInstalledPack(profileID)
	if err != nil {
		return err
	}
	if !replace {
		return errPackInstallCancelled
	}
	return nil
}

func (a *App) confirmReplaceInstalledPack(profileID string) (bool, error) {
	msg := fmt.Sprintf("Pack profile %q is already installed.\n\nReplace the existing copy?", profileID)
	btn, err := runtime.MessageDialog(a.startupCtx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "Replace installed pack?",
		Message:       msg,
		DefaultButton: "No",
	})
	if err != nil {
		return false, err
	}
	return btn == "Yes", nil
}

func (a *App) ApplyPack(p skpack.Pack) error {
	st := summarizePack(&p)
	a.writeLog("pack: apply requested (name=%q rows=%d id=%d file=%d texturepack=%d unique_keys=%d dup_keys=%d)", p.Name, st.Rows, st.IDRows, st.FileRows, st.TextureRows, st.UniqueKeys, st.DuplicateKeys)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := (&p).ValidateReplaceDelivery(ctx); err != nil {
		a.writeLog("pack: id reachability warning (continuing apply): %v", err)
	} else {
		a.writeLog("pack: apply id reachability validation ok")
	}
	tab, err := (&p).ReplacementTable()
	if err != nil {
		a.writeLog("pack: apply failed while building replacement table: %v", err)
		return err
	}
	idEntries := 0
	fileEntries := 0
	removeEntries := 0
	for _, e := range tab {
		switch {
		case e.IsFile():
			fileEntries++
		case e.IsRemove():
			removeEntries++
		case e.IsID():
			idEntries++
		}
	}
	a.rep.Swap(tab)
	a.files.Swap(tab)
	a.writeLog("pack: apply ok (active_keys=%d id_entries=%d file_entries=%d clear_entries=%d)", len(tab), idEntries, fileEntries, removeEntries)
	for k, e := range tab {
		switch {
		case e.IsFile():
			a.writeLog("pack: registered key assetID=%d slot=%d -> file %s", k.AssetID, k.SlotIndex, e.FilePath)
		case e.IsRemove():
			a.writeLog("pack: registered key assetID=%d slot=%d -> clear (asset 0)", k.AssetID, k.SlotIndex)
		default:
			a.writeLog("pack: registered key assetID=%d slot=%d -> id %d", k.AssetID, k.SlotIndex, e.AssetID)
		}
	}
	return nil
}

func (a *App) ClearPack() {
	a.writeLog("pack: clear swaps requested")
	a.rep.Swap(make(map[replacement.Key]replacement.Entry))
	a.files.Clear()
	a.writeLog("pack: clear swaps ok")
}

func (a *App) IsMegiddoEnabled() bool {
	return a.rep.Len() > 0
}

func (a *App) EnableMegiddo(p skpack.Pack) error {
	return a.ApplyPack(p)
}

func (a *App) EnableMegiddoProfiles(profileIDs []string) error {
	packs, err := a.loadInstalledProfiles(profileIDs)
	if err != nil {
		return err
	}
	merged, err := skpack.MergePacks(packs)
	if err != nil {
		return err
	}
	return a.ApplyPack(*merged)
}

func (a *App) DisableMegiddo() {
	a.ClearPack()
}

func (a *App) loadInstalledProfiles(profileIDs []string) ([]*skpack.Pack, error) {
	packsDir, err := paths.PacksDir()
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	out := make([]*skpack.Pack, 0, len(profileIDs))
	for _, raw := range profileIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		p, err := skpack.LoadInstalled(packsDir, id)
		if err != nil {
			return nil, fmt.Errorf("load profile %q: %w", id, err)
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no profiles selected")
	}
	return out, nil
}

func (a *App) PickLocalAssetFile(assetType string) (string, error) {
	fp, err := runtime.OpenFileDialog(a.startupCtx, runtime.OpenDialogOptions{
		Title: "Pick local asset file",
		Filters: []runtime.FileFilter{
			{DisplayName: "Roblox assets (*.ktx2;*.png;*.jpg;*.jpeg;*.webp;*.mesh;*.ogg;*.mp3;*.wav)", Pattern: "*.ktx2;*.png;*.jpg;*.jpeg;*.webp;*.mesh;*.ogg;*.mp3;*.wav"},
			{DisplayName: "All files", Pattern: "*"},
		},
	})
	if err != nil {
		a.writeLog("pack: local file picker failed: %v", err)
		return "", err
	}
	out := strings.TrimSpace(fp)
	if out == "" {
		a.writeLog("pack: local file picker cancelled")
		return "", nil
	}

	if strings.EqualFold(strings.TrimSpace(assetType), "texturepack") && ktx2encode.IsSupported(out) {
		a.writeLog("pack: converting %s -> KTX2…", filepath.Base(out))
		converted, convErr := convertToKTX2Cached(out)
		if convErr != nil {
			a.writeLog("pack: KTX2 conversion failed (%v), returning original path", convErr)
		} else {
			a.writeLog("pack: converted to KTX2: %s", filepath.Base(converted))
			out = converted
		}
	}

	a.writeLog("pack: local file selected (%s)", out)
	return out, nil
}

func convertToKTX2Cached(src string) (string, error) {
	cacheDir, err := paths.KTX2CacheDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	abs, err := filepath.Abs(src)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	key := fmt.Sprintf("%s|%d|%d", abs, st.Size(), st.ModTime().UnixNano())
	sum := sha1.Sum([]byte(key))
	dst := filepath.Join(cacheDir, fmt.Sprintf("%x.ktx2", sum))
	if _, err := os.Stat(dst); err == nil {
		return dst, nil
	}
	data, err := ktx2encode.FromImageFile(abs)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return "", err
	}
	return dst, nil
}

func certPatchStatusFrom(states []roblox.TrustBundleState) CertPatchStatus {
	out := CertPatchStatus{
		Total:   len(states),
		Details: make([]string, 0, len(states)),
	}
	for _, s := range states {
		name := filepath.Base(s.Root)
		switch {
		case s.Error != "":
			out.Unpatched++
			out.Details = append(out.Details, fmt.Sprintf("%s: error: %s", name, s.Error))
		case !s.Exists:
			out.Unpatched++
			out.Details = append(out.Details, fmt.Sprintf("%s: missing ssl/cacert.pem", name))
		case s.Patched:
			out.Patched++
			out.Details = append(out.Details, fmt.Sprintf("%s: patched", name))
		default:
			out.Unpatched++
			out.Details = append(out.Details, fmt.Sprintf("%s: not patched", name))
		}
	}
	return out
}

func ensureCAPEM() ([]byte, error) {
	caDir, err := paths.ProxyCADir()
	if err != nil {
		return nil, err
	}
	if err := certmgr.EnsureCA(caDir); err != nil {
		return nil, fmt.Errorf("CA not ready: %w", err)
	}
	caPEM, err := certmgr.ReadCAPEM(caDir)
	if err != nil {
		return nil, err
	}
	return caPEM, nil
}

func (a *App) GetRobloxCertPatchStatus() (CertPatchStatus, error) {
	roots := roblox.DiscoverInstallRoots()
	if _, err := ensureCAPEM(); err != nil {
		return CertPatchStatus{}, err
	}
	return certPatchStatusFrom(roblox.TrustBundleStatus(roots)), nil
}

func (a *App) PatchRobloxCerts() (CertPatchStatus, error) {
	roots := roblox.DiscoverInstallRoots()
	a.writeLog("cert: patch requested (installs=%d)", len(roots))
	caPEM, err := ensureCAPEM()
	if err != nil {
		a.writeLog("cert: patch failed to ensure CA: %v", err)
		return CertPatchStatus{}, err
	}
	notes, err := roblox.UpsertTrustBundle(roots, caPEM)
	if err != nil {
		a.writeLog("cert: patch failed: %v", err)
		return CertPatchStatus{}, err
	}
	for _, line := range notes {
		a.writeLog("cert: %s", line)
	}
	states := roblox.TrustBundleStatus(roots)
	out := certPatchStatusFrom(states)
	a.writeLog("cert: patch ok (patched=%d unpatched=%d total=%d)", out.Patched, out.Unpatched, out.Total)
	return out, nil
}

func (a *App) UnpatchRobloxCerts() (CertPatchStatus, error) {
	roots := roblox.DiscoverInstallRoots()
	a.writeLog("cert: unpatch requested (installs=%d)", len(roots))
	notes, err := roblox.RemoveTrustBundle(roots)
	if err != nil {
		a.writeLog("cert: unpatch failed: %v", err)
		return CertPatchStatus{}, err
	}
	for _, line := range notes {
		a.writeLog("cert: %s", line)
	}
	states := roblox.TrustBundleStatus(roots)
	out := certPatchStatusFrom(states)
	a.writeLog("cert: unpatch ok (patched=%d unpatched=%d total=%d)", out.Patched, out.Unpatched, out.Total)
	return out, nil
}

func (a *App) ClearRobloxCache() (string, error) {
	local := os.Getenv("LOCALAPPDATA")
	if strings.TrimSpace(local) == "" {
		return "", fmt.Errorf("LOCALAPPDATA is not set")
	}
	root := filepath.Join(local, "Roblox")
	fileTargets := []string{
		filepath.Join(local, "Roblox", "rbx-storage.db"),
		filepath.Join(local, "RobloxPCGDK", "rbx-storage.db"),
	}
	dirTargets := []string{
		filepath.Join(local, "Roblox", "rbx-storage"),
	}

	a.writeLog("cache: clear requested (root=%s)", root)
	deleted := 0
	missing := 0
	failed := 0

	for _, fp := range fileTargets {
		miss, err := deletePath(fp)
		switch {
		case miss:
			missing++
			a.writeLog("cache: skip missing %s", fp)
		case err != nil:
			failed++
			a.writeLog("cache: clear failed %s: %v", fp, err)
		default:
			deleted++
			a.writeLog("cache: deleted %s", fp)
		}
	}

	for _, dir := range dirTargets {
		miss, err := deletePath(dir)
		switch {
		case miss:
			missing++
			a.writeLog("cache: skip missing %s", dir)
		case err != nil:
			failed++
			a.writeLog("cache: clear failed %s: %v", dir, err)
		default:
			deleted++
			a.writeLog("cache: deleted %s", dir)
		}
	}

	summary := fmt.Sprintf("cache clear done (deleted=%d missing=%d failed=%d)", deleted, missing, failed)
	a.writeLog("cache: %s", summary)
	if failed > 0 {
		return summary, fmt.Errorf("one or more cache paths could not be removed (close Roblox/Studio and retry)")
	}
	return summary, nil
}

func deletePath(path string) (missing bool, err error) {
	if _, stErr := os.Stat(path); stErr != nil {
		if errors.Is(stErr, os.ErrNotExist) {
			return true, nil
		}
		return false, stErr
	}
	if rmErr := os.RemoveAll(path); rmErr != nil {
		return false, rmErr
	}
	return false, nil
}
