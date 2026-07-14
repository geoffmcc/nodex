package task

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestParseUPIDFullFormat(t *testing.T) {
	u, err := ParseUPID("UPID:pve1:00000A1B:0023A45B:6789ABCD:vzdump:100:root@pam:")
	if err != nil {
		t.Fatalf("ParseUPID: %v", err)
	}
	if u.Node != "pve1" {
		t.Errorf("Node = %q, want pve1", u.Node)
	}
	if u.PID != 2587 { // 0xA1B == 2587
		t.Errorf("PID = %d, want 2587", u.PID)
	}
	if u.PStart != 2335835 { // 0x23A45B == 2335835
		t.Errorf("PStart = %d, want 2335851", u.PStart)
	}
}

func TestParseUPIDColonFormat(t *testing.T) {
	// Minimal colon format: UPID:node:pid
	u, err := ParseUPID("UPID:proxmox:00012345")
	if err != nil {
		t.Fatalf("ParseUPID: %v", err)
	}
	if u.Node != "proxmox" {
		t.Errorf("Node = %q, want proxmox", u.Node)
	}
	if u.PID != 74565 { // 0x12345
		t.Errorf("PID = %d, want 74565", u.PID)
	}
}

func TestParseUPIDSlashFormat(t *testing.T) {
	u, err := ParseUPID("UPID:proxmox/00012345/0")
	if err != nil {
		t.Fatalf("ParseUPID: %v", err)
	}
	if u.Node != "proxmox" {
		t.Errorf("Node = %q, want proxmox", u.Node)
	}
	if u.PID != 12345 {
		t.Errorf("PID = %d, want 12345", u.PID)
	}
	if u.PStart != 0 {
		t.Errorf("PStart = %d, want 0", u.PStart)
	}
}

func TestParseUPIDSlashFormatWithPStart(t *testing.T) {
	u, err := ParseUPID("UPID:pve1/100/1700000000")
	if err != nil {
		t.Fatalf("ParseUPID: %v", err)
	}
	if u.Node != "pve1" {
		t.Errorf("Node = %q, want pve1", u.Node)
	}
	if u.PID != 100 {
		t.Errorf("PID = %d, want 100", u.PID)
	}
	if u.PStart != 1700000000 {
		t.Errorf("PStart = %d, want 1700000000", u.PStart)
	}
}

func TestParseUPIDRejectsEmpty(t *testing.T) {
	_, err := ParseUPID("")
	if err == nil {
		t.Fatal("expected error for empty UPID")
	}
}

func TestParseUPIDRejectsNoPrefix(t *testing.T) {
	_, err := ParseUPID("pve1:00012345:0")
	if err == nil {
		t.Fatal("expected error for missing UPID prefix")
	}
}

func TestParseUPIDRejectsEmptyNode(t *testing.T) {
	_, err := ParseUPID("UPID::00012345")
	if err == nil {
		t.Fatal("expected error for empty node")
	}
}

func TestParseUPIDRejectsMalformedPID(t *testing.T) {
	_, err := ParseUPID("UPID:pve1:nothex")
	if err == nil {
		t.Fatal("expected error for non-hex PID")
	}
}

func TestParseUPIDPreservesRaw(t *testing.T) {
	raw := "UPID:pve1:00000A1B:0023A45B:6789ABCD:vzdump:100:root@pam:"
	u, err := ParseUPID(raw)
	if err != nil {
		t.Fatalf("ParseUPID: %v", err)
	}
	if u.Raw != raw {
		t.Errorf("Raw = %q, want %q", u.Raw, raw)
	}
}

// --- Poller tests ---

type mockTaskClient struct {
	statuses []*TaskStatus
	index    int
	failLeft int // number of remaining failures to inject before serving data
}

func (m *mockTaskClient) GetTask(_ context.Context, _, _ string) (*TaskStatus, error) {
	if m.failLeft > 0 {
		m.failLeft--
		return nil, errors.New("transient error")
	}
	if m.index >= len(m.statuses) {
		return m.statuses[len(m.statuses)-1], nil
	}
	s := m.statuses[m.index]
	m.index++
	return s, nil
}

