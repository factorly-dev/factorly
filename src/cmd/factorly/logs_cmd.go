// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/factorly-dev/factorly/internal/config"
	"github.com/factorly-dev/factorly/internal/logger"
	"github.com/spf13/cobra"
)

// resolveLogPath returns the active project's log path: FACTORLY_LOG_PATH
// wins, then --config (if set), then the same FindConfig walk the
// proxy uses, then the global default.
func resolveLogPath() string {
	if p := os.Getenv("FACTORLY_LOG_PATH"); p != "" {
		return p
	}
	cfgPath := configPath
	if cfgPath == "" {
		cfgPath = config.FindConfig()
	}
	return logger.ProjectLogPath(cfgPath)
}

var (
	logsLines     int
	logsTool      string
	logsStatus    string
	logsInterface string
	logsAgent     string
	logsFollow    bool
	logsDetail    bool
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "View the tool call audit log",
	RunE:  runLogs,
}

var logsStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show summary statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkCommandAllowed("logs"); err != nil {
			return err
		}
		return runLogsStats()
	},
}

var logsVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify hash chain integrity",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkCommandAllowed("logs"); err != nil {
			return err
		}
		return runLogsVerify()
	},
}

var logsRepairCmd = &cobra.Command{
	Use:   "repair",
	Short: "Repair broken hash chain (appends reset marker)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkCommandAllowed("logs"); err != nil {
			return err
		}
		return runLogsRepair()
	},
}

func init() {
	logsCmd.Flags().IntVarP(&logsLines, "lines", "n", 20, "number of entries to show")
	logsCmd.Flags().StringVar(&logsTool, "tool", "", "filter by tool name (substring match)")
	logsCmd.Flags().StringVar(&logsStatus, "status", "", "filter by status (success, error, blocked)")
	logsCmd.Flags().StringVar(&logsInterface, "interface", "", "filter by interface (cli, mcp)")
	logsCmd.Flags().StringVar(&logsAgent, "agent", "", "filter by agent ID")
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "follow mode (tail -f style)")
	logsCmd.Flags().BoolVar(&logsDetail, "detail", false, "show full entry details")

	logsCmd.AddCommand(logsStatsCmd)
	logsCmd.AddCommand(logsVerifyCmd)
	logsCmd.AddCommand(logsRepairCmd)
}

func runLogs(cmd *cobra.Command, args []string) error {
	if err := checkCommandAllowed("logs"); err != nil {
		return err
	}

	logPath := resolveLogPath()
	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No log file found.")
			return nil
		}
		return fmt.Errorf("opening log file: %w", err)
	}
	defer f.Close()

	// Scan all entries, filter, keep last N
	var entries []logger.Entry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 512*1024)
	for scanner.Scan() {
		var entry logger.Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if !matchesFilter(&entry) {
			continue
		}
		entries = append(entries, entry)
	}

	// Keep last N entries
	if len(entries) > logsLines {
		entries = entries[len(entries)-logsLines:]
	}

	if len(entries) == 0 && !logsFollow {
		fmt.Println("No matching log entries.")
		return nil
	}

	// Determine if shadow column is needed (any non-"allowed" shadow action)
	showShadow := false
	for _, e := range entries {
		if e.ShadowAction != "" && e.ShadowAction != "allowed" {
			showShadow = true
			break
		}
	}

	// Determine if entries span multiple days
	multiDay := false
	if len(entries) > 1 {
		first := entries[0].Timestamp
		last := entries[len(entries)-1].Timestamp
		if first.YearDay() != last.YearDay() || first.Year() != last.Year() {
			multiDay = true
		}
	}

	if logsDetail {
		printLogsDetail(entries)
	} else {
		printLogsTable(entries, showShadow, multiDay)
	}

	if !logsFollow {
		return nil
	}

	// Follow mode: track file position and poll for new entries
	offset, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("seeking log file: %w", err)
	}
	f.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println()
			return nil
		case <-time.After(2 * time.Second):
		}

		f2, err := os.Open(logPath)
		if err != nil {
			continue
		}
		if _, err := f2.Seek(offset, io.SeekStart); err != nil {
			f2.Close()
			continue
		}
		followScanner := bufio.NewScanner(f2)
		followScanner.Buffer(make([]byte, 0, 64*1024), 512*1024)
		for followScanner.Scan() {
			var entry logger.Entry
			if err := json.Unmarshal(followScanner.Bytes(), &entry); err != nil {
				continue
			}
			if !matchesFilter(&entry) {
				continue
			}
			if logsDetail {
				printLogsDetail([]logger.Entry{entry})
			} else {
				printLogsTable([]logger.Entry{entry}, showShadow, multiDay)
			}
		}
		newOffset, _ := f2.Seek(0, io.SeekCurrent)
		offset = newOffset
		f2.Close()
	}
}

