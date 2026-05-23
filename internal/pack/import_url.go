package pack

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxPackDownloadBytes = 12 << 20

var fetchClient = &http.Client{
	Timeout: 2 * time.Minute,
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		TLSHandshakeTimeout:   20 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          8,
		MaxIdleConnsPerHost:   8,
	},
}

func FetchPackFromURL(ctx context.Context, raw string) (pack *Pack, zipBytes []byte, profileID string, err error) {
	body, err := DownloadBytesFromURL(ctx, raw)
	if err != nil {
		return nil, nil, "", err
	}
	if IsZipBytes(body) {
		profileID, _, err = PeekProfileIDFromBytes(body)
		return nil, body, profileID, err
	}
	pack, err = DecodePackLoose(body)
	return pack, nil, "", err
}

func DownloadBytesFromURL(ctx context.Context, raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("bad url")
	}
	if scheme := strings.ToLower(u.Scheme); scheme != "http" && scheme != "https" {
		return nil, fmt.Errorf("unsupported url scheme")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Megiddo/2.0 (ALTER EGO pack import)")
	resp, err := fetchClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("http %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPackDownloadBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxPackDownloadBytes {
		return nil, fmt.Errorf("pack download exceeds byte limit (%d)", maxPackDownloadBytes)
	}
	return body, nil
}
