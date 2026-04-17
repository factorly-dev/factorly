// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
