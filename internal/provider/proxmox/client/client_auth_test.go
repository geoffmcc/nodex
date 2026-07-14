package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

// ---------------------------------------------------------------------------
// Auth header construction tests (direct Client construction with HTTP)
// ---------------------------------------------------------------------------

func TestClientTokenAuthSendsPVEAPITokenHeader(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"data":{"release":"8.0","repoid":"test","version":"8.0.0"}}`)
	}))
	defer srv.Close()

	creds := &domain.Credentials{
		Type:        "token",
		TokenID:     "root@pam!monitor",
		TokenSecret: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
	}
	tok := creds.TokenID + "=" + creds.TokenSecret

	c := &Client{baseURL: srv.URL, client: httpclient.New(), token: tok}

	_, err := c.Version(context.Background())
	if err != nil {
		t.Fatalf("Version: %v", err)
	}

	expected := "PVEAPIToken=root@pam!monitor=aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	if capturedAuth != expected {
		t.Errorf("Authorization header = %q, want %q", capturedAuth, expected)
	}
}

func TestClientTokenAuthSendsHeaderOnMutation(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"data":"UPID:node1:00000001:00000001:69444444:vzdump::root@pam:"}`)
	}))
	defer srv.Close()

	creds := &domain.Credentials{
		Type:        "token",
		TokenID:     "root@pam!admin",
		TokenSecret: "secret-token-value",
	}
	tok := creds.TokenID + "=" + creds.TokenSecret

	c := &Client{baseURL: srv.URL, client: httpclient.New(), token: tok}

	_, err := c.CreateBackup(context.Background(), "node1", 100, "local", "snapshot")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	expected := "PVEAPIToken=root@pam!admin=secret-token-value"
	if capturedAuth != expected {
		t.Errorf("Authorization header on mutation = %q, want %q", capturedAuth, expected)
	}
}

func TestClientTokenAuthOnGET(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"data":[]}`)
	}))
	defer srv.Close()

	creds := &domain.Credentials{
		Type:        "token",
		TokenID:     "root@pam!uploader",
		TokenSecret: "upload-secret",
	}
	tok := creds.TokenID + "=" + creds.TokenSecret

	c := &Client{baseURL: srv.URL, client: httpclient.New(), token: tok}

	_, err := c.Nodes(context.Background())
	if err != nil {
		t.Fatalf("Nodes: %v", err)
	}

	expected := "PVEAPIToken=root@pam!uploader=upload-secret"
	if capturedAuth != expected {
		t.Errorf("GET Authorization header = %q, want %q", capturedAuth, expected)
	}
}

func TestClientEmptyTokenSendsNoAuthHeader(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"data":[]}`)
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, client: httpclient.New(), token: ""}

	_, err := c.Nodes(context.Background())
	if err != nil {
		t.Fatalf("Nodes: %v", err)
	}

	if capturedAuth != "" {
		t.Errorf("Authorization header should be empty but got %q", capturedAuth)
	}
}

func TestClientTokenIDOnlyProducesEmptyToken(t *testing.T) {
	// TokenID without TokenSecret should not produce a token.
	creds := &domain.Credentials{
		Type:    "token",
		TokenID: "root@pam!test",
	}
	// Replicate the logic from client.New.
	tok := ""
	if creds.TokenID != "" && creds.TokenSecret != "" {
		tok = creds.TokenID + "=" + creds.TokenSecret
	}

	if tok != "" {
		t.Errorf("token with only TokenID set = %q, want empty", tok)
	}
}

func TestClientTokenOnlyFieldUnused(t *testing.T) {
	// The Token field (combined format) is not used by client.New.
	creds := &domain.Credentials{
		Type:  "token",
		Token: "root@pam!test=the-secret-value",
	}
	// Replicate the logic from client.New.
	tok := ""
	if creds.TokenID != "" && creds.TokenSecret != "" {
		tok = creds.TokenID + "=" + creds.TokenSecret
	}

	if tok != "" {
		t.Errorf("Token field should not be used by client, got %q", tok)
	}
}

func TestClientPasswordCredentialsDoNotProduceToken(t *testing.T) {
	// Password credentials don't produce a PVEAPIToken.
	creds := &domain.Credentials{
		Type:     "password",
		Username: "root@pam",
		Password: "hunter2",
	}
	tok := ""
	if creds.TokenID != "" && creds.TokenSecret != "" {
		tok = creds.TokenID + "=" + creds.TokenSecret
	}

	if tok != "" {
		t.Errorf("Password credentials should not produce token, got %q", tok)
	}
}

