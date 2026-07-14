package httpclient

import (
	"context"
	"errors"
	"fmt"
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

// --- Redirect safety tests (CR-003) ---

// TestCheckRedirectUnitHTTPSDowngrade tests the HTTPS → HTTP downgrade guard
// in isolation using crafted request objects. This is the most reliable way to
// verify the scheme check independently of the host check.
func TestCheckRedirectUnitHTTPSDowngrade(t *testing.T) {
	c := New()

	original, _ := http.NewRequest(http.MethodGet, "https://pve.example.com:8006/api2/json/nodes", nil)
	redirect, _ := http.NewRequest(http.MethodGet, "http://pve.example.com:8006/api2/json/nodes", nil)

	err := c.checkRedirect(redirect, []*http.Request{original})
	if err == nil {
		t.Fatal("expected error for HTTPS → HTTP downgrade")
	}
	if !errors.Is(err, ErrHTTPSDowngrade) {
		t.Errorf("expected ErrHTTPSDowngrade, got: %v", err)
	}
}

// TestCheckRedirectUnitCrossOrigin tests the cross-origin redirect guard
// in isolation using crafted request objects. Both requests use HTTP so
// the HTTPS downgrade check does not interfere.
func TestCheckRedirectUnitCrossOrigin(t *testing.T) {
	c := New()

	original, _ := http.NewRequest(http.MethodGet, "http://pve.example.com:8006/api2/json/nodes", nil)
	redirect, _ := http.NewRequest(http.MethodGet, "http://evil.example.com/steal", nil)

	err := c.checkRedirect(redirect, []*http.Request{original})
	if err == nil {
		t.Fatal("expected error for cross-origin redirect")
	}
	if !errors.Is(err, ErrCrossOriginRedirect) {
		t.Errorf("expected ErrCrossOriginRedirect, got: %v", err)
	}
}

// TestCheckRedirectUnitAllowsSameHostSameScheme verifies that same-host,
// same-scheme redirects pass through the guard.
func TestCheckRedirectUnitAllowsSameHostSameScheme(t *testing.T) {
	c := New()

	original, _ := http.NewRequest(http.MethodGet, "https://pve.example.com:8006/start", nil)
	redirect, _ := http.NewRequest(http.MethodGet, "https://pve.example.com:8006/destination", nil)

	err := c.checkRedirect(redirect, []*http.Request{original})
	if err != nil {
		t.Fatalf("expected nil for same-host same-scheme redirect, got: %v", err)
	}
}

// TestCheckRedirectUnitEmptyVia verifies the guard handles an empty via slice.
func TestCheckRedirectUnitEmptyVia(t *testing.T) {
	c := New()

	redirect, _ := http.NewRequest(http.MethodGet, "http://evil.example.com/steal", nil)

	err := c.checkRedirect(redirect, []*http.Request{})
	if err != nil {
		t.Fatalf("expected nil for empty via slice, got: %v", err)
	}
}

// TestCheckRedirectUnitCaseInsensitiveHost verifies that host comparison is
// case-insensitive (per RFC 4343).
func TestCheckRedirectUnitCaseInsensitiveHost(t *testing.T) {
	c := New()

	original, _ := http.NewRequest(http.MethodGet, "https://PVE.Example.COM:8006/api", nil)
	redirect, _ := http.NewRequest(http.MethodGet, "https://pve.example.com:8006/other", nil)

	err := c.checkRedirect(redirect, []*http.Request{original})
	if err != nil {
		t.Fatalf("expected nil for case-insensitive same-host redirect, got: %v", err)
	}
}

// TestCheckRedirectUnitDifferentPortSameHost verifies that a redirect to the
// same hostname but a different port is treated as cross-origin (port is part
// of the host). Both requests use HTTP so the HTTPS downgrade check does not
// interfere.
func TestCheckRedirectUnitDifferentPortSameHost(t *testing.T) {
	c := New()

	original, _ := http.NewRequest(http.MethodGet, "http://pve.example.com:8006/api", nil)
	redirect, _ := http.NewRequest(http.MethodGet, "http://pve.example.com:80/api", nil)

	err := c.checkRedirect(redirect, []*http.Request{original})
	if err == nil {
		t.Fatal("expected error for redirect to same host different port")
	}
	if !errors.Is(err, ErrCrossOriginRedirect) {
		t.Errorf("expected ErrCrossOriginRedirect, got: %v", err)
	}
}

// TestCheckRedirectSentinelErrorsUnwrap verifies the sentinel errors can be
// identified with errors.Is when wrapped with fmt.Errorf %w, matching how
// checkRedirect wraps them.
func TestCheckRedirectSentinelErrorsUnwrap(t *testing.T) {
	err := fmt.Errorf("prefix: %w", ErrHTTPSDowngrade)
	if !errors.Is(err, ErrHTTPSDowngrade) {
		t.Errorf("errors.Is failed for wrapped ErrHTTPSDowngrade")
	}

	err = fmt.Errorf("prefix: %w", ErrCrossOriginRedirect)
	if !errors.Is(err, ErrCrossOriginRedirect) {
		t.Errorf("errors.Is failed for wrapped ErrCrossOriginRedirect")
	}
}

// --- Integration tests using real httptest servers ---

// TestCheckRedirectBlocksHTTPSDowngradeIntegration uses a TLS test server that
// redirects to the same host:port on the HTTP scheme. The Go HTTP client calls
// CheckRedirect before following, so the downgrade is blocked before any
// connection is attempted.
func TestCheckRedirectBlocksHTTPSDowngradeIntegration(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Redirect to same host:port but on HTTP scheme.
		// The TLS server's address is r.Host for the redirect, which
		// matches the original host exactly.
		httpRedirectURL := "http://" + r.Host + r.URL.Path + "?downgraded=1"
		http.Redirect(w, r, httpRedirectURL, http.StatusFound)
	}))
	defer srv.Close()

	// The client must use the TLS server's certificate pool.
	c := New(WithMaxRetries(0))
	tlsTransport := &http.Transport{
		TLSClientConfig: srv.Client().Transport.(*http.Transport).TLSClientConfig,
	}
	c.httpClient.Transport = tlsTransport

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/secure", nil)
	resp, err := c.Do(context.Background(), req)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("expected error for HTTPS → HTTP downgrade redirect, got nil")
	}
	if !errors.Is(err, ErrHTTPSDowngrade) {
		t.Errorf("expected ErrHTTPSDowngrade, got: %v", err)
	}
}

