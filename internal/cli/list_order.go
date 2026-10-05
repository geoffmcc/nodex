package cli

import (
	"sort"
)

// A listing that is sliced by --limit must already be in a deterministic order,
// or --limit N is not a prefix of --limit M for M > N.
//
// Proxmox's /cluster/resources endpoint documents no ordering guarantee and in
// practice returns items in an order that varies between calls. NodeX used to
// hand that order straight through and then slice it, so raising --limit could
// both remove a row and re-add another. A caller paging by incrementing the
// limit skipped and repeated rows and could not detect it from the output.
//
// Sorting before slicing is what makes the limit a prefix. The ordering applied
// here is stable and total, so every call produces the same sequence.

// guestLess orders two "<node>/<vmid>" guest identifiers.
//
// The VMID is compared numerically rather than lexically: "proxmox/1000" sorts
// before "proxmox/101" numerically but after it lexically, so a lexical order
// would change again once a cluster passes VMID 999. An identifier that does not
// parse sorts after every one that does, and equal keys fall back to the raw
// string so the order is total and never depends on the input order.
func guestLess(a, b string) bool {
	nodeA, vmidA, errA := parseNodeVMID(a)
	nodeB, vmidB, errB := parseNodeVMID(b)
	switch {
	case errA != nil && errB != nil:
		return a < b
	case errA != nil:
		return false
	case errB != nil:
		return true
	}
	if nodeA != nodeB {
		return nodeA < nodeB
	}
	if vmidA != vmidB {
		return vmidA < vmidB
	}
	return a < b
}

// sortGuestsByID orders a guest listing by node, then by numeric VMID.
func sortGuestsByID[T any](items []T, id func(T) string) {
	sort.SliceStable(items, func(i, j int) bool {
		return guestLess(id(items[i]), id(items[j]))
	})
}

// sortByField orders a listing by the comparable key supplied by the caller.
// Callers with non-unique primary fields include stable identity fields in the
// key as tie-breakers.
func sortByField[T any, K cmpOrdered](items []T, field func(T) K) {
	sort.SliceStable(items, func(i, j int) bool {
		return field(items[i]) < field(items[j])
	})
}

// cmpOrdered is the set of types sortByField can compare.
type cmpOrdered interface {
	~int | ~int64 | ~string | ~float64
}
