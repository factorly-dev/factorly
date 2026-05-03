// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/factorly-dev/factorly/internal/ui"
	"github.com/factorly-dev/factorly/internal/vault"
	"github.com/spf13/cobra"
)

var uiPort int

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Open the Factorly web UI",
	Long:  "Starts a localhost web server with a visual interface for configuring tools, running them, and managing workflows.",
	RunE:  runUI,
}

func init() {
	uiCmd.Flags().IntVar(&uiPort, "port", 3741, "port for the UI server")
}

func runUI(cmd *cobra.Command, args []string) error {
	cfg, reg, err := loadConfig()
	if err != nil {
		return err
	}

	p, err := bootstrapProviders(cfg, reg)
	if err != nil {
		return err
	}
	defer p.Teardown()

	// Get vault backend (cached singleton from bootstrapProviders, won't re-prompt)
	var vaultBackend vault.Backend
	vaultBackend, _ = getCachedVault()

	addr := fmt.Sprintf("localhost:%d", uiPort)

	srv, err := ui.New(ui.Options{
		Config:   cfg,
		CfgPath:  configPath,
		ToolsDir: cfg.ToolsDir,
		Registry: reg,
		Proxy:    p,
		Vault:    vaultBackend,
	})
	if err != nil {
		return err
	}

	// Open browser
	go openBrowser(fmt.Sprintf("http://%s", addr))

	return srv.Start(addr)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	}
	if cmd != nil {
		_ = cmd.Start()
	}
}