func TestPollerSuccess(t *testing.T) {
	mock := &mockTaskClient{
		statuses: []*TaskStatus{
			{UPID: "UPID:pve1/100/0", State: StateRunning},
			{UPID: "UPID:pve1/100/0", State: StateRunning},
			{UPID: "UPID:pve1/100/0", State: StateStopped, Status: "OK"},
		},
	}
	poller := NewPoller(mock,
		WithPollInterval(1*time.Millisecond),
		WithMaxInterval(5*time.Millisecond),
		WithMaxWait(5*time.Second),
	)
	result := poller.Wait(context.Background(), "pve1", "UPID:pve1/100/0")
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if !result.OK {
		t.Error("expected OK result")
	}
	if result.State != StateStopped {
		t.Errorf("State = %q, want stopped", result.State)
	}
}

func TestPollerTaskFailure(t *testing.T) {
	mock := &mockTaskClient{
		statuses: []*TaskStatus{
			{UPID: "UPID:pve1/100/0", State: StateStopped, Status: "error"},
		},
	}
	poller := NewPoller(mock,
		WithPollInterval(1*time.Millisecond),
		WithMaxWait(5*time.Second),
	)
	result := poller.Wait(context.Background(), "pve1", "UPID:pve1/100/0")
	if result.Error != nil {
		t.Fatalf("unexpected polling error for known failed task: %v", result.Error)
	}
	if result.OK {
		t.Error("expected non-OK result")
	}
	if result.Status != "error" {
		t.Errorf("Status = %q, want error", result.Status)
	}
}

func TestPollerContextCancellation(t *testing.T) {
	mock := &mockTaskClient{
		statuses: []*TaskStatus{
			{UPID: "UPID:pve1/100/0", State: StateRunning},
			{UPID: "UPID:pve1/100/0", State: StateRunning},
			{UPID: "UPID:pve1/100/0", State: StateRunning},
		},
	}
	poller := NewPoller(mock,
		WithPollInterval(50*time.Millisecond),
		WithMaxWait(5*time.Second),
	)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	result := poller.Wait(ctx, "pve1", "UPID:pve1/100/0")
	if result.Error == nil {
		t.Fatal("expected error from cancellation")
	}
	if result.UPID != "UPID:pve1/100/0" {
		t.Errorf("UPID = %q, want pve1/100/0", result.UPID)
	}
}

func TestPollerTimeout(t *testing.T) {
	mock := &mockTaskClient{
		statuses: []*TaskStatus{
			{UPID: "UPID:pve1/100/0", State: StateRunning},
			{UPID: "UPID:pve1/100/0", State: StateRunning},
			{UPID: "UPID:pve1/100/0", State: StateRunning},
		},
	}
	poller := NewPoller(mock,
		WithPollInterval(10*time.Millisecond),
		WithMaxWait(25*time.Millisecond),
	)
	result := poller.Wait(context.Background(), "pve1", "UPID:pve1/100/0")
	if result.Error == nil {
		t.Fatal("expected timeout error")
	}
	if result.UPID != "UPID:pve1/100/0" {
		t.Errorf("UPID = %q, want preserved", result.UPID)
	}
}

func TestPollerTransientRecovery(t *testing.T) {
	mock := &mockTaskClient{
		statuses: []*TaskStatus{
			{UPID: "UPID:pve1/100/0", State: StateRunning},
			{UPID: "UPID:pve1/100/0", State: StateStopped, Status: "OK"},
		},
		failLeft: 1, // first query after the running one fails
	}
	poller := NewPoller(mock,
		WithPollInterval(1*time.Millisecond),
		WithMaxInterval(5*time.Millisecond),
		WithMaxWait(5*time.Second),
	)
	result := poller.Wait(context.Background(), "pve1", "UPID:pve1/100/0")
	if result.Error != nil {
		t.Fatalf("unexpected error after transient failure: %v", result.Error)
	}
	if !result.OK {
		t.Error("expected OK result after recovery")
	}
}

func TestNextIntervalExponentialBackoff(t *testing.T) {
	p := NewPoller(nil,
		WithPollInterval(100*time.Millisecond),
		WithMaxInterval(1*time.Second),
	)
	i := p.pollInterval
	for range 5 {
		next := p.nextInterval(i)
		if next < i {
			t.Errorf("next interval %v < current %v", next, i)
		}
		i = next
	}
	if i > p.maxInterval {
		t.Errorf("interval %v exceeded max %v", i, p.maxInterval)
	}
}

func TestNextIntervalCapsAtMax(t *testing.T) {
	p := NewPoller(nil,
		WithPollInterval(100*time.Millisecond),
		WithMaxInterval(200*time.Millisecond),
	)
	i := 150 * time.Millisecond
	next := p.nextInterval(i)
	if next > 200*time.Millisecond {
		t.Errorf("next interval %v exceeds max 200ms", next)
	}
}
