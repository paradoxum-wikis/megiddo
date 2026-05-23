package pack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

func FetchAlterEgoRevPack(ctx context.Context, revID int64) (pack *Pack, zipBytes []byte, profileID string, err error) {
	pl, err := resolveAlterEgoRevPayload(ctx, revID)
	if err != nil {
		return nil, nil, "", err
	}
	if len(pl.download) > 0 {
		if IsZipBytes(pl.download) {
			pid, _, err := PeekProfileIDFromBytes(pl.download)
			if err != nil {
				return nil, nil, "", err
			}
			return nil, pl.download, pid, nil
		}
		p, err := DecodePackLoose(pl.download)
		return p, nil, "", err
	}
	p, err := DecodePackLoose([]byte(pl.inline))
	return p, nil, "", err
}

func DecodePackFromAlterEgoWikiRev(ctx context.Context, revID int64) (*Pack, error) {
	p, zipBytes, _, err := FetchAlterEgoRevPack(ctx, revID)
	if err != nil {
		return nil, err
	}
	if len(zipBytes) > 0 {
		return nil, fmt.Errorf("revision points to binary mgpack; install via packs directory")
	}
	return p, nil
}

type alterEgoRevPayload struct {
	inline   string
	download []byte
}

func resolveAlterEgoRevPayload(ctx context.Context, revID int64) (*alterEgoRevPayload, error) {
	content, err := fetchWikiRevisionContent(ctx, revID)
	if err != nil {
		return nil, err
	}
	if mgURL, ok := extractFirstCommentURL(content); ok {
		raw, err := DownloadBytesFromURL(ctx, mgURL)
		if err != nil {
			return nil, fmt.Errorf("download mgpack from %s: %w", mgURL, err)
		}
		return &alterEgoRevPayload{download: raw}, nil
	}
	return &alterEgoRevPayload{inline: content}, nil
}

func fetchWikiRevisionContent(ctx context.Context, revID int64) (string, error) {
	u := fmt.Sprintf(
		"https://alter-ego.fandom.com/api.php?action=query&prop=revisions&rvprop=content&rvslots=main&format=json&revids=%s",
		strconv.FormatInt(revID, 10))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Megiddo/2.0 (ALTER EGO oldid importer)")
	resp, err := fetchClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("wiki http %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPackDownloadBytes+1))
	if err != nil {
		return "", err
	}
	if len(body) > maxPackDownloadBytes {
		return "", fmt.Errorf("wiki payload exceeds byte limit (%d)", maxPackDownloadBytes)
	}
	content, err := wikiRevisionMarkdown(body)
	if err != nil {
		return "", err
	}
	return content, nil
}

var commentURLRe = regexp.MustCompile(`(?is)<!--\s*(https?://[^\s>]+)\s*-->`)

func extractFirstCommentURL(s string) (string, bool) {
	m := commentURLRe.FindStringSubmatch(s)
	if len(m) < 2 {
		return "", false
	}
	return strings.TrimSpace(m[1]), true
}

func wikiRevisionMarkdown(apiResp []byte) (string, error) {
	var root map[string]any
	if err := json.Unmarshal(apiResp, &root); err != nil {
		return "", fmt.Errorf("wiki json: %w", err)
	}
	if errObj, ok := root["error"].(map[string]any); ok {
		info, _ := errObj["info"].(string)
		if info == "" {
			info, _ = errObj["code"].(string)
		}
		if info == "" {
			info = "unknown wiki api error"
		}
		return "", fmt.Errorf("wiki api error: %s", info)
	}
	qany, ok := root["query"]
	if !ok {
		if msg := wikiAPIWarning(root); msg != "" {
			return "", fmt.Errorf("wiki response missing query (%s)", msg)
		}
		return "", fmt.Errorf("wiki response missing query")
	}
	query, ok := qany.(map[string]any)
	if !ok {
		return "", fmt.Errorf("wiki response query malformed")
	}
	if _, bad := query["badrevids"]; bad {
		return "", fmt.Errorf("unknown revision id (badrevids)")
	}
	pagesAny, ok := query["pages"]
	if !ok {
		return "", fmt.Errorf("wiki response missing pages")
	}
	pages, ok := pagesAny.(map[string]any)
	if !ok {
		return "", fmt.Errorf("wiki pages map malformed")
	}
	for _, pageVal := range pages {
		pg, ok := pageVal.(map[string]any)
		if !ok {
			continue
		}
		if miss, ok := pg["missing"].(bool); ok && miss {
			continue
		}
		revs, ok := pg["revisions"].([]any)
		if !ok || len(revs) == 0 {
			continue
		}
		revObj, ok := revs[0].(map[string]any)
		if !ok {
			continue
		}
		if slots, ok := revObj["slots"].(map[string]any); ok {
			if main, ok := slots["main"].(map[string]any); ok {
				for _, field := range []string{"*", "content"} {
					if s, ok := stringField(main[field]); ok {
						return s, nil
					}
				}
			}
		}
		if s, ok := stringField(revObj["*"]); ok {
			return s, nil
		}
	}
	return "", fmt.Errorf("wiki revision contained no textual content slots")
}

func wikiAPIWarning(root map[string]any) string {
	warnAny, ok := root["warnings"]
	if !ok {
		return ""
	}
	warn, ok := warnAny.(map[string]any)
	if !ok {
		return ""
	}
	main, ok := warn["main"].(map[string]any)
	if !ok {
		return ""
	}
	msg, _ := main["*"].(string)
	return strings.TrimSpace(msg)
}

func stringField(v any) (string, bool) {
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return "", false
	}
	return s, true
}
