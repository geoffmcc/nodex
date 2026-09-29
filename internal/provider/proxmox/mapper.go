package proxmox

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/provider/proxmox/client"
)

// MapNode converts a client.NodeItem to a domain.Node.
func MapNode(item client.NodeItem) domain.Node {
	name := item.Node
	if name == "" {
		name = item.Name
	}
	id := item.ID
	if id == "" {
		id = name
	}
	var uptime *time.Duration
	if item.Uptime != nil {
		d := time.Duration(*item.Uptime) * time.Second
		uptime = &d
	}
	return domain.Node{
		ID:       id,
		Name:     name,
		Status:   item.Status,
		Role:     item.Type,
		IP:       item.IP,
		Platform: "proxmox",
		Version:  "",
		Uptime:   uptime,
	}
}

// MapNodes converts a slice of client.NodeItem to domain.Node.
func MapNodes(items []client.NodeItem) []domain.Node {
	nodes := make([]domain.Node, 0, len(items))
	for _, item := range items {
		nodes = append(nodes, MapNode(item))
	}
	return nodes
}

func enrichNodeIPs(nodes []domain.Node, status []client.ClusterStatusItem) {
	ips := make(map[string]string, len(status))
	for _, item := range status {
		if item.Type != "node" || item.IP == "" {
			continue
		}
		name := item.Name
		if name == "" {
			name = strings.TrimPrefix(item.ID, "node/")
		}
		if name != "" {
			ips[name] = item.IP
		}
	}
	for i := range nodes {
		if nodes[i].IP == "" {
			nodes[i].IP = ips[nodes[i].Name]
		}
	}
}

// MapVM converts a client.ClusterResource to a domain.VM.
func MapVM(res client.ClusterResource) domain.VM {
	return domain.VM{
		ID:       vmID(res),
		Name:     res.Name,
		Status:   res.Status,
		Node:     res.Node,
		CPU:      res.MaxCPU,
		Memory:   res.MaxMem,
		Disk:     res.MaxDisk,
		Template: res.Template != 0,
		IP:       res.IP,
	}
}

// MapContainer converts a client.ClusterResource to a domain.Container.
func MapContainer(res client.ClusterResource) domain.Container {
	return domain.Container{
		ID:     vmID(res),
		Name:   res.Name,
		Status: res.Status,
		Node:   res.Node,
		CPU:    res.MaxCPU,
		Memory: res.MaxMem,
		Disk:   res.MaxDisk,
		IP:     res.IP,
	}
}

// MapStorage converts a client.ClusterResource to a domain.Storage.
func MapStorage(res client.ClusterResource) domain.Storage {
	name := res.Name
	if name == "" {
		name = res.Storage
	}
	return domain.Storage{
		ID:      res.ID,
		Name:    name,
		Type:    res.Type,
		Status:  res.Status,
		Node:    res.Node,
		Total:   res.MaxDisk,
		Used:    res.Disk,
		Avail:   res.MaxDisk - res.Disk,
		Content: splitContent(res.Content),
	}
}

// MapCluster converts version data to a domain.Cluster.
func MapCluster(version *client.VersionData, nodeCount int, name string) *domain.Cluster {
	return &domain.Cluster{
		Name:    name,
		Version: version.Version,
		Nodes:   nodeCount,
	}
}

// MapClusterStatus converts cluster status items to a domain.Cluster.
func MapClusterStatus(items []client.ClusterStatusItem) *domain.Cluster {
	cluster := &domain.Cluster{}
	nodeCount := 0
	for _, item := range items {
		if item.Type == "cluster" {
			cluster.Name = item.Name
			if item.Version > 0 {
				cluster.Version = strconv.Itoa(item.Version)
			}
		}
		if item.Type == "node" {
			nodeCount++
		}
	}
	cluster.Nodes = nodeCount
	return cluster
}

func vmID(res client.ClusterResource) string {
	return fmt.Sprintf("%s/%d", res.Node, res.VMID)
}

// splitContent parses a storage content list into a canonical slice.
//
// PVE builds the comma-joined string from unordered hash iteration, so the
// same unchanged state returns a different order on every call. Sorting
// normalises that at the provider boundary, so repeated `nodex --output json`
// calls stay byte-comparable. The field is an unordered set of content types
// semantically, so this is lossless; no consumer can depend on PVE's ordering
// because PVE does not provide one.
func splitContent(content string) []string {
	if content == "" {
		return nil
	}
	out := strings.Split(content, ",")
	sort.Strings(out)
	return out
}

// MapNodeStatus converts a client.NodeStatusData to a domain.Node with extended status.
func MapNodeStatus(status *client.NodeStatusData) domain.Node {
	name := status.Node
	if name == "" {
		name = status.ID
	}
	id := status.ID
	if id == "" {
		id = name
	}
	var uptime *time.Duration
	if status.Uptime > 0 {
		d := time.Duration(status.Uptime) * time.Second
		uptime = &d
	}
	return domain.Node{
		ID:       id,
		Name:     name,
		Status:   status.Status,
		Role:     status.Type,
		IP:       "",
		Platform: "proxmox",
		Version:  status.PVEVersion,
		Uptime:   uptime,
	}
}
