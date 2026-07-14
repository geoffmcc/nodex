// Package task provides Proxmox task UPID parsing and polling infrastructure.
package task

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultPollInterval is the initial polling interval.
	DefaultPollInterval = 500 * time.Millisecond

	// DefaultMaxInterval is the maximum polling interval after backoff.
	DefaultMaxInterval = 5 * time.Second

	// DefaultMaxWait is the maximum total time to wait for a task.
	DefaultMaxWait = 30 * time.Minute

	// DefaultBackoffFactor is the exponential backoff multiplier.
	DefaultBackoffFactor = 2.0
)

// UPID represents a parsed Proxmox task UPID.
type UPID struct {
	// Raw is the original UPID string.
	Raw string

	// Node is the originating node name.
	Node string

	// PID is the process ID (decimal).
	PID int

	// PStart is the process start time (hex epoch).
	PStart int64
}

// ParseUPID parses a Proxmox task UPID.
// Supports the standard Proxmox UPID format:
//
//	UPID:<node>:<hex_pid>:<hex_pstart>:<hex_serial>:<type>:<type_id>:<user>
//
// and the simplified test format:
//
//	UPID:<node>/<dec_pid>/<dec_pstart>
func ParseUPID(raw string) (*UPID, error) {
	if raw == "" {
		return nil, fmt.Errorf("invalid UPID: empty string")
	}
	if !strings.HasPrefix(raw, "UPID:") {
		return nil, fmt.Errorf("invalid UPID: missing UPID prefix")
	}
	upidStr := raw[5:] // strip "UPID:"

	// Try the full format first: node:pid:pstart:...
	if strings.Contains(upidStr, ":") {
		parts := strings.SplitN(upidStr, ":", 4)
		if len(parts) < 2 {
			return nil, fmt.Errorf("invalid UPID: malformed colon-separated parts in %q", raw)
		}
		node := parts[0]
		if node == "" {
			return nil, fmt.Errorf("invalid UPID: empty node in %q", raw)
		}
		pid, err := parseHex(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid UPID: cannot parse PID %q: %w", parts[1], err)
		}
		var pstart int64
		if len(parts) > 2 && parts[2] != "" {
			// parts[2] may contain additional colon-separated fields; extract just the hex.
			hexPart := strings.SplitN(parts[2], ":", 2)[0]
			pstart, err = parseHex(hexPart)
			if err != nil {
				return nil, fmt.Errorf("invalid UPID: cannot parse pstart %q: %w", hexPart, err)
			}
		}
		return &UPID{Raw: raw, Node: node, PID: int(pid), PStart: pstart}, nil
	}

	// Try the simplified test format: node/pid/pstart
	if strings.Contains(upidStr, "/") {
		parts := strings.SplitN(upidStr, "/", 3)
		if len(parts) < 2 {
			return nil, fmt.Errorf("invalid UPID: malformed slash-separated parts in %q", raw)
		}
		node := parts[0]
		if node == "" {
			return nil, fmt.Errorf("invalid UPID: empty node in %q", raw)
		}
		pid, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid UPID: cannot parse PID %q: %w", parts[1], err)
		}
		var pstart int64
		if len(parts) > 2 {
			pstart, err = strconv.ParseInt(parts[2], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid UPID: cannot parse pstart %q: %w", parts[2], err)
			}
		}
		return &UPID{Raw: raw, Node: node, PID: pid, PStart: pstart}, nil
	}

	return nil, fmt.Errorf("invalid UPID: unrecognized format %q", raw)
}

func parseHex(s string) (int64, error) {
	return strconv.ParseInt(s, 16, 64)
}

// State represents the state of a Proxmox task.
type State string

const (
	StateRunning State = "running"
	StateStopped State = "stopped"
)

// TaskStatus represents the current status of a task retrieved from the API.
type TaskStatus struct {
	UPID     string
	State    State
	Status   string // "OK" on success
	ExitCode string // error code on failure
}

// TaskStatusClient is the interface for querying task status.
type TaskStatusClient interface {
	GetTask(ctx context.Context, node, upid string) (*TaskStatus, error)
}

// TaskResult represents the final outcome of a task.
type TaskResult struct {
	UPID  string
	State State
	OK    bool  // true when status is "OK"
	Error error // set when task fails or polling times out
}