func matchesFilter(entry *logger.Entry) bool {
	if logsTool != "" && !strings.Contains(entry.Tool, logsTool) {
		return false
	}
	if logsStatus != "" && entry.Status != logsStatus {
		return false
	}
	if logsInterface != "" && entry.Interface != logsInterface {
		return false
	}
	if logsAgent != "" && entry.AgentID != logsAgent {
		return false
	}
	return true
}

func printLogsTable(entries []logger.Entry, showShadow bool, multiDay bool) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Header
	header := "TIME\tTOOL\tSTATUS\tDURATION\t"
	if showShadow {
		header += "SHADOW\t"
	}
	header += "SAVINGS"
	fmt.Fprintln(w, header)

	timeFmt := "15:04:05"
	if multiDay {
		timeFmt = "Jan 02 15:04:05"
	}

	for _, e := range entries {
		ts := e.Timestamp.Format(timeFmt)

		dur := "—"
		if e.DurationMs > 0 {
			dur = fmt.Sprintf("%dms", e.DurationMs)
		}

		savings := "—"
		if e.OriginalBytes > 0 {
			savings = fmt.Sprintf("%d→%d", e.OriginalBytes, e.ProcessedBytes)
		}

		line := fmt.Sprintf("%s\t%s\t%s\t%s\t", ts, e.Tool, e.Status, dur)
		if showShadow {
			shadow := e.ShadowAction
			if shadow == "" {
				shadow = "—"
			}
			line += shadow + "\t"
		}
		line += savings
		fmt.Fprintln(w, line)
	}
	w.Flush()
}

func printLogsDetail(entries []logger.Entry) {
	for _, e := range entries {
		fmt.Printf("——— %s ———————————————————————————————\n", e.Timestamp.Format("2006-01-02 15:04:05"))
		fmt.Printf("  Tool:      %s\n", e.Tool)
		fmt.Printf("  Status:    %s\n", e.Status)
		if e.DurationMs > 0 {
			fmt.Printf("  Duration:  %dms\n", e.DurationMs)
		}
		if e.Interface != "" {
			fmt.Printf("  Interface: %s\n", e.Interface)
		}
		if e.AgentID != "" {
			fmt.Printf("  Agent:     %s\n", e.AgentID)
		}
		if e.ShadowAction != "" {
			fmt.Printf("  Shadow:    %s\n", e.ShadowAction)
		}
		if len(e.Params) > 0 {
			parts := make([]string, 0, len(e.Params))
			for k, v := range e.Params {
				parts = append(parts, fmt.Sprintf("%s=%s", k, v))
			}
			sort.Strings(parts)
			fmt.Printf("  Params:    %s\n", strings.Join(parts, ", "))
		}
		if e.Output != "" {
			output := e.Output
			if len(output) > 100 {
				output = output[:100] + "..."
			}
			fmt.Printf("  Output:    %s (%d bytes)\n", output, len(e.Output))
		}
		if e.Error != "" {
			errMsg := e.Error
			if len(errMsg) > 100 {
				errMsg = errMsg[:100] + "..."
			}
			fmt.Printf("  Error:     %s\n", errMsg)
		}
		if e.OriginalBytes > 0 {
			saved := 0.0
			if e.OriginalBytes > 0 {
				saved = float64(e.OriginalBytes-e.ProcessedBytes) / float64(e.OriginalBytes) * 100
			}
			fmt.Printf("  Savings:   %d → %d bytes (%.0f%%)\n", e.OriginalBytes, e.ProcessedBytes, saved)
		}
		if e.Hash != "" {
			fmt.Printf("  Hash:      %s\n", e.Hash[:16]+"...")
		}
		if len(e.HighlightParams) > 0 {
			parts := make([]string, 0, len(e.HighlightParams))
			for k, v := range e.HighlightParams {
				parts = append(parts, fmt.Sprintf("%s=%s", k, v))
			}
			sort.Strings(parts)
			fmt.Printf("  Highlight: %s\n", strings.Join(parts, ", "))
		}
		fmt.Println()
	}
}

