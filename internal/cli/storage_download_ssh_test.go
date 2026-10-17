package cli

import (
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/config"
)

func TestStorageDownloadSSHUsesProfileBoundPVEInventory(t *testing.T) {
	cfg := storageDownloadTestConfig()
	got, found, err := storageDownloadSSHFromInventory(cfg, "test", "pve1")
	if err != nil || !found {
		t.Fatalf("storageDownloadSSHFromInventory() = found %v, err %v", found, err)
	}
	want := storageDownloadSSHSettings{
		Host: "10.0.0.10", User: "root", KeyFile: "/keys/pve1", KnownHostsFile: "/keys/known_hosts", Port: 2222,
	}
	if got != want {
		t.Fatalf("storageDownloadSSHFromInventory() = %+v, want %+v", got, want)
	}
}

func TestStorageDownloadSSHDoesNotUseUnboundInventory(t *testing.T) {
	cfg := storageDownloadTestConfig()
	got, found, err := storageDownloadSSHFromInventory(cfg, "test", "other-node")
	if err != nil || found || got != (storageDownloadSSHSettings{}) {
		t.Fatalf("unbound inventory fallback = %+v, found %v, err %v", got, found, err)
	}
}

func TestStorageDownloadSSHRejectsAmbiguousInventory(t *testing.T) {
	cfg := storageDownloadTestConfig()
	cfg.Inventory.Hosts["pve1-copy"] = cfg.Inventory.Hosts["pve1"]
	_, found, err := storageDownloadSSHFromInventory(cfg, "test", "pve1")
	if found || err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous inventory result: found %v, err %v", found, err)
	}
}

func TestStorageDownloadSSHPrefersProfileValues(t *testing.T) {
	cfg := storageDownloadTestConfig()
	cfg.Profiles["test"] = config.Profile{
		Provider: config.ProviderProxmox, SSHHost: "192.0.2.10", SSHUser: "operator", SSHPort: 2200,
	}
	got, found, err := storageDownloadSSHFromInventory(cfg, "test", "pve1")
	if err != nil || !found {
		t.Fatalf("storageDownloadSSHFromInventory() = found %v, err %v", found, err)
	}
	want := storageDownloadSSHSettings{
		Host: "192.0.2.10", User: "operator", KeyFile: "/keys/pve1", KnownHostsFile: "/keys/known_hosts", Port: 2200,
	}
	if got != want {
		t.Fatalf("storageDownloadSSHFromInventory() = %+v, want %+v", got, want)
	}
}

func storageDownloadTestConfig() *config.Config {
	return &config.Config{
		Profiles: map[string]config.Profile{
			"test": {Provider: config.ProviderProxmox},
		},
		Inventory: &config.Inventory{Hosts: map[string]config.InventoryHost{
			"pve1": {
				Address: "10.0.0.10", Role: config.RolePVE, PVENode: "pve1", PVEProfile: "test",
				SSHUser: "root", SSHPort: 2222, SSHKeyFile: "/keys/pve1", KnownHostsFile: "/keys/known_hosts",
			},
			"other": {
				Address: "10.0.0.20", Role: config.RolePVE, PVENode: "other-node", PVEProfile: "other",
				SSHUser: "root", SSHKeyFile: "/keys/other",
			},
		}},
	}
}
