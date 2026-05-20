// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestGeneratePKCE(t *testing.T) {
	pkce, err := GeneratePKCE()
	if err != nil {
		t.Fatal(err)
	}

	// Verifier should be 43 chars (32 bytes base64url-encoded without padding)
	if len(pkce.Verifier) != 43 {
		t.Errorf("expected verifier length 43, got %d", len(pkce.Verifier))
	}

	// Challenge should be 43 chars (SHA256 = 32 bytes, base64url-encoded)
	if len(pkce.Challenge) != 43 {
		t.Errorf("expected challenge length 43, got %d", len(pkce.Challenge))
	}

	if pkce.Method != "S256" {
		t.Errorf("expected method S256, got %s", pkce.Method)
	}

	// Two calls should produce different verifiers
	pkce2, _ := GeneratePKCE()
	if pkce.Verifier == pkce2.Verifier {
		t.Error("expected different verifiers")
	}
}

func TestTokenBundleExpiry(t *testing.T) {
	tests := []struct {
		name    string
		expiry  time.Time
		skew    time.Duration
		expired bool
	}{
		{"future", time.Now().Add(1 * time.Hour), 0, false},
		{"past", time.Now().Add(-1 * time.Hour), 0, true},
		{"within skew", time.Now().Add(20 * time.Second), 30 * time.Second, true},
		{"outside skew", time.Now().Add(1 * time.Minute), 30 * time.Second, false},
		{"zero expiry", time.Time{}, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bundle := &TokenBundle{Expiry: tt.expiry}
			if bundle.IsExpired(tt.skew) != tt.expired {
				t.Errorf("expected expired=%v, got %v", tt.expired, !tt.expired)
			}
		})
	}
}

func TestRefreshAccessToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.FormValue("grant_type") != "refresh_token" {
			t.Errorf("expected grant_type=refresh_token, got %s", r.FormValue("grant_type"))
		}
		if r.FormValue("refresh_token") != "old-refresh-token" {
			t.Errorf("expected old refresh token, got %s", r.FormValue("refresh_token"))
		}
		if r.FormValue("client_id") != "test-client" {
			t.Errorf("expected client_id=test-client, got %s", r.FormValue("client_id"))
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access-token",
			"refresh_token": "new-refresh-token",
			"token_type":    "bearer",
			"expires_in":    3600,
		})
	}))
	defer srv.Close()

	cfg := ProviderConfig{
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		TokenURL:     srv.URL,
	}

	bundle, err := RefreshAccessToken(context.Background(), cfg, "old-refresh-token")
	if err != nil {
		t.Fatal(err)
	}

	if bundle.AccessToken != "new-access-token" {
		t.Errorf("expected new-access-token, got %s", bundle.AccessToken)
	}
	if bundle.RefreshToken != "new-refresh-token" {
		t.Errorf("expected new-refresh-token, got %s", bundle.RefreshToken)
	}
	if bundle.TokenType != "bearer" {
		t.Errorf("expected bearer, got %s", bundle.TokenType)
	}
	if bundle.Expiry.IsZero() {
		t.Error("expected non-zero expiry")
	}
}

func TestRefreshAccessTokenKeepsOldRefreshToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Some providers don't return a new refresh token
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "new-access-token",
			"token_type":   "bearer",
			"expires_in":   3600,
		})
	}))
	defer srv.Close()

	cfg := ProviderConfig{ClientID: "test", TokenURL: srv.URL}
	bundle, err := RefreshAccessToken(context.Background(), cfg, "original-refresh")
	if err != nil {
		t.Fatal(err)
	}

	if bundle.RefreshToken != "original-refresh" {
		t.Errorf("expected original refresh token preserved, got %s", bundle.RefreshToken)
	}
}

func TestRefreshAccessTokenError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":             "invalid_grant",
			"error_description": "refresh token expired",
		})
	}))
	defer srv.Close()

	cfg := ProviderConfig{ClientID: "test", TokenURL: srv.URL}
	_, err := RefreshAccessToken(context.Background(), cfg, "expired-refresh")
	if err == nil {
		t.Fatal("expected error for expired refresh token")
	}
}

