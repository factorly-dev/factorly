// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	factorlyServer "github.com/factorly-dev/factorly/internal/server"
	"github.com/factorly-dev/factorly/internal/vault"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

var httpAddr string
var httpToken string

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start an MCP server exposing all configured tools",
	Long: `Start an MCP server that exposes all configured Factorly tools
to MCP clients like Claude Code and Cursor.

By default, uses stdio transport. Use --http to start an HTTP server instead.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkCommandAllowed("serve"); err != nil {
			return err
		}
		if httpAddr != "" {
			serveMode = "http"
		}
		cfg, reg, err := loadConfig()
		if err != nil {
			return err
		}

		p, err := bootstrapProviders(cfg, reg, mcpElicitConfirm)
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
			// Resolve token: flag → env var, then resolve vault refs
			if httpToken == "" {
				httpToken = os.Getenv("FACTORLY_HTTP_TOKEN")
			}
			if httpToken != "" && vault.HasVaultRefs(httpToken) {
				backend, err := openVault()
				if err != nil {
					return fmt.Errorf("resolving http-token vault ref: %w", err)
				}
				defer backend.Close()
				resolver := vault.NewResolver()
				resolver.Register("vault", backend)
				httpToken = resolveVaultRef(resolver, httpToken)
			}
			if httpToken == "" {
				fmt.Fprintln(os.Stderr, "WARNING: HTTP server has no authentication. Use --http-token or FACTORLY_HTTP_TOKEN for production.")
			}
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

	var handler http.Handler = httpServer
	if httpToken != "" {
		handler = tokenAuthMiddleware(httpServer, httpToken)
		vlog("HTTP token authentication enabled")
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	case <-ctx.Done():
		vlog("shutting down HTTP server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

func tokenAuthMiddleware(next http.Handler, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != token {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// mcpElicitConfirm uses MCP elicitation to prompt the user for confirmation
// via the MCP client (e.g., Claude Code shows the prompt in the chat UI).
func mcpElicitConfirm(ctx context.Context, toolName string, params map[string]string) bool {
	session := server.ClientSessionFromContext(ctx)
	if session == nil {
		vlog("shadow confirm: no MCP session available, denying")
		return false
	}

	elicitSession, ok := session.(server.SessionWithElicitation)
	if !ok {
		vlog("shadow confirm: session does not support elicitation, denying")
		return false
	}

	req := mcp.ElicitationRequest{}
	req.Params.Message = fmt.Sprintf("Tool %q requires confirmation. Allow this call?", toolName)
	req.Params.RequestedSchema = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"approve": map[string]any{
				"type":        "boolean",
				"description": "Approve this tool call?",
				"default":     true,
			},
		},
	}

	result, err := elicitSession.RequestElicitation(ctx, req)
	if err != nil {
		vlog("shadow confirm: elicitation failed: %v", err)
		return false
	}

	return result.Action == mcp.ElicitationResponseActionAccept
}

func init() {
	serveCmd.Flags().StringVar(&httpAddr, "http", "",
		"start HTTP transport on this address (e.g. :3000) instead of stdio")
	serveCmd.Flags().StringVar(&httpToken, "http-token", "",
		"require Bearer token authentication for HTTP transport")
}