// TestCheckRedirectBlocksCrossOriginIntegration starts a test server that
// redirects to a completely different host. The guard must block the redirect
// before attempting a connection.
func TestCheckRedirectBlocksCrossOriginIntegration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://evil.example.com/steal", http.StatusFound)
	}))
	defer srv.Close()

	c := New(WithMaxRetries(0))
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/origin", nil)

	resp, err := c.Do(context.Background(), req)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("expected error for cross-origin redirect, got nil")
	}
	if !errors.Is(err, ErrCrossOriginRedirect) {
		t.Errorf("expected ErrCrossOriginRedirect, got: %v", err)
	}
}

// TestCheckRedirectAllowsSameHostSameSchemeIntegration verifies that a
// same-host redirect within the same HTTP server succeeds end-to-end.
func TestCheckRedirectAllowsSameHostSameSchemeIntegration(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, srv.URL+"/destination", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("arrived"))
	}))
	defer srv.Close()

	c := New(WithMaxRetries(0))
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/start", nil)

	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("expected successful same-host redirect, got: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

// TestCheckRedirectBlocksMultiHopCrossOriginIntegration verifies that a
// multi-hop redirect where the second hop crosses origin is blocked.
func TestCheckRedirectBlocksMultiHopCrossOriginIntegration(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/hop1" {
			// Same-host redirect — allowed.
			http.Redirect(w, r, srv.URL+"/hop2", http.StatusFound)
			return
		}
		// Second hop: cross-origin redirect.
		http.Redirect(w, r, "http://evil.example.com/steal", http.StatusFound)
	}))
	defer srv.Close()

	c := New(WithMaxRetries(0))
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/hop1", nil)

	resp, err := c.Do(context.Background(), req)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("expected error for multi-hop cross-origin redirect, got nil")
	}
	if !errors.Is(err, ErrCrossOriginRedirect) {
		t.Errorf("expected ErrCrossOriginRedirect, got: %v", err)
	}
}

