package proxmox

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/netip"
	"regexp"
	"strconv"
	"strings"

	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/provider/proxmox/client"
)

// Length-delimited encoding prevents delimiter collisions. No endpoint,
// credentials, or raw config is exposed in a capacity key.
func capacityKey(parts ...string) string {
	data, _ := json.Marshal(parts)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

var storageComponent = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)
var dnsLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

func pbsCapacityIdentity(cfg *client.StorageConfig, err error) *domain.StorageCapacityIdentity {
	id := &domain.StorageCapacityIdentity{Scope: "unknown", Evidence: "none"}
	if err != nil {
		id.Reason = destinationFailureReason(err)
		return id
	}
	if cfg == nil {
		id.Reason = "config_not_visible_or_ambiguous"
		return id
	}
	if cfg.Disable != nil {
		enabled := *cfg.Disable == 0
		id.Enabled = &enabled
	}
	if cfg.Namespace != "" {
		for _, part := range strings.Split(cfg.Namespace, "/") {
			if !storageComponent.MatchString(part) || part == "." || part == ".." {
				id.Reason = "invalid_namespace"
				return id
			}
		}
	}
	id.Namespace = cfg.Namespace
	if cfg.Server == "" || cfg.Datastore == "" {
		id.Reason = "missing_identity_fields"
		return id
	}
	if cfg.Type != "" && cfg.Type != "pbs" {
		id.Reason = "conflicting_storage_type"
		return id
	}
	host, ok := capacityHost(cfg.Server)
	port := cfg.Port
	if port == 0 {
		port = 8007
	}
	if !ok || port < 1 || port > 65535 || !storageComponent.MatchString(cfg.Datastore) {
		id.Reason = "invalid_or_node_relative_endpoint"
		return id
	}
	id.Key = capacityKey("pbs-datastore", host, strconv.Itoa(port), cfg.Datastore)
	id.Scope = "pbs_datastore"
	id.Evidence = "pve_storage_config_server_port_datastore"
	return id
}

// Normalize spelling only. DNS aliases, certificates, and equal usage do not
// establish server identity. Loopback/link-local and single-label names depend
// on the querying node and cannot establish a cluster-wide relationship.
func capacityHost(host string) (string, bool) {
	if addr, err := netip.ParseAddr(host); err == nil {
		addr = addr.Unmap()
		if addr.Zone() != "" || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsUnspecified() || addr.IsMulticast() {
			return "", false
		}
		return addr.String(), true
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if len(host) > 253 || !strings.Contains(host, ".") || strings.HasSuffix(host, ".localhost") {
		return "", false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) > 63 || !dnsLabel.MatchString(label) {
			return "", false
		}
	}
	return host, true
}
