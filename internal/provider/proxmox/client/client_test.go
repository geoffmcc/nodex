package client

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
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

func TestMutationAcceptsNilResult(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/cluster/firewall/aliases" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"data":null}`)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	if err := c.CreateFirewallAlias(context.Background(), "NodexTest", "198.51.100.10", ""); err != nil {
		t.Fatalf("CreateFirewallAlias: %v", err)
	}
}

func TestCreateFirewallGroupUsesGroupField(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/cluster/firewall/groups" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if got := r.Form.Get("group"); got != "nodexgroup" {
			t.Fatalf("group = %q, want nodexgroup", got)
		}
		if got := r.Form.Get("name"); got != "" {
			t.Fatalf("name = %q, want empty", got)
		}
		_, _ = fmt.Fprint(w, `{"data":null}`)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	if err := c.CreateFirewallGroup(context.Background(), "nodexgroup", "test group"); err != nil {
		t.Fatalf("CreateFirewallGroup: %v", err)
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

func TestGetNodeStatusDecodesStringLoadAverage(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/proxmox/status" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"data":{"cpu":0.1,"maxcpu":4,"mem":1,"maxmem":2,"disk":3,"maxdisk":4,"uptime":5,"node":"proxmox","type":"node","status":"online","loadavg":["0.12","0.34","0.56"]}}`)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	status, err := c.GetNodeStatus(context.Background(), "proxmox")
	if err != nil {
		t.Fatalf("GetNodeStatus: %v", err)
	}
	if len(status.LoadAvg) != 3 || status.LoadAvg[0] != 0.12 || status.LoadAvg[1] != 0.34 || status.LoadAvg[2] != 0.56 {
		t.Fatalf("loadavg = %v", status.LoadAvg)
	}
}

func TestGetNodeStatusDecodesObjectKSM(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/proxmox/status" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"data":{"cpu":0.1,"maxcpu":4,"mem":1,"maxmem":2,"disk":3,"maxdisk":4,"uptime":5,"node":"proxmox","type":"node","status":"online","ksm":{"shared":0}}}`)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	status, err := c.GetNodeStatus(context.Background(), "proxmox")
	if err != nil {
		t.Fatalf("GetNodeStatus: %v", err)
	}
	if status.Ksm != 0 {
		t.Fatalf("ksm = %d, want zero for object response", status.Ksm)
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

func TestGetClusterStatusDecodesStringNumericFields(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cluster/status" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"data":[{"type":"cluster","id":"cluster/0","name":"mycluster","status":"online","quorate":"1","version":"3"}]}`)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	items, err := c.GetClusterStatus(context.Background())
	if err != nil {
		t.Fatalf("GetClusterStatus: %v", err)
	}
	if len(items) != 1 || items[0].Quorate != 1 || items[0].Version != 3 {
		t.Fatalf("cluster items = %+v", items)
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

func TestGetVMConfigDecodesStringNumericFields(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/proxmox/qemu/100/config" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"data":{"vmid":"100","name":"test-vm","cores":"2","memory":"2048","onboot":"1","agent":"1","numa":"0","protection":"1"}}`)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	config, err := c.GetVMConfig(context.Background(), "proxmox", 100)
	if err != nil {
		t.Fatalf("GetVMConfig: %v", err)
	}
	if config.VMID != 100 || config.CPU != 2 || config.Memory != 2048 || config.OnBoot != 1 || config.Agent != 1 || config.Protection != 1 {
		t.Fatalf("config = %+v", config)
	}
}

func TestGetVMConfigInjectsVMIDFromParameter(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Proxmox 9 omits vmid from response body.
		_, _ = fmt.Fprint(w, `{"data":{"name":"test-vm","cores":2,"memory":2048}}`)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	config, err := c.GetVMConfig(context.Background(), "proxmox", 42)
	if err != nil {
		t.Fatalf("GetVMConfig: %v", err)
	}
	if config.VMID != 42 {
		t.Errorf("VMID = %d, want 42 (injected from parameter)", config.VMID)
	}
}

func TestGetContainerConfigInjectsVMIDFromParameter(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Proxmox 9 omits vmid from response body.
		_, _ = fmt.Fprint(w, `{"data":{"hostname":"test-ct","cores":1,"memory":512}}`)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	config, err := c.GetContainerConfig(context.Background(), "proxmox", 77)
	if err != nil {
		t.Fatalf("GetContainerConfig: %v", err)
	}
	if config.VMID != 77 {
		t.Errorf("VMID = %d, want 77 (injected from parameter)", config.VMID)
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

func TestGetNodeTimeDecodesNumericLocalTime(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/proxmox/time" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"data":{"timezone":"America/New_York","epoch":1784073342,"localtime":1784073342}}`)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	got, err := c.GetNodeTime(context.Background(), "proxmox")
	if err != nil {
		t.Fatalf("GetNodeTime: %v", err)
	}
	if got.TimeZone != "America/New_York" || got.Epoch != 1784073342 || got.Local != "1784073342" {
		t.Fatalf("node time = %+v", got)
	}
}

func TestGetNodeUpdatesUsesAptUpdatePath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/proxmox/apt/update" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"data":[{"package":"pve-manager","version":"9.2.2"}]}`)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	updates, err := c.GetNodeUpdates(context.Background(), "proxmox")
	if err != nil {
		t.Fatalf("GetNodeUpdates: %v", err)
	}
	if len(updates) != 1 || updates[0].Package != "pve-manager" || updates[0].Version != "9.2.2" {
		t.Fatalf("updates = %+v", updates)
	}
}

func TestGetHAStatusDecodesArrayResponse(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cluster/ha/status" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"data":[{"id":"vm:100","type":"service","state":"started","node":"proxmox"}]}`)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	got, err := c.GetHAStatus(context.Background())
	if err != nil {
		t.Fatalf("GetHAStatus: %v", err)
	}
	if got.Status != "ok" || got.Quorum != 1 {
		t.Fatalf("HA status = %+v", got)
	}
}

func TestGetHACurrentUsesStatusCurrentPath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cluster/ha/status/current" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"data":[{"sid":"vm:100","type":"service","state":"started","node":"proxmox"}]}`)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	items, err := c.GetHACurrent(context.Background())
	if err != nil {
		t.Fatalf("GetHACurrent: %v", err)
	}
	if len(items) != 1 || items[0].State != "started" || items[0].Node != "proxmox" {
		t.Fatalf("HA current = %+v", items)
	}
}

func TestGetHAGroupsFallsBackToRulesAfterMigration(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cluster/ha/groups":
			http.Error(w, `cannot index groups: ha groups have been migrated to rules`, http.StatusInternalServerError)
		case "/cluster/ha/rules":
			_, _ = fmt.Fprint(w, `{"data":[{"rule":"prefer-a","type":"node-affinity","nodes":"proxmox:1","comment":"Prefer node A"}]}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	groups, err := c.GetHAGroups(context.Background())
	if err != nil {
		t.Fatalf("GetHAGroups: %v", err)
	}
	if len(groups) != 1 || groups[0].ID != "prefer-a" || groups[0].Type != "node-affinity" || groups[0].Nodes != "proxmox:1" || groups[0].Comment != "Prefer node A" {
		t.Fatalf("HA groups = %+v", groups)
	}
}

func TestGetHAGroupsKeepsUnrelatedServerError(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cluster/ha/groups" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		http.Error(w, `database unavailable`, http.StatusInternalServerError)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	_, err := c.GetHAGroups(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "/cluster/ha/rules") {
		t.Fatalf("unexpected fallback for unrelated error: %v", err)
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

func TestGetStorageContentDecodesContentItems(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/proxmox/storage/local/content" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"data":[{"content":"iso","volid":"local:iso/debian-12.iso","size":5368709120,"format":"iso","ctime":1700000000},{"content":"images","volid":"local-lvm:vm-100-disk-0","size":34359738368,"format":"raw","vmid":100}]}`)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	items, err := c.GetStorageContent(context.Background(), "proxmox", "local")
	if err != nil {
		t.Fatalf("GetStorageContent: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].Content != "iso" || items[0].Volid != "local:iso/debian-12.iso" || items[0].Size != 5368709120 {
		t.Fatalf("first item = %+v", items[0])
	}
	if items[1].Content != "images" || items[1].VMID != 100 || items[1].Size != 34359738368 {
		t.Fatalf("second item = %+v", items[1])
	}
}

func TestGetStorageContentRejectsEmptyNode(t *testing.T) {
	c := &Client{baseURL: "https://example.com", client: httpclient.New()}
	_, err := c.GetStorageContent(context.Background(), "", "local")
	if err == nil || !strings.Contains(err.Error(), "node name is required") {
		t.Fatalf("GetStorageContent('') error = %v, want node name required", err)
	}
}

func TestGetStorageContentRejectsEmptyStorage(t *testing.T) {
	c := &Client{baseURL: "https://example.com", client: httpclient.New()}
	_, err := c.GetStorageContent(context.Background(), "proxmox", "")
	if err == nil || !strings.Contains(err.Error(), "storage name is required") {
		t.Fatalf("GetStorageContent('') error = %v, want storage name required", err)
	}
}

func TestGetCephOSDsDecodesStringIDs(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/proxmox/ceph/osd" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"data":{"root":{"children":[{"id":"-1","name":"default","type":"root","children":[{"id":"0","name":"osd.0","type":"osd","status":"up","in":"1","leaf":"1"}]}]}}}`)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	osds, err := c.GetCephOSDs(context.Background(), "proxmox")
	if err != nil {
		t.Fatalf("GetCephOSDs: %v", err)
	}
	children := osds.Data.Root.Children
	if len(children) != 1 || children[0].ID != -1 || children[0].Name != "default" {
		t.Fatalf("root children = %+v", children)
	}
	if len(children[0].Children) != 1 || children[0].Children[0].ID != 0 || children[0].Children[0].In != 1 || children[0].Children[0].Leaf != 1 {
		t.Fatalf("nested children = %+v", children[0].Children)
	}
}

func TestGetTasksDecodesTaskList(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/proxmox/tasks" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"data":[{"upid":"UPID:proxmox/00012345/0","type":"vzdump","state":"running","starttime":1700000000,"pid":1234},{"upid":"UPID:proxmox/00012346/0","type":"qmshutdown","state":"stopped","starttime":1700000001,"endtime":1700000002,"status":"OK","pid":1235}]}`)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	tasks, err := c.GetTasks(context.Background(), "proxmox")
	if err != nil {
		t.Fatalf("GetTasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("len(tasks) = %d, want 2", len(tasks))
	}
	if tasks[0].UPID != "UPID:proxmox/00012345/0" || tasks[0].State != "running" || tasks[0].Type != "vzdump" {
		t.Fatalf("first task = %+v", tasks[0])
	}
	if tasks[1].State != "stopped" || tasks[1].Status != "OK" {
		t.Fatalf("second task = %+v", tasks[1])
	}
}

func TestGetTasksRejectsEmptyNode(t *testing.T) {
	c := &Client{baseURL: "https://example.com", client: httpclient.New()}
	_, err := c.GetTasks(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "node name is required") {
		t.Fatalf("GetTasks('') error = %v, want node name required", err)
	}
}

func TestGetTaskDecodesTaskDetail(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/proxmox/tasks/UPID:proxmox/00012345/0/status" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"data":{"upid":"UPID:proxmox/00012345/0","type":"vzdump","status":"stopped","exitstatus":"OK","starttime":1700000000,"pid":1234}}`)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	task, err := c.GetTask(context.Background(), "proxmox", "UPID:proxmox/00012345/0")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.UPID != "UPID:proxmox/00012345/0" || task.Status != "stopped" || task.ExitStatus != "OK" {
		t.Fatalf("task = %+v", task)
	}
}

func TestGetTaskRejectsEmptyNode(t *testing.T) {
	c := &Client{baseURL: "https://example.com", client: httpclient.New()}
	_, err := c.GetTask(context.Background(), "", "UPID:test/1/0")
	if err == nil || !strings.Contains(err.Error(), "node name is required") {
		t.Fatalf("GetTask('') error = %v, want node name required", err)
	}
}

func TestGetTaskRejectsEmptyUPID(t *testing.T) {
	c := &Client{baseURL: "https://example.com", client: httpclient.New()}
	_, err := c.GetTask(context.Background(), "proxmox", "")
	if err == nil || !strings.Contains(err.Error(), "task UPID is required") {
		t.Fatalf("GetTask('') error = %v, want UPID required", err)
	}
}

func TestGetVMSnapshotsDecodesList(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/proxmox/qemu/100/snapshot" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"data":[{"name":"before-upgrade","vmid":100,"ctime":1700000000,"parent":"current"},{"name":"current","vmid":100,"ctime":1700000010}]}`)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	snaps, err := c.GetVMSnapshots(context.Background(), "proxmox", 100)
	if err != nil {
		t.Fatalf("GetVMSnapshots: %v", err)
	}
	if len(snaps) != 2 {
		t.Fatalf("len(snaps) = %d, want 2", len(snaps))
	}
	if snaps[0].Name != "before-upgrade" || snaps[0].Parent != "current" {
		t.Fatalf("first snapshot = %+v", snaps[0])
	}
	if snaps[1].Name != "current" {
		t.Fatalf("second snapshot = %+v", snaps[1])
	}
}

func TestGetVMSnapshotsRejectsEmptyNode(t *testing.T) {
	c := &Client{baseURL: "https://example.com", client: httpclient.New()}
	_, err := c.GetVMSnapshots(context.Background(), "", 100)
	if err == nil || !strings.Contains(err.Error(), "node name is required") {
		t.Fatalf("GetVMSnapshots('') error = %v, want node name required", err)
	}
}

func TestGetVMSnapshotsRejectsZeroVMID(t *testing.T) {
	c := &Client{baseURL: "https://example.com", client: httpclient.New()}
	_, err := c.GetVMSnapshots(context.Background(), "proxmox", 0)
	if err == nil || !strings.Contains(err.Error(), "VMID is required") {
		t.Fatalf("GetVMSnapshots(0) error = %v, want VMID required", err)
	}
}

func TestGetContainerSnapshotsDecodesList(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/proxmox/lxc/200/snapshot" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"data":[{"name":"clean","vmid":200,"ctime":1700000000}]}`)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	snaps, err := c.GetContainerSnapshots(context.Background(), "proxmox", 200)
	if err != nil {
		t.Fatalf("GetContainerSnapshots: %v", err)
	}
	if len(snaps) != 1 || snaps[0].Name != "clean" {
		t.Fatalf("snapshots = %+v", snaps)
	}
}

func TestGetContainerSnapshotsRejectsEmptyNode(t *testing.T) {
	c := &Client{baseURL: "https://example.com", client: httpclient.New()}
	_, err := c.GetContainerSnapshots(context.Background(), "", 200)
	if err == nil || !strings.Contains(err.Error(), "node name is required") {
		t.Fatalf("GetContainerSnapshots('') error = %v, want node name required", err)
	}
}

func TestGetContainerSnapshotsRejectsZeroVMID(t *testing.T) {
	c := &Client{baseURL: "https://example.com", client: httpclient.New()}
	_, err := c.GetContainerSnapshots(context.Background(), "proxmox", 0)
	if err == nil || !strings.Contains(err.Error(), "VMID is required") {
		t.Fatalf("GetContainerSnapshots(0) error = %v, want VMID required", err)
	}
}

func TestGetEventsDecodesEventList(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cluster/events" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"data":[{"type":"node","time":1700000000,"node":"proxmox","id":"node/proxmox","message":"node online"},{"type":"vm","time":1700000001,"node":"proxm","id":"vm/100","message":"VM started"}]}`)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	events, err := c.GetEvents(context.Background())
	if err != nil {
		t.Fatalf("GetEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}
	if events[0].Type != "node" || events[0].Message != "node online" {
		t.Fatalf("first event = %+v", events[0])
	}
	if events[1].Type != "vm" || events[1].ID != "vm/100" {
		t.Fatalf("second event = %+v", events[1])
	}
}

func TestGetEventsFallsBackToClusterLog(t *testing.T) {
	var paths []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/cluster/events":
			http.Error(w, `{"data":null,"message":"Method not implemented"}`, http.StatusNotImplemented)
		case "/cluster/log":
			_, _ = fmt.Fprint(w, `{"data":[{"time":1784073342,"node":"pve-test","id":"162:pve-test","tag":"pvedaemon","msg":"successful auth for user 'root@pam'"}]}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	events, err := c.GetEvents(context.Background())
	if err != nil {
		t.Fatalf("GetEvents: %v", err)
	}
	if len(paths) < 2 || paths[0] != "/cluster/events" || paths[len(paths)-1] != "/cluster/log" {
		t.Fatalf("paths = %v", paths)
	}
	if len(events) != 1 || events[0].Msg == "" || events[0].Tag != "pvedaemon" {
		t.Fatalf("events = %+v", events)
	}
}

func TestGetSyslogDecodesEntries(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/proxmox/syslog" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"data":[{"n":1,"t":"Jul 14 18:09:13 pve-test kernel: Linux version"},{"n":2,"t":"Jul 14 18:09:14 pve-test kernel: Command line"}]}`)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	entries, err := c.GetSyslog(context.Background(), "proxmox")
	if err != nil {
		t.Fatalf("GetSyslog: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].N != 1 || entries[0].T == "" {
		t.Fatalf("first entry = %+v", entries[0])
	}
	if entries[1].N != 2 || entries[1].T == "" {
		t.Fatalf("second entry = %+v", entries[1])
	}
}

func TestGetSyslogRejectsEmptyNode(t *testing.T) {
	c := &Client{baseURL: "https://example.com", client: httpclient.New()}
	_, err := c.GetSyslog(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "node name is required") {
		t.Fatalf("GetSyslog('') error = %v, want node name required", err)
	}
}

// --- Exit-code classification integration tests ---
// These tests use a mock HTTP server to verify that each HTTP status
// code produces the correct typed ProviderError and maps to the
// expected exit code through app.ExitCodeFromError.

func TestProviderError_401_ExitsAuth(t *testing.T) {
	pe := newProviderError(http.StatusUnauthorized, "401 Unauthorized")
	if pe.StatusCode != http.StatusUnauthorized {
		t.Fatalf("StatusCode = %d", pe.StatusCode)
	}
	code := app.ExitCodeFromError(pe)
	if code != app.ExitAuth {
		t.Errorf("exit code for 401 = %d, want ExitAuth(%d)", code, app.ExitAuth)
	}
}

func TestProviderError_403_ExitsAuthorization(t *testing.T) {
	pe := newProviderError(http.StatusForbidden, "403 Forbidden")
	code := app.ExitCodeFromError(pe)
	if code != app.ExitAuthorization {
		t.Errorf("exit code for 403 = %d, want ExitAuthorization(%d)", code, app.ExitAuthorization)
	}
}

func TestProviderError_404_ExitsNotFound(t *testing.T) {
	pe := newProviderError(http.StatusNotFound, "404 Not Found")
	code := app.ExitCodeFromError(pe)
	if code != app.ExitNotFound {
		t.Errorf("exit code for 404 = %d, want ExitNotFound(%d)", code, app.ExitNotFound)
	}
}

func TestProviderError_409_ExitsConflict(t *testing.T) {
	pe := newProviderError(http.StatusConflict, "409 Conflict")
	code := app.ExitCodeFromError(pe)
	if code != app.ExitConflict {
		t.Errorf("exit code for 409 = %d, want ExitConflict(%d)", code, app.ExitConflict)
	}
}

func TestProviderError_500_ExitsProvider(t *testing.T) {
	pe := newProviderError(http.StatusInternalServerError, "500 Internal Server Error")
	code := app.ExitCodeFromError(pe)
	if code != app.ExitProvider {
		t.Errorf("exit code for 500 = %d, want ExitProvider(%d)", code, app.ExitProvider)
	}
}

func TestProviderError_503_ExitsProvider(t *testing.T) {
	pe := newProviderError(http.StatusServiceUnavailable, "503 Unavailable")
	code := app.ExitCodeFromError(pe)
	if code != app.ExitProvider {
		t.Errorf("exit code for 503 = %d, want ExitProvider(%d)", code, app.ExitProvider)
	}
}

func TestProviderError_400_ExitsValidation(t *testing.T) {
	pe := newProviderError(http.StatusBadRequest, "400 Bad Request")
	code := app.ExitCodeFromError(pe)
	if code != app.ExitValidationError {
		t.Errorf("exit code for 400 = %d, want ExitValidationError(%d)", code, app.ExitValidationError)
	}
}

func TestProviderError_429_ExitsRateLimit(t *testing.T) {
	pe := newProviderError(http.StatusTooManyRequests, "429 Too Many Requests")
	code := app.ExitCodeFromError(pe)
	if code != app.ExitRateLimit {
		t.Errorf("exit code for 429 = %d, want ExitRateLimit(%d)", code, app.ExitRateLimit)
	}
}

func TestProviderError_504_ExitsTimeout(t *testing.T) {
	pe := newProviderError(http.StatusGatewayTimeout, "504 Gateway Timeout")
	code := app.ExitCodeFromError(pe)
	if code != app.ExitTimeout {
		t.Errorf("exit code for 504 = %d, want ExitTimeout(%d)", code, app.ExitTimeout)
	}
}

// TestProviderError_WrappedInChain verifies that ProviderError survives
// error wrapping (fmt.Errorf("... %w", ...)) and the exit code is still
// correctly extracted.
func TestProviderError_WrappedInChain(t *testing.T) {
	pe := newProviderError(http.StatusNotFound, "not found")
	wrapped := fmt.Errorf("list VMs: %w", pe)
	code := app.ExitCodeFromError(wrapped)
	if code != app.ExitNotFound {
		t.Errorf("wrapped 404 exit code = %d, want ExitNotFound(%d)", code, app.ExitNotFound)
	}
}

// TestProviderError_MockHTTPServer_401 proves the full chain:
// httptest → get() → decodeResponse → ProviderError → ExitCode.
func TestProviderError_MockHTTPServer_401(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "401 Unauthorized", http.StatusUnauthorized)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	var out map[string]any
	err := c.get(context.Background(), "/test", &out)
	if err == nil {
		t.Fatal("expected error for 401")
	}
	code := app.ExitCodeFromError(err)
	if code != app.ExitAuth {
		t.Errorf("mock 401 exit code = %d, want ExitAuth(%d)", code, app.ExitAuth)
	}
}

func TestProviderError_MockHTTPServer_403(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "403 Forbidden", http.StatusForbidden)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	var out map[string]any
	err := c.get(context.Background(), "/test", &out)
	if err == nil {
		t.Fatal("expected error for 403")
	}
	code := app.ExitCodeFromError(err)
	if code != app.ExitAuthorization {
		t.Errorf("mock 403 exit code = %d, want ExitAuthorization(%d)", code, app.ExitAuthorization)
	}
}

func TestProviderError_MockHTTPServer_404(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "404 Not Found", http.StatusNotFound)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	var out map[string]any
	err := c.get(context.Background(), "/test", &out)
	if err == nil {
		t.Fatal("expected error for 404")
	}
	code := app.ExitCodeFromError(err)
	if code != app.ExitNotFound {
		t.Errorf("mock 404 exit code = %d, want ExitNotFound(%d)", code, app.ExitNotFound)
	}
}

func TestProviderError_MockHTTPServer_409(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "409 Conflict", http.StatusConflict)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	var out map[string]any
	err := c.get(context.Background(), "/test", &out)
	if err == nil {
		t.Fatal("expected error for 409")
	}
	code := app.ExitCodeFromError(err)
	if code != app.ExitConflict {
		t.Errorf("mock 409 exit code = %d, want ExitConflict(%d)", code, app.ExitConflict)
	}
}

func TestProviderError_MockHTTPServer_500(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	var out map[string]any
	err := c.get(context.Background(), "/test", &out)
	if err == nil {
		t.Fatal("expected error for 500")
	}
	code := app.ExitCodeFromError(err)
	if code != app.ExitProvider {
		t.Errorf("mock 500 exit code = %d, want ExitProvider(%d)", code, app.ExitProvider)
	}
}

func TestProviderError_MockHTTPServer_RedactsSecrets(t *testing.T) {
	secret := "PVEAPIToken=user@pam!tok=secret123" // #nosec G101 -- test fixture
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, secret, http.StatusForbidden)
	}))
	defer s.Close()
	c := &Client{baseURL: s.URL, client: httpclient.New()}
	var out map[string]any
	err := c.get(context.Background(), "/test", &out)
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("error leaked secret: %s", err.Error())
	}
}

