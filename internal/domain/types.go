package domain

import "time"

// Node represents a physical or virtual machine host.
type Node struct {
	ID       string            `json:"id" yaml:"id"`
	Name     string            `json:"name" yaml:"name"`
	Status   string            `json:"status" yaml:"status"` // online, offline, unknown
	Role     string            `json:"role" yaml:"role"`     // node, storage
	IP       string            `json:"ip,omitempty" yaml:"ip,omitempty"`
	Platform string            `json:"platform" yaml:"platform"` // proxxmox, vmware, etc.
	Version  string            `json:"version,omitempty" yaml:"version,omitempty"`
	Uptime   *time.Duration    `json:"uptime,omitempty" yaml:"uptime,omitempty"`
	Labels   map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
}

// VM represents a virtual machine.
type VM struct {
	ID     string            `json:"id" yaml:"id"`
	Name   string            `json:"name" yaml:"name"`
	Status string            `json:"status" yaml:"status"` // running, stopped, paused
	Node   string            `json:"node" yaml:"node"`
	CPU    int               `json:"cpu" yaml:"cpu"`
	Memory int64             `json:"memory" yaml:"memory"` // bytes
	Disk   int64             `json:"disk" yaml:"disk"`     // bytes
	IP     string            `json:"ip,omitempty" yaml:"ip,omitempty"`
	OS     string            `json:"os,omitempty" yaml:"os,omitempty"`
	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
}

// Container represents a container (e.g., LXC).
type Container struct {
	ID     string            `json:"id" yaml:"id"`
	Name   string            `json:"name" yaml:"name"`
	Status string            `json:"status" yaml:"status"` // running, stopped, paused
	Node   string            `json:"node" yaml:"node"`
	OS     string            `json:"os,omitempty" yaml:"os,omitempty"`
	Memory int64             `json:"memory" yaml:"memory"` // bytes
	Disk   int64             `json:"disk" yaml:"disk"`     // bytes
	IP     string            `json:"ip,omitempty" yaml:"ip,omitempty"`
	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
}

// Storage represents a storage pool or device.
type Storage struct {
	ID      string            `json:"id" yaml:"id"`
	Name    string            `json:"name" yaml:"name"`
	Type    string            `json:"type" yaml:"type"`     // local, nfs, zfs, etc.
	Status  string            `json:"status" yaml:"status"` // active, inactive
	Node    string            `json:"node,omitempty" yaml:"node,omitempty"`
	Total   int64             `json:"total" yaml:"total"`                         // bytes
	Used    int64             `json:"used" yaml:"used"`                           // bytes
	Avail   int64             `json:"avail" yaml:"avail"`                         // bytes
	Content []string          `json:"content,omitempty" yaml:"content,omitempty"` // images, iso, backup, etc.
	Labels  map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
}

// Cluster represents a cluster of nodes.
type Cluster struct {
	Name    string `json:"name" yaml:"name"`
	Version string `json:"version" yaml:"version"`
	Nodes   int    `json:"nodes" yaml:"nodes"`
}

// StorageContentItem represents a single content item in storage.
type StorageContentItem struct {
	Content string `json:"content" yaml:"content"`
	Ctime   int    `json:"ctime,omitempty" yaml:"ctime,omitempty"`
	Format  string `json:"format,omitempty" yaml:"format,omitempty"`
	Volid   string `json:"volid,omitempty" yaml:"volid,omitempty"`
	Size    int64  `json:"size,omitempty" yaml:"size,omitempty"`
	Subtype string `json:"subtype,omitempty" yaml:"subtype,omitempty"`
	VMID    int    `json:"vmid,omitempty" yaml:"vmid,omitempty"`
}

