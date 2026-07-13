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
