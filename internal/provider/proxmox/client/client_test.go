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

func TestGetNodeStatusDecodesProxmoxNodeStatusFields(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/proxmox/status" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"data":{"cpu":0.1234,"maxcpu":4,"mem":1073741824,"maxmem":4294967296,"disk":536870912,"maxdisk":107374182400,"uptime":12345,"level":"","id":"node/proxmox","node":"proxmox","type":"node","status":"online","kversion":"6.8.12-1-pve","pveversion":"pve-manager/8.2.4","loadavg":[0.12,0.34,0.56]}}`)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	status, err := c.GetNodeStatus(context.Background(), "proxmox")
	if err != nil {
		t.Fatalf("GetNodeStatus: %v", err)
	}
	if status.Node != "proxmox" || status.Status != "online" {
		t.Fatalf("node/status = %q/%q", status.Node, status.Status)
	}
	if status.CPU != 0.1234 || status.MaxCPU != 4 {
		t.Fatalf("cpu/maxcpu = %v/%d", status.CPU, status.MaxCPU)
	}
	if status.Mem != 1073741824 || status.MaxMem != 4294967296 {
		t.Fatalf("mem/maxmem = %d/%d", status.Mem, status.MaxMem)
	}
	if status.Disk != 536870912 || status.MaxDisk != 107374182400 {
		t.Fatalf("disk/maxdisk = %d/%d", status.Disk, status.MaxDisk)
	}
	if status.Uptime != 12345 {
		t.Fatalf("uptime = %d", status.Uptime)
	}
	if status.KVersion != "6.8.12-1-pve" || status.PVEVersion != "pve-manager/8.2.4" {
		t.Fatalf("kversion/pveversion = %q/%q", status.KVersion, status.PVEVersion)
	}
	if len(status.LoadAvg) != 3 || status.LoadAvg[0] != 0.12 || status.LoadAvg[1] != 0.34 || status.LoadAvg[2] != 0.56 {
		t.Fatalf("loadavg = %v", status.LoadAvg)
	}
}

func TestGetNodeStatusRejectsEmptyNode(t *testing.T) {
	c := &Client{baseURL: "https://example.com", client: httpclient.New()}
	_, err := c.GetNodeStatus(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "node name is required") {
		t.Fatalf("GetNodeStatus('') error = %v, want node name required", err)
	}
}

func TestVersionAtLeastComparesVersions(t *testing.T) {
	tests := []struct {
		name    string
		version string
		major   int
		minor   int
		want    bool
	}{
		{"8.1.4 >= 8.1", "8.1.4", 8, 1, true},
		{"8.1.4 >= 8.2", "8.1.4", 8, 2, false},
		{"8.1.4 >= 9.0", "8.1.4", 9, 0, false},
		{"9.0.0 >= 8.1", "9.0.0", 8, 1, true},
		{"empty version", "", 8, 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{version: &VersionData{Version: tt.version}}
			got := c.VersionAtLeast(tt.major, tt.minor)
			if got != tt.want {
				t.Errorf("VersionAtLeast(%d, %d) = %v, want %v", tt.major, tt.minor, got, tt.want)
			}
		})
	}
}

func TestReleaseReturnsVersionRelease(t *testing.T) {
	c := &Client{version: &VersionData{Release: "pve-manager/8.2.4", Version: "8.2.4"}}
	if got := c.Release(); got != "pve-manager/8.2.4" {
		t.Errorf("Release() = %q, want %q", got, "pve-manager/8.2.4")
	}
}

func TestReleaseReturnsEmptyWhenNoVersion(t *testing.T) {
	c := &Client{}
	if got := c.Release(); got != "" {
		t.Errorf("Release() = %q, want empty", got)
	}
}

