package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

// --- POST tests ---

func TestPostMethodAndPath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/nodes/pve1/qemu/100/status/start" {
			t.Errorf("path = %s, want /nodes/pve1/qemu/100/status/start", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:00000A1B:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	var result map[string]interface{}
	body := url.Values{}
	body.Set("timeout", "30")
	err := c.post(context.Background(), "/nodes/pve1/qemu/100/status/start", body, &result)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if result["data"] != "UPID:pve1:00000A1B:0023A45B:" {
		t.Errorf("data = %v, want UPID", result["data"])
	}
}

func TestPostContentTypeAndBody(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if !strings.Contains(ct, "application/x-www-form-urlencoded") {
			t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", ct)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:00000A1B:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	var result map[string]interface{}
	body := url.Values{}
	body.Set("timeout", "30")
	if err := c.post(context.Background(), "/test", body, &result); err != nil {
		t.Fatalf("post: %v", err)
	}
}

func TestPostAuthorizationHeader(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "PVEAPIToken=user@pam!tok=test-secret" {
			t.Errorf("Authorization = %q, want PVEAPIToken=user@pam!tok=test-secret", auth)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"ok"}`))
	}))
	defer s.Close()

	c := &Client{
		baseURL: s.URL,
		client:  httpclient.New(),
		token:   "user@pam!tok=test-secret",
	}
	var result map[string]interface{}
	if err := c.post(context.Background(), "/test", url.Values{}, &result); err != nil {
		t.Fatalf("post: %v", err)
	}
}

func TestPostDecodesResponse(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"taskid":"UPID:pve1:00000A1B:0023A45B:"}}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	var result map[string]interface{}
	if err := c.post(context.Background(), "/test", url.Values{}, &result); err != nil {
		t.Fatalf("post: %v", err)
	}
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data is not a map: %T", result["data"])
	}
	if data["taskid"] != "UPID:pve1:00000A1B:0023A45B:" {
		t.Errorf("taskid = %v, want UPID", data["taskid"])
	}
}

func TestPostHandlesErrorResponse(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"permission denied"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	var result any
	err := c.post(context.Background(), "/test", url.Values{}, &result)
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	if !strings.Contains(err.Error(), "API error 403") {
		t.Errorf("error = %q, want API error 403", err)
	}
}

func TestPostAccepts201Created(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":"created"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	var result map[string]interface{}
	if err := c.post(context.Background(), "/test", url.Values{}, &result); err != nil {
		t.Fatalf("post 201: %v", err)
	}
	if result["data"] != "created" {
		t.Errorf("data = %v, want created", result["data"])
	}
}

func TestPostAccepts202Accepted(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"data":"accepted"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	var result map[string]interface{}
	if err := c.post(context.Background(), "/test", url.Values{}, &result); err != nil {
		t.Fatalf("post 202: %v", err)
	}
	if result["data"] != "accepted" {
		t.Errorf("data = %v, want accepted", result["data"])
	}
}

// --- PUT tests ---

func TestPutMethodAndPath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		if r.URL.Path != "/nodes/pve1/qemu/100/config" {
			t.Errorf("path = %s, want /nodes/pve1/qemu/100/config", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":null}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	body := url.Values{}
	body.Set("memory", "4096")
	var result map[string]interface{}
	if err := c.put(context.Background(), "/nodes/pve1/qemu/100/config", body, &result); err != nil {
		t.Fatalf("put: %v", err)
	}
}

func TestPutContentType(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if !strings.Contains(ct, "application/x-www-form-urlencoded") {
			t.Errorf("Content-Type = %q", ct)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":null}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	var result map[string]interface{}
	if err := c.put(context.Background(), "/test", url.Values{}, &result); err != nil {
		t.Fatalf("put: %v", err)
	}
}

func TestPutHandlesErrorResponse(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"invalid config"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	var result any
	err := c.put(context.Background(), "/test", url.Values{}, &result)
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
}

// --- DELETE tests ---

func TestDelMethodAndPath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		if r.URL.Path != "/nodes/pve1/qemu/100" {
			t.Errorf("path = %s, want /nodes/pve1/qemu/100", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:00000A1B:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	var result map[string]interface{}
	if err := c.del(context.Background(), "/nodes/pve1/qemu/100", &result); err != nil {
		t.Fatalf("del: %v", err)
	}
	if result["data"] != "UPID:pve1:00000A1B:0023A45B:" {
		t.Errorf("data = %v, want UPID", result["data"])
	}
}

func TestDelNoContentType(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if ct != "" {
			t.Errorf("DELETE should not have Content-Type, got %q", ct)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":null}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	var result any
	if err := c.del(context.Background(), "/test", &result); err != nil {
		t.Fatalf("del: %v", err)
	}
}

func TestDelHandlesErrorResponse(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	var result any
	err := c.del(context.Background(), "/test", &result)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

// --- No-retry verification ---

func TestPostNeverRetries(t *testing.T) {
	var calls int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New(httpclient.WithMaxRetries(5))}
	var result any
	err := c.post(context.Background(), "/test", url.Values{}, &result)
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("calls = %d, want 1 (no retry for POST)", calls)
	}
}

func TestPutNeverRetries(t *testing.T) {
	var calls int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New(httpclient.WithMaxRetries(5))}
	var result any
	err := c.put(context.Background(), "/test", url.Values{}, &result)
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("calls = %d, want 1 (no retry for PUT)", calls)
	}
}

func TestDelNeverRetries(t *testing.T) {
	var calls int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New(httpclient.WithMaxRetries(5))}
	var result any
	err := c.del(context.Background(), "/test", &result)
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("calls = %d, want 1 (no retry for DELETE)", calls)
	}
}

// --- Error redaction ---

func TestPostRedactsTokenInErrorBody(t *testing.T) {
	secret := "PVEAPIToken=user@pam!tok=supersecret" // #nosec G101
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, secret)
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	var result any
	err := c.post(context.Background(), "/test", url.Values{}, &result)
	if err == nil {
		t.Fatal("expected API error")
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "\x1b") {
		t.Fatalf("error leaked unsafe body: %q", err.Error())
	}
}

// --- Trailing data ---

func TestPostRejectsTrailingJSON(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"data":"ok"} {}`)
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	var result map[string]interface{}
	err := c.post(context.Background(), "/test", url.Values{}, &result)
	if err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("post error = %v, want trailing data error", err)
	}
}
