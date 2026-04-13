package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/factorly-dev/factorly-cli/internal/logger"
	"github.com/factorly-dev/factorly-cli/internal/output"
	"github.com/factorly-dev/factorly-cli/internal/provider"
	"github.com/spf13/cobra"
)

var (
	execMaxOutput    int
	execCompress     string
	execEnvIsolation string
	execInteractive  bool
)

var execCmd = &cobra.Command{
	Use:   "exec [flags] -- <command> [args...]",
	Short: "Run a command through Factorly's safety layer",
	Long: `Run a single shell command with output compression, truncation,
and audit logging. The zero-config equivalent of a CLI tool definition.

Examples:
  factorly exec -- git status
  factorly exec -- curl https://api.github.com/users/octocat
  factorly exec --compress json -- npm test
  factorly exec --env-isolation strict -- ./deploy.sh`,
	RunE: runExec,
}

func runExec(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: factorly exec -- <command> [args...]")
	}

	// Build compression hints
	var hints []output.Hint
	switch execCompress {
	case "none":
		// no compression
	case "json":
		hints = []output.Hint{output.HintJSON}
	case "logs":
		hints = []output.Hint{output.HintLogs}
	default:
		hints = []output.Hint{output.HintAll}
	}

	maxOutput := execMaxOutput

	// Build environment
	env := provider.BuildEnv(nil, execEnvIsolation == "strict")

	vlog("exec: %s", strings.Join(args, " "))

	toolName := strings.Join(args, " ")

	// Run command
	c := exec.Command(args[0], args[1:]...)
	c.Env = env

	// Interactive mode: connect directly to terminal, skip compression
	if execInteractive {
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr

		start := time.Now()
		err := c.Run()
		duration := time.Since(start)

		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = 1
			}
		}

		// Log (no output captured in interactive mode)
		logExec(&logger.Entry{
			Timestamp:  time.Now(),
			Interface:  "exec",
			Tool:       toolName,
			DurationMs: duration.Milliseconds(),
			Status:     map[bool]string{true: "error", false: "success"}[exitCode != 0],
		})

		if exitCode != 0 {
			os.Exit(exitCode)
		}
		return nil
	}

	// Captured mode: compress and log output
	c.Stdin = os.Stdin

	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr

	start := time.Now()
	err := c.Run()
	duration := time.Since(start)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return nil
		}
	}

	// Process output
	raw := stdout.String()
	originalBytes := len(raw)
	processed := raw

	if len(hints) > 0 || maxOutput > 0 {
		processed = output.Process(raw, maxOutput, hints...)
	}
	processedBytes := len(processed)

	// Log to audit trail
	logEntry := &logger.Entry{
		Timestamp:  time.Now(),
		Interface:  "exec",
		Tool:       toolName,
		DurationMs: duration.Milliseconds(),
	}
	if originalBytes != processedBytes {
		logEntry.OriginalBytes = originalBytes
		logEntry.ProcessedBytes = processedBytes
	}
	if exitCode != 0 {
		logEntry.Status = "error"
		logEntry.Error = stderr.String()
	} else {
		logEntry.Status = "success"
		logEntry.Output = processed
	}
	logExec(logEntry)

	// Print output
	fmt.Print(processed)
	if stderrStr := stderr.String(); stderrStr != "" {
		fmt.Fprint(os.Stderr, stderrStr)
	}

	if exitCode != 0 {
		os.Exit(exitCode)
	}
	return nil
}

func logExec(entry *logger.Entry) {
	if os.Getenv("FACTORLY_NO_LOG") == "" {
		log, err := logger.NewJSONL("")
		if err == nil {
			_ = log.Log(entry)
			_ = log.Close()
		}
	}
}

func init() {
	execCmd.Flags().IntVar(&execMaxOutput, "max-output", 50000, "max output bytes per call")
	execCmd.Flags().StringVar(&execCompress, "compress", "all", "compression mode: all, json, logs, none")
	execCmd.Flags().StringVar(&execEnvIsolation, "env-isolation", "", "environment isolation: strict (minimal env) or standard (default, inherit parent)")
	execCmd.Flags().BoolVarP(&execInteractive, "interactive", "i", false, "connect directly to terminal (skip compression, for TTY tools)")
}
