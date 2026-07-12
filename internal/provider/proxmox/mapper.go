package proxmox

import (
	"fmt"
	"strings"
	"time"

	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/provider/proxmox/client"
)

// MapNode converts a client.NodeItem to a domain.Node.
func MapNode(item client.NodeItem) domain.Node {
	return domain.Node{
		ID:       item.Name,
		Name:     item.Name,
		Status:   item.Status,
		Role:     item.Type,
		IP:       item.IP,
		Platform: "proxmox",
		Version:  "",
		Uptime:   time.Duration(item.Uptime) * time.Second,
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

// MapVM converts a client.ClusterResource to a domain.VM.
func MapVM(res client.ClusterResource) domain.VM {
	return domain.VM{
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

// MapContainer converts a client.ClusterResource to a domain.Container.
func MapContainer(res client.ClusterResource) domain.Container {
	return domain.Container{
		ID:     vmID(res),
		Name:   res.Name,
		Status: res.Status,
		Node:   res.Node,
		Memory: res.MaxMem,
		Disk:   res.MaxDisk,
		IP:     res.IP,
	}
}

// MapStorage converts a client.ClusterResource to a domain.Storage.
func MapStorage(res client.ClusterResource) domain.Storage {
	return domain.Storage{
		ID:      res.ID,
		Name:    res.Name,
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
func MapCluster(version *client.VersionData, nodeCount int) *domain.Cluster {
	return &domain.Cluster{
		Name:    "",
		Version: version.Version,
		Nodes:   nodeCount,
	}
}

func vmID(res client.ClusterResource) string {
	return fmt.Sprintf("%s/%d", res.Node, res.VMID)
}

func splitContent(content string) []string {
	if content == "" {
		return nil
	}
	return strings.Split(content, ",")
}
