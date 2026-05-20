// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"os"
	"time"

	"github.com/factorly-dev/factorly/internal/logger"
)

// logKVOp is the shared audit-write path for vault and store CLI
// operations. Before this existed, vault and store had near-duplicate
// helpers (`logVaultOp`, `logStoreOp`) that drifted independently —
// in particular, the vault helper fell back to the global audit log
// (~/.config/factorly/audit.jsonl) when invoked pre-bootstrap, while
// the store helper correctly routed through resolveLogPath() to the
// project's .factorly/audit.jsonl. Unifying here standardizes on the
// project-aware behavior so per-project audit trails are complete.
//
// Never logs secret values — only the iface, op, and key name.
// Uses the shared process-wide logger to maintain hash chain
// integrity when bootstrap has completed; falls back to a one-shot
// JSONL logger pointed at the project log otherwise.
//
// FACTORLY_NO_LOG=1 disables the fallback path entirely (used by
// tests + scripted runs that want to suppress audit writes).
func logKVOp(iface, op, key, status string) {
	log := sharedLogger
	if log == nil {
		if os.Getenv("FACTORLY_NO_LOG") != "" {
			return
		}
		fallback, err := logger.NewJSONL(resolveLogPath())
		if err != nil {
			return
		}
		defer fallback.Close()
		log = fallback
	}
	entry := &logger.Entry{
		Timestamp: time.Now(),
		Interface: iface,
		Tool:      iface + "." + op,
		Status:    status,
		Workspace: workspaceName,
	}
	if key != "" {
		entry.Params = map[string]string{"key": key}
	}
	_ = log.Log(entry)
}
