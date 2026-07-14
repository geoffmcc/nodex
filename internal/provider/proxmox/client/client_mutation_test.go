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

// --- VM Lifecycle Contract Tests ---

func TestVMStartPathAndMethod(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/nodes/pve1/qemu/100/status/start" {
			t.Errorf("path = %s, want /nodes/pve1/qemu/100/status/start", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:00003039:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	upid, err := c.VMStart(context.Background(), "pve1", 100)
	if err != nil {
		t.Fatalf("VMStart: %v", err)
	}
	if upid != "UPID:pve1:00003039:0023A45B:" {
		t.Errorf("upid = %q, want UPID:pve1:00003039:0023A45B:", upid)
	}
}

func TestVMStopPath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/qemu/100/status/stop" {
			t.Errorf("path = %s, want /nodes/pve1/qemu/100/status/stop", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:0000303A:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	_, err := c.VMStop(context.Background(), "pve1", 100)
	if err != nil {
		t.Fatalf("VMStop: %v", err)
	}
}

func TestVMPausePath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/qemu/100/status/pause" {
			t.Errorf("path = %s, want /nodes/pve1/qemu/100/status/pause", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:0000303B:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	_, err := c.VMPause(context.Background(), "pve1", 100)
	if err != nil {
		t.Fatalf("VMPause: %v", err)
	}
}

func TestVMResumePath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/qemu/100/status/resume" {
			t.Errorf("path = %s, want /nodes/pve1/qemu/100/status/resume", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:0000303C:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	_, err := c.VMResume(context.Background(), "pve1", 100)
	if err != nil {
		t.Fatalf("VMResume: %v", err)
	}
}

func TestVMResetPath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/qemu/100/status/reset" {
			t.Errorf("path = %s, want /nodes/pve1/qemu/100/status/reset", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:0000303D:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	_, err := c.VMReset(context.Background(), "pve1", 100)
	if err != nil {
		t.Fatalf("VMReset: %v", err)
	}
}

func TestVMRebootPath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/qemu/100/status/reboot" {
			t.Errorf("path = %s, want /nodes/pve1/qemu/100/status/reboot", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:0000303E:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	_, err := c.VMReboot(context.Background(), "pve1", 100)
	if err != nil {
		t.Fatalf("VMReboot: %v", err)
	}
}

func TestVMSuspendPath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/qemu/100/status/suspend" {
			t.Errorf("path = %s, want /nodes/pve1/qemu/100/status/suspend", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:0000303F:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	_, err := c.VMSuspend(context.Background(), "pve1", 100)
	if err != nil {
		t.Fatalf("VMSuspend: %v", err)
	}
}

func TestVMUnpausePath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/qemu/100/status/unpause" {
			t.Errorf("path = %s, want /nodes/pve1/qemu/100/status/unpause", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:00003040:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	_, err := c.VMUnpause(context.Background(), "pve1", 100)
	if err != nil {
		t.Fatalf("VMUnpause: %v", err)
	}
}

func TestVMShutdownSendsTimeout(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/qemu/100/status/shutdown" {
			t.Errorf("path = %s, want /nodes/pve1/qemu/100/status/shutdown", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if got := r.FormValue("timeout"); got != "60" {
			t.Errorf("timeout = %q, want 60", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:00003041:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	_, err := c.VMShutdown(context.Background(), "pve1", 100, 60)
	if err != nil {
		t.Fatalf("VMShutdown: %v", err)
	}
}

// --- Container Lifecycle Contract Tests ---

func TestCTStartPath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/lxc/200/status/start" {
			t.Errorf("path = %s, want /nodes/pve1/lxc/200/status/start", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:00003042:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	_, err := c.CTStart(context.Background(), "pve1", 200)
	if err != nil {
		t.Fatalf("CTStart: %v", err)
	}
}

func TestCTStopPath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/lxc/200/status/stop" {
			t.Errorf("path = %s, want /nodes/pve1/lxc/200/status/stop", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:00003043:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	_, err := c.CTStop(context.Background(), "pve1", 200)
	if err != nil {
		t.Fatalf("CTStop: %v", err)
	}
}

func TestCTShutdownSendsTimeout(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/lxc/200/status/shutdown" {
			t.Errorf("path = %s, want /nodes/pve1/lxc/200/status/shutdown", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if got := r.FormValue("timeout"); got != "60" {
			t.Errorf("timeout = %q, want 60", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:00003044:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	_, err := c.CTShutdown(context.Background(), "pve1", 200, 60)
	if err != nil {
		t.Fatalf("CTShutdown: %v", err)
	}
}

func TestCTRebootPath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/lxc/200/status/reboot" {
			t.Errorf("path = %s, want /nodes/pve1/lxc/200/status/reboot", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:00003045:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	_, err := c.CTReboot(context.Background(), "pve1", 200)
	if err != nil {
		t.Fatalf("CTReboot: %v", err)
	}
}

func TestCTSuspendPath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/lxc/200/status/suspend" {
			t.Errorf("path = %s, want /nodes/pve1/lxc/200/status/suspend", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:00003046:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	_, err := c.CTSuspend(context.Background(), "pve1", 200)
	if err != nil {
		t.Fatalf("CTSuspend: %v", err)
	}
}

func TestCTResumePath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/lxc/200/status/resume" {
			t.Errorf("path = %s, want /nodes/pve1/lxc/200/status/resume", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:00003047:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	_, err := c.CTResume(context.Background(), "pve1", 200)
	if err != nil {
		t.Fatalf("CTResume: %v", err)
	}
}

// --- Input Validation Tests ---

func TestVMStartRejectsEmptyNode(t *testing.T) {
	c := &Client{baseURL: "https://example.com", client: httpclient.New()}
	_, err := c.VMStart(context.Background(), "", 100)
	if err == nil || !strings.Contains(err.Error(), "node name is required") {
		t.Fatalf("VMStart('') error = %v, want node name required", err)
	}
}

func TestVMStartRejectsInvalidVMID(t *testing.T) {
	c := &Client{baseURL: "https://example.com", client: httpclient.New()}
	_, err := c.VMStart(context.Background(), "pve1", 0)
	if err == nil || !strings.Contains(err.Error(), "VMID is required") {
		t.Fatalf("VMStart(0) error = %v, want VMID required", err)
	}
}

func TestCTStartRejectsEmptyNode(t *testing.T) {
	c := &Client{baseURL: "https://example.com", client: httpclient.New()}
	_, err := c.CTStart(context.Background(), "", 200)
	if err == nil || !strings.Contains(err.Error(), "node name is required") {
		t.Fatalf("CTStart('') error = %v, want node name required", err)
	}
}

func TestCTStartRejectsInvalidVMID(t *testing.T) {
	c := &Client{baseURL: "https://example.com", client: httpclient.New()}
	_, err := c.CTStart(context.Background(), "pve1", -1)
	if err == nil || !strings.Contains(err.Error(), "VMID is required") {
		t.Fatalf("CTStart(-1) error = %v, want VMID required", err)
	}
}

func TestVMShutdownSucceedsWithoutTimeout(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.FormValue("timeout") != "" {
			t.Errorf("timeout present when not set: %q", r.FormValue("timeout"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:00003041:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	_, err := c.VMShutdown(context.Background(), "pve1", 100, 0)
	if err != nil {
		t.Fatalf("VMShutdown(timeout=0): %v", err)
	}
}

// --- Phase 3: Config, Snapshot, Delete, Template Contract Tests ---

func TestVMConfigUpdatePathAndBody(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/nodes/pve1/qemu/100/config" {
			t.Errorf("path = %s, want /nodes/pve1/qemu/100/config", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if got := r.FormValue("memory"); got != "4096" {
			t.Errorf("memory = %q, want 4096", got)
		}
		if got := r.FormValue("cores"); got != "4" {
			t.Errorf("cores = %q, want 4", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:00003048:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	params := url.Values{}
	params.Set("memory", "4096")
	params.Set("cores", "4")
	upid, err := c.VMConfigUpdate(context.Background(), "pve1", 100, params)
	if err != nil {
		t.Fatalf("VMConfigUpdate: %v", err)
	}
	if upid != "UPID:pve1:00003048:0023A45B:" {
		t.Errorf("upid = %q, want UPID:pve1:00003048:0023A45B:", upid)
	}
}

func TestCTConfigUpdatePath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/lxc/200/config" {
			t.Errorf("path = %s, want /nodes/pve1/lxc/200/config", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:00003049:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	_, err := c.CTConfigUpdate(context.Background(), "pve1", 200, url.Values{})
	if err != nil {
		t.Fatalf("CTConfigUpdate: %v", err)
	}
}

func TestVMDeletePath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		if r.URL.Path != "/nodes/pve1/qemu/100" {
			t.Errorf("path = %s, want /nodes/pve1/qemu/100", r.URL.Path)
		}
		// purge=1 is sent as a query parameter since DELETE bodies are not
		// reliably forwarded by all proxies.
		if got := r.URL.Query().Get("purge"); got != "1" {
			t.Errorf("purge query param = %q, want \"1\"", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:0000304A:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	upid, err := c.VMDelete(context.Background(), "pve1", 100)
	if err != nil {
		t.Fatalf("VMDelete: %v", err)
	}
	if upid != "UPID:pve1:0000304A:0023A45B:" {
		t.Errorf("upid = %q, want UPID:pve1:0000304A:0023A45B:", upid)
	}
}

func TestCTDeletePath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/lxc/200" {
			t.Errorf("path = %s, want /nodes/pve1/lxc/200", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:0000304B:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	_, err := c.CTDelete(context.Background(), "pve1", 200)
	if err != nil {
		t.Fatalf("CTDelete: %v", err)
	}
}

func TestVMSnapshotCreatePathAndBody(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/nodes/pve1/qemu/100/snapshot" {
			t.Errorf("path = %s, want /nodes/pve1/qemu/100/snapshot", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if got := r.FormValue("snapname"); got != "pre-upgrade" {
			t.Errorf("snapname = %q, want pre-upgrade", got)
		}
		if got := r.FormValue("description"); got != "Before upgrade" {
			t.Errorf("description = %q, want 'Before upgrade'", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:0000304C:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	upid, err := c.VMSnapshotCreate(context.Background(), "pve1", 100, "pre-upgrade", "Before upgrade")
	if err != nil {
		t.Fatalf("VMSnapshotCreate: %v", err)
	}
	if upid != "UPID:pve1:0000304C:0023A45B:" {
		t.Errorf("upid = %q, want UPID:pve1:0000304C:0023A45B:", upid)
	}
}

func TestVMSnapshotCreateWithoutDescription(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if got := r.FormValue("description"); got != "" {
			t.Errorf("description = %q, want empty", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:0000304D:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	_, err := c.VMSnapshotCreate(context.Background(), "pve1", 100, "snap1", "")
	if err != nil {
		t.Fatalf("VMSnapshotCreate: %v", err)
	}
}

func TestVMSnapshotDeletePath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		if r.URL.Path != "/nodes/pve1/qemu/100/snapshot/pre-upgrade" {
			t.Errorf("path = %s, want /nodes/pve1/qemu/100/snapshot/pre-upgrade", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:0000304E:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	upid, err := c.VMSnapshotDelete(context.Background(), "pve1", 100, "pre-upgrade")
	if err != nil {
		t.Fatalf("VMSnapshotDelete: %v", err)
	}
	if upid != "UPID:pve1:0000304E:0023A45B:" {
		t.Errorf("upid = %q, want UPID:pve1:0000304E:0023A45B:", upid)
	}
}

func TestVMSnapshotRollbackPath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/nodes/pve1/qemu/100/snapshot/pre-upgrade/rollback" {
			t.Errorf("path = %s, want /nodes/pve1/qemu/100/snapshot/pre-upgrade/rollback", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:0000304F:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	upid, err := c.VMSnapshotRollback(context.Background(), "pve1", 100, "pre-upgrade")
	if err != nil {
		t.Fatalf("VMSnapshotRollback: %v", err)
	}
	if upid != "UPID:pve1:0000304F:0023A45B:" {
		t.Errorf("upid = %q, want UPID:pve1:0000304F:0023A45B:", upid)
	}
}

func TestCTSnapshotCreatePath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/lxc/200/snapshot" {
			t.Errorf("path = %s, want /nodes/pve1/lxc/200/snapshot", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:00003050:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	_, err := c.CTSnapshotCreate(context.Background(), "pve1", 200, "clean", "")
	if err != nil {
		t.Fatalf("CTSnapshotCreate: %v", err)
	}
}

func TestCTSnapshotDeletePath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/lxc/200/snapshot/clean" {
			t.Errorf("path = %s, want /nodes/pve1/lxc/200/snapshot/clean", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:00003051:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	_, err := c.CTSnapshotDelete(context.Background(), "pve1", 200, "clean")
	if err != nil {
		t.Fatalf("CTSnapshotDelete: %v", err)
	}
}

func TestCTSnapshotRollbackPath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/lxc/200/snapshot/clean/rollback" {
			t.Errorf("path = %s, want /nodes/pve1/lxc/200/snapshot/clean/rollback", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:00003052:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	_, err := c.CTSnapshotRollback(context.Background(), "pve1", 200, "clean")
	if err != nil {
		t.Fatalf("CTSnapshotRollback: %v", err)
	}
}

func TestVMCloudInitPath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/nodes/pve1/qemu/100/cloudinit" {
			t.Errorf("path = %s, want /nodes/pve1/qemu/100/cloudinit", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:00003053:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	upid, err := c.VMCloudInit(context.Background(), "pve1", 100)
	if err != nil {
		t.Fatalf("VMCloudInit: %v", err)
	}
	if upid != "UPID:pve1:00003053:0023A45B:" {
		t.Errorf("upid = %q, want UPID:pve1:00003053:0023A45B:", upid)
	}
}

func TestVMTemplatePath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/nodes/pve1/qemu/100/template" {
			t.Errorf("path = %s, want /nodes/pve1/qemu/100/template", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:00003054:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	upid, err := c.VMTemplate(context.Background(), "pve1", 100)
	if err != nil {
		t.Fatalf("VMTemplate: %v", err)
	}
	if upid != "UPID:pve1:00003054:0023A45B:" {
		t.Errorf("upid = %q, want UPID:pve1:00003054:0023A45B:", upid)
	}
}

func TestCTTemplatePath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/lxc/200/template" {
			t.Errorf("path = %s, want /nodes/pve1/lxc/200/template", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:00003055:0023A45B:"}`))
	}))
	defer s.Close()

	c := &Client{baseURL: s.URL, client: httpclient.New()}
	_, err := c.CTTemplate(context.Background(), "pve1", 200)
	if err != nil {
		t.Fatalf("CTTemplate: %v", err)
	}
}

// --- Phase 3: Input Validation Tests ---

func TestVMConfigUpdateRejectsEmptyNode(t *testing.T) {
	c := &Client{baseURL: "https://example.com", client: httpclient.New()}
	_, err := c.VMConfigUpdate(context.Background(), "", 100, url.Values{})
	if err == nil || !strings.Contains(err.Error(), "node name is required") {
		t.Fatalf("VMConfigUpdate('') error = %v, want node name required", err)
	}
}

func TestVMConfigUpdateRejectsInvalidVMID(t *testing.T) {
	c := &Client{baseURL: "https://example.com", client: httpclient.New()}
	_, err := c.VMConfigUpdate(context.Background(), "pve1", 0, url.Values{})
	if err == nil || !strings.Contains(err.Error(), "VMID is required") {
		t.Fatalf("VMConfigUpdate(0) error = %v, want VMID required", err)
	}
}

func TestVMSnapshotCreateRejectsEmptyName(t *testing.T) {
	c := &Client{baseURL: "https://example.com", client: httpclient.New()}
	_, err := c.VMSnapshotCreate(context.Background(), "pve1", 100, "", "")
	if err == nil || !strings.Contains(err.Error(), "snapshot name is required") {
		t.Fatalf("VMSnapshotCreate('') error = %v, want snapshot name required", err)
	}
}

func TestVMSnapshotDeleteRejectsEmptyName(t *testing.T) {
	c := &Client{baseURL: "https://example.com", client: httpclient.New()}
	_, err := c.VMSnapshotDelete(context.Background(), "pve1", 100, "")
	if err == nil || !strings.Contains(err.Error(), "snapshot name is required") {
		t.Fatalf("VMSnapshotDelete('') error = %v, want snapshot name required", err)
	}
}
