package texpacklookup

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type parentSlotKey struct {
	ParentID int64
	Slot     int
}

type Store struct {
	mu      sync.RWMutex
	reverse map[parentSlotKey]int64
	seen    map[int64]bool
	cookie  string
	placeID string
}

func New() *Store {
	return &Store{
		reverse: make(map[parentSlotKey]int64),
		seen:    make(map[int64]bool),
	}
}

func (s *Store) ObserveAuth(cookie, placeID string) {
	if cookie == "" && placeID == "" {
		return
	}
	s.mu.Lock()
	if cookie != "" {
		s.cookie = cookie
	}
	if placeID != "" {
		s.placeID = placeID
	}
	s.mu.Unlock()
}

func (s *Store) ReverseLookup(parentID int64, slot int) (int64, bool) {
	s.mu.RLock()
	subID, ok := s.reverse[parentSlotKey{ParentID: parentID, Slot: slot}]
	if !ok && slot >= 32 {
		subID, ok = s.reverse[parentSlotKey{ParentID: parentID, Slot: slot - 32}]
	}
	s.mu.RUnlock()
	return subID, ok
}

var fidelitySlotForTag = map[string]int{
	"color": 0, "albedo": 0, "diffuse": 0, "basecolor": 0,
	"normal": 1, "normalmap": 1, "bumpmap": 1,
	"metalness": 2, "orm": 2, "roughness": 2,
	"emissive": 2, "emissivemap": 2,
	"height": 2, "displacement": 2, "heightmap": 2,
}

func (s *Store) Learn(parentID int64, xmlBody []byte) int {
	type anyElem struct {
		XMLName xml.Name
		Text    string `xml:",chardata"`
	}
	type root struct {
		Children []anyElem `xml:",any"`
	}
	var r root
	if err := xml.Unmarshal(xmlBody, &r); err != nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	added := 0
	for _, child := range r.Children {
		tag := strings.ToLower(child.XMLName.Local)
		slot, ok := fidelitySlotForTag[tag]
		if !ok {
			continue
		}
		text := strings.TrimSpace(child.Text)
		if text == "" || text == "0" {
			continue
		}
		subID, err := strconv.ParseInt(text, 10, 64)
		if err != nil || subID == 0 {
			continue
		}
		s.reverse[parentSlotKey{ParentID: parentID, Slot: slot}] = subID
		added++
	}
	return added
}

func (s *Store) DiscoverAsync(parentID int64, logf func(string, ...any)) {
	s.mu.Lock()
	if s.seen[parentID] {
		s.mu.Unlock()
		return
	}
	s.seen[parentID] = true
	cookie := s.cookie
	placeID := s.placeID
	s.mu.Unlock()

	go func() {
		body, status, err := fetchTexpackXML(parentID, cookie, placeID)
		if err != nil {
			if (status == 401 || status == 403) && cookie == "" {
				s.mu.Lock()
				delete(s.seen, parentID)
				s.mu.Unlock()
			}
			if logf != nil {
				authHint := ""
				if cookie == "" {
					authHint = " (no auth yet)"
				}
				logf("texpacklookup: parent %d fetch failed (status=%d)%s: %v", parentID, status, authHint, err)
			}
			return
		}
		n := s.Learn(parentID, body)
		if logf != nil {
			if n > 0 {
				logf("texpacklookup: parent %d -> %d sub-asset(s) learned", parentID, n)
			} else {
				logf("texpacklookup: parent %d - XML returned 0 entries (body prefix: %q)", parentID, truncate(body, 80))
			}
		}
	}()
}

func fetchTexpackXML(parentID int64, cookie, placeID string) (body []byte, status int, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cl := &http.Client{
		Timeout: 12 * time.Second,
		Transport: &http.Transport{
			Proxy: nil,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, //nolint:gosec // hosts redirect hits our mitm ca on loopback
			},
			TLSHandshakeTimeout: 8 * time.Second,
		},
	}

	u := fmt.Sprintf("https://assetdelivery.roblox.com/v1/asset/?id=%d", parentID)

	doReq := func(extraPlaceID string) ([]byte, int, error) {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if reqErr != nil {
			return nil, 0, reqErr
		}
		req.Header.Set("User-Agent", "Roblox/WinInet")
		if cookie != "" {
			req.Header.Set("Cookie", cookie)
		}
		if extraPlaceID != "" {
			req.Header.Set("Roblox-Place-Id", extraPlaceID)
		}
		resp, doErr := cl.Do(req)
		if doErr != nil {
			return nil, 0, doErr
		}
		defer resp.Body.Close()
		b, readErr := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
		if readErr != nil {
			return nil, resp.StatusCode, readErr
		}
		return b, resp.StatusCode, nil
	}

	b, code, doErr := doReq(placeID)
	if doErr != nil {
		return nil, 0, doErr
	}
	if code != http.StatusOK {
		return nil, code, fmt.Errorf("HTTP %d", code)
	}
	if len(b) == 0 {
		return nil, code, fmt.Errorf("empty body")
	}
	if !bytes.HasPrefix(b, []byte("<")) && !bytes.HasPrefix(b, []byte("\xef\xbb\xbf<")) {
		return nil, code, fmt.Errorf("not XML (first bytes: %x)", truncate(b, 8))
	}
	return b, code, nil
}

func truncate(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	return b[:n]
}
