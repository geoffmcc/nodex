package httpclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// pveErrorBody renders the shape Proxmox uses for API errors.
func pveErrorBody(message string) string {
	return `{"data":null,"message":"` + message + `"}`
}

// TestDoNotImplementedIsTerminal pins §13. 501 means the endpoint does not
// exist in this build, so no retry can change the outcome. It was previously
// retried three times with 600ms of backoff, making a permanent conclusion
// take ~23x longer.
func TestDoNotImplementedIsTerminal(t *testing.T) {
	var calls int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNotImplemented)
		_, _ = w.Write([]byte(pveErrorBody("Method 'GET /cluster/sdn/subnets' not implemented")))
	}))
	defer s.Close()

	c := New(WithMaxRetries(3))
	req, _ := http.NewRequest(http.MethodGet, s.URL, nil)
	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("501 must be returned to the caller, not turned into an error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1 (501 is terminal)", got)
	}
	// The body must still be readable: it carries the useful message.
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "not implemented") {
		t.Errorf("body not restored for the caller: %q", body)
	}
}

// TestDoDeterministic500IsTerminal pins §5. These bodies describe permanent
// conditions, so the retry budget is wasted latency. "binary not installed"
// was the reported case: the user was told "max retries exceeded" instead of
// "Ceph is not installed on this node".
func TestDoDeterministic500IsTerminal(t *testing.T) {
	cases := []string{
		"binary not installed: /usr/bin/ceph-mon",
		"does not exist: /usr/sbin/qm",
		"Configuration file does not exist",
		"VM 999999 does not exist",
		"no such file or directory",
		"storage 'local-lvm' not found",
		"Permission check failed (/var/lib/vz, uid=0)",
		"Parameter verification failed. vmid: value does not look like an integer",
		"service is not running",
		"VM 9200 is running - destroy failed",
	}
	for _, message := range cases {
		t.Run(message[:min(len(message), 24)], func(t *testing.T) {
			var calls int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&calls, 1)
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(pveErrorBody(message)))
			}))
			defer s.Close()

			c := New(WithMaxRetries(3))
			req, _ := http.NewRequest(http.MethodGet, s.URL, nil)
			resp, err := c.Do(context.Background(), req)
			if err != nil {
				t.Fatalf("deterministic 500 must be returned to the caller, not an error: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()
			if got := atomic.LoadInt32(&calls); got != 1 {
				t.Errorf("calls = %d, want 1 (deterministic 500 is terminal)", got)
			}
			body, _ := io.ReadAll(resp.Body)
			if !strings.Contains(string(body), "message") {
				t.Errorf("body not restored for the caller: %q", body)
			}
		})
	}
}

// TestDoUnrecognised500Retries pins the other half of the rule: a 500 whose
// body is unrecognised may be transient, so it is retried.
func TestDoUnrecognised500Retries(t *testing.T) {
	var calls int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(pveErrorBody("internal server hiccup")))
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
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("calls = %d, want 3 (unrecognised 500 must retry)", got)
	}
}

// TestDoEmptyBody500Retries pins that a bodyless 500 keeps its old behaviour.
func TestDoEmptyBody500Retries(t *testing.T) {
	var calls int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
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
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("calls = %d, want 3", got)
	}
}

// TestDoGatewayStatusesRetry pins that 502/503/504 remain retryable, including
// when their body would otherwise look deterministic — the rule is
// status-first for these.
func TestDoGatewayStatusesRetry(t *testing.T) {
	for _, status := range []int{http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if atomic.AddInt32(&calls, 1) < 3 {
					w.WriteHeader(status)
					_, _ = w.Write([]byte(pveErrorBody("backend not found")))
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
			if got := atomic.LoadInt32(&calls); got != 3 {
				t.Errorf("calls = %d, want 3 (gateway statuses stay retryable)", got)
			}
		})
	}
}

// TestDoOtherServerErrorsTerminal pins that 5xx outside the retryable set are
// not retried either, so the "no more statuses are retried" claim holds.
func TestDoOtherServerErrorsTerminal(t *testing.T) {
	for _, status := range []int{501, 505, 507, 511} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&calls, 1)
				w.WriteHeader(status)
				_, _ = w.Write([]byte(pveErrorBody("nope")))
			}))
			defer s.Close()

			c := New(WithMaxRetries(3))
			req, _ := http.NewRequest(http.MethodGet, s.URL, nil)
			resp, err := c.Do(context.Background(), req)
			if err != nil {
				t.Fatalf("status %d must be returned to the caller: %v", status, err)
			}
			_ = resp.Body.Close()
			if got := atomic.LoadInt32(&calls); got != 1 {
				t.Errorf("calls = %d, want 1 (status %d is terminal)", got, status)
			}
		})
	}
}

// TestDoExhaustedRetriesSurfacesPVEMessage pins §5's second half: when retries
// really are exhausted, the useful part is the decoded PVE message, not the
// wrapper and not the raw JSON.
func TestDoExhaustedRetriesSurfacesPVEMessage(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(pveErrorBody("the backend is starting up, try again shortly")))
	}))
	defer s.Close()

	c := New(WithMaxRetries(1))
	req, _ := http.NewRequest(http.MethodGet, s.URL, nil)
	_, err := c.Do(context.Background(), req)
	if err == nil {
		t.Fatal("expected error after max retries")
	}
	msg := err.Error()
	if !strings.Contains(msg, "the backend is starting up, try again shortly") {
		t.Errorf("error = %q, want the decoded PVE message", msg)
	}
	if strings.Contains(msg, `"data":null`) {
		t.Errorf("error = %q, want the message rather than the raw JSON body", msg)
	}
	if !strings.Contains(msg, "server error: 503") {
		t.Errorf("error = %q, want the status retained", msg)
	}
}

// TestDoTerminalPathAvoidsBackoff is a timing-free structural check that the
// reported ~23x regression is gone: a terminal status costs exactly one
// request and no backoff delay is ever selected.
//
// The previous version of this test measured wall-clock elapsed time against a
// 300ms budget. That is not a property of the client at all — it is a property
// of how loaded the machine running the test is — so it passed on Linux
// runners and failed intermittently on slower macOS and Windows runners. The
// retry delays are 400ms and 1s here, so counting server hits distinguishes
// "retried" from "did not retry" exactly and instantly.
func TestDoTerminalPathAvoidsBackoff(t *testing.T) {
	var calls int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNotImplemented)
		_, _ = w.Write([]byte(pveErrorBody("not implemented")))
	}))
	defer s.Close()

	c := New(WithMaxRetries(2), WithRetryDelays(400*time.Millisecond, time.Second))
	req, _ := http.NewRequest(http.MethodGet, s.URL, nil)
	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_ = resp.Body.Close()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("terminal 501 hit the server %d times, want 1: a retried terminal "+
			"status sleeps the full 400ms+1s backoff for no possible benefit", got)
	}
}
