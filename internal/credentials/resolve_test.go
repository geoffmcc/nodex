package credentials

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/geoffmcc/nodex/internal/domain"
)

func TestResolver_Resolve_EnvFallback(t *testing.T) {
	dir := t.TempDir()
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

func TestResolver_Resolve_FileFallback(t *testing.T) {
	dir := t.TempDir()
	r := NewResolver(dir)
	ctx := context.Background()

	// Write a credential file in the resolver's directory.
	creds := &domain.Credentials{Type: "token", TokenID: "file-id", TokenSecret: "file-secret"}
	data, _ := json.Marshal(creds) // #nosec G117 -- fixture intentionally matches credential schema.
	if err := os.WriteFile(filepath.Join(dir, "myprofile.json"), data, 0o600); err != nil {
		t.Fatalf("write credential fixture: %v", err)
	}

	got, err := r.Resolve(ctx, "myprofile", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.TokenID != "file-id" {
		t.Errorf("TokenID = %q, want %q", got.TokenID, "file-id")
	}
}

func TestResolver_Resolve_FromRef(t *testing.T) {
	dir := t.TempDir()
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
	dir := t.TempDir()
	r := NewResolver(dir)
	ctx := context.Background()

	_, err := r.Resolve(ctx, "nonexistent", "")
	if err == nil {
		t.Fatal("expected error for missing credentials")
	}
}

func TestResolver_Resolve_UnknownBackend(t *testing.T) {
	dir := t.TempDir()
	r := NewResolver(dir)
	ctx := context.Background()

	_, err := r.Resolve(ctx, "test", "nonexistent:profile")
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
}

func TestResolver_GetBackend(t *testing.T) {
	dir := t.TempDir()
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