// --- CR-001: Endpoint host validation tests ---

func TestValidateEndpoint_MatchingHost_SamePort(t *testing.T) {
	c := &Client{endpointHost: "pve.example.com"}
	reqURL, _ := url.Parse("https://pve.example.com:8006/api2/json/nodes")
	if err := c.validateEndpoint(reqURL); err != nil {
		t.Fatalf("validateEndpoint: unexpected error: %v", err)
	}
}

func TestValidateEndpoint_MatchingHost_DifferentPort(t *testing.T) {
	c := &Client{endpointHost: "pve.example.com"}
	reqURL, _ := url.Parse("https://pve.example.com:9006/api2/json/nodes")
	if err := c.validateEndpoint(reqURL); err != nil {
		t.Fatalf("validateEndpoint: port should be ignored, got: %v", err)
	}
}

func TestValidateEndpoint_MatchingHost_NoPort(t *testing.T) {
	c := &Client{endpointHost: "pve.example.com"}
	reqURL, _ := url.Parse("https://pve.example.com/api2/json/nodes")
	if err := c.validateEndpoint(reqURL); err != nil {
		t.Fatalf("validateEndpoint: unexpected error: %v", err)
	}
}

func TestValidateEndpoint_CaseInsensitive(t *testing.T) {
	c := &Client{endpointHost: "pve.example.com"}
	reqURL, _ := url.Parse("https://PVE.EXAMPLE.COM:8006/api2/json/nodes")
	if err := c.validateEndpoint(reqURL); err != nil {
		t.Fatalf("validateEndpoint: DNS names are case-insensitive, got: %v", err)
	}
}

