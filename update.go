package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// build time via -ldflags "-X main.version=build-N"
var version = "seven"

type UpdateInfo struct {
	Tag string `json:"tag"`
	URL string `json:"url"`
}

func (a *App) CheckUpdate() (*UpdateInfo, error) {
	if !strings.HasPrefix(version, "build-") {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/repos/paradoxum-wikis/megiddo/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var rel struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}

	if buildNum(rel.TagName) <= buildNum(version) {
		return nil, nil
	}
	return &UpdateInfo{Tag: rel.TagName, URL: rel.HTMLURL}, nil
}

func buildNum(tag string) int {
	n, _ := strconv.Atoi(strings.TrimPrefix(tag, "build-"))
	return n
}
