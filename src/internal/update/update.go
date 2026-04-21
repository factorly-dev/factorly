// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/factorly-dev/factorly/internal"
)

const (
	checkInterval = 24 * time.Hour
	cacheFile     = "version-check.json"
)

// releaseURL is a var so tests can override it.
var releaseURL = "https://factorly.com/.releases/latest"

type cachedCheck struct {
	LatestVersion string    `json:"latest_version"`
	CheckedAt     time.Time `json:"checked_at"`
}

// Result describes the outcome of a version check.
type Result struct {
	UpToDate bool   // true if current version is the latest
	Message  string // update message (non-empty only if update available)
}

// Check returns the version check result, using the 24-hour cache.
func Check() Result {
	cache := loadCache()

	if cache != nil && time.Since(cache.CheckedAt) < checkInterval {
		return makeResult(cache.LatestVersion)
	}

	return checkAndCache()
}

// CheckNow returns the version check result, bypassing the cache.
func CheckNow() Result {
	return checkAndCache()
}

func checkAndCache() Result {
	latest, err := fetchLatestVersion()
	if err != nil {
		return Result{} // network failure — unknown state
	}

	saveCache(&cachedCheck{
		LatestVersion: latest,
		CheckedAt:     time.Now(),
	})

	return makeResult(latest)
}

func makeResult(latest string) Result {
	current := internal.Version
	if latest == "" {
		return Result{}
	}
	if !isNewer(latest, current) {
		return Result{UpToDate: true}
	}
	return Result{
		Message: fmt.Sprintf("Update available: v%s → v%s (%s)", current, latest, updateCommand()),
	}
}

func updateCommand() string {
	exe, err := os.Executable()
	if err != nil {
		return "https://github.com/factorly-dev/factorly/releases"
	}
	exe, _ = filepath.EvalSymlinks(exe)

	switch {
	case strings.Contains(exe, "node_modules"):
		return "npm install -g factorly"
	case strings.Contains(exe, "site-packages"):
		return "pip install --upgrade factorly"
	default:
		return "https://github.com/factorly-dev/factorly/releases"
	}
}

func fetchLatestVersion() (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", releaseURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", fmt.Sprintf("factorly/%s", internal.Version))

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}

	return strings.TrimPrefix(release.TagName, "v"), nil
}

func isNewer(latest, current string) bool {
	lp := parseSemver(latest)
	cp := parseSemver(current)
	if lp[0] != cp[0] {
		return lp[0] > cp[0]
	}
	if lp[1] != cp[1] {
		return lp[1] > cp[1]
	}
	return lp[2] > cp[2]
}

func parseSemver(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	var parts [3]int
	n, err := fmt.Sscanf(v, "%d.%d.%d", &parts[0], &parts[1], &parts[2])
	if err != nil || n != 3 {
		return [3]int{0, 0, 0}
	}
	return parts
}

func cacheFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "factorly", cacheFile)
}

func loadCache() *cachedCheck {
	path := cacheFilePath()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cache cachedCheck
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil
	}
	return &cache
}

func saveCache(cache *cachedCheck) {
	path := cacheFilePath()
	if path == "" {
		return
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, data, 0o600)
}
