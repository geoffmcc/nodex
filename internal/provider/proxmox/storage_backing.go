package proxmox

import (
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/provider/proxmox/client"
)

// storageBacking exposes the safe, useful parts of Proxmox's configured backend
// without claiming that a configured pool maps to a particular disk.
func storageBacking(cfg *client.StorageConfig, fallbackType string) *domain.StorageBacking {
	if cfg == nil {
		return nil
	}
	typ := cfg.Type
	if typ == "" {
		typ = fallbackType
	}
	b := &domain.StorageBacking{Type: typ}
	switch typ {
	case "dir":
		b.Path = cfg.Path
	case "lvmthin":
		b.VolumeGroup, b.ThinPool = cfg.VGName, cfg.Thinpool
	case "zfspool":
		b.ZFSPool = cfg.Pool
	default:
		return nil
	}
	if b.Path == "" && b.VolumeGroup == "" && b.ThinPool == "" && b.ZFSPool == "" {
		return nil
	}
	return b
}