// Poller polls a Proxmox task until completion.
type Poller struct {
	client       TaskStatusClient
	pollInterval time.Duration
	maxInterval  time.Duration
	maxWait      time.Duration
	backoff      float64
	endpoint     string // Proxmox API endpoint, used in UnknownOutcome diagnostics
}

// PollerOption configures a Poller.
type PollerOption func(*Poller)

// WithPollInterval sets the initial polling interval.
func WithPollInterval(d time.Duration) PollerOption {
	return func(p *Poller) {
		p.pollInterval = d
	}
}

// WithMaxInterval sets the maximum backoff interval.
func WithMaxInterval(d time.Duration) PollerOption {
	return func(p *Poller) {
		p.maxInterval = d
	}
}

// WithMaxWait sets the maximum total wait time.
func WithMaxWait(d time.Duration) PollerOption {
	return func(p *Poller) {
		p.maxWait = d
	}
}

// WithEndpoint sets the Proxmox API endpoint for diagnostic context in
// UnknownOutcome errors. This is optional and used only for diagnostics.
func WithEndpoint(endpoint string) PollerOption {
	return func(p *Poller) {
		p.endpoint = endpoint
	}
}

// NewPoller creates a new task Poller.
func NewPoller(client TaskStatusClient, opts ...PollerOption) *Poller {
	p := &Poller{
		client:       client,
		pollInterval: DefaultPollInterval,
		maxInterval:  DefaultMaxInterval,
		maxWait:      DefaultMaxWait,
		backoff:      DefaultBackoffFactor,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Wait polls the task until completion, timeout, or context cancellation.
// Returns the final TaskResult.
//
// On deadline exceeded (poller maxWait or context deadline), the Error
// field contains an *UnknownOutcome: the task may still be running and
// the caller MUST NOT assume success or failure.
//
// On context cancellation, the Error field contains a plain cancellation
// error — the caller chose to stop waiting.
//
// The UPID is always preserved in the result so callers can follow up
// manually.
func (p *Poller) Wait(ctx context.Context, node, upid string) *TaskResult {
	deadline := time.Now().Add(p.maxWait)
	interval := p.pollInterval

	for {
		// Check overall timeout (poller's own maxWait).
		if time.Now().After(deadline) {
			return &TaskResult{
				UPID:  upid,
				Error: newUnknownOutcome(upid, node, p.endpoint),
			}
		}

		// Check context cancellation or deadline.
		select {
		case <-ctx.Done():
			return p.classifyCtxDone(upid, node, ctx)
		default:
		}

		// Query task status.
		status, err := p.client.GetTask(ctx, node, upid)
		if err != nil {
			// Transient failures: back off and retry.
			select {
			case <-ctx.Done():
				return p.classifyCtxDone(upid, node, ctx)
			case <-time.After(interval):
				interval = p.nextInterval(interval)
				continue
			}
		}

		if status.State == StateStopped {
			result := &TaskResult{
				UPID:  upid,
				State: StateStopped,
				OK:    status.Status == "OK",
			}
			if !result.OK {
				result.Error = fmt.Errorf("task %s failed with status %q", upid, status.Status)
			}
			return result
		}

		// Still running — wait and increase interval.
		select {
		case <-ctx.Done():
			return p.classifyCtxDone(upid, node, ctx)
		case <-time.After(interval):
			interval = p.nextInterval(interval)
		}
	}
}

// classifyCtxDone returns the appropriate error for a context Done signal.
// DeadlineExceeded produces an UnknownOutcome (the task may still be running);
// explicit cancellation produces a plain error (the caller chose to stop).
func (p *Poller) classifyCtxDone(upid, node string, ctx context.Context) *TaskResult {
	if ctx.Err() == context.DeadlineExceeded {
		return &TaskResult{
			UPID:  upid,
			Error: newUnknownOutcomeFromErr(upid, node, p.endpoint, ctx.Err()),
		}
	}
	return &TaskResult{
		UPID:  upid,
		Error: fmt.Errorf("task polling cancelled: %w", ctx.Err()),
	}
}

// nextInterval computes the next polling interval with exponential backoff.
func (p *Poller) nextInterval(current time.Duration) time.Duration {
	next := time.Duration(math.Min(float64(current)*p.backoff, float64(p.maxInterval)))
	if next < p.pollInterval {
		next = p.pollInterval
	}
	return next
}
