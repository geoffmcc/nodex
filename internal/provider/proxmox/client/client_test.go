package client

import (
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

func TestNormalizeEndpointRejectsUnsafeURLs(t *testing.T) {
	tests := []string{
		"http://pve.example:8006",
		"https://user:pass@pve.example:8006",
		"https://pve.example:8006/api2/json",
		"https://pve.example:8006?token=secret",
		"not a url",
	}
	for _, endpoint := range tests {
		if _, err := NormalizeEndpoint(endpoint); err == nil {
			t.Fatalf("NormalizeEndpoint(%q) succeeded, want error", endpoint)
		}
	}
}

func TestGetRejectsOversizedSuccessBody(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":"` + strings.Repeat("A", 128) + `"}`))
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New(httpclient.WithMaxBodySize(32))}
	var out map[string]any
	err := c.get(context.Background(), "/version", &out)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("get error = %v, want oversized body error", err)
	}
}

func TestGetRejectsCompressedExpansionOverLimit(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		_, _ = gz.Write([]byte(`{"data":"` + strings.Repeat("B", 128) + `"}`))
		_ = gz.Close()
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New(httpclient.WithMaxBodySize(32))}
	var out map[string]any
	err := c.get(context.Background(), "/version", &out)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("get error = %v, want decompressed oversized body error", err)
	}
}

func TestGetTruncatesAndRedactsErrorBody(t *testing.T) {
	secret := "PVEAPIToken=user@pam!tok=supersecret" // #nosec G101 -- test fixture verifies redaction.
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, secret+strings.Repeat("X", 64), http.StatusForbidden)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New(httpclient.WithMaxErrorBodySize(16))}
	var out map[string]any
	err := c.get(context.Background(), "/version", &out)
	if err == nil {
		t.Fatal("expected API error")
	}
	msg := err.Error()
	if strings.Contains(msg, secret) || strings.Contains(msg, "\x1b") {
		t.Fatalf("error leaked unsafe body: %q", msg)
	}
	if !strings.Contains(msg, "truncated") {
		t.Fatalf("error = %q, want truncation marker", msg)
	}
}

func TestGetAcceptsBoundaryBody(t *testing.T) {
	body := []byte(`{"data":{"version":"8.2"}}`)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New(httpclient.WithMaxBodySize(int64(len(body))))}
	var out VersionResponse
	if err := c.get(context.Background(), "/version", &out); err != nil {
		t.Fatalf("get boundary body: %v (len=%d)", err, len(body))
	}
	if out.Data.Version != "8.2" {
		t.Fatalf("version = %q", out.Data.Version)
	}
}

func TestGetRejectsTrailingJSON(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"data":{"version":"8.2"}} {}`)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	var out VersionResponse
	if err := c.get(context.Background(), "/version", &out); err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("get error = %v, want trailing data error", err)
	}
}

func TestNodesDecodesProxmoxNodeFields(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"data":[{"id":"node/proxmox","node":"proxmox","status":"online","type":"node"},{"id":"node/backup","node":"backup","status":"offline","type":"node","uptime":42}]}`)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	nodes, err := c.Nodes(context.Background())
	if err != nil {
		t.Fatalf("Nodes: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("len(nodes) = %d, want 2", len(nodes))
	}
	if nodes[0].ID != "node/proxmox" || nodes[0].Node != "proxmox" || nodes[0].Name != "" || nodes[0].Uptime != nil {
		t.Fatalf("first node = %+v", nodes[0])
	}
	if nodes[1].Uptime == nil || *nodes[1].Uptime != 42 {
		t.Fatalf("second uptime = %v, want 42", nodes[1].Uptime)
	}
}

func TestClusterResourcesDecodesProxmoxGuestAndStorageFields(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"data":[{"id":"qemu/100","type":"qemu","vmid":100,"name":"vm-one","node":"proxmox","status":"running","maxcpu":2,"maxmem":2147483648,"maxdisk":34359738368},{"id":"lxc/200","type":"lxc","vmid":200,"name":"ct-one","node":"proxmox","status":"stopped","maxmem":1073741824,"maxdisk":8589934592},{"id":"storage/proxmox/local-lvm","type":"storage","storage":"local-lvm","node":"proxmox","status":"available","disk":1024,"maxdisk":4096,"content":"images,rootdir"}]}`)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	resources, err := c.ClusterResources(context.Background())
	if err != nil {
		t.Fatalf("ClusterResources: %v", err)
	}
	if len(resources) != 3 {
		t.Fatalf("len(resources) = %d, want 3", len(resources))
	}
	if resources[0].Type != "qemu" || resources[0].VMID != 100 || resources[0].MaxMem != 2147483648 {
		t.Fatalf("first resource = %+v", resources[0])
	}
	if resources[1].Type != "lxc" || resources[1].VMID != 200 || resources[1].Name != "ct-one" {
		t.Fatalf("second resource = %+v", resources[1])
	}
	if resources[2].Type != "storage" || resources[2].Storage != "local-lvm" || resources[2].Content != "images,rootdir" {
		t.Fatalf("third resource = %+v", resources[2])
	}
}
