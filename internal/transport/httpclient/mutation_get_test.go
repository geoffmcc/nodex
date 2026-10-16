package httpclient

import (
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestSideEffectingGETPreservesTLSVerification(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "fixture")
	}))
	defer server.Close()
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
	if resp, err := New().DoMutation(t.Context(), request); err == nil {
		_ = resp.Body.Close()
		t.Fatal("untrusted certificate accepted")
	}
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	ca, err := WithCACert(caPath)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := New(ca).DoMutation(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatal(resp.StatusCode)
	}
}

func TestSideEffectingGETDoesNotReplayLostResponseOnReusedConnection(t *testing.T) {
	var mutations atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/warmup" {
			_, _ = io.WriteString(w, "ready")
			return
		}
		if mutations.Add(1) == 1 {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = conn.Close()
			return
		}
		_, _ = io.WriteString(w, "replayed")
	}))
	defer server.Close()
	c := New()
	warmup, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/warmup", nil)
	resp, err := c.Do(t.Context(), warmup)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/cache", nil)
	resp, err = c.DoMutation(t.Context(), request)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil || mutations.Load() != 1 {
		t.Fatalf("lost response: err=%v submissions=%d", err, mutations.Load())
	}
}

func TestSideEffectingGETDoesNotFollowRedirect(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/cache" {
			http.Redirect(w, r, "/redirected-cache", http.StatusFound)
			return
		}
		_, _ = io.WriteString(w, "redirect followed")
	}))
	defer server.Close()
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/cache", nil)
	resp, err := New().DoMutation(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if requests.Load() != 1 || resp.StatusCode != http.StatusFound {
		t.Fatalf("redirect: requests=%d status=%d", requests.Load(), resp.StatusCode)
	}
}
