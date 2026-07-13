package credentials

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/geoffmcc/nodex/internal/domain"
)

func TestFileBackend_StoreAndGet(t *testing.T) {
	dir := t.TempDir()
	b := NewFileBackend(dir)
	ctx := context.Background()

	creds := &domain.Credentials{
		Type:        "token",
		TokenID:     "id-123",
		TokenSecret: "secret-456",
	}

	if err := b.Store(ctx, "test", creds); err != nil {
		t.Fatalf("Store: %v", err)
	}

	got, err := b.Get(ctx, "test")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.Type != creds.Type {
		t.Errorf("Type = %q, want %q", got.Type, creds.Type)
	}
	if got.TokenID != creds.TokenID {
		t.Errorf("TokenID = %q, want %q", got.TokenID, creds.TokenID)
	}
	if got.TokenSecret != creds.TokenSecret {
		t.Errorf("TokenSecret = %q, want %q", got.TokenSecret, creds.TokenSecret)
	}
}

func TestFileBackend_GetNotFound(t *testing.T) {
	dir := t.TempDir()
	b := NewFileBackend(dir)
	ctx := context.Background()

	_, err := b.Get(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent profile")
	}
}

func TestFileBackend_Delete(t *testing.T) {
	dir := t.TempDir()
	b := NewFileBackend(dir)
	ctx := context.Background()

	creds := &domain.Credentials{Type: "token", TokenID: "id", TokenSecret: "secret"}
	if err := b.Store(ctx, "test", creds); err != nil {
		t.Fatalf("Store: %v", err)
	}

	if err := b.Delete(ctx, "test"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := b.Get(ctx, "test")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestFileBackend_DeleteNotFound(t *testing.T) {
	dir := t.TempDir()
	b := NewFileBackend(dir)
	ctx := context.Background()

	err := b.Delete(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent profile")
	}
}

func TestFileBackend_List(t *testing.T) {
	dir := t.TempDir()
	b := NewFileBackend(dir)
	ctx := context.Background()

	// Write some credential files directly.
	for _, name := range []string{"alpha", "beta", "gamma"} {
		data, _ := json.Marshal(&domain.Credentials{Type: "token"}) // #nosec G117 -- fixture intentionally matches credential schema.
		if err := os.WriteFile(filepath.Join(dir, name+".json"), data, 0o600); err != nil {
			t.Fatalf("write credential fixture: %v", err)
		}
	}
	// Write a non-json file (should be ignored).
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hi"), 0o600); err != nil {
		t.Fatalf("write text fixture: %v", err)
	}

	profiles, err := b.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(profiles) != 3 {
		t.Errorf("List returned %d profiles, want 3: %v", len(profiles), profiles)
	}
}

func TestFileBackend_ListEmpty(t *testing.T) {
	dir := t.TempDir()
	b := NewFileBackend(dir)
	ctx := context.Background()

	profiles, err := b.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(profiles) != 0 {
		t.Errorf("List returned %d profiles, want 0", len(profiles))
	}
}

func TestFileBackend_Name(t *testing.T) {
	b := NewFileBackend(t.TempDir())
	if b.Name() != "file" {
		t.Errorf("Name() = %q, want %q", b.Name(), "file")
	}
}

func TestEnvBackend_Get(t *testing.T) {
	b := NewEnvBackend("nodex")
	ctx := context.Background()

	t.Setenv("NODEX_TESTPROFILE_TOKEN_ID", "env-id")
	t.Setenv("NODEX_TESTPROFILE_TOKEN_SECRET", "env-secret")

	creds, err := b.Get(ctx, "testprofile")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if creds.TokenID != "env-id" {
		t.Errorf("TokenID = %q, want %q", creds.TokenID, "env-id")
	}
	if creds.TokenSecret != "env-secret" {
		t.Errorf("TokenSecret = %q, want %q", creds.TokenSecret, "env-secret")
	}
}

func TestEnvBackend_GetNotFound(t *testing.T) {
	b := NewEnvBackend("nodex")
	ctx := context.Background()

	_, err := b.Get(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent profile")
	}
}

func TestEnvBackend_List(t *testing.T) {
	b := NewEnvBackend("nodex")
	ctx := context.Background()

	t.Setenv("NODEX_ALPHA_TOKEN", "tok")
	t.Setenv("NODEX_BETA_USERNAME", "user")

	profiles, err := b.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(profiles) != 2 {
		t.Errorf("List returned %d profiles, want 2: %v", len(profiles), profiles)
	}
}

func TestEnvBackend_StoreNotSupported(t *testing.T) {
	b := NewEnvBackend("nodex")
	err := b.Store(context.Background(), "x", &domain.Credentials{})
	if err == nil {
		t.Fatal("expected error for Store on env backend")
	}
}

func TestEnvBackend_DeleteNotSupported(t *testing.T) {
	b := NewEnvBackend("nodex")
	err := b.Delete(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error for Delete on env backend")
	}
}

func TestEnvBackend_Name(t *testing.T) {
	b := NewEnvBackend("nodex")
	if b.Name() != "env" {
		t.Errorf("Name() = %q, want %q", b.Name(), "env")
	}
}

func TestStdinBackend_Get(t *testing.T) {
	calls := []string{"id-stdin", "secret-stdin"}
	idx := 0
	b := NewStdinBackend(func(prompt string) (string, error) {
		val := calls[idx]
		idx++
		return val, nil
	})

	creds, err := b.Get(context.Background(), "test")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if creds.TokenID != "id-stdin" {
		t.Errorf("TokenID = %q, want %q", creds.TokenID, "id-stdin")
	}
	if creds.TokenSecret != "secret-stdin" {
		t.Errorf("TokenSecret = %q, want %q", creds.TokenSecret, "secret-stdin")
	}
}

func TestStdinBackend_StoreNotSupported(t *testing.T) {
	b := NewStdinBackend(nil)
	err := b.Store(context.Background(), "x", &domain.Credentials{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestStdinBackend_DeleteNotSupported(t *testing.T) {
	b := NewStdinBackend(nil)
	err := b.Delete(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestStdinBackend_ListEmpty(t *testing.T) {
	b := NewStdinBackend(nil)
	list, err := b.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("List returned %d, want 0", len(list))
	}
}

func TestParseCredentialRef(t *testing.T) {
	tests := []struct {
		ref      string
		wantBE   string
		wantProf string
	}{
		{"", "", ""},
		{"myprofile", "file", "myprofile"},
		{"keyring:myprofile", "keyring", "myprofile"},
		{"env:prod", "env", "prod"},
		{"file:staging", "file", "staging"},
	}

	for _, tt := range tests {
		be, prof := ParseCredentialRef(tt.ref)
		if be != tt.wantBE || prof != tt.wantProf {
			t.Errorf("ParseCredentialRef(%q) = (%q, %q), want (%q, %q)",
				tt.ref, be, prof, tt.wantBE, tt.wantProf)
		}
	}
}

func TestParseCredentialRefStrictRejectsPathEscapes(t *testing.T) {
	tests := []string{
		"",
		":name",
		"file:",
		"file:../secret",
		"file:..\\secret",
		"file:/tmp/secret",
		"file:C:\\secret",
		`file:\\server\\share`,
		"file:café",
		"keyring:some:complex:name",
	}
	for _, ref := range tests {
		if _, _, err := ParseCredentialRefStrict(ref); err == nil {
			t.Fatalf("ParseCredentialRefStrict(%q) succeeded, want error", ref)
		}
	}
}

func TestValidateCredentialsRejectsIncompleteCombinations(t *testing.T) {
	tests := []*domain.Credentials{
		{Type: "token", TokenID: "id"},
		{Type: "token", TokenSecret: "secret"},
		{Type: "password", Username: "user"},
		{Type: "password", Password: "pass"},
		{Type: "insecure", TokenID: "id", TokenSecret: "secret"},
	}
	for _, creds := range tests {
		if err := ValidateCredentials("prof", creds); err == nil {
			t.Fatalf("ValidateCredentials(%+v) succeeded, want error", creds)
		}
	}
}

func TestStdinBackendNilReaderDoesNotPanic(t *testing.T) {
	b := NewStdinBackend(nil)
	if _, err := b.Get(context.Background(), "profile"); err == nil {
		t.Fatal("expected nil reader error")
	}
}

func TestFileBackendInvalidStoreLeavesExistingCredential(t *testing.T) {
	dir := t.TempDir()
	b := NewFileBackend(dir)
	ctx := context.Background()
	good := &domain.Credentials{Type: "token", TokenID: "id", TokenSecret: "secret"}
	if err := b.Store(ctx, "profile", good); err != nil {
		t.Fatalf("Store good: %v", err)
	}
	if err := b.Store(ctx, "profile", &domain.Credentials{Type: "token", TokenID: "new"}); err == nil {
		t.Fatal("expected incomplete credential error")
	}
	got, err := b.Get(ctx, "profile")
	if err != nil {
		t.Fatalf("Get existing: %v", err)
	}
	if got.TokenSecret != "secret" {
		t.Fatalf("existing credential changed: %+v", got)
	}
}

func TestCheckCredentialFilePermissionsSecure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	data := []byte(`{"type":"token","token_id":"id","token_secret":"secret"}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := CheckCredentialFilePermissions(path); err != nil {
		t.Fatalf("CheckCredentialFilePermissions: %v", err)
	}
}

func TestCheckCredentialFilePermissionsInsecure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	data := []byte(`{"type":"token","token_id":"id","token_secret":"secret"}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := CheckCredentialFilePermissions(path); err == nil {
		t.Fatal("expected permission error for 0644 file")
	}
}

func TestCheckCredentialFilePermissionsGroupReadable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	data := []byte(`{"type":"token","token_id":"id","token_secret":"secret"}`)
	if err := os.WriteFile(path, data, 0o640); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := CheckCredentialFilePermissions(path); err == nil {
		t.Fatal("expected permission error for 0640 file")
	}
}
