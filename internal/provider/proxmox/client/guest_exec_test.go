package client

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

func TestVMGuestExecPostsStructuredArgvAndStatusDecodesOutput(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path == "/nodes/node-a/qemu/321/agent/exec" {
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if got, want := r.Form["command"], []string{"/usr/bin/apt-get", "install", "--", "curl"}; !reflect.DeepEqual(got, want) {
				t.Errorf("command args = %#v, want %#v", got, want)
			}
			if r.Form.Get("capture-output") != "1" {
				t.Errorf("capture-output = %q", r.Form.Get("capture-output"))
			}
			_, _ = w.Write([]byte(`{"data":{"pid":42}}`))
			return
		}
		if r.URL.Path == "/nodes/node-a/qemu/321/agent/exec-status" {
			if r.URL.Query().Get("pid") != "42" {
				t.Errorf("status pid = %q", r.URL.Query().Get("pid"))
			}
			_, _ = w.Write([]byte(`{"data":{"exited":1,"exitcode":0,"out-data":"` + base64.StdEncoding.EncodeToString([]byte("done")) + `","err-data":""}}`))
			return
		}
		t.Errorf("unexpected request: %s", r.URL.String())
		http.NotFound(w, r)
	}))
	defer server.Close()
	c := &Client{baseURL: server.URL, client: httpclient.New()}
	pid, err := c.VMGuestExec(context.Background(), "node-a", 321, []string{"/usr/bin/apt-get", "install", "--", "curl"})
	if err != nil || pid != 42 {
		t.Fatalf("VMGuestExec = %d, %v", pid, err)
	}
	status, err := c.VMGuestExecStatus(context.Background(), "node-a", 321, pid)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Exited || status.ExitCode == nil || *status.ExitCode != 0 || status.Stdout != "done" {
		t.Fatalf("status = %+v", status)
	}
	if requests != 2 {
		t.Fatalf("request count = %d, want 2", requests)
	}
}

func TestVMGuestExecValidatesArgumentsBeforeRequest(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	c := &Client{baseURL: server.URL, client: httpclient.New()}
	for _, argv := range [][]string{nil, {"/usr/bin/true", "bad\x00arg"}} {
		if _, err := c.VMGuestExec(context.Background(), "node-a", 321, argv); err == nil {
			t.Fatalf("VMGuestExec(%q) succeeded", argv)
		}
	}
	if requests != 0 {
		t.Fatalf("invalid requests sent = %d", requests)
	}
}

func TestVMGuestExecDoesNotRetryFailedSubmission(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "temporary API failure", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	c := &Client{baseURL: server.URL, client: httpclient.New()}
	if _, err := c.VMGuestExec(context.Background(), "node-a", 321, []string{"/usr/bin/true"}); err == nil {
		t.Fatal("VMGuestExec unexpectedly succeeded")
	}
	if requests != 1 {
		t.Fatalf("exec submission requests = %d, want one", requests)
	}
}

func TestVMGuestExecRejectsInvalidProcessIdentifier(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"data":{"pid":0}}`))
	}))
	defer server.Close()
	c := &Client{baseURL: server.URL, client: httpclient.New()}
	if _, err := c.VMGuestExec(context.Background(), "node-a", 321, []string{"/usr/bin/true"}); err == nil {
		t.Fatal("invalid process identifier was accepted")
	}
	if requests != 1 {
		t.Fatalf("exec submission requests = %d, want one", requests)
	}
}

func TestVMGuestExecStatusRejectsMalformedStateAndOutput(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		want string
	}{
		{name: "invalid exited state", body: `{"data":{"exited":2,"out-data":"","err-data":""}}`, want: "invalid process state"},
		{name: "malformed base64", body: `{"data":{"exited":1,"exitcode":0,"out-data":"%%%","err-data":""}}`, want: "malformed encoded output"},
		{name: "missing exit code", body: `{"data":{"exited":1,"out-data":"","err-data":""}}`, want: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(tt.body)) }))
			defer server.Close()
			c := &Client{baseURL: server.URL, client: httpclient.New()}
			status, err := c.VMGuestExecStatus(context.Background(), "node-a", 321, 42)
			if tt.want != "" {
				if err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("status/error = %+v / %v, want error containing %q", status, err, tt.want)
				}
				return
			}
			if err != nil || status == nil || status.ExitCode != nil {
				t.Fatalf("missing exit code result = %+v, %v; command layer must classify it unknown", status, err)
			}
		})
	}
}

func TestVMGuestExecStatusBoundsCapturedOutput(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString(make([]byte, maxVMGuestExecOutputBytes+1))
	body := `{"data":{"exited":1,"exitcode":0,"out-data":"` + encoded + `","err-data":""}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
	defer server.Close()
	c := &Client{baseURL: server.URL, client: httpclient.New()}
	if _, err := c.VMGuestExecStatus(context.Background(), "node-a", 321, 42); err == nil || !strings.Contains(err.Error(), "1 MiB limit") {
		t.Fatalf("oversized output error = %v", err)
	}
}