func TestGetClusterStatusDecodesQuorumAndNodes(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cluster/status" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"data":[{"type":"cluster","id":"cluster/0","name":"mycluster","status":"online","quorate":1,"version":3},{"type":"node","id":"node/proxmox","name":"proxmox","status":"online","ip":"10.0.0.1","localmem":4294967296,"maxmem":17179869184,"localdisk":107374182400,"maxdisk":1073741824000}]}`)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	items, err := c.GetClusterStatus(context.Background())
	if err != nil {
		t.Fatalf("GetClusterStatus: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].Type != "cluster" || items[0].Name != "mycluster" || items[0].Quorate != 1 || items[0].Version != 3 {
		t.Fatalf("cluster item = %+v", items[0])
	}
	if items[1].Type != "node" || items[1].Name != "proxmox" || items[1].IP != "10.0.0.1" {
		t.Fatalf("node item = %+v", items[1])
	}
	if items[1].Localmem != 4294967296 || items[1].Maxmem != 17179869184 {
		t.Fatalf("node memory = %d/%d", items[1].Localmem, items[1].Maxmem)
	}
	if items[1].Localdisk != 107374182400 || items[1].Maxdisk != 1073741824000 {
		t.Fatalf("node disk = %d/%d", items[1].Localdisk, items[1].Maxdisk)
	}
}

func TestGetVMConfigDecodesConfigFields(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/proxmox/qemu/100/config" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"data":{"vmid":100,"name":"test-vm","cores":2,"memory":2048,"net0":"virtio=AA:BB:CC:DD:EE:FF,bridge=vmbr0","scsi0":"local-lvm:vm-100-disk-0,size=32G","onboot":1,"ostype":"l26","description":"Test VM","tags":"test,dev"}}`)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	config, err := c.GetVMConfig(context.Background(), "proxmox", 100)
	if err != nil {
		t.Fatalf("GetVMConfig: %v", err)
	}
	if config.VMID != 100 || config.Name != "test-vm" {
		t.Fatalf("vmid/name = %d/%q", config.VMID, config.Name)
	}
	if config.CPU != 2 || config.Memory != 2048 {
		t.Fatalf("cpu/memory = %d/%d", config.CPU, config.Memory)
	}
	if config.OnBoot != 1 || config.OSType != "l26" {
		t.Fatalf("onboot/ostype = %d/%q", config.OnBoot, config.OSType)
	}
	if config.Description != "Test VM" || config.Tags != "test,dev" {
		t.Fatalf("description/tags = %q/%q", config.Description, config.Tags)
	}
}

func TestGetVMConfigRejectsEmptyNode(t *testing.T) {
	c := &Client{baseURL: "https://example.com", client: httpclient.New()}
	_, err := c.GetVMConfig(context.Background(), "", 100)
	if err == nil || !strings.Contains(err.Error(), "node name is required") {
		t.Fatalf("GetVMConfig('') error = %v, want node name required", err)
	}
}

func TestGetVMConfigRejectsZeroVMID(t *testing.T) {
	c := &Client{baseURL: "https://example.com", client: httpclient.New()}
	_, err := c.GetVMConfig(context.Background(), "proxmox", 0)
	if err == nil || !strings.Contains(err.Error(), "VMID is required") {
		t.Fatalf("GetVMConfig(0) error = %v, want VMID required", err)
	}
}

func TestGetContainerConfigDecodesConfigFields(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/proxmox/lxc/200/config" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"data":{"vmid":200,"hostname":"test-ct","cores":1,"memory":512,"swap":256,"rootfs":"local-lvm:vm-200-disk-0,size=8G","net0":"name=eth0,bridge=vmbr0,hwaddr=AA:BB:CC:DD:EE:FF,ip=dhcp","onboot":1,"ostype":"debian","description":"Test Container"}}`)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	config, err := c.GetContainerConfig(context.Background(), "proxmox", 200)
	if err != nil {
		t.Fatalf("GetContainerConfig: %v", err)
	}
	if config.VMID != 200 || config.Hostname != "test-ct" {
		t.Fatalf("vmid/hostname = %d/%q", config.VMID, config.Hostname)
	}
	if config.CPU != 1 || config.Memory != 512 || config.Swap != 256 {
		t.Fatalf("cpu/memory/swap = %d/%d/%d", config.CPU, config.Memory, config.Swap)
	}
	if config.OnBoot != 1 || config.OSType != "debian" {
		t.Fatalf("onboot/ostype = %d/%q", config.OnBoot, config.OSType)
	}
	if config.Description != "Test Container" {
		t.Fatalf("description = %q", config.Description)
	}
}

func TestGetContainerConfigRejectsEmptyNode(t *testing.T) {
	c := &Client{baseURL: "https://example.com", client: httpclient.New()}
	_, err := c.GetContainerConfig(context.Background(), "", 200)
	if err == nil || !strings.Contains(err.Error(), "node name is required") {
		t.Fatalf("GetContainerConfig('') error = %v, want node name required", err)
	}
}

func TestGetContainerConfigRejectsZeroVMID(t *testing.T) {
	c := &Client{baseURL: "https://example.com", client: httpclient.New()}
	_, err := c.GetContainerConfig(context.Background(), "proxmox", 0)
	if err == nil || !strings.Contains(err.Error(), "VMID is required") {
		t.Fatalf("GetContainerConfig(0) error = %v, want VMID required", err)
	}
}