func TestValidateEndpoint_DifferentHost(t *testing.T) {
	c := &Client{endpointHost: "pve.example.com"}
	reqURL, _ := url.Parse("https://evil.attacker.com:8006/api2/json/nodes")
	err := c.validateEndpoint(reqURL)
	if err == nil {
		t.Fatal("validateEndpoint: expected error for different host")
	}
	if !errors.Is(err, ErrEndpointMismatch) {
		t.Fatalf("validateEndpoint: error = %v, want ErrEndpointMismatch", err)
	}
	if !strings.Contains(err.Error(), "evil.attacker.com") {
		t.Fatalf("validateEndpoint: error should contain request host, got: %v", err)
	}
	if !strings.Contains(err.Error(), "pve.example.com") {
		t.Fatalf("validateEndpoint: error should contain configured host, got: %v", err)
	}
}

func TestValidateEndpoint_DifferentHost_SameBaseDomain(t *testing.T) {
	c := &Client{endpointHost: "pve.example.com"}
	reqURL, _ := url.Parse("https://admin.pve.example.com:8006/api2/json/nodes")
	err := c.validateEndpoint(reqURL)
	if err == nil {
		t.Fatal("validateEndpoint: expected error for subdomain mismatch")
	}
	if !errors.Is(err, ErrEndpointMismatch) {
		t.Fatalf("validateEndpoint: error = %v, want ErrEndpointMismatch", err)
	}
}