// Task represents a Proxmox task.
type Task struct {
	UPID      string `json:"upid" yaml:"upid"`
	Type      string `json:"type" yaml:"type"`
	State     string `json:"state" yaml:"state"` // running, stopped
	StartTime int    `json:"starttime" yaml:"starttime"`
	EndTime   int    `json:"endtime,omitempty" yaml:"endtime,omitempty"`
	Status    string `json:"status,omitempty" yaml:"status,omitempty"`
	Node      string `json:"node,omitempty" yaml:"node,omitempty"`
}

// Snapshot represents a VM or container snapshot.
type Snapshot struct {
	Name   string `json:"name" yaml:"name"`
	VMID   int    `json:"vmid,omitempty" yaml:"vmid,omitempty"`
	Ctime  int    `json:"ctime,omitempty" yaml:"ctime,omitempty"`
	Parent string `json:"parent,omitempty" yaml:"parent,omitempty"`
	Node   string `json:"node,omitempty" yaml:"node,omitempty"`
	Target string `json:"target,omitempty" yaml:"target,omitempty"` // vm or container ID
}

// Event represents a cluster event.
type Event struct {
	Type    string `json:"type" yaml:"type"`
	Time    int64  `json:"time" yaml:"time"`
	Node    string `json:"node,omitempty" yaml:"node,omitempty"`
	ID      string `json:"id,omitempty" yaml:"id,omitempty"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
}

// SyslogEntry represents a syslog line.
type SyslogEntry struct {
	Time    int64  `json:"time" yaml:"time"`
	Node    string `json:"node,omitempty" yaml:"node,omitempty"`
	Level   string `json:"level,omitempty" yaml:"level,omitempty"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
}

// Backup represents a backup task.
type Backup struct {
	UPID      string `json:"upid" yaml:"upid"`
	Type      string `json:"type" yaml:"type"`
	State     string `json:"state" yaml:"state"`
	StartTime int    `json:"starttime" yaml:"starttime"`
	EndTime   int    `json:"endtime,omitempty" yaml:"endtime,omitempty"`
	Status    string `json:"status,omitempty" yaml:"status,omitempty"`
	Node      string `json:"node,omitempty" yaml:"node,omitempty"`
	Storage   string `json:"storage,omitempty" yaml:"storage,omitempty"`
}

// FirewallRule represents a firewall rule.
type FirewallRule struct {
	Type     string `json:"type" yaml:"type"`
	Action   string `json:"action" yaml:"action"`
	Enable   int    `json:"enable,omitempty" yaml:"enable,omitempty"`
	Pos      int    `json:"pos,omitempty" yaml:"pos,omitempty"`
	Proto    string `json:"proto,omitempty" yaml:"proto,omitempty"`
	Dest     string `json:"dest,omitempty" yaml:"dest,omitempty"`
	Dport    string `json:"dport,omitempty" yaml:"dport,omitempty"`
	Source   string `json:"source,omitempty" yaml:"source,omitempty"`
	Sport    string `json:"sport,omitempty" yaml:"sport,omitempty"`
	ICMPType string `json:"icmp_type,omitempty" yaml:"icmp_type,omitempty"`
	Log      string `json:"log,omitempty" yaml:"log,omitempty"`
	Comment  string `json:"comment,omitempty" yaml:"comment,omitempty"`
}

// HAResource represents an HA resource.
type HAResource struct {
	ID       string `json:"id" yaml:"id"`
	Type     string `json:"type" yaml:"type"`
	State    string `json:"state" yaml:"state"`
	Node     string `json:"node,omitempty" yaml:"node,omitempty"`
	Group    string `json:"group,omitempty" yaml:"group,omitempty"`
	MaxRelay int    `json:"max_relocate,omitempty" yaml:"max_relocate,omitempty"`
}

// HAGroup represents an HA group.
type HAGroup struct {
	ID         string `json:"id" yaml:"id"`
	Type       string `json:"type" yaml:"type"`
	Nodes      string `json:"nodes" yaml:"nodes"`
	Comment    string `json:"comment,omitempty" yaml:"comment,omitempty"`
	NoFailback int    `json:"nofailback,omitempty" yaml:"nofailback,omitempty"`
}