// TestCheckRedirectBlocksHTTPSDowngradeViaDoMutation verifies the guard
// protects the DoMutation (non-retrying) path for HTTPS downgrade.
func TestCheckRedirectBlocksHTTPSDowngradeViaDoMutation(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpRedirectURL := "http://" + r.Host + "/downgraded"
		http.Redirect(w, r, httpRedirectURL, http.StatusFound)
	}))
	defer srv.Close()

	c := New()
	tlsTransport := &http.Transport{
		TLSClientConfig: srv.Client().Transport.(*http.Transport).TLSClientConfig,
	}
	c.httpClient.Transport = tlsTransport

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/secure", nil)

	_, err := c.DoMutation(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for HTTPS → HTTP downgrade via DoMutation, got nil")
	}
	if !errors.Is(err, ErrHTTPSDowngrade) {
		t.Errorf("expected ErrHTTPSDowngrade, got: %v", err)
	}
}

// TestCheckRedirectBlocksCrossOriginViaDoMutation verifies the guard protects
// the DoMutation (non-retrying) path for cross-origin redirects.
func TestCheckRedirectBlocksCrossOriginViaDoMutation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://evil.example.com/steal", http.StatusFound)
	}))
	defer srv.Close()

	c := New()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/origin", nil)

	_, err := c.DoMutation(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for cross-origin redirect via DoMutation, got nil")
	}
	if !errors.Is(err, ErrCrossOriginRedirect) {
		t.Errorf("expected ErrCrossOriginRedirect, got: %v", err)
	}
}

// --- Retry policy tests (CR-004) ---

// TestIsRetryableMethod verifies the method classification for each policy.
func TestIsRetryableMethod(t *testing.T) {
	tests := []struct {
		method string
		policy RetryPolicy
		want   bool
	}{
		{http.MethodGet, RetryIdempotent, true},
		{http.MethodHead, RetryIdempotent, true},
		{http.MethodPost, RetryIdempotent, false},
		{http.MethodPut, RetryIdempotent, false},
		{http.MethodDelete, RetryIdempotent, false},
		{http.MethodPatch, RetryIdempotent, false},

		{http.MethodGet, RetryNone, false},
		{http.MethodHead, RetryNone, false},
		{http.MethodPost, RetryNone, false},
		{http.MethodPut, RetryNone, false},
		{http.MethodDelete, RetryNone, false},

		{http.MethodGet, RetrySafe, true},
		{http.MethodHead, RetrySafe, true},
		{http.MethodPost, RetrySafe, false},
		{http.MethodPut, RetrySafe, true},
		{http.MethodDelete, RetrySafe, true},
		{http.MethodPatch, RetrySafe, false},
	}

	for _, tt := range tests {
		got := isRetryableMethod(tt.method, tt.policy)
		if got != tt.want {
			t.Errorf("isRetryableMethod(%q, %v) = %v, want %v",
				tt.method, tt.policy, got, tt.want)
		}
	}
}

// TestDoPostNeverRetries verifies that POST through Do() is never retried
// under the default (RetryIdempotent) policy.
func TestDoPostNeverRetries(t *testing.T) {
	var calls int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer s.Close()

	c := New(WithMaxRetries(5))
	req, _ := http.NewRequest(http.MethodPost, s.URL, nil)
	_, err := c.Do(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for POST to 500 with no retry")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("calls = %d, want 1 (POST must not retry)", calls)
	}
}

// TestDoPutNeverRetriesDefaultPolicy verifies that PUT through Do() is never
// retried under the default RetryIdempotent policy.
func TestDoPutNeverRetriesDefaultPolicy(t *testing.T) {
	var calls int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer s.Close()

	c := New(WithMaxRetries(5))
	req, _ := http.NewRequest(http.MethodPut, s.URL, nil)
	_, err := c.Do(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for PUT to 500 with no retry")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("calls = %d, want 1 (PUT must not retry under RetryIdempotent)", calls)
	}
}

