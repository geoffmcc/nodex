package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDoRetriesOn5xx(t *testing.T) {
	var calls int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := atomic.AddInt32(&calls, 1)
		if c < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"ok"}`))
	}))
	defer s.Close()

	c := New(WithMaxRetries(2))
	req, _ := http.NewRequest(http.MethodGet, s.URL, nil)
	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_ = resp.Body.Close()
	if atomic.LoadInt32(&calls) != 3 {
		t.Errorf("calls = %d, want 3 (initial + 2 retries)", calls)
	}
}

func TestDoFailsAfterMaxRetries(t *testing.T) {
	var calls int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer s.Close()

	c := New(WithMaxRetries(1))
	req, _ := http.NewRequest(http.MethodGet, s.URL, nil)
	_, err := c.Do(context.Background(), req)
	if err == nil {
		t.Fatal("expected error after max retries")
	}
	if !strings.Contains(err.Error(), "max retries exceeded") {
		t.Errorf("error = %q, want max retries exceeded", err)
	}
}

func TestDoMutationNeverRetries(t *testing.T) {
	var calls int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer s.Close()

	c := New(WithMaxRetries(5))
	req, _ := http.NewRequest(http.MethodPost, s.URL, nil)
	resp, err := c.DoMutation(context.Background(), req)
	if err != nil {
		t.Fatalf("DoMutation transport error: %v (should succeed at transport level)", err)
	}
	_ = resp.Body.Close()
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("calls = %d, want 1 (no retry)", calls)
	}
}

func TestDoMutationSuccessful(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"ok"}`))
	}))
	defer s.Close()

	c := New()
	req, _ := http.NewRequest(http.MethodPost, s.URL, nil)
	resp, err := c.DoMutation(context.Background(), req)
	if err != nil {
		t.Fatalf("DoMutation: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func TestDoContextCancellation(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until context is cancelled.
		<-r.Context().Done()
	}))
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	c := New(WithMaxRetries(0))
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	_, err := c.Do(ctx, req)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestDoMutationContextCancellation(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := New()
	req, _ := http.NewRequestWithContext(ctx, http.MethodDelete, s.URL, nil)
	_, err := c.DoMutation(ctx, req)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestJitteredDelayProducesVariableOutput(t *testing.T) {
	c := New()
	results := make(map[float64]bool)
	for range 100 {
		d := c.jitteredDelay(1)
		results[float64(d)] = true
	}
	if len(results) < 2 {
		t.Errorf("jitteredDelay produced only %d unique values; want >1 (jitter not working)", len(results))
	}
}

// --- Redirect policy tests (SEC-REDIR) ---

func TestDoRejectsRedirectToDifferentHost(t *testing.T) {
	// Server A redirects to Server B (different host).
	// The redirect should be rejected to prevent credential forwarding.
	sA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://evil.example.com/steal", http.StatusFound)
	}))
	defer sA.Close()

	c := New(WithMaxRetries(0))
	req, _ := http.NewRequest(http.MethodGet, sA.URL, nil)
	req.Header.Set("Authorization", "PVEAPIToken=user@pam!tok=secret123") // #nosec G101
	_, err := c.Do(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for redirect to different host")
	}
	if !strings.Contains(err.Error(), "different host") {
		t.Errorf("error = %q, want 'different host' message", err)
	}
}

func TestDoRejectsHTTPSToHTTPRedirect(t *testing.T) {
	// Set up an HTTPS server that redirects to HTTP.
	// Since httptest.NewTLSServer is available, we can test scheme downgrade.
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Redirect from HTTPS to HTTP (scheme downgrade).
		http.Redirect(w, r, "http://"+r.Host+"/downgrade", http.StatusFound)
	}))
	defer s.Close()

	c := New(WithMaxRetries(0))
	req, _ := http.NewRequest(http.MethodGet, s.URL, nil)
	// Use the test server's TLS config to trust the self-signed cert.
	c.httpClient.Transport = s.Client().Transport
	_, err := c.Do(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for HTTPS to HTTP redirect")
	}
	if !strings.Contains(err.Error(), "https to http") {
		t.Errorf("error = %q, want 'https to http' message", err)
	}
}

func TestDoAllowsRedirectToSameHost(t *testing.T) {
	var calls int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = atomic.AddInt32(&calls, 1)
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"ok"}`))
	}))
	defer s.Close()

	c := New(WithMaxRetries(0))
	req, _ := http.NewRequest(http.MethodGet, s.URL+"/redirect", nil)
	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_ = resp.Body.Close()
	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("calls = %d, want 2 (initial + redirect)", calls)
	}
}
