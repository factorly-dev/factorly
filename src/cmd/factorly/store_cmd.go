// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/factorly-dev/factorly/internal/logger"
	"github.com/factorly-dev/factorly/internal/store"
	"github.com/spf13/cobra"
)

// storeTTLFlag is the CLI-supplied TTL for `factorly store set`.
// "30d" default matches store.DefaultTTL; "0" means never expire.
// Empty (unset) falls through to the backend default.
var storeTTLFlag string

// storeGlobal pins store operations to the global tier
// (~/.config/factorly/store.db), mirroring vault's --global flag.
// Useful when the user wants to write to the global store from
// inside a project directory (where the project tier would normally
// win). Snapshot into tierSelector.StoreGlobal via currentSelector().
var storeGlobal bool

// Process-wide store Manager. Mirrors the vault Manager pattern at
// vault_cmd.go: lazily constructed on first access, owns a per-scope
// cache, shared by CLI commands and (eventually) the UI server.
//
// The chainOpener dispatches on scope strings:
//   - "project"           → .factorly/store.db
//   - "global"            → ~/.config/factorly/store.db
//   - "workspace:<name>"  → .factorly/workspaces/<name>/store.db
//
// No password resolution machinery — store is unencrypted.
var (
	storeManagerOnce sync.Once
	storeManager     *store.Manager
)

func getStoreManager() *store.Manager {
	storeManagerOnce.Do(func() {
		storeManager = store.NewManager(storeChainOpener)
	})
	return storeManager
}

// storeChainOpener resolves a scope to a bbolt-backed Backend. This
// is the only place in cmd/factorly that knows about the
// scope → on-disk path mapping for store; everything else goes
// through the Manager.
func storeChainOpener(scope string) (store.Backend, error) {
	var t storeTier
	switch {
	case scope == "project":
		t = projectStoreTier()
	case scope == "global":
		t = globalStoreTier()
	case strings.HasPrefix(scope, "workspace:"):
		name := strings.TrimPrefix(scope, "workspace:")
		t = workspaceStoreTier(name)
		if t.Path == "" {
			return nil, fmt.Errorf("store: invalid workspace name %q", name)
		}
	default:
		return nil, fmt.Errorf("store: unknown scope %q", scope)
	}
	if t.Path == "" {
		return nil, fmt.Errorf("store: no path for scope %q (check HOME for global tier)", scope)
	}
	return store.OpenLocalAt(t.Path)
}

// getActiveStore returns the store Backend the current CLI invocation
// should write/read against. Single-tier selection (no fallback
// chain) because writes need to be unambiguous — like vault
// set/delete, store operations target exactly one tier.
//
// The returned Backend's lifetime is owned by the Manager; do not
// Close it directly.
func getActiveStore() (store.Backend, error) {
	if err := validateActiveStoreName(); err != nil {
		return nil, err
	}
	t := activeStoreTier(currentSelector())
	scope := t.Name
	return getStoreManager().GetOrOpen(scope)
}

// getCachedStoreReader returns the read-side cascade for the active
// scope: when --workspace is set, the workspace store with project
// fallback; otherwise the project store directly.
//
// This is what gets registered as the {{store:KEY}} resolver backend
// — reads cascade so a project-wide store value is visible inside a
// workspace context unless the workspace specifically overrides it.
//
// Writes don't go through this (they use getActiveStore to target
// exactly one tier). Mirrors the read/write asymmetry vault has.
func getCachedStoreReader() (store.Backend, error) {
	if err := validateActiveStoreName(); err != nil {
		return nil, err
	}
	t := activeStoreTier(currentSelector())
	mgr := getStoreManager()
	primary, err := mgr.GetOrOpen(t.Name)
	if err != nil {
		return nil, err
	}
	// Project tier already IS the project — no fallback. Same for
	// global. Only workspace tier gets the cascade wrapper.
	if !strings.HasPrefix(t.Name, "workspace:") {
		return primary, nil
	}
	// Workspace tier: wrap with a lazy project fallback so reads of
	// keys that don't exist in the workspace fall through to the
	// project store. Project tier opens on first miss; if it doesn't
	// exist either (fresh install with no project store yet), the
	// fallback opener silently returns nil and Get yields ErrNotFound.
	return &fallbackStoreReader{
		primary:  primary,
		fallback: func() (store.Backend, error) { return mgr.GetOrOpen("project") },
	}, nil
}

// fallbackStoreReader gives store the workspace→project read
// cascade. Mirrors vault.FallbackBackend's shape and concurrency
// contract (sync.Once-style lazy open of the fallback), but lives
// here because store's Backend interface has an extra Search method
// vault's FallbackBackend doesn't know about.
type fallbackStoreReader struct {
	primary  store.Backend
	fallback func() (store.Backend, error)

	openOnce     sync.Once
	cachedSec    store.Backend
	cachedSecErr error
}

func (f *fallbackStoreReader) loadFallback() (store.Backend, error) {
	f.openOnce.Do(func() {
		if f.fallback == nil {
			return
		}
		b, err := f.fallback()
		f.cachedSec = b
		f.cachedSecErr = err
	})
	return f.cachedSec, f.cachedSecErr
}

