package app

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

// TestNotFoundErrorExitCode pins §3. A client-side lookup miss used to be
// tagged ExitProvider, so `node show <bad>` exited 12 — the code documented as
// "provider error" — for what is a missing resource. It must be 13.
func TestNotFoundErrorExitCode(t *testing.T) {
	err := NotFoundError("node %q", "no-such-node")
	if got := ExitCodeFromError(err); got != ExitNotFound {
		t.Errorf("ExitCodeFromError = %d, want ExitNotFound(%d)", got, ExitNotFound)
	}
}

// TestNotFoundErrorIsMatchableStructurally is the point of the sentinel: a
// caller that wants to know "does this resource not exist?" must be able to
// ask with errors.Is instead of pattern-matching a message string.
func TestNotFoundErrorIsMatchableStructurally(t *testing.T) {
	if !errors.Is(NotFoundError("VM %q", "proxmox/999999"), ErrNotFound) {
		t.Error("errors.Is(NotFoundError(...), ErrNotFound) = false, want true")
	}
	if !IsNotFoundError(NotFoundError("storage %q", "nfs-old")) {
		t.Error("IsNotFoundError = false, want true")
	}
	// Wrapping in more context must not break the match.
	wrapped := fmt.Errorf("show container: %w", NotFoundError("container %q", "proxmox/999999"))
	if !errors.Is(wrapped, ErrNotFound) {
		t.Error("errors.Is through an added context wrap = false, want true")
	}
}

// TestNotFoundErrorMessageDoesNotRepeatItself guards the message shape. The
// sentinel supplies "not found", so a detail that already said it would read
// "node \"x\" not found: not found".
func TestNotFoundErrorMessageDoesNotRepeatItself(t *testing.T) {
	msg := NotFoundError("node %q", "pve1").Error()
	if msg != `node "pve1": not found` {
		t.Errorf("message = %q, want %q", msg, `node "pve1": not found`)
	}
	if count := occurrences(msg, "not found"); count != 1 {
		t.Errorf("message %q contains %d occurrences of %q, want 1", msg, count, "not found")
	}
}

// TestIsNotFoundErrorStillMatchesProvider404 guards against regressing the
// original behaviour while adding the sentinel.
func TestIsNotFoundErrorStillMatchesProvider404(t *testing.T) {
	pe := NewProviderError(http.StatusNotFound, "no such VM", nil)
	if !IsNotFoundError(pe) {
		t.Error("provider 404 no longer recognised as not-found")
	}
	if got := ExitCodeFromError(pe); got != ExitNotFound {
		t.Errorf("ExitCodeFromError(404) = %d, want ExitNotFound(%d)", got, ExitNotFound)
	}
	// A different status must not be swept up by the sentinel path.
	other := NewProviderError(http.StatusForbidden, "denied", nil)
	if IsNotFoundError(other) {
		t.Error("provider 403 wrongly classified as not-found")
	}
	if got := ExitCodeFromError(other); got != ExitAuthorization {
		t.Errorf("ExitCodeFromError(403) = %d, want ExitAuthorization(%d)", got, ExitAuthorization)
	}
}

// TestNotFoundErrorIsNotConfusedWithProfileErrors guards the interaction with
// the pre-existing profile sentinels, which are a different failure class and
// keep their own exit code.
func TestNotFoundErrorIsNotConfusedWithProfileErrors(t *testing.T) {
	if IsNotFoundError(ErrProfileNotFound) {
		t.Error("ErrProfileNotFound must not be reclassified as a resource not-found")
	}
}

// TestSameOutcomeGetsSameCodeFromEveryCommandPath is the actual complaint in
// §3: a missing container was 13 via container_os_update and 12 via
// resource.go. Both paths now agree, and both are matchable.
func TestSameOutcomeGetsSameCodeFromEveryCommandPath(t *testing.T) {
	paths := map[string]error{
		"resource.go container show":  NotFoundError("container %q", "proxmox/999999"),
		"container_os_update":         NotFoundError("container %q", "proxmox/999999"),
		"resource.go node show":       NotFoundError("node %q", "no-such-node"),
		"resource.go vm show":         NotFoundError("VM %q", "proxmox/999999"),
		"resource.go storage show":    NotFoundError("storage %q", "nfs-old"),
		"ceph_sdn replication detail": NotFoundError("replication job"),
		"monitor check":               NotFoundError("monitor target %q", "nfs-old"),
		"pbs sync job":                NotFoundError("sync job %q", "abc"),
		"pbs prune job":               NotFoundError("prune job %q", "abc"),
	}
	for name, err := range paths {
		if got := ExitCodeFromError(err); got != ExitNotFound {
			t.Errorf("%s: exit = %d, want %d", name, got, ExitNotFound)
		}
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: not matchable with errors.Is(err, ErrNotFound)", name)
		}
	}
}

func occurrences(haystack, needle string) int {
	n := 0
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			n++
		}
	}
	return n
}