func TestRefreshAccessTokenNetworkError(t *testing.T) {
	cfg := ProviderConfig{ClientID: "test", TokenURL: "http://127.0.0.1:1/token"}
	_, err := RefreshAccessToken(context.Background(), cfg, "token")
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestTokenBundleJSON(t *testing.T) {
	bundle := &TokenBundle{
		AccessToken:  "access",
		RefreshToken: "refresh",
		TokenType:    "bearer",
		Expiry:       time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}

	var decoded TokenBundle
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.AccessToken != "access" {
		t.Errorf("expected access, got %s", decoded.AccessToken)
	}
	if decoded.RefreshToken != "refresh" {
		t.Errorf("expected refresh, got %s", decoded.RefreshToken)
	}
	if !decoded.Expiry.Equal(bundle.Expiry) {
		t.Errorf("expected same expiry, got %v", decoded.Expiry)
	}
}

// --- exchangeCode tests ---

func TestExchangeCodeSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("grant_type") != "authorization_code" {
			t.Errorf("expected grant_type=authorization_code, got %s", r.FormValue("grant_type"))
		}
		if r.FormValue("code") != "test-auth-code" {
			t.Errorf("expected code=test-auth-code, got %s", r.FormValue("code"))
		}
		if r.FormValue("code_verifier") != "test-verifier" {
			t.Errorf("expected code_verifier=test-verifier, got %s", r.FormValue("code_verifier"))
		}
		if r.FormValue("redirect_uri") != "http://localhost:18019/callback" {
			t.Errorf("expected redirect_uri, got %s", r.FormValue("redirect_uri"))
		}
		if r.FormValue("client_id") != "my-client" {
			t.Errorf("expected client_id=my-client, got %s", r.FormValue("client_id"))
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "exchanged-token",
			"refresh_token": "exchanged-refresh",
			"token_type":    "bearer",
			"expires_in":    7200,
		})
	}))
	defer srv.Close()

	cfg := ProviderConfig{ClientID: "my-client", TokenURL: srv.URL}
	bundle, err := exchangeCode(context.Background(), cfg, "test-auth-code", "http://localhost:18019/callback", "test-verifier")
	if err != nil {
		t.Fatal(err)
	}
	if bundle.AccessToken != "exchanged-token" {
		t.Errorf("expected exchanged-token, got %s", bundle.AccessToken)
	}
	if bundle.RefreshToken != "exchanged-refresh" {
		t.Errorf("expected exchanged-refresh, got %s", bundle.RefreshToken)
	}
}

func TestExchangeCodeWithClientSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("client_secret") != "my-secret" {
			t.Errorf("expected client_secret=my-secret, got %q", r.FormValue("client_secret"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "token",
			"token_type":   "bearer",
		})
	}))
	defer srv.Close()

	cfg := ProviderConfig{ClientID: "id", ClientSecret: "my-secret", TokenURL: srv.URL}
	_, err := exchangeCode(context.Background(), cfg, "code", "http://localhost/cb", "verifier")
	if err != nil {
		t.Fatal(err)
	}
}

func TestExchangeCodeWithoutClientSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("client_secret") != "" {
			t.Errorf("expected no client_secret, got %q", r.FormValue("client_secret"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "token",
			"token_type":   "bearer",
		})
	}))
	defer srv.Close()

	cfg := ProviderConfig{ClientID: "id", TokenURL: srv.URL}
	_, err := exchangeCode(context.Background(), cfg, "code", "http://localhost/cb", "verifier")
	if err != nil {
		t.Fatal(err)
	}
}

// --- tokenRequest error path tests ---

func TestTokenRequestInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "not json at all")
	}))
	defer srv.Close()

	_, err := tokenRequest(context.Background(), srv.URL, url.Values{"grant_type": {"test"}})
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}
	if !strings.Contains(err.Error(), "parsing token response") {
		t.Errorf("expected 'parsing token response' in error, got: %s", err.Error())
	}
}

func TestTokenRequestMissingAccessToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token_type": "bearer",
			"expires_in": 3600,
		})
	}))
	defer srv.Close()

	_, err := tokenRequest(context.Background(), srv.URL, url.Values{"grant_type": {"test"}})
	if err == nil {
		t.Fatal("expected error for missing access_token")
	}
	if !strings.Contains(err.Error(), "missing access_token") {
		t.Errorf("expected 'missing access_token' in error, got: %s", err.Error())
	}
}

func TestTokenRequestOAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":             "invalid_client",
			"error_description": "client authentication failed",
		})
	}))
	defer srv.Close()

	_, err := tokenRequest(context.Background(), srv.URL, url.Values{"grant_type": {"test"}})
	if err == nil {
		t.Fatal("expected error for OAuth error response")
	}
	if !strings.Contains(err.Error(), "invalid_client") {
		t.Errorf("expected 'invalid_client' in error, got: %s", err.Error())
	}
}

func TestTokenRequestNoExpiresIn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "token-no-expiry",
			"token_type":   "bearer",
		})
	}))
	defer srv.Close()

	bundle, err := tokenRequest(context.Background(), srv.URL, url.Values{"grant_type": {"test"}})
	if err != nil {
		t.Fatal(err)
	}
	if !bundle.Expiry.IsZero() {
		t.Errorf("expected zero expiry when expires_in not set, got %v", bundle.Expiry)
	}
}

func TestTokenRequestHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "internal server error")
	}))
	defer srv.Close()

	_, err := tokenRequest(context.Background(), srv.URL, url.Values{"grant_type": {"test"}})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected '500' in error, got: %s", err.Error())
	}
}

// --- LoginFlow tests ---

// waitForPort polls until the given port is listening or timeout expires.
func waitForPort(t *testing.T, port int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 50*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("port %d not listening after %s", port, timeout)
}

// waitForCallback receives the auth URL the LoginFlow goroutine
// passed to openBrowserFn, extracts the callback port from
// redirect_uri, waits for it to be listening, and returns both the
// URL and the port. Returning the URL means the caller doesn't need
// a separately-synchronized variable to read it back.
//
// Channel handoff (not a spin-loop on a shared string) gives the
// race detector the happens-before edge it needs: the URL string's
// underlying memory is fully constructed on the sending goroutine
// before the receive completes.
func waitForCallback(t *testing.T, urlCh <-chan string, timeout time.Duration) (capturedURL string, port int) {
	t.Helper()
	select {
	case capturedURL = <-urlCh:
	case <-time.After(timeout):
		t.Fatal("auth URL was not captured within timeout")
	}
	parsed, err := url.Parse(capturedURL)
	if err != nil {
		t.Fatalf("parsing auth URL: %v", err)
	}
	redirectURI := parsed.Query().Get("redirect_uri")
	if redirectURI == "" {
		t.Fatal("no redirect_uri in auth URL")
	}
	rParsed, err := url.Parse(redirectURI)
	if err != nil {
		t.Fatalf("parsing redirect_uri: %v", err)
	}
	port, err = strconv.Atoi(rParsed.Port())
	if err != nil {
		t.Fatalf("parsing port from redirect_uri %q: %v", redirectURI, err)
	}
	waitForPort(t, port, timeout)
	return capturedURL, port
}

