// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package update

import (
	"os"
	"testing"

	"github.com/factorly-dev/factorly/internal"
)

func TestIsNewer(t *testing.T) {
	tests := []struct {
		latest, current string
		want            bool
	}{
		{"1.0.0", "0.9.0", true},
		{"0.2.0", "0.1.9", true},
		{"0.1.10", "0.1.9", true},
		{"0.1.9", "0.1.9", false},
		{"0.1.8", "0.1.9", false},
		{"1.0.0", "1.0.0", false},
		{"2.0.0", "1.99.99", true},
	}
	for _, tt := range tests {
		t.Run(tt.latest+"_vs_"+tt.current, func(t *testing.T) {
			got := isNewer(tt.latest, tt.current)
			if got != tt.want {
				t.Errorf("isNewer(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
			}
		})
	}
}

func TestParseSemver(t *testing.T) {
	tests := []struct {
		input string
		want  [3]int
	}{
		{"1.2.3", [3]int{1, 2, 3}},
		{"v1.2.3", [3]int{1, 2, 3}},
		{"0.5.10", [3]int{0, 5, 10}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseSemver(tt.input)
			if got != tt.want {
				t.Errorf("parseSemver(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestUpdateCommandDetection(t *testing.T) {
	// Just verify it returns a non-empty string
	cmd := updateCommand()
	if cmd == "" {
		t.Error("expected non-empty update command")
	}
}

func TestMakeResultUpToDate(t *testing.T) {
	// When latest matches current, should be up to date
	result := makeResult(internal.Version)
	if !result.UpToDate {
		t.Error("expected UpToDate=true when versions match")
	}
	if result.Message != "" {
		t.Errorf("expected empty message, got %q", result.Message)
	}
}

func TestMakeResultUpdateAvailable(t *testing.T) {
	result := makeResult("999.0.0")
	if result.UpToDate {
		t.Error("expected UpToDate=false when newer version exists")
	}
	if result.Message == "" {
		t.Error("expected non-empty update message")
	}
}

func TestMakeResultEmpty(t *testing.T) {
	// Empty latest version (network failure) — unknown state
	result := makeResult("")
	if result.UpToDate {
		t.Error("expected UpToDate=false for empty version")
	}
	if result.Message != "" {
		t.Errorf("expected empty message for unknown state, got %q", result.Message)
	}
}

func TestFetchLatestVersionBadURL(t *testing.T) {
	// Save and restore the release URL
	origURL := releaseURL
	defer func() { releaseURL = origURL }()

	// Point to an unreachable host to simulate network failure
	releaseURL = "http://192.0.2.1:1/nonexistent" // RFC 5737 TEST-NET, guaranteed unreachable

	_, err := fetchLatestVersion()
	if err == nil {
		t.Error("expected error for unreachable host")
	}
}

func TestCheckNetworkDown(t *testing.T) {
	// Clear any cache so it forces a network call
	path := cacheFilePath()
	if path != "" {
		os.Remove(path)
	}

	origURL := releaseURL
	defer func() { releaseURL = origURL }()
	releaseURL = "http://192.0.2.1:1/nonexistent"

	result := Check()
	// Should not crash, not report up-to-date, not show update
	if result.UpToDate {
		t.Error("should not report up-to-date when network is down")
	}
	if result.Message != "" {
		t.Errorf("should not have update message when network is down, got %q", result.Message)
	}
}
