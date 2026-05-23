package pack

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"megiddo/internal/blankktx"
	"megiddo/internal/replacement"
	"megiddo/internal/resolving"
)

func (p *Pack) ReplacementTable() (map[replacement.Key]replacement.Entry, error) {
	if p == nil {
		return nil, fmt.Errorf("nil pack")
	}
	out := make(map[replacement.Key]replacement.Entry)
	for _, r := range p.Replacements {
		entry, err := buildEntry(r)
		if err != nil {
			return nil, err
		}
		entry, err = resolveRemoveEntry(r, entry)
		if err != nil {
			return nil, err
		}

		at := strings.ToLower(strings.TrimSpace(r.AssetType))
		var k replacement.Key
		switch at {
		case "texturepack":
			if r.Slot == nil {
				return nil, fmt.Errorf("texturepack %q (%d) missing slot column", r.Label, r.TargetID)
			}
			if *r.Slot < 0 {
				return nil, fmt.Errorf("texturepack slot may not be negative (asset %d)", r.TargetID)
			}
			if r.TargetID == 0 {
				return nil, fmt.Errorf("texturepack %q: target_id must be non-zero", r.Label)
			}
			k = replacement.Key{AssetID: r.TargetID, SlotIndex: -1}
		default:
			k = replacement.Key{AssetID: r.TargetID, SlotIndex: -1}
		}
		if prev, dup := out[k]; dup && prev != entry {
			return nil, fmt.Errorf("conflicting replacements for %+v", k)
		}
		out[k] = entry
	}
	return out, nil
}

func buildEntry(r Replacement) (replacement.Entry, error) {
	path := strings.TrimSpace(r.ReplaceWithFile)
	if path != "" {
		at := strings.ToLower(strings.TrimSpace(r.AssetType))
		if at == "texturepack" {
			if ext := strings.ToLower(filepath.Ext(path)); ext != ".ktx2" {
				return replacement.Entry{}, fmt.Errorf("row %q (target %d) texturepack local file must be .ktx2 (got %q)", r.Label, r.TargetID, ext)
			}
		}
		if !filepath.IsAbs(path) {
			return replacement.Entry{}, fmt.Errorf("row %q (target %d) replace_with_file %q must be absolute (loaded packs auto-resolve relative paths; hand-edits should pick a file)", r.Label, r.TargetID, path)
		}
		if _, err := os.Stat(path); err != nil {
			return replacement.Entry{}, fmt.Errorf("row %q (target %d) replace_with_file: %w", r.Label, r.TargetID, err)
		}
		return replacement.Entry{FilePath: filepath.Clean(path)}, nil
	}
	if r.ReplaceWith == 0 {
		return replacement.RemoveEntry(), nil
	}
	if r.ReplaceWith < 0 {
		return replacement.Entry{}, fmt.Errorf("row %q (target %d) needs replace_with or replace_with_file", r.Label, r.TargetID)
	}
	return replacement.Entry{AssetID: r.ReplaceWith}, nil
}

func resolveRemoveEntry(r Replacement, entry replacement.Entry) (replacement.Entry, error) {
	if !entry.IsRemove() {
		return entry, nil
	}
	at := strings.ToLower(strings.TrimSpace(r.AssetType))
	switch at {
	case "texturepack":
		fp, err := blankktx.Ensure()
		if err != nil {
			return replacement.Entry{}, fmt.Errorf("row %q blank texture: %w", r.Label, err)
		}
		return replacement.Entry{FilePath: fp}, nil
	case "texture", "image", "decal", "mesh":
		return entry, nil
	default:
		return replacement.Entry{}, fmt.Errorf("row %q (target %d): replace_with 0 only for texture/image/decal/texturepack/mesh", r.Label, r.TargetID)
	}
}

func (p *Pack) ValidateReplaceDelivery(ctx context.Context) error {
	if p == nil {
		return fmt.Errorf("nil pack")
	}
	ids := uniqueReplaceWith(p)
	if len(ids) == 0 {
		return nil
	}
	const workers = 12
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var first error
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, id := range ids {
		id := id
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := headRobloxDelivery(ctx, id); err != nil {
				mu.Lock()
				if first == nil {
					first = fmt.Errorf("replace_with %d: %w", id, err)
					cancel()
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return first
}

func (p *Pack) ValidateAndSwap(ctx context.Context, m *replacement.Map) error {
	if p == nil {
		return fmt.Errorf("nil pack")
	}
	if m == nil {
		return fmt.Errorf("nil replacement map")
	}
	if err := p.ValidateReplaceDelivery(ctx); err != nil {
		return err
	}
	tab, err := p.ReplacementTable()
	if err != nil {
		return err
	}
	m.Swap(tab)
	return nil
}

func (p *Pack) ApplyValidated(ctx context.Context) (*replacement.Map, error) {
	if p == nil {
		return nil, fmt.Errorf("nil pack")
	}
	m := replacement.NewMap()
	if err := p.ValidateAndSwap(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

func uniqueReplaceWith(p *Pack) []int64 {
	if p == nil {
		return nil
	}
	seen := make(map[int64]struct{})
	out := make([]int64, 0, len(p.Replacements))
	for _, r := range p.Replacements {
		if strings.TrimSpace(r.ReplaceWithFile) != "" {
			continue
		}
		if r.ReplaceWith == 0 {
			continue
		}
		if _, ok := seen[r.ReplaceWith]; ok {
			continue
		}
		seen[r.ReplaceWith] = struct{}{}
		out = append(out, r.ReplaceWith)
	}
	return out
}

var assetHeadClient = &http.Client{
	Timeout: 18 * time.Second,
	Transport: &http.Transport{
		Proxy:                 nil,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          32,
		MaxIdleConnsPerHost:   32,
	},
}

func headRobloxDelivery(ctx context.Context, id int64) error {
	u := fmt.Sprintf("https://assetdelivery.roblox.com/v1/asset?id=%d", id)
	ips, err := resolving.RealIPs(ctx, "assetdelivery.roblox.com")
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("resolve real assetdelivery ip: %w", err)
	}
	var firstErr error
	for _, ip := range ips {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if reqErr != nil {
			return reqErr
		}
		req.Header.Set("User-Agent", "Megiddo/1.0")
		req.Header.Set("Accept", "*/*")

		cl := &http.Client{
			Timeout: assetHeadClient.Timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Transport: &http.Transport{
				Proxy:               nil,
				TLSHandshakeTimeout: 10 * time.Second,
				DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, network, net.JoinHostPort(ip, "443"))
				},
			},
		}
		resp, doErr := cl.Do(req)
		if doErr != nil {
			if firstErr == nil {
				firstErr = doErr
			}
			continue
		}
		defer resp.Body.Close()
		_, _ = io.CopyN(io.Discard, resp.Body, 1024)
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil // head probe lacks in-game auth; client fetch maaaaay still work
		}
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			return nil // redirect to CDN means assetdelivery recognized the id
		}
		if resp.StatusCode >= http.StatusBadRequest {
			return fmt.Errorf("status %s", resp.Status)
		}
		return nil
	}
	if firstErr != nil {
		return firstErr
	}
	return fmt.Errorf("no reachable assetdelivery endpoints")
}
