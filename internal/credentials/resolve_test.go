package credentials

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/domain"
)

func TestResolver_Resolve_EnvFallback(t *testing.T) {
	dir := privateCredentialTestDir(t)
	r := NewResolver(dir)
	ctx := context.Background()

	t.Setenv("NODEX_MYPROFILE_TOKEN", "tok-123")

	creds, err := r.Resolve(ctx, "myprofile", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if creds.Token != "tok-123" {
		t.Errorf("Token = %q, want %q", creds.Token, "tok-123")
	}
}

func TestResolver_EnvironmentCredentialPrecedesFileFallback(t *testing.T) {
	dir := privateCredentialTestDir(t)
	r := NewResolver(dir)
	fileCreds := &domain.Credentials{Type: "token", TokenID: "file-id", TokenSecret: "file-secret"}
	data, err := json.Marshal(fileCreds) // #nosec G117 -- synthetic test credentials exercise backend precedence.
	if err != nil {
		t.Fatal(err)
	}
	writeSecureCredentialFileForTest(t, filepath.Join(dir, "myprofile.json"), data)
	t.Setenv("NODEX_MYPROFILE_TOKEN", "environment-token")
	got, err := r.Resolve(context.Background(), "myprofile", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Token != "environment-token" || got.TokenID != "" {
		t.Fatalf("credential = %+v, want environment credential to take precedence", got.Redacted())
	}
}

func TestResolver_InvalidEnvironmentCredentialsDoNotFallBackToFile(t *testing.T) {
	dir := privateCredentialTestDir(t)
	r := NewResolver(dir)
	profile := "myprofile"
	fileCreds := &domain.Credentials{Type: "token", TokenID: "old-file-id", TokenSecret: "old-file-secret"}
	data, err := json.Marshal(fileCreds) // #nosec G117 -- synthetic credentials test source precedence.
	if err != nil {
		t.Fatal(err)
	}
	writeSecureCredentialFileForTest(t, filepath.Join(dir, profile+".json"), data)

	tests := []struct {
		name        string
		vars        map[string]string
		want        string
		secretValue string
	}{
		{
			name: "token id without secret",
			vars: map[string]string{"TOKEN_ID": "new-environment-id"},
			want: "token credentials missing token_secret",
		},
		{
			name: "username without password",
			vars: map[string]string{"USERNAME": "new-environment-user"},
			want: "password credentials missing password",
		},
		{
			name:        "token secret without id",
			vars:        map[string]string{"TOKEN_SECRET": "partial-environment-secret"},
			want:        "token credentials missing token_id",
			secretValue: "partial-environment-secret",
		},
		{
			name:        "password without username",
			vars:        map[string]string{"PASSWORD": "partial-environment-password"},
			want:        "password credentials missing username",
			secretValue: "partial-environment-password",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, suffix := range []string{"TOKEN", "TOKEN_ID", "TOKEN_SECRET", "USERNAME", "PASSWORD"} {
				t.Setenv("NODEX_MYPROFILE_"+suffix, "")
			}
			for suffix, value := range tt.vars {
				t.Setenv("NODEX_MYPROFILE_"+suffix, value)
			}

			got, err := r.Resolve(context.Background(), profile, "")
			if err == nil {
				t.Fatal("Resolve succeeded using another credential source despite an invalid environment credential")
			}
			if got != nil {
				t.Fatal("Resolve returned credentials alongside an invalid environment credential error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Resolve error = %q, want %q", err, tt.want)
			}
			if strings.Contains(err.Error(), "old-file-secret") {
				t.Fatal("Resolve error exposed fallback file credential")
			}
			if tt.secretValue != "" && strings.Contains(err.Error(), tt.secretValue) {
				t.Fatal("Resolve error exposed an environment secret")
			}
		})
	}
}

func TestResolver_Resolve_FileFallback(t *testing.T) {
	dir := privateCredentialTestDir(t)
	r := NewResolver(dir)
	ctx := context.Background()

	// Write a credential file in the resolver's directory.
	creds := &domain.Credentials{Type: "token", TokenID: "file-id", TokenSecret: "file-secret"}
	data, _ := json.Marshal(creds) // #nosec G117 -- fixture intentionally matches credential schema.
	writeSecureCredentialFileForTest(t, filepath.Join(dir, "myprofile.json"), data)

	got, err := r.Resolve(ctx, "myprofile", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.TokenID != "file-id" {
		t.Errorf("TokenID = %q, want %q", got.TokenID, "file-id")
	}
}

func TestResolver_ReportsInvalidFileCredential(t *testing.T) {
	dir := privateCredentialTestDir(t)
	r := NewResolver(dir)
	for _, suffix := range []string{"TOKEN", "TOKEN_ID", "TOKEN_SECRET", "USERNAME", "PASSWORD"} {
		t.Setenv("NODEX_MYPROFILE_"+suffix, "")
	}
	writeSecureCredentialFileForTest(t, filepath.Join(dir, "myprofile.json"), []byte("not-json"))

	_, err := r.Resolve(context.Background(), "myprofile", "")
	if err == nil || !strings.Contains(err.Error(), "parse credential file") {
		t.Fatalf("Resolve error = %v, want the existing file credential parse failure", err)
	}
}

func TestResolver_Resolve_FromRef(t *testing.T) {
	dir := privateCredentialTestDir(t)
	r := NewResolver(dir)
	ctx := context.Background()

	// Store in file backend.
	fb := NewFileBackend(dir)
	creds := &domain.Credentials{Type: "token", TokenID: "ref-id", TokenSecret: "ref-secret"}
	if err := fb.Store(ctx, "ref-profile", creds); err != nil {
		t.Fatalf("store credential fixture: %v", err)
	}

	// Resolve using credential_ref.
	got, err := r.Resolve(ctx, "anything", "file:ref-profile")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.TokenID != "ref-id" {
		t.Errorf("TokenID = %q, want %q", got.TokenID, "ref-id")
	}
	if got.TokenSecret != "ref-secret" {
		t.Errorf("TokenSecret = %q, want %q", got.TokenSecret, "ref-secret")
	}
}

func TestResolver_Resolve_NoCredentials(t *testing.T) {
	dir := privateCredentialTestDir(t)
	r := NewResolver(dir)
	ctx := context.Background()

	_, err := r.Resolve(ctx, "nonexistent", "")
	if err == nil {
		t.Fatal("expected error for missing credentials")
	}
}

func TestResolver_RejectsMalformedProfileBeforeCredentialLookup(t *testing.T) {
	dir := privateCredentialTestDir(t)
	r := NewResolver(dir)
	for _, profile := range []string{"../outside", `..\\outside`, "profile/name"} {
		t.Run(profile, func(t *testing.T) {
			if _, err := r.Resolve(context.Background(), profile, ""); err == nil || !strings.Contains(err.Error(), "malformed credential name") {
				t.Fatalf("Resolve(%q) error = %v, want profile validation error", profile, err)
			}
		})
	}
}

func TestResolver_Resolve_UnknownBackend(t *testing.T) {
	dir := privateCredentialTestDir(t)
	r := NewResolver(dir)
	ctx := context.Background()

	_, err := r.Resolve(ctx, "test", "nonexistent:profile")
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
}

func TestResolver_GetBackend(t *testing.T) {
	dir := privateCredentialTestDir(t)
	r := NewResolver(dir)

	if _, ok := r.GetBackend("file"); !ok {
		t.Error("expected file backend to exist")
	}
	if _, ok := r.GetBackend("keyring"); !ok {
		t.Error("expected keyring backend to exist")
	}
	if _, ok := r.GetBackend("env"); !ok {
		t.Error("expected env backend to exist")
	}
	if _, ok := r.GetBackend("nonexistent"); ok {
		t.Error("expected nonexistent backend to not exist")
	}
}
