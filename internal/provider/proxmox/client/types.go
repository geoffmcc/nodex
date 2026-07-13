package client

import "encoding/json"

// APIResponse wraps a Proxmox API response.
type APIResponse struct {
	Data json.RawMessage `json:"data"`
}

// NodeListResponse is the response from /nodes.
type NodeListResponse struct {
	Data []NodeItem `json:"data"`
}

// NodeItem represents a single node from the API.
type NodeItem struct {
	Status  string  `json:"status"`
	Maxmem  int64   `json:"maxmem"`
	CPU     float64 `json:"cpu"`
	Maxcpu  int     `json:"maxcpu"`
	Uptime  *int    `json:"uptime,omitempty"`
	Node    string  `json:"node"`
	Name    string  `json:"name"`
	ID      string  `json:"id"`
	Level   string  `json:"level"`
	Mem     int64   `json:"mem"`
	Disk    int64   `json:"disk"`
	Maxdisk int64   `json:"maxdisk"`
	Type    string  `json:"type"`
	IP      string  `json:"ip"`
}

// ClusterResourcesResponse is the response from /cluster/resources.
type ClusterResourcesResponse struct {
	Data []ClusterResource `json:"data"`
}

// ClusterResource represents a single resource from the cluster.
type ClusterResource struct {
	ID        string  `json:"id"`
	Type      string  `json:"type"`
	Status    string  `json:"status"`
	Name      string  `json:"name"`
	Node      string  `json:"node"`
	CPU       float64 `json:"cpu,omitempty"`
	MaxCPU    int     `json:"maxcpu,omitempty"`
	Mem       int64   `json:"mem,omitempty"`
	MaxMem    int64   `json:"maxmem,omitempty"`
	Disk      int64   `json:"disk,omitempty"`
	MaxDisk   int64   `json:"maxdisk,omitempty"`
	IP        string  `json:"ip,omitempty"`
	Template  int     `json:"template,omitempty"`
	VMID      int     `json:"vmid,omitempty"`
	Storage   string  `json:"storage,omitempty"`
	Content   string  `json:"content,omitempty"`
	MaxAge    int     `json:"maxage,omitempty"`
	Shared    int     `json:"shared,omitempty"`
	Heartbeat int     `json:"heartbeat,omitempty"`
	Tags      string  `json:"tags,omitempty"`
	StartTime int     `json:"uptime,omitempty"`
}

// VersionResponse is the response from /version.
type VersionResponse struct {
	Data VersionData `json:"data"`
}

// VersionData holds version information.
type VersionData struct {
	Release string `json:"release"`
	Version string `json:"version"`
	Repoid  string `json:"repoid"`
}

// NodeList is a convenience alias.
type NodeList = NodeListResponse

// ClusterResources is a convenience alias.
type ClusterResources = ClusterResourcesResponse

// NodeStatusResponse is the response from /nodes/{node}/status.
type NodeStatusResponse struct {
	Data NodeStatusData `json:"data"`
}

// NodeStatusData holds detailed node status information.
type NodeStatusData struct {
	CPU            float64   `json:"cpu"`
	MaxCPU         int       `json:"maxcpu"`
	Mem            int64     `json:"mem"`
	MaxMem         int64     `json:"maxmem"`
	Disk           int64     `json:"disk"`
	MaxDisk        int64     `json:"maxdisk"`
	Uptime         int       `json:"uptime"`
	Level          string    `json:"level"`
	SSLFingerprint string    `json:"ssl_fingerprint,omitempty"`
	ID             string    `json:"id"`
	Node           string    `json:"node"`
	Type           string    `json:"type"`
	Status         string    `json:"status"`
	KVersion       string    `json:"kversion,omitempty"`
	PVEVersion     string    `json:"pveversion,omitempty"`
	LoadAvg        []float64 `json:"loadavg,omitempty"`
	Wait           float64   `json:"wait,omitempty"`
	Ksm            int       `json:"ksm,omitempty"`
	Numa           int       `json:"numa,omitempty"`
	IOMax          float64   `json:"io,omitempty"`
}

// NodeStatus is a convenience alias.
type NodeStatus = NodeStatusResponse

// Version is a convenience alias.
type Version = VersionResponse

// ClusterStatusResponse is the response from /cluster/status.
type ClusterStatusResponse struct {
	Data []ClusterStatusItem `json:"data"`
}

