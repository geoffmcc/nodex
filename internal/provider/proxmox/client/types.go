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
