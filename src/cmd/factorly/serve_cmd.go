package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	factorlyServer "github.com/factorly-dev/factorly/internal/server"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

var httpAddr string

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start an MCP server exposing all configured tools",
	Long: `Start an MCP server that exposes all configured Factorly tools
to MCP clients like Claude Code and Cursor.

By default, uses stdio transport. Use --http to start an HTTP server instead.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, reg, err := loadConfig()
		if err != nil {
			return err
		}

		p, err := bootstrapProviders(cfg, reg)
		if err != nil {
			return err
		}

		if verbose {
			factorlyServer.Verbose = func(format string, args ...any) {
				fmt.Fprintf(os.Stderr, "[factorly] "+format+"\n", args...)
			}
		}

		s := factorlyServer.New(reg, p)

		ctx, stop := signal.NotifyContext(context.Background(),
			os.Interrupt, syscall.SIGTERM)
		defer stop()

		if httpAddr != "" {
			return serveHTTP(ctx, s, httpAddr)
		}
		return serveStdio(ctx, s)
	},
}

func serveStdio(ctx context.Context, s *server.MCPServer) error {
	vlog("starting MCP server (stdio)")
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ServeStdio(s)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		vlog("shutting down MCP server")
		return nil
	}
}

func serveHTTP(ctx context.Context, s *server.MCPServer, addr string) error {
	vlog("starting MCP server (HTTP on %s)", addr)
	httpServer := server.NewStreamableHTTPServer(s)

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.Start(addr)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		vlog("shutting down HTTP server")
		return httpServer.Shutdown(ctx)
	}
}

func init() {
	serveCmd.Flags().StringVar(&httpAddr, "http", "",
		"start HTTP transport on this address (e.g. :3000) instead of stdio")
}
