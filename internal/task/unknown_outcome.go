package task

import (
	"context"
	"errors"
	"fmt"
)

// UnknownOutcome is returned when task polling ends without determining
// the task's final state. This occurs when the poller's own deadline,
// the caller's context deadline, or any other deadline is exceeded while
// the task may still be running on the Proxmox host.
//
// Callers MUST NOT treat an UnknownOutcome as success or safe failure.
// The task's actual final state is indeterminate and must be investigated
// manually (e.g. by inspecting the task log on the Proxmox node).
//
// UnknownOutcome wraps context.DeadlineExceeded so that existing
// IsTimeoutError checks continue to work, but callers should prefer
// IsUnknownOutcome for a precise check.
type UnknownOutcome struct {
	// UPID is the raw Proxmox task UPID that was being polled.
	UPID string

	// Node is the Proxmox node that owns the task.
	Node string

	// Endpoint is the Proxmox API endpoint that was queried (optional).
	// May be empty when the poller does not have this information.
	Endpoint string

	// Cause is the underlying deadline-exceeded error.
	// Always non-nil; typically context.DeadlineExceeded.
	Cause error
}

// Error returns a human-readable description of the unknown outcome.
func (u *UnknownOutcome) Error() string {
	if u.Endpoint != "" {
		return fmt.Sprintf(
			"unknown outcome: task %s on node %s (endpoint %s) may still be running: %s",
			u.UPID, u.Node, u.Endpoint, u.Cause.Error(),
		)
	}
	return fmt.Sprintf(
		"unknown outcome: task %s on node %s may still be running: %s",
		u.UPID, u.Node, u.Cause.Error(),
	)
}

// Unwrap returns the underlying cause for errors.Is / errors.As chain traversal.
func (u *UnknownOutcome) Unwrap() error {
	return u.Cause
}

// IsUnknownOutcome reports whether err is or wraps an *UnknownOutcome.
func IsUnknownOutcome(err error) bool {
	var uo *UnknownOutcome
	return errors.As(err, &uo)
}

// AsUnknownOutcome extracts the *UnknownOutcome from err if present.
// Returns nil if err does not contain an *UnknownOutcome in its chain.
func AsUnknownOutcome(err error) *UnknownOutcome {
	var uo *UnknownOutcome
	if errors.As(err, &uo) {
		return uo
	}
	return nil
}

// newUnknownOutcome creates an UnknownOutcome for a deadline exceeded during
// task polling. The cause is always set to context.DeadlineExceeded.
func newUnknownOutcome(upid, node, endpoint string) *UnknownOutcome {
	return &UnknownOutcome{
		UPID:     upid,
		Node:     node,
		Endpoint: endpoint,
		Cause:    context.DeadlineExceeded,
	}
}

// newUnknownOutcomeFromErr creates an UnknownOutcome from an existing error
// (typically ctx.Err() when it is context.DeadlineExceeded).
func newUnknownOutcomeFromErr(upid, node, endpoint string, cause error) *UnknownOutcome {
	return &UnknownOutcome{
		UPID:     upid,
		Node:     node,
		Endpoint: endpoint,
		Cause:    cause,
	}
}