// TestDoDeleteNeverRetriesDefaultPolicy verifies that DELETE through Do() is
// never retried under the default RetryIdempotent policy.
func TestDoDeleteNeverRetriesDefaultPolicy(t *testing.T) {
	var calls int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer s.Close()

	c := New(WithMaxRetries(5))
	req, _ := http.NewRequest(http.MethodDelete, s.URL, nil)
	_, err := c.Do(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for DELETE to 500 with no retry")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("calls = %d, want 1 (DELETE must not retry under RetryIdempotent)", calls)
	}
}

// TestDoRetryNoneDisablesRetriesForGET verifies that RetryNone disables
// retries even for GET.
func TestDoRetryNoneDisablesRetriesForGET(t *testing.T) {
	var calls int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer s.Close()

	c := New(WithMaxRetries(5), WithRetryPolicy(RetryNone))
	req, _ := http.NewRequest(http.MethodGet, s.URL, nil)
	_, err := c.Do(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for GET with RetryNone")
	}
	if !strings.Contains(err.Error(), "non-retryable method GET") {
		t.Errorf("error = %q, want 'non-retryable method GET'", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("calls = %d, want 1 (RetryNone must not retry)", calls)
	}
}

// TestDoRetrySafeRetriesPUT verifies that RetrySafe allows retries for PUT.
func TestDoRetrySafeRetriesPUT(t *testing.T) {
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

	c := New(WithMaxRetries(2), WithRetryPolicy(RetrySafe))
	req, _ := http.NewRequest(http.MethodPut, s.URL, nil)
	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_ = resp.Body.Close()
	if atomic.LoadInt32(&calls) != 3 {
		t.Errorf("calls = %d, want 3 (RetrySafe should retry PUT)", calls)
	}
}

// TestDoRetrySafeRetriesDELETE verifies that RetrySafe allows retries for DELETE.
func TestDoRetrySafeRetriesDELETE(t *testing.T) {
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

	c := New(WithMaxRetries(2), WithRetryPolicy(RetrySafe))
	req, _ := http.NewRequest(http.MethodDelete, s.URL, nil)
	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_ = resp.Body.Close()
	if atomic.LoadInt32(&calls) != 3 {
		t.Errorf("calls = %d, want 3 (RetrySafe should retry DELETE)", calls)
	}
}

// TestDoRetrySafeNeverRetriesPOST verifies that RetrySafe still never retries POST.
func TestDoRetrySafeNeverRetriesPOST(t *testing.T) {
	var calls int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer s.Close()

	c := New(WithMaxRetries(5), WithRetryPolicy(RetrySafe))
	req, _ := http.NewRequest(http.MethodPost, s.URL, nil)
	_, err := c.Do(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for POST with RetrySafe")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("calls = %d, want 1 (RetrySafe must not retry POST)", calls)
	}
}

// TestDoPostNonRetryableErrorContainsMethod verifies the error message for a
// failed POST includes the method name and wraps the underlying error.
func TestDoPostNonRetryableErrorContainsMethod(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer s.Close()

	c := New(WithMaxRetries(3))
	req, _ := http.NewRequest(http.MethodPost, s.URL, nil)
	_, err := c.Do(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "non-retryable method POST") {
		t.Errorf("error = %q, want 'non-retryable method POST'", err)
	}
	if !strings.Contains(err.Error(), "server error: 500") {
		t.Errorf("error = %q, want wrapped 'server error: 500'", err)
	}
}

// TestDoGetRetriesWithDefaultPolicy verifies the existing GET retry behavior
// is preserved under the default policy.
func TestDoGetRetriesWithDefaultPolicy(t *testing.T) {
	var calls int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := atomic.AddInt32(&calls, 1)
		if c < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
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
		t.Errorf("calls = %d, want 3 (GET should retry under default policy)", calls)
	}
}