func TestValidateEndpoint_EmptyRequestHost(t *testing.T) {
	c := &Client{endpointHost: "pve.example.com"}
	reqURL := &url.URL{Scheme: "https", Path: "/api2/json"}
	err := c.validateEndpoint(reqURL)
	if err == nil {
		t.Fatal("validateEndpoint: expected error for empty request host")
	}
	if !errors.Is(err, ErrEndpointMismatch) {
		t.Fatalf("validateEndpoint: error = %v, want ErrEndpointMismatch", err)
	}
}

func TestValidateEndpoint_NilURL(t *testing.T) {
	c := &Client{endpointHost: "pve.example.com"}
	err := c.validateEndpoint(nil)
	if err == nil {
		t.Fatal("validateEndpoint: expected error for nil URL")
	}
	if !errors.Is(err, ErrEndpointMismatch) {
		t.Fatalf("validateEndpoint: error = %v, want ErrEndpointMismatch", err)
	}
}

func TestValidateEndpoint_EmptyEndpointHost_SkipsValidation(t *testing.T) {
	// Test-only clients constructed without New() have empty endpointHost;
	// validation is skipped so existing tests remain functional.
	c := &Client{endpointHost: ""}
	reqURL, _ := url.Parse("https://evil.attacker.com:8006/api2/json/nodes")
	if err := c.validateEndpoint(reqURL); err != nil {
		t.Fatalf("validateEndpoint: empty endpointHost should skip validation, got: %v", err)
	}
}