func runLogsStats() error {
	logPath := resolveLogPath()
	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No log file found.")
			return nil
		}
		return fmt.Errorf("opening log file: %w", err)
	}
	defer f.Close()

	var totalEntries int
	statusCounts := make(map[string]int)
	toolCounts := make(map[string]int)
	shadowCounts := make(map[string]int)
	var totalOriginal, totalProcessed int64
	var processedCalls int
	var blockedCalls int

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 512*1024)
	for scanner.Scan() {
		var entry logger.Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		totalEntries++
		statusCounts[entry.Status]++
		toolCounts[entry.Tool]++
		if entry.Status == "blocked" {
			blockedCalls++
			if entry.ShadowAction != "" {
				shadowCounts[entry.ShadowAction]++
			}
		}
		if entry.OriginalBytes > 0 {
			processedCalls++
			totalOriginal += int64(entry.OriginalBytes)
			totalProcessed += int64(entry.ProcessedBytes)
		}
	}

	if totalEntries == 0 {
		fmt.Println("Log file is empty.")
		return nil
	}

	fmt.Printf("  Log: %s (%s entries)\n", logPath, formatCount(totalEntries))
	fmt.Println()

	// By Status
	fmt.Println("  By Status:")
	statusOrder := []string{"success", "error", "blocked"}
	for _, s := range statusOrder {
		if count, ok := statusCounts[s]; ok {
			pct := float64(count) / float64(totalEntries) * 100
			fmt.Printf("    %-10s %s  (%.1f%%)\n", s, formatCount(count), pct)
		}
	}
	// Any other statuses not in the standard list
	for s, count := range statusCounts {
		if s != "success" && s != "error" && s != "blocked" {
			pct := float64(count) / float64(totalEntries) * 100
			fmt.Printf("    %-10s %s  (%.1f%%)\n", s, formatCount(count), pct)
		}
	}
	fmt.Println()

	// By Tool (top 10)
	fmt.Println("  By Tool (top 10):")
	type toolCount struct {
		name  string
		count int
	}
	var toolList []toolCount
	for name, count := range toolCounts {
		toolList = append(toolList, toolCount{name, count})
	}
	sort.Slice(toolList, func(i, j int) bool {
		return toolList[i].count > toolList[j].count
	})
	limit := 10
	if len(toolList) < limit {
		limit = len(toolList)
	}
	for _, tc := range toolList[:limit] {
		fmt.Printf("    %-30s %s\n", tc.name, formatCount(tc.count))
	}
	fmt.Println()

	// Output Savings
	fmt.Println("  Output Savings:")
	if processedCalls > 0 {
		saved := totalOriginal - totalProcessed
		pct := float64(saved) / float64(totalOriginal) * 100
		fmt.Printf("    %d calls processed, %s → %s (%.0f%% saved)\n",
			processedCalls, formatBytes(totalOriginal), formatBytes(totalProcessed), pct)
	} else {
		fmt.Println("    no output processing recorded")
	}
	fmt.Println()

	// Blocked Calls
	if blockedCalls > 0 {
		fmt.Println("  Blocked Calls:")
		parts := make([]string, 0, len(shadowCounts))
		for action, count := range shadowCounts {
			parts = append(parts, fmt.Sprintf("%d %s", count, action))
		}
		sort.Strings(parts)
		fmt.Printf("    %d total (%s)\n", blockedCalls, strings.Join(parts, ", "))
		fmt.Println()
	}

	// Hash Chain Integrity
	fmt.Println("  Hash Chain:")
	verified, skipped, verifyErr := logger.VerifyChain(logPath)
	if verifyErr != nil {
		fmt.Printf("    BROKEN — %s\n", verifyErr)
	} else {
		msg := fmt.Sprintf("    %s entries verified", formatCount(verified))
		if skipped > 0 {
			msg += fmt.Sprintf(", %s entries before chain reset skipped", formatCount(skipped))
		}
		fmt.Println(msg)
	}

	return nil
}

func runLogsRepair() error {
	logPath := resolveLogPath()

	repaired, err := logger.RepairChain(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No log file found.")
			return nil
		}
		return fmt.Errorf("repair failed: %w", err)
	}

	if !repaired {
		fmt.Println("No chain breaks found — nothing to repair.")
	} else {
		fmt.Println("Chain reset marker appended. Run `factorly logs verify` to confirm.")
	}
	return nil
}

func runLogsVerify() error {
	logPath := resolveLogPath()

	verified, skipped, err := logger.VerifyChain(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No log file found.")
			return nil
		}
		return fmt.Errorf("verification failed: %w", err)
	}

	fmt.Printf("Chain verified: %d entries OK", verified)
	if skipped > 0 {
		fmt.Printf(" (%d entries before chain reset skipped)", skipped)
	}
	fmt.Println()
	return nil
}

// formatCount formats an integer with comma separators.
func formatCount(n int) string {
	s := fmt.Sprintf("%d", n)
	if n < 1000 {
		return s
	}
	// Insert commas from the right
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}
