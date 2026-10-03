package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

// The per-guest status endpoint is the only read that can confirm what state one
// specific guest is in, so its path and status handling are pinned here.

func TestGetGuestStatusUsesPerGuestEndpoint(t *testing.T) {
	tests := []struct {
		guestType string
		vmid      int
		wantPath  string
	}{
		{"qemu", 101, "/nodes/pve1/qemu/101/status/current"},
		{"lxc", 201, "/nodes/pve1/lxc/201/status/current"},
	}
	for _, tt := range tests {
		t.Run(tt.guestType, func(t *testing.T) {
			var gotPath string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				if r.Method != http.MethodGet {
					t.Errorf("method = %s, want GET", r.Method)
				}
				_, _ = w.Write([]byte(`{"data":{"status":"running","name":"web","qmpstatus":"running"}}`))
			}))
			defer server.Close()

			c := &Client{baseURL: server.URL, client: httpclient.New()}
			status, err := c.GetGuestStatus(context.Background(), "pve1", tt.guestType, tt.vmid)
			if err != nil {
				t.Fatalf("GetGuestStatus error = %v", err)
			}
			if gotPath != tt.wantPath {
				t.Fatalf("path = %q, want %q", gotPath, tt.wantPath)
			}
			if status.Status != "running" {
				t.Fatalf("status = %q, want running", status.Status)
			}
			if status.Name != "web" {
				t.Fatalf("name = %q, want web", status.Name)
			}
			// The response omits vmid for the container case, so it must be
			// filled from the request rather than invented.
			if status.VMID != tt.vmid {
				t.Fatalf("vmid = %d, want %d", status.VMID, tt.vmid)
			}
		})
	}
}

// A response without a status proves nothing, so it must not be presented as
// an empty or absent state.
func TestGetGuestStatusRejectsResponseWithoutStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"vmid":101}}`))
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, client: httpclient.New()}
	if _, err := c.GetGuestStatus(context.Background(), "pve1", "qemu", 101); err == nil {
		t.Fatal("expected an error when the response carries no status field")
	}
}

// The VMID is taken from the request when the response omits it, so callers can
// still identify which guest was read.
func TestGetGuestStatusFillsVMIDFromRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"status":"stopped"}}`))
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, client: httpclient.New()}
	status, err := c.GetGuestStatus(context.Background(), "pve1", "lxc", 205)
	if err != nil {
		t.Fatalf("GetGuestStatus error = %v", err)
	}
	if status.VMID != 205 || status.Status != "stopped" {
		t.Fatalf("status = %+v, want vmid 205 stopped", status)
	}
}

func TestGetGuestStatusRejectsInvalidArguments(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called = true }))
	defer server.Close()
	c := &Client{baseURL: server.URL, client: httpclient.New()}

	if _, err := c.GetGuestStatus(context.Background(), "", "qemu", 101); err == nil {
		t.Fatal("expected an error for an empty node name")
	}
	if _, err := c.GetGuestStatus(context.Background(), "pve1", "qemu", 0); err == nil {
		t.Fatal("expected an error for a non-positive VMID")
	}
	if _, err := c.GetGuestStatus(context.Background(), "pve1", "storage", 101); err == nil ||
		!strings.Contains(err.Error(), "unsupported guest type") {
		t.Fatalf("expected an unsupported guest type error, got %v", err)
	}
	if called {
		t.Fatal("invalid arguments reached the network")
	}
}

// Not-found must propagate as a 404 so callers can distinguish a missing guest
// from an unreadable one.
func TestGetGuestStatusPropagatesNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"data":null}`))
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, client: httpclient.New()}
	if _, err := c.GetGuestStatus(context.Background(), "pve1", "qemu", 999); err == nil {
		t.Fatal("expected an error for a missing guest")
	}
}