func TestValidateEndpoint_IPAddress_Matches(t *testing.T) {
	c := &Client{endpointHost: "192.168.1.100"}
	reqURL, _ := url.Parse("https://192.168.1.100:8006/api2/json/nodes")
	if err := c.validateEndpoint(reqURL); err != nil {
		t.Fatalf("validateEndpoint: IP address match should succeed, got: %v", err)
	}
}

func TestValidateEndpoint_IPAddress_Different(t *testing.T) {
	c := &Client{endpointHost: "192.168.1.100"}
	reqURL, _ := url.Parse("https://10.0.0.1:8006/api2/json/nodes")
	err := c.validateEndpoint(reqURL)
	if err == nil {
		t.Fatal("validateEndpoint: expected error for different IP address")
	}
	if !errors.Is(err, ErrEndpointMismatch) {
		t.Fatalf("validateEndpoint: error = %v, want ErrEndpointMismatch", err)
	}
}

func TestValidateEndpoint_IPv6_Matches(t *testing.T) {
	c := &Client{endpointHost: "::1"}
	reqURL, _ := url.Parse("https://[::1]:8006/api2/json/nodes")
	if err := c.validateEndpoint(reqURL); err != nil {
		t.Fatalf("validateEndpoint: IPv6 match should succeed, got: %v", err)
	}
}

