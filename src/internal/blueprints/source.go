// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

// Package blueprints implements the install pipeline for sharable tool/workflow
// blueprint files. A blueprint is a single YAML document (see internal/config)
// that can be fetched from a local path, a raw URL, or a GitHub shorthand.
package blueprints

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// MaxBlueprintSize bounds how much we'll read from a remote source. Blueprints
// are config, not data — 1 MiB is generous.
const MaxBlueprintSize = 1 << 20

// Source represents a parsed input to Resolve. Callers don't need to
// interact with this directly; it's exposed for tests.
type Source struct {
	// Raw is the original string the user passed.
	Raw string
	// Kind is "file", "url", or "github".
	Kind string
	// URLs is the list of URLs to try in order (the github kind tries
	// main and then master, for example).
	URLs []string
	// LocalPath is set when Kind == "file".
	LocalPath string
	// DisplayName is a human-readable identifier derived from the source.
	DisplayName string
}

// Resolve parses a source string into a Source describing where to fetch the
// blueprint from. Accepted forms:
//
//   - Local file path:            ./blueprints/gmail.yaml, /abs/path/to/foo.yaml
//   - Raw URL:                    https://example.com/foo.yaml
//   - GitHub shorthand:           github.com/owner/repo[@ref][/path/to/file.yaml]
//
// For the GitHub form, if no path is given we try common defaults
// (factorly.yaml, blueprint.yaml) and if no ref is given we try main then master.
func Resolve(source string) (*Source, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil, errors.New("blueprints: empty source")
	}

	// Raw URL
	if strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "http://") {
		return &Source{
			Raw:         source,
			Kind:        "url",
			URLs:        []string{source},
			DisplayName: deriveNameFromURL(source),
		}, nil
	}

	// GitHub shorthand
	if strings.HasPrefix(source, "github.com/") {
		return resolveGitHub(source)
	}

	// Otherwise treat as a local path.
	abs, err := filepath.Abs(source)
	if err != nil {
		return nil, fmt.Errorf("blueprints: resolve %q: %w", source, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("blueprints: %q does not exist (try a github.com/owner/repo URL or a local .yaml file)", source)
		}
		return nil, fmt.Errorf("blueprints: stat %q: %w", source, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("blueprints: %q is a directory; pass a single .yaml file", source)
	}
	if info.Size() > MaxBlueprintSize {
		return nil, fmt.Errorf("blueprints: %q is larger than %d bytes; blueprint files are config, not data", source, MaxBlueprintSize)
	}
	return &Source{
		Raw:         source,
		Kind:        "file",
		LocalPath:   abs,
		DisplayName: deriveNameFromPath(abs),
	}, nil
}

// resolveGitHub expands a `github.com/owner/repo[@ref][/path]` shorthand into
// a list of candidate raw URLs to try in order.
func resolveGitHub(source string) (*Source, error) {
	rest := strings.TrimPrefix(source, "github.com/")
	// Split off @ref if present.
	ref := ""
	if at := strings.Index(rest, "@"); at >= 0 {
		// @ might appear in the path portion later; but for v1 we expect it
		// before any /. If we ever support nested @, refine here.
		atSlash := strings.Index(rest[at:], "/")
		if atSlash < 0 {
			ref = rest[at+1:]
			rest = rest[:at]
		} else {
			ref = rest[at+1 : at+atSlash]
			rest = rest[:at] + rest[at+atSlash:]
		}
	}
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("blueprints: github source must be github.com/owner/repo[@ref][/path]; got %q", source)
	}
	owner, repo := parts[0], parts[1]
	subpath := ""
	if len(parts) == 3 {
		subpath = parts[2]
	}

	refs := []string{ref}
	if ref == "" {
		refs = []string{"main", "master"}
	}
	paths := []string{subpath}
	if subpath == "" {
		// Common default filenames in a blueprint repo.
		paths = []string{"factorly.yaml", "blueprint.yaml"}
	}

	var urls []string
	for _, r := range refs {
		for _, p := range paths {
			urls = append(urls, fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", owner, repo, r, p))
		}
	}

	display := owner + "/" + repo
	if ref != "" {
		display += "@" + ref
	}
	return &Source{
		Raw:         source,
		Kind:        "github",
		URLs:        urls,
		DisplayName: display,
	}, nil
}

// Fetch downloads the blueprint contents from a resolved Source. For "file"
// sources it reads the local path; for "url"/"github" it tries each candidate
// URL in order and returns the first 2xx response. Bodies are capped at
// MaxBlueprintSize.
//
// httpClient may be nil; the default uses a 10s timeout.
func Fetch(src *Source, httpClient *http.Client) ([]byte, error) {
	if src.Kind == "file" {
		f, err := os.Open(src.LocalPath)
		if err != nil {
			return nil, fmt.Errorf("blueprints: open %s: %w", src.LocalPath, err)
		}
		defer f.Close()
		return io.ReadAll(io.LimitReader(f, MaxBlueprintSize+1))
	}

	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	var lastErr error
	for _, url := range src.URLs {
		body, err := fetchOne(httpClient, url)
		if err == nil {
			return body, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no candidate URLs")
	}
	return nil, fmt.Errorf("blueprints: fetching %s: %w", src.DisplayName, lastErr)
}

func fetchOne(client *http.Client, url string) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBlueprintSize+1))
	if err != nil {
		return nil, err
	}
	if len(body) > MaxBlueprintSize {
		return nil, fmt.Errorf("response from %s exceeds %d bytes", url, MaxBlueprintSize)
	}
	return body, nil
}

func deriveNameFromPath(p string) string {
	base := filepath.Base(p)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

func deriveNameFromURL(u string) string {
	base := path.Base(u)
	ext := path.Ext(base)
	return strings.TrimSuffix(base, ext)
}
