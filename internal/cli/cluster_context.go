package cli

import (
	"context"
	"strconv"

	"github.com/geoffmcc/nodex/internal/domain"
)

// clusterOnlyServices are services that only exist on a clustered Proxmox host.
// On a standalone node PVE still lists them, typically as "dead"/inactive, which
// reads as a broken service when the real explanation is that the host is not
// clustered at all (nit #28).
var clusterOnlyServices = map[string]bool{
	"corosync": true,
	"pmxcfs":   true,
}

// standaloneNote is the annotation shown for cluster-only services on a host
// that is not part of a cluster.
const standaloneNote = "not applicable (standalone host)"

// quorumInfo records what could be determined about cluster membership.
//
// Standalone and "could not find out" are deliberately distinct. Treating a
// failed or unsupported cluster query as "no cluster" would tell operators their
// healthy cluster does not exist, which is worse than the original misleading
// zero (nit #36).
type quorumInfo struct {
	// Standalone is true only when the provider successfully answered and
	// reported no cluster entry at all.
	Standalone bool
	// Known is true when a cluster entry with quorum data was reported.
	Known  bool
	Quorum int
	Name   string
}

// detectQuorum queries cluster status and classifies the result. A provider
// without cluster status support, or a failed query, yields the zero value:
// nothing is claimed about membership.
func detectQuorum(ctx context.Context, prov any) quorumInfo {
	cp, ok := prov.(domain.ClusterStatusProvider)
	if !ok {
		return quorumInfo{}
	}
	items, err := cp.ClusterStatuses(ctx)
	if err != nil {
		return quorumInfo{}
	}
	var info quorumInfo
	for _, item := range items {
		if item.Type != "cluster" {
			continue
		}
		info.Name = item.Name
		info.Quorum = item.Quorate
		info.Known = true
	}
	info.Standalone = !info.Known
	return info
}

// quorumDisplay renders the quorum value for human-facing output, keeping
// "not clustered" distinct from "could not determine".
func (q quorumInfo) quorumDisplay() string {
	switch {
	case q.Known:
		return strconv.Itoa(q.Quorum)
	case q.Standalone:
		return "n/a (standalone host)"
	default:
		return "unavailable"
	}
}

// annotateStandaloneServices marks cluster-only services as not applicable when
// the host is confirmed to be standalone. The reported state and active flag
// are left untouched so the raw provider values stay available.
func annotateStandaloneServices(services []domain.NodeService, q quorumInfo) {
	if !q.Standalone {
		return
	}
	for i := range services {
		if clusterOnlyServices[services[i].Name] {
			services[i].NotApplicable = true
			services[i].Note = standaloneNote
		}
	}
}