func TestSendMutation_BlocksWrongHost(t *testing.T) {
	// Create a test server that should NOT receive any request.
	var received int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"ok"}`))
	}))
	defer s.Close()

	// Create a client pointing at a DIFFERENT endpoint host.
	// The test server's URL host won't match endpointHost.
	c := &Client{
		endpointHost: "wrong-host.example.com",
		baseURL:      s.URL,
		client:       httpclient.New(),
	}
	var result map[string]interface{}
	err := c.post(context.Background(), "/test", url.Values{}, &result)
	if err == nil {
		t.Fatal("sendMutation: expected error for wrong host")
	}
	if !errors.Is(err, ErrEndpointMismatch) {
		t.Fatalf("sendMutation: error = %v, want ErrEndpointMismatch", err)
	}
	if received > 0 {
		t.Fatalf("sendMutation: request reached server despite wrong host (received=%d)", received)
	}
}

func TestSendMutation_AllowsCorrectHost(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"ok"}`))
	}))
	defer s.Close()

	// Parse the test server URL to extract its host for endpointHost.
	sURL, _ := url.Parse(s.URL)
	c := &Client{
		endpointHost: sURL.Hostname(),
		baseURL:      s.URL,
		client:       httpclient.New(),
	}
	var result map[string]interface{}
	err := c.post(context.Background(), "/test", url.Values{}, &result)
	if err != nil {
		t.Fatalf("sendMutation: unexpected error for correct host: %v", err)
	}
}