// ClusterStatusItem represents a single item from the cluster status API.
type ClusterStatusItem struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Level     string `json:"level,omitempty"`
	IP        string `json:"ip,omitempty"`
	Localmem  int64  `json:"localmem,omitempty"`
	Maxmem    int64  `json:"maxmem,omitempty"`
	Localdisk int64  `json:"localdisk,omitempty"`
	Maxdisk   int64  `json:"maxdisk,omitempty"`
	Quorate   int    `json:"quorate,omitempty"`
	Version   int    `json:"version,omitempty"`
	Commit    string `json:"commit,omitempty"`
}

// ClusterStatus is a convenience alias.
type ClusterStatus = ClusterStatusResponse

// VMConfigResponse is the response from /nodes/{node}/qemu/{vmid}/config.
type VMConfigResponse struct {
	Data VMConfigData `json:"data"`
}

// VMConfigData holds VM configuration information.
type VMConfigData struct {
	VMID        int               `json:"vmid"`
	Name        string            `json:"name,omitempty"`
	CPU         int               `json:"cores,omitempty"`
	Memory      int               `json:"memory,omitempty"`
	Net0        string            `json:"net0,omitempty"`
	Scsi0       string            `json:"scsi0,omitempty"`
	Boot        string            `json:"boot,omitempty"`
	OnBoot      int               `json:"onboot,omitempty"`
	Agent       int               `json:"agent,omitempty"`
	SMBIOS1     string            `json:"smbios1,omitempty"`
	Numa        int               `json:"numa,omitempty"`
	OSType      string            `json:"ostype,omitempty"`
	Description string            `json:"description,omitempty"`
	Protection  int               `json:"protection,omitempty"`
	Tags        string            `json:"tags,omitempty"`
	VMGenID     string            `json:"vmgenid,omitempty"`
	Args        string            `json:"args,omitempty"`
	Bios        string            `json:"bios,omitempty"`
	IDE2        string            `json:"ide2,omitempty"`
	ScsiHW      string            `json:"scsihw,omitempty"`
	Unused0     string            `json:"unused0,omitempty"`
	Raw         map[string]string `json:"raw,omitempty"`
}

// VMConfig is a convenience alias.
type VMConfig = VMConfigResponse

// ContainerConfigResponse is the response from /nodes/{node}/lxc/{vmid}/config.
type ContainerConfigResponse struct {
	Data ContainerConfigData `json:"data"`
}

// ContainerConfigData holds container configuration information.
type ContainerConfigData struct {
	VMID         int               `json:"vmid"`
	Hostname     string            `json:"hostname,omitempty"`
	CPU          int               `json:"cores,omitempty"`
	Memory       int               `json:"memory,omitempty"`
	Swap         int               `json:"swap,omitempty"`
	Rootfs       string            `json:"rootfs,omitempty"`
	MP0          string            `json:"mp0,omitempty"`
	Net0         string            `json:"net0,omitempty"`
	OnBoot       int               `json:"onboot,omitempty"`
	OSType       string            `json:"ostype,omitempty"`
	Description  string            `json:"description,omitempty"`
	Protection   int               `json:"protection,omitempty"`
	Tags         string            `json:"tags,omitempty"`
	Unfiltered   int               `json:"unfiltered,omitempty"`
	Features     string            `json:"features,omitempty"`
	Architecture string            `json:"architecture,omitempty"`
	Nameserver   string            `json:"nameserver,omitempty"`
	SearchDomain string            `json:"searchdomain,omitempty"`
	Dev0         string            `json:"dev0,omitempty"`
	Fstab        string            `json:"fstab,omitempty"`
	Hookscript   string            `json:"hookscript,omitempty"`
	Raw          map[string]string `json:"raw,omitempty"`
}

// ContainerConfig is a convenience alias.
type ContainerConfig = ContainerConfigResponse

// StorageContentResponse is the response from /nodes/{node}/storage/{storage}/content.
type StorageContentResponse struct {
	Data []StorageContentItem `json:"data"`
}

// StorageContentItem represents a single content item in storage.
type StorageContentItem struct {
	Content string `json:"content"`
	Ctime   int    `json:"ctime,omitempty"`
	Format  string `json:"format,omitempty"`
	Volid   string `json:"volid,omitempty"`
	Size    int64  `json:"size,omitempty"`
	Subtype string `json:"subtype,omitempty"`
	VMID    int    `json:"vmid,omitempty"`
	Store   string `json:"store,omitempty"`
	Checked int    `json:"checked,omitempty"`
	Encrypt string `json:"encrypt,omitempty"`
	Source  string `json:"source,omitempty"`
}

// StorageContent is a convenience alias.
type StorageContent = StorageContentResponse