func (f *fallbackStoreReader) Get(key string) (string, error) {
	v, err := f.primary.Get(key)
	if err == nil {
		return v, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return "", err
	}
	sec, secErr := f.loadFallback()
	if secErr != nil || sec == nil {
		return "", store.ErrNotFound
	}
	return sec.Get(key)
}

// Set / Delete are unambiguously workspace-scoped — fallback for
// writes would be surprising (you'd think you were writing to the
// active workspace but landing in the project). Route to primary.
func (f *fallbackStoreReader) Set(key, value string) error { return f.primary.Set(key, value) }
func (f *fallbackStoreReader) Delete(key string) error     { return f.primary.Delete(key) }

// List and Search union the primary's keys with the fallback's,
// deduplicating. This way `factorly store list` from a workspace
// context shows everything readable in that scope. Workspace
// entries shadow project entries with the same name (we add
// workspace keys to the set first, then project entries that
// aren't already present).
func (f *fallbackStoreReader) List() ([]string, error) {
	primaryKeys, err := f.primary.List()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(primaryKeys))
	for _, k := range primaryKeys {
		seen[k] = true
	}
	sec, secErr := f.loadFallback()
	if secErr == nil && sec != nil {
		secondaryKeys, err := sec.List()
		if err == nil {
			for _, k := range secondaryKeys {
				if !seen[k] {
					primaryKeys = append(primaryKeys, k)
					seen[k] = true
				}
			}
		}
	}
	// List() on individual backends is already sorted, but the union
	// breaks ordering. Re-sort for a stable result.
	sortStringsInPlace(primaryKeys)
	return primaryKeys, nil
}

func (f *fallbackStoreReader) Search(query string) ([]string, error) {
	primaryKeys, err := f.primary.Search(query)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(primaryKeys))
	for _, k := range primaryKeys {
		seen[k] = true
	}
	sec, secErr := f.loadFallback()
	if secErr == nil && sec != nil {
		secondaryKeys, err := sec.Search(query)
		if err == nil {
			for _, k := range secondaryKeys {
				if !seen[k] {
					primaryKeys = append(primaryKeys, k)
					seen[k] = true
				}
			}
		}
	}
	sortStringsInPlace(primaryKeys)
	return primaryKeys, nil
}

// Close is a no-op — the Manager owns the lifetimes of the wrapped
// backends. Closing here would yank the cached backends out from
// under any other holder.
func (f *fallbackStoreReader) Close() error { return nil }

// sortStringsInPlace is a tiny helper to keep the import surface
// trim. Pulled inline so we don't need to depend on sort just for
// the union case.
func sortStringsInPlace(s []string) {
	// Insertion sort: fine for the small slices we get here (a
	// handful to a few hundred keys at most for v1 caps).
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// logStoreOp logs a store operation to the JSONL audit trail.
// Thin wrapper over the shared logKVOp helper.
//
// Get and Search are NOT routed through this — they're high-
// frequency low-value events. Set and Delete (state-changing
// operations) ARE logged.
func logStoreOp(op, key, status string) {
	logKVOp("store", op, key, status)
}

// parseStoreTTL converts the --ttl flag value to a duration.
// Empty → returns 0 + ok=false (caller uses backend default).
// "0" → returns 0 + ok=true (never expire).
// Anything else → time.ParseDuration semantics (h, m, s, plus "d"
// for days because users will expect that).
func parseStoreTTL(s string) (time.Duration, bool, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false, nil
	}
	if s == "0" {
		return 0, true, nil
	}
	// Allow trailing 'd' for days (time.ParseDuration doesn't).
	if strings.HasSuffix(s, "d") {
		days, err := time.ParseDuration(strings.TrimSuffix(s, "d") + "h")
		if err != nil {
			return 0, false, fmt.Errorf("invalid ttl %q: %w", s, err)
		}
		return days * 24, true, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, false, fmt.Errorf("invalid ttl %q: %w", s, err)
	}
	return d, true, nil
}