func TestPutMutation_BlocksWrongHost(t *testing.T) {
	var received int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"ok"}`))
	}))
	defer s.Close()

	c := &Client{
		endpointHost: "wrong-host.example.com",
		baseURL:      s.URL,
		client:       httpclient.New(),
	}
	var result map[string]interface{}
	err := c.put(context.Background(), "/test", url.Values{}, &result)
	if err == nil {
		t.Fatal("sendMutation: expected error for wrong host")
	}
	if !errors.Is(err, ErrEndpointMismatch) {
		t.Fatalf("sendMutation: error = %v, want ErrEndpointMismatch", err)
	}
	if received > 0 {
		t.Fatalf("sendMutation: PUT request reached server despite wrong host")
	}
}

func TestDelMutation_BlocksWrongHost(t *testing.T) {
	var received int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"ok"}`))
	}))
	defer s.Close()

	c := &Client{
		endpointHost: "wrong-host.example.com",
		baseURL:      s.URL,
		client:       httpclient.New(),
	}
	var result map[string]interface{}
	err := c.del(context.Background(), "/test", &result)
	if err == nil {
		t.Fatal("sendMutation: expected error for wrong host")
	}
	if !errors.Is(err, ErrEndpointMismatch) {
		t.Fatalf("sendMutation: error = %v, want ErrEndpointMismatch", err)
	}
	if received > 0 {
		t.Fatalf("sendMutation: DELETE request reached server despite wrong host")
	}
}