func TestLoginFlowSuccess(t *testing.T) {
	// Mock token endpoint
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "login-access-token",
			"refresh_token": "login-refresh-token",
			"token_type":    "bearer",
			"expires_in":    3600,
		})
	}))
	defer tokenSrv.Close()

	// Capture auth URL via channel handoff. The buffered channel
	// gives the race detector the happens-before edge it needs:
	// the URL string's bytes are fully constructed on the LoginFlow
	// goroutine before the send, and only visible to the test
	// goroutine after the receive.
	urlCh := make(chan string, 1)
	origFn := openBrowserFn
	openBrowserFn = func(u string) { urlCh <- u }
	defer func() { openBrowserFn = origFn }()

	cfg := ProviderConfig{
		ClientID: "test-client",
		AuthURL:  "http://example.com/auth",
		TokenURL: tokenSrv.URL,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resultCh := make(chan *TokenBundle, 1)
	errCh := make(chan error, 1)
	go func() {
		bundle, err := LoginFlow(ctx, cfg)
		if err != nil {
			errCh <- err
		} else {
			resultCh <- bundle
		}
	}()

	// Wait for callback server to start and extract port + URL.
	capturedURL, port := waitForCallback(t, urlCh, 2*time.Second)

	// Extract state from captured auth URL
	parsed, err := url.Parse(capturedURL)
	if err != nil {
		t.Fatalf("parsing captured URL: %v", err)
	}
	state := parsed.Query().Get("state")
	if state == "" {
		t.Fatal("expected state in auth URL")
	}

	// Simulate browser callback
	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/callback?code=test-code&state=%s", port, url.QueryEscape(state))
	resp, err := http.Get(callbackURL)
	if err != nil {
		t.Fatalf("hitting callback: %v", err)
	}
	resp.Body.Close()

	select {
	case bundle := <-resultCh:
		if bundle.AccessToken != "login-access-token" {
			t.Errorf("expected login-access-token, got %s", bundle.AccessToken)
		}
		if bundle.RefreshToken != "login-refresh-token" {
			t.Errorf("expected login-refresh-token, got %s", bundle.RefreshToken)
		}
	case err := <-errCh:
		t.Fatalf("LoginFlow error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("LoginFlow timed out")
	}
}

func TestLoginFlowStateMismatch(t *testing.T) {
	urlCh := make(chan string, 1)
	origFn := openBrowserFn
	openBrowserFn = func(u string) { urlCh <- u }
	defer func() { openBrowserFn = origFn }()

	cfg := ProviderConfig{
		ClientID: "test",
		AuthURL:  "http://example.com/auth",
		TokenURL: "http://example.com/token",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		_, err := LoginFlow(ctx, cfg)
		errCh <- err
	}()

	_, port := waitForCallback(t, urlCh, 2*time.Second)

	// Send callback with wrong state
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/callback?code=test-code&state=wrong-state", port))
	if err != nil {
		t.Fatalf("hitting callback: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for state mismatch, got %d", resp.StatusCode)
	}

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error for state mismatch")
		}
		if !strings.Contains(err.Error(), "state mismatch") {
			t.Errorf("expected 'state mismatch' in error, got: %s", err.Error())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("LoginFlow didn't return after state mismatch")
	}
}

func TestLoginFlowMissingCode(t *testing.T) {
	urlCh := make(chan string, 1)
	origFn := openBrowserFn
	openBrowserFn = func(u string) { urlCh <- u }
	defer func() { openBrowserFn = origFn }()

	cfg := ProviderConfig{
		ClientID: "test",
		AuthURL:  "http://example.com/auth",
		TokenURL: "http://example.com/token",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		_, err := LoginFlow(ctx, cfg)
		errCh <- err
	}()

	capturedURL, port := waitForCallback(t, urlCh, 2*time.Second)

	parsed, _ := url.Parse(capturedURL)
	state := parsed.Query().Get("state")

	// Send callback with correct state but no code
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/callback?state=%s", port, url.QueryEscape(state)))
	if err != nil {
		t.Fatalf("hitting callback: %v", err)
	}
	resp.Body.Close()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error for missing code")
		}
		if !strings.Contains(err.Error(), "no authorization code") {
			t.Errorf("expected 'no authorization code' in error, got: %s", err.Error())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("LoginFlow didn't return")
	}
}

func TestLoginFlowProviderError(t *testing.T) {
	urlCh := make(chan string, 1)
	origFn := openBrowserFn
	openBrowserFn = func(u string) { urlCh <- u }
	defer func() { openBrowserFn = origFn }()

	cfg := ProviderConfig{
		ClientID: "test",
		AuthURL:  "http://example.com/auth",
		TokenURL: "http://example.com/token",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		_, err := LoginFlow(ctx, cfg)
		errCh <- err
	}()

	capturedURL, port := waitForCallback(t, urlCh, 2*time.Second)

	parsed, _ := url.Parse(capturedURL)
	state := parsed.Query().Get("state")

	// Send callback with OAuth error
	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/callback?state=%s&error=access_denied&error_description=user+denied", port, url.QueryEscape(state))
	resp, err := http.Get(callbackURL)
	if err != nil {
		t.Fatalf("hitting callback: %v", err)
	}
	resp.Body.Close()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error for provider error")
		}
		if !strings.Contains(err.Error(), "access_denied") {
			t.Errorf("expected 'access_denied' in error, got: %s", err.Error())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("LoginFlow didn't return")
	}
}

func TestLoginFlowTimeout(t *testing.T) {
	origFn := openBrowserFn
	openBrowserFn = func(string) {}
	defer func() { openBrowserFn = origFn }()

	cfg := ProviderConfig{
		ClientID: "test",
		AuthURL:  "http://example.com/auth",
		TokenURL: "http://example.com/token",
	}

	// Already-expired context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := LoginFlow(ctx, cfg)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected 'timed out' in error, got: %s", err.Error())
	}
}
