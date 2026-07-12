package domain

import "time"

// Node represents a physical or virtual machine host.
type Node struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Status   string            `json:"status"` // online, offline, unknown
	Role     string            `json:"role"`   // node, storage
	IP       string            `json:"ip"`
	Platform string            `json:"platform"` // proxxmox, vmware, etc.
	Version  string            `json:"version"`
	Uptime   time.Duration     `json:"uptime"`
	Labels   map[string]string `json:"labels,omitempty"`
}

// VM represents a virtual machine.
type VM struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Status string            `json:"status"` // running, stopped, paused
	Node   string            `json:"node"`
	CPU    int               `json:"cpu"`
	Memory int64             `json:"memory"` // bytes
	Disk   int64             `json:"disk"`   // bytes
	IP     string            `json:"ip,omitempty"`
	OS     string            `json:"os,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
}

// Container represents a container (e.g., LXC).
type Container struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Status string            `json:"status"` // running, stopped, paused
	Node   string            `json:"node"`
	OS     string            `json:"os,omitempty"`
	Memory int64             `json:"memory"` // bytes
	Disk   int64             `json:"disk"`   // bytes
	IP     string            `json:"ip,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
}

// Storage represents a storage pool or device.
type Storage struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Type    string            `json:"type"`   // local, nfs, zfs, etc.
	Status  string            `json:"status"` // active, inactive
	Node    string            `json:"node,omitempty"`
	Total   int64             `json:"total"`             // bytes
	Used    int64             `json:"used"`              // bytes
	Avail   int64             `json:"avail"`             // bytes
	Content []string          `json:"content,omitempty"` // images, iso, backup, etc.
	Labels  map[string]string `json:"labels,omitempty"`
}

// Cluster represents a cluster of nodes.
type Cluster struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Nodes   int    `json:"nodes"`
}