func TestGet_NotBlockedByEndpointGuard(t *testing.T) {
	// GET requests should NOT be subject to endpoint validation.
	var received int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"version":"8.2"}}`))
	}))
	defer s.Close()

	// Client with a wrong endpointHost should still allow GET.
	c := &Client{
		endpointHost: "wrong-host.example.com",
		baseURL:      s.URL,
		client:       httpclient.New(),
	}
	var out VersionResponse
	err := c.get(context.Background(), "/version", &out)
	if err != nil {
		t.Fatalf("get: should not be blocked by endpoint guard, got: %v", err)
	}
	if received != 1 {
		t.Fatalf("get: expected request to reach server, received=%d", received)
	}
}

func TestNewClient_SetsEndpointHost(t *testing.T) {
	c, err := New("https://pve.example.com:8006", &domain.Credentials{
		TokenID:     "user@pam!tok",
		TokenSecret: "secret",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.endpointHost != "pve.example.com" {
		t.Fatalf("endpointHost = %q, want %q", c.endpointHost, "pve.example.com")
	}
}

func TestNewClient_IPAddress_EndpointHost(t *testing.T) {
	c, err := New("https://10.0.0.1:8006", &domain.Credentials{
		TokenID:     "user@pam!tok",
		TokenSecret: "secret",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.endpointHost != "10.0.0.1" {
		t.Fatalf("endpointHost = %q, want %q", c.endpointHost, "10.0.0.1")
	}
}
