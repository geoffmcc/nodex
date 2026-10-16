package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
)

func TestStatusWatchFlagsAndGates(t *testing.T) {
	for _, args := range [][]string{{"--interval", "2s"}, {"--watch", "--interval", "0s"}, {"--watch", "--interval", "500ms"}, {"--watch", "--interval", "6m"}, {"--watch", "--interval", "bad"}, {"--watch", "extra"}} {
		if _, _, err := parseStatusFlags(args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	watch, interval, err := parseStatusFlags([]string{"--watch"})
	if err != nil || !watch || interval != 5*time.Second {
		t.Fatalf("default %v %v %v", watch, interval, err)
	}
	for _, args := range [][]string{{"status", "--watch", "--interval", "2s", "--profile", "lab"}, {"--profile=lab", "status", "--watch=true", "--interval=2s"}} {
		opts, _, remaining, err := parseGlobal(args)
		if err != nil {
			t.Fatal(err)
		}
		watch, interval, err = parseStatusFlags(remaining[1:])
		if err != nil || !watch || interval != 2*time.Second || opts.Profile != "lab" {
			t.Fatalf("parse %v: %+v %v", args, opts, err)
		}
	}
	for _, args := range [][]string{
		{"status", "--watch", "--output", "json"}, {"status", "--watch", "--output", "yaml"},
		{"status", "--watch", "--output", "table"}, {"status", "--watch", "--all"},
		{"status", "--watch", "--non-interactive"},
	} {
		var out, errOut bytes.Buffer
		err := Run(t.Context(), args, &out, &errOut)
		if err == nil || app.ExitCodeFromError(err) != app.ExitUsage || strings.Contains(out.String(), "\x1b") {
			t.Fatalf("unsafe gate for %v: %v %q", args, err, out.String())
		}
	}
	if err := runStatusWatch(t.Context(), &Context{Opts: Options{Agent: true, Output: output.FormatTable}}, time.Second); app.ExitCodeFromError(err) != app.ExitUsage {
		t.Fatal("agent watch allowed", err)
	}
	contract := describeOperation(*LookupOperation("status"), true)
	base := map[string]any{"flags": map[string]any{"--agent": true, "--profile": "lab"}}
	if err := validateSchemaValue(base, contract.InputSchema, schemaDocument{root: contract.InputSchema}, "$", 0); err != nil {
		t.Fatal("valid one-shot agent status rejected", err)
	}
	for _, input := range []map[string]any{{"flags": map[string]any{"--agent": true, "--profile": "lab", "--watch": true}}, {"flags": map[string]any{"--agent": true, "--profile": "lab", "--interval": "5s"}}} {
		if err := validateSchemaValue(input, contract.InputSchema, schemaDocument{root: contract.InputSchema}, "$", 0); err == nil {
			t.Fatal("agent schema advertises watch", input)
		}
	}
}

type fakeWatchTerminal struct {
	keys                                         chan byte
	beginErr, endErr, sizeErr, inputErr, drawErr error
	begin, end                                   int
	frames                                       []string
	onDraw                                       func(string)
	reading                                      atomic.Int32
}

func (f *fakeWatchTerminal) Begin() error { f.begin++; return f.beginErr }
func (f *fakeWatchTerminal) End() error {
	f.end++
	if f.reading.Load() != 0 {
		return errors.New("input reader not joined")
	}
	return f.endErr
}
func (f *fakeWatchTerminal) ReadKey(ctx context.Context) (byte, error) {
	f.reading.Add(1)
	defer f.reading.Add(-1)
	if f.inputErr != nil {
		return 0, f.inputErr
	}
	select {
	case key := <-f.keys:
		return key, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}
func (f *fakeWatchTerminal) Size() (int, int, error) { return 120, 80, f.sizeErr }
func (f *fakeWatchTerminal) Draw(s string) error {
	f.frames = append(f.frames, s)
	if f.onDraw != nil {
		f.onDraw(s)
	}
	return f.drawErr
}

func healthyWatchSnapshot() *statusOverview {
	return &statusOverview{Cluster: "lab", Version: "test", Nodes: 1, NodesDetail: []statusNode{{Name: "node", Status: "online"}}, Standalone: true,
		Sections: map[string]string{"nodes": "available", "vms": "available", "containers": "available", "storage": "available", "cluster": "available", "quorum": "available"}}
}

func TestStatusWatchStaleRecoveryAndNoOverlap(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	terminal := &fakeWatchTerminal{keys: make(chan byte, 1)}
	completed := 0
	terminal.onDraw = func(frame string) {
		if !strings.Contains(frame, "refreshing") {
			completed++
			if completed == 4 {
				terminal.keys <- 'q'
			}
		}
	}
	calls, active := 0, 0
	var starts, ends []time.Time
	err := watchStatus(ctx, terminal, "lab", 5*time.Millisecond, time.Second, func(context.Context) *statusOverview {
		active++
		if active != 1 {
			t.Error("overlapping refresh")
		}
		defer func() { active--; ends = append(ends, time.Now()) }()
		starts = append(starts, time.Now())
		calls++
		time.Sleep(12 * time.Millisecond) // Longer than interval; must not queue cycles.
		if calls == 2 {
			return nil
		}
		s := healthyWatchSnapshot()
		if calls >= 3 {
			s.VMs = 2
			s.VMsRunning = 2
			s.NodesDetail[0].Status = "offline"
		}
		return s
	})
	if err != nil || terminal.end != 1 || calls != 4 {
		t.Fatalf("lifecycle: calls=%d end=%d err=%v", calls, terminal.end, err)
	}
	for i := 1; i < len(starts); i++ {
		if starts[i].Sub(ends[i-1]) < 5*time.Millisecond {
			t.Fatal("queued/overlapping polling cycle")
		}
	}
	var completedFrames []string
	for _, f := range terminal.frames {
		if !strings.Contains(f, "refreshing") {
			completedFrames = append(completedFrames, f)
		}
	}
	if !strings.Contains(completedFrames[1], "UNREACHABLE / STALE") || !strings.Contains(completedFrames[1], "Cluster: lab") {
		t.Fatal("failed refresh lost or freshened cached data", completedFrames[1])
	}
	if strings.Contains(completedFrames[2], "STALE") || !strings.Contains(completedFrames[2], "Changed: VM counts, node connectivity") || !strings.Contains(completedFrames[2], "DEGRADED") {
		t.Fatal("recovery/changed-state wrong", completedFrames[2])
	}
	if strings.Contains(completedFrames[3], "Changed:") {
		t.Fatal("transient change highlight persisted")
	}
}

func TestStatusWatchAlwaysRestores(t *testing.T) {
	broken := errors.New("terminal broken")
	for _, tc := range []struct {
		name  string
		setup func(*fakeWatchTerminal)
		want  error
	}{
		{"begin", func(f *fakeWatchTerminal) { f.beginErr = broken }, broken},
		{"draw", func(f *fakeWatchTerminal) { f.drawErr = broken }, broken},
		{"size", func(f *fakeWatchTerminal) { f.sizeErr = broken }, broken},
		{"input", func(f *fakeWatchTerminal) { f.inputErr = broken }, broken},
		{"cleanup", func(f *fakeWatchTerminal) { f.endErr = broken; f.keys <- 'q' }, broken},
		{"quit", func(f *fakeWatchTerminal) { f.keys <- 'q' }, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeWatchTerminal{keys: make(chan byte, 1)}
			tc.setup(f)
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			err := watchStatus(ctx, f, "lab", time.Millisecond, time.Second, func(ctx context.Context) *statusOverview { <-ctx.Done(); return nil })
			if !errors.Is(err, tc.want) || f.end != 1 {
				t.Fatalf("end=%d err=%v", f.end, err)
			}
		})
	}
	for _, key := range []byte{'q', 3} {
		f := &fakeWatchTerminal{keys: make(chan byte, 1)}
		err := watchStatus(t.Context(), f, "lab", time.Second, time.Second, func(ctx context.Context) *statusOverview { f.keys <- key; <-ctx.Done(); return nil })
		if key == 3 && app.ExitCodeFromError(err) != app.ExitInterrupted {
			t.Fatal("Ctrl+C exit", err)
		}
		if key == 'q' && err != nil {
			t.Fatal(err)
		}
		if f.end != 1 {
			t.Fatal("not restored")
		}
	}
}

func TestStatusWatchTimeoutAndParentCancellation(t *testing.T) {
	f := &fakeWatchTerminal{keys: make(chan byte, 1)}
	f.onDraw = func(s string) {
		if strings.Contains(s, "UNREACHABLE") && !strings.Contains(s, "refreshing") {
			f.keys <- 'q'
		}
	}
	err := watchStatus(t.Context(), f, "lab", time.Millisecond, 10*time.Millisecond, func(ctx context.Context) *statusOverview { <-ctx.Done(); return nil })
	if err != nil || f.end != 1 {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	f = &fakeWatchTerminal{keys: make(chan byte, 1)}
	err = watchStatus(ctx, f, "lab", time.Second, time.Second, func(ctx context.Context) *statusOverview { cancel(); <-ctx.Done(); return nil })
	if !errors.Is(err, context.Canceled) || f.end != 1 {
		t.Fatal("parent cancellation", err)
	}
	if statusPollDelay(5*time.Second, 1) != 10*time.Second || statusPollDelay(5*time.Second, 100) != time.Minute || statusPollDelay(5*time.Minute, 100) != 5*time.Minute {
		t.Fatal("unbounded/incorrect backoff")
	}
}

func TestStatusWatchBoundedSanitizedFrames(t *testing.T) {
	s := healthyWatchSnapshot()
	s.Cluster = "bad \x1b[2Jpassword=secret"
	s.NodesDetail[0].Name = "node\nINJECT\x1b]0;owned\a"
	for _, size := range [][2]int{{120, 40}, {40, 10}, {10, 4}, {1, 1}, {5000, 5000}} {
		frame := renderStatusWatch("lab\nprofile", statusWatchView{Snapshot: s, Health: "healthy"}, size[0], size[1])
		if strings.Contains(frame, "secret") || strings.Contains(frame, "owned") || strings.Contains(frame, "\x1b[2J") {
			t.Fatal("unsafe frame", frame)
		}
		plain := output.SanitizeTerminal(frame)
		lines := strings.Split(plain, "\n")
		if len(lines) > size[1] {
			t.Fatal("frame exceeds terminal height")
		}
		for _, line := range lines {
			if len(strings.TrimSuffix(line, "\r")) > size[0]-1 {
				t.Fatal("frame exceeds width")
			}
		}
	}
	var b watchBuffer
	_, _ = io.WriteString(&b, strings.Repeat("a", 100000))
	_, _ = b.Write([]byte(strings.Repeat("b", 100000)))
	if b.Len() != 64*1024 {
		t.Fatal("unbounded frame buffer", b.Len())
	}
}

type watchSnapshotProvider struct {
	*e2eMockProvider
	fail      bool
	taskCalls int
}

func (p *watchSnapshotProvider) VMs(context.Context) ([]domain.VM, error) {
	if p.fail {
		return nil, errors.New("password=secret")
	}
	return []domain.VM{{Status: "running"}, {Status: "stopped"}, {Status: "unknown"}}, nil
}
func (p *watchSnapshotProvider) Containers(context.Context) ([]domain.Container, error) {
	return []domain.Container{{Status: "running"}, {Status: "unknown"}}, nil
}
func (p *watchSnapshotProvider) Tasks(context.Context, string) ([]domain.Task, error) {
	p.taskCalls++
	return []domain.Task{{Type: "backup", State: "running"}}, nil
}

func TestStatusSharedSnapshotUnknownStatesAndTasks(t *testing.T) {
	p := &watchSnapshotProvider{e2eMockProvider: &e2eMockProvider{connected: true}}
	one := collectStatusSnapshot(t.Context(), p, false)
	if p.taskCalls != 0 || one.VMsStopped != 1 || one.VMsUnknown != 1 || one.CTsStopped != 0 || one.CTsUnknown != 1 {
		t.Fatalf("unknown states counted stopped: %+v", one)
	}
	watch := collectStatusSnapshot(t.Context(), p, true)
	if p.taskCalls != 1 || len(watch.RecentTasks) != 1 || !reflect.DeepEqual(one.Storage, watch.Storage) {
		t.Fatal("watch used different storage semantics")
	}
	one.StorageCapacity.CollectedAt = ""
	watch.StorageCapacity.CollectedAt = ""
	if !reflect.DeepEqual(one.StorageCapacity, watch.StorageCapacity) {
		t.Fatal("different capacity model")
	}
	p.fail = true
	failed := collectStatusSnapshot(t.Context(), p, false)
	var buf bytes.Buffer
	if err := writeStatus(&Context{Writer: &buf, Opts: Options{Output: output.FormatTable}}, failed); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "VMs: unavailable") || strings.Contains(buf.String(), "secret") || statusHealth(failed) != "degraded" {
		t.Fatal(buf.String())
	}
	if statusHealth(collectStatusSnapshot(t.Context(), &bareProvider{}, false)) != "unknown" {
		t.Fatal("unsupported provider marked healthy")
	}
}

type sampledTaskProvider struct {
	*e2eMockProvider
	calls []string
	tasks []domain.Task
}

func (p *sampledTaskProvider) Tasks(_ context.Context, node string) ([]domain.Task, error) {
	p.calls = append(p.calls, node)
	return p.tasks, nil
}

func TestStatusTaskSampleBoundsAndHealth(t *testing.T) {
	s := healthyWatchSnapshot()
	s.NodesDetail = nil
	for i := 9; i >= 0; i-- {
		s.NodesDetail = append(s.NodesDetail, statusNode{Name: fmt.Sprintf("node%d", i), Status: "online"})
	}
	p := &sampledTaskProvider{}
	for i := 0; i < 100; i++ {
		p.tasks = append(p.tasks, domain.Task{StartTime: i, State: "stopped"})
	}
	p.tasks[0].State = "running"
	collectStatusTasks(t.Context(), p, s)
	if len(p.calls) != 8 || p.calls[0] != "node0" || p.calls[7] != "node7" || len(s.RecentTasks) != 40 || s.RecentTasks[0].State != "running" || s.RecentTasks[1].StartTime != 99 {
		t.Fatalf("incorrect bounded sample: calls=%v tasks=%v", p.calls, s.RecentTasks)
	}
	if s.NodesDetail[0].Name != "node9" || p.tasks[1].StartTime != 1 {
		t.Fatal("collector mutated source slices")
	}
	s.Sections["tasks"] = "unavailable"
	if statusRefreshComplete(s) || statusHealth(s) != "degraded" {
		t.Fatal("failed optional query marked successful/healthy")
	}
	s.Sections["tasks"] = "unsupported"
	if !statusRefreshComplete(s) {
		t.Fatal("unsupported optional task query blocked successful refresh")
	}
	s.Standalone = false
	s.Sections["ha"] = "unavailable"
	if statusRefreshComplete(s) || statusHealth(s) != "degraded" {
		t.Fatal("failed cluster HA query marked successful/healthy")
	}
}