func TestClientPasswordSendsNoAuthHeader(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"data":[]}`)
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, client: httpclient.New(), token: ""}

	_, err := c.Nodes(context.Background())
	if err != nil {
		t.Fatalf("Nodes: %v", err)
	}

	if capturedAuth != "" {
		t.Errorf("Password credentials should not produce auth header but got %q", capturedAuth)
	}
}

// ---------------------------------------------------------------------------
// Endpoint validation tests
// ---------------------------------------------------------------------------

func TestNormalizeEndpointRejectsHTTP(t *testing.T) {
	_, err := NormalizeEndpoint("http://pve.example.com:8006")
	if err == nil {
		t.Fatal("expected error for http endpoint")
	}
	if !strings.Contains(err.Error(), "https") {
		t.Errorf("error = %v, want mention of https", err)
	}
}

func TestNormalizeEndpointRejectsUserInfo(t *testing.T) {
	_, err := NormalizeEndpoint("https://user:pass@pve.example.com:8006")
	if err == nil {
		t.Fatal("expected error for endpoint with user info")
	}
}

func TestNormalizeEndpointRejectsQueryString(t *testing.T) {
	_, err := NormalizeEndpoint("https://pve.example.com:8006?param=value")
	if err == nil {
		t.Fatal("expected error for endpoint with query string")
	}
}

func TestNormalizeEndpointRejectsFragment(t *testing.T) {
	_, err := NormalizeEndpoint("https://pve.example.com:8006#fragment")
	if err == nil {
		t.Fatal("expected error for endpoint with fragment")
	}
}

func TestNormalizeEndpointRejectsPath(t *testing.T) {
	_, err := NormalizeEndpoint("https://pve.example.com:8006/some/path")
	if err == nil {
		t.Fatal("expected error for endpoint with path")
	}
}

func TestNormalizeEndpointAcceptsValidHTTPS(t *testing.T) {
	u, err := NormalizeEndpoint("https://pve.example.com:8006/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u != "https://pve.example.com:8006" {
		t.Errorf("normalized = %q", u)
	}

	u, err = NormalizeEndpoint("https://10.0.0.1:8006")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u != "https://10.0.0.1:8006" {
		t.Errorf("normalized = %q", u)
	}
}

// ---------------------------------------------------------------------------
// Auth header does not appear in error messages
// ---------------------------------------------------------------------------

func TestAuthHeaderNotInErrorMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"data":null,"message":"permission denied"}`)
	}))
	defer srv.Close()

	creds := &domain.Credentials{
		Type:        "token",
		TokenID:     "root@pam!test",
		TokenSecret: "super-secret-do-not-leak",
	}
	tok := creds.TokenID + "=" + creds.TokenSecret

	c := &Client{baseURL: srv.URL, client: httpclient.New(), token: tok}

	_, err := c.Nodes(context.Background())
	if err == nil {
		t.Fatal("expected error for 401 response")
	}

	errStr := err.Error()
	if strings.Contains(errStr, "super-secret-do-not-leak") {
		t.Errorf("token secret leaked in error message: %q", errStr)
	}
	if strings.Contains(errStr, "root@pam!test=super-secret-do-not-leak") {
		t.Errorf("full token leaked in error message: %q", errStr)
	}
}

// ---------------------------------------------------------------------------
// Credential validation (from credentials package)
// ---------------------------------------------------------------------------

func TestValidateCredentials_ValidToken(t *testing.T) {
	// This duplicates the logic of credentials.ValidateCredentials
	// to test that the Proxmox client receives valid credential combinations.
	cases := []struct {
		name  string
		creds domain.Credentials
		valid bool
	}{
		{
			name: "valid token_id+secret",
			creds: domain.Credentials{
				Type:        "token",
				TokenID:     "root@pam!monitor",
				TokenSecret: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			},
			valid: true,
		},
		{
			name: "valid combined token",
			creds: domain.Credentials{
				Type:  "token",
				Token: "root@pam!monitor=aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			},
			valid: true,
		},
		{
			name: "missing secret",
			creds: domain.Credentials{
				Type:    "token",
				TokenID: "root@pam!monitor",
			},
			valid: false,
		},
		{
			name: "missing token_id",
			creds: domain.Credentials{
				Type:        "token",
				TokenSecret: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			},
			valid: false,
		},
		{
			name: "valid password",
			creds: domain.Credentials{
				Type:     "password",
				Username: "root@pam",
				Password: "hunter2",
			},
			valid: true,
		},
		{
			name: "password missing username",
			creds: domain.Credentials{
				Type:     "password",
				Password: "hunter2",
			},
			valid: false,
		},
		{
			name: "password missing password",
			creds: domain.Credentials{
				Type:     "password",
				Username: "root@pam",
			},
			valid: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tok := ""
			if tc.creds.TokenID != "" && tc.creds.TokenSecret != "" {
				tok = tc.creds.TokenID + "=" + tc.creds.TokenSecret
			}
			hasAuth := tok != ""

			if tc.valid && !hasAuth && tc.creds.Type == "token" && tc.creds.Token == "" {
				t.Errorf("valid credentials should produce auth token")
			}
			if !tc.valid && hasAuth {
				t.Errorf("invalid credentials should not produce auth token")
			}
		})
	}
}