var storeCmd = &cobra.Command{
	Use:   "store",
	Short: "Manage agent-writable workspace key/value state",
	Long: `Store is the agent-writable workspace data primitive — the
fourth flavor of workspace data alongside vault (secrets), env (config),
and auth (OAuth tokens). It holds cross-run scratchpad state the agent
maintains itself: "I researched these URLs already", "the Trello board
ID for X is Y", "last successful deployment SHA".

Default TTL is 30d with refresh-on-read, so frequently-touched entries
stay alive indefinitely. Pass --ttl 0 for never-expire.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return checkCommandAllowed("store")
	},
}

var storeSetCmd = &cobra.Command{
	Use:   "set <key> [value]",
	Short: "Save a key/value pair to the store",
	Args:  requireArgs(1, "factorly store set <key> [value]"),
	RunE: func(cmd *cobra.Command, args []string) error {
		backend, err := getActiveStore()
		if err != nil {
			return err
		}
		key := args[0]
		var value string
		if len(args) > 1 {
			value = args[1]
		} else {
			// Read from stdin so callers can pipe in larger payloads
			// without putting them on the command line. Matches the
			// vault set ergonomic.
			fmt.Fprint(os.Stderr, "Value (end with Ctrl-D): ")
			scanner := bufio.NewScanner(os.Stdin)
			scanner.Buffer(make([]byte, 0, 16*1024), 64*1024)
			var lines []string
			for scanner.Scan() {
				lines = append(lines, scanner.Text())
			}
			value = strings.Join(lines, "\n")
		}

		ttl, hasTTL, err := parseStoreTTL(storeTTLFlag)
		if err != nil {
			return err
		}
		// Local cast to call SetWithTTL — store.Backend is the resolver-
		// compatible narrow interface; the concrete LocalBackend exposes
		// the TTL-bearing variant.
		if hasTTL {
			if lb, ok := backend.(*store.LocalBackend); ok {
				if err := lb.SetWithTTL(key, value, ttl); err != nil {
					logStoreOp("save", key, "error")
					return err
				}
			} else {
				return fmt.Errorf("store: TTL not supported on backend %T", backend)
			}
		} else if err := backend.Set(key, value); err != nil {
			logStoreOp("save", key, "error")
			return err
		}
		logStoreOp("save", key, "success")
		fmt.Fprintf(os.Stderr, "Saved %s to store\n", key)
		return nil
	},
}

var storeGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Retrieve a value from the store",
	Args:  requireArgs(1, "factorly store get <key>"),
	RunE: func(cmd *cobra.Command, args []string) error {
		backend, err := getActiveStore()
		if err != nil {
			return err
		}
		value, err := backend.Get(args[0])
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return fmt.Errorf("key %q not found", args[0])
			}
			return err
		}
		// Get is high-frequency and low-information for audit
		// purposes — deliberately NOT logged. If users need
		// per-tool gating they can wrap store reads in a code tool.
		fmt.Print(value)
		return nil
	},
}

var storeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all keys in the store",
	RunE: func(cmd *cobra.Command, args []string) error {
		backend, err := getActiveStore()
		if err != nil {
			return err
		}
		keys, err := backend.List()
		if err != nil {
			return err
		}
		for _, k := range keys {
			fmt.Println(k)
		}
		return nil
	},
}

var storeSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search keys by substring (case-insensitive)",
	Args:  requireArgs(1, "factorly store search <query>"),
	RunE: func(cmd *cobra.Command, args []string) error {
		backend, err := getActiveStore()
		if err != nil {
			return err
		}
		keys, err := backend.Search(args[0])
		if err != nil {
			return err
		}
		for _, k := range keys {
			fmt.Println(k)
		}
		return nil
	},
}

var storeDeleteCmd = &cobra.Command{
	Use:   "delete <key>",
	Short: "Remove a key from the store",
	Args:  requireArgs(1, "factorly store delete <key>"),
	RunE: func(cmd *cobra.Command, args []string) error {
		backend, err := getActiveStore()
		if err != nil {
			return err
		}
		if err := backend.Delete(args[0]); err != nil {
			logStoreOp("delete", args[0], "error")
			return err
		}
		logStoreOp("delete", args[0], "success")
		fmt.Fprintf(os.Stderr, "Deleted %s from store\n", args[0])
		return nil
	},
}

// storeHistoryCmd surfaces the audit trail for a single key. Reads
// the JSONL log directly with the same 1MB buffered scanner pattern
// the promote package uses (so embedded factorly.code scripts don't
// silently get skipped).
var storeHistoryCmd = &cobra.Command{
	Use:   "history <key>",
	Short: "Show audit log entries for a store key",
	Args:  requireArgs(1, "factorly store history <key>"),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		logPath := resolveLogPath()
		f, err := os.Open(logPath) // #nosec G304 -- log path is operator-supplied
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintln(os.Stderr, "No audit log yet.")
				return nil
			}
			return fmt.Errorf("opening audit log: %w", err)
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		printedAny := false
		for scanner.Scan() {
			var entry logger.Entry
			if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
				continue
			}
			if !strings.HasPrefix(entry.Tool, "store.") {
				continue
			}
			if entry.Params["key"] != key {
				continue
			}
			when := entry.Timestamp.Format("2006-01-02 15:04:05")
			op := strings.TrimPrefix(entry.Tool, "store.")
			fmt.Printf("%s  %-8s  %s\n", when, op, entry.Status)
			printedAny = true
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("scanning audit log: %w", err)
		}
		if !printedAny {
			fmt.Fprintf(os.Stderr, "No history for key %q.\n", key)
		}
		return nil
	},
}

func init() {
	storeSetCmd.Flags().StringVar(&storeTTLFlag, "ttl", "",
		"entry lifetime (e.g. 7d, 24h, 30m); 0 = never expire; default = 30d")
	storeCmd.PersistentFlags().BoolVar(&storeGlobal, "global", false,
		"use global store (~/.config/factorly/store.db) instead of project store")
	storeCmd.AddCommand(storeSetCmd, storeGetCmd, storeListCmd, storeSearchCmd, storeDeleteCmd, storeHistoryCmd)
}
