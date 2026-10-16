package cli

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/redact"
)

const defaultStatusInterval = 5 * time.Second

func parseStatusFlags(args []string) (bool, time.Duration, error) {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	watch := fs.Bool("watch", false, "refresh in place")
	interval := fs.Duration("interval", defaultStatusInterval, "refresh interval")
	if err := fs.Parse(args); err != nil {
		return false, 0, err
	}
	if fs.NArg() != 0 {
		return false, 0, fmt.Errorf("usage: nodex status [--watch] [--interval 5s]")
	}
	intervalSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "interval" {
			intervalSet = true
		}
	})
	if intervalSet && !*watch {
		return false, 0, fmt.Errorf("--interval requires --watch")
	}
	if *interval < time.Second || *interval > 5*time.Minute {
		return false, 0, fmt.Errorf("--interval must be between 1s and 5m")
	}
	return *watch, *interval, nil
}

func runStatusWatch(ctx context.Context, cmdCtx *Context, interval time.Duration) error {
	if cmdCtx.Opts.Agent || cmdCtx.AgentConfig != nil || cmdCtx.Opts.All || cmdCtx.Opts.NonInteractive || cmdCtx.Opts.Output != output.FormatTable {
		return app.NewExitError(fmt.Errorf("status --watch requires interactive table output and one profile; incompatible with --agent, --all, --non-interactive, JSON or YAML"), app.ExitUsage)
	}
	in, out := cmdCtx.InteractiveIn, cmdCtx.InteractiveOut
	if in == nil {
		in = cmdCtx.Stdin
	}
	if out == nil {
		out = cmdCtx.Writer
	}
	if !isTTY(in) || !isTTY(out) || os.Getenv("TERM") == "dumb" {
		return app.NewExitError(fmt.Errorf("status --watch requires an interactive terminal on stdin and stdout; use status --output json for a one-shot snapshot"), app.ExitUsage)
	}
	// Pin the complete profile configuration for the session, so an external
	// profile switch cannot silently redirect a running dashboard.
	cfg, err := config.Read()
	if err != nil {
		return err
	}
	local := *cmdCtx
	local.AgentConfig = cfg
	name, err := resolveProfileName(&local)
	if err != nil {
		return err
	}
	local.Opts.Profile = name
	timeout := local.Opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if timeout > 5*time.Minute {
		timeout = 5 * time.Minute
	}
	connectCtx, cancel := context.WithTimeout(ctx, timeout)
	prov, cleanup, err := connectProfile(connectCtx, &local, name)
	cancel()
	if err != nil {
		return err
	}
	defer cleanup()
	terminal := &statusTerminal{in: in.(*os.File), out: out.(*os.File)}
	return watchStatus(ctx, terminal, name, interval, timeout, func(ctx context.Context) *statusOverview {
		return collectStatusSnapshot(ctx, prov, true)
	})
}

type watchTerminal interface {
	Begin() error
	End() error
	ReadKey(context.Context) (byte, error)
	Size() (int, int, error)
	Draw(string) error
}

type statusWatchView struct {
	Snapshot    *statusOverview
	Previous    *statusOverview
	Attempt     time.Time
	LastSuccess time.Time
	Health      string
	Stale       bool
	Refreshing  bool
	Changes     string
	NextDelay   time.Duration
}

// A single collector runs synchronously. A separate cancellable input reader
// handles q/Ctrl+C even during a slow request; it is joined before restoring the
// terminal. There is no ticker backlog, overlapping refresh, or unbounded history.
func watchStatus(parent context.Context, terminal watchTerminal, profile string, interval, timeout time.Duration, collect func(context.Context) *statusOverview) (result error) {
	defer func() { result = errors.Join(result, terminal.End()) }()
	if err := terminal.Begin(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(parent)
	inputResult := make(chan error, 1)
	go func() {
		defer cancel()
		for {
			key, err := terminal.ReadKey(ctx)
			if err != nil {
				if ctx.Err() != nil {
					err = nil
				}
				inputResult <- err
				return
			}
			if key == 'q' || key == 'Q' {
				inputResult <- nil
				return
			}
			if key == 3 {
				inputResult <- app.NewExitError(fmt.Errorf("interrupted"), app.ExitInterrupted)
				return
			}
		}
	}()
	defer func() { cancel(); result = errors.Join(result, <-inputResult) }()
	view := statusWatchView{Health: "unknown", NextDelay: interval}
	draw := func() error {
		width, height, err := terminal.Size()
		if err != nil {
			return err
		}
		return terminal.Draw(renderStatusWatch(profile, view, width, height))
	}
	failures := 0
	for ctx.Err() == nil {
		view.Refreshing = true
		view.Stale = view.Snapshot != nil
		view.Changes = ""
		if err := draw(); err != nil {
			return err
		}
		cycle, cancelCycle := context.WithTimeout(ctx, timeout)
		next := collect(cycle)
		timedOut := cycle.Err() != nil
		cancelCycle()
		if ctx.Err() != nil {
			break
		}
		view.Refreshing = false
		view.Attempt = time.Now()
		complete := next != nil && !timedOut && statusRefreshComplete(next)
		if complete {
			view.Changes = statusChanges(view.Previous, next)
			view.Previous = next
			view.LastSuccess = view.Attempt
			failures = 0
		} else {
			failures++
		}
		view.Health = "unreachable"
		if next != nil {
			view.Health = statusHealth(next)
		}
		// Keep the last snapshot only on a completely failed attempt. Partial
		// snapshots expose current per-section failures rather than stale zeros.
		if next != nil && view.Health != "unreachable" {
			view.Snapshot = next
			view.Stale = false
		} else {
			view.Stale = view.Snapshot != nil
		}
		if timedOut {
			if view.Health != "unreachable" {
				view.Health = "degraded"
			}
			view.Stale = view.Snapshot != nil
		}
		view.NextDelay = statusPollDelay(interval, failures)
		if err := draw(); err != nil {
			return err
		}
		timer := time.NewTimer(view.NextDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
	if parent.Err() != nil {
		return parent.Err()
	}
	return nil
}

func statusPollDelay(interval time.Duration, failures int) time.Duration {
	delay := interval
	for i := 0; i < failures && delay < time.Minute; i++ {
		delay *= 2
	}
	if interval <= time.Minute && delay > time.Minute {
		return time.Minute
	}
	return delay
}

func statusChanges(previous, next *statusOverview) string {
	if previous == nil {
		return ""
	}
	var changes []string
	if previous.VMsRunning != next.VMsRunning || previous.VMsStopped != next.VMsStopped || previous.VMsUnknown != next.VMsUnknown {
		changes = append(changes, "VM counts")
	}
	if previous.CTsRunning != next.CTsRunning || previous.CTsStopped != next.CTsStopped || previous.CTsUnknown != next.CTsUnknown {
		changes = append(changes, "container counts")
	}
	nodeStates := func(s *statusOverview) map[string]string {
		states := map[string]string{}
		for _, n := range s.NodesDetail {
			states[n.Name] = n.Status
		}
		return states
	}
	if !reflect.DeepEqual(nodeStates(previous), nodeStates(next)) {
		changes = append(changes, "node connectivity")
	}
	if !reflect.DeepEqual(previous.Storage, next.Storage) {
		changes = append(changes, "storage")
	}
	return strings.Join(changes, ", ")
}

// A bounded buffer prevents large inventories from generating an unbounded
// terminal frame. Structured one-shot output still exposes the full inventory.
type watchBuffer struct{ bytes.Buffer }

func (b *watchBuffer) WriteString(s string) (int, error) { return b.Write([]byte(s)) }

func (b *watchBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := 64*1024 - b.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.Buffer.Write(p)
	}
	return n, nil
}

func renderStatusWatch(profile string, v statusWatchView, width, height int) string {
	width = max(1, min(width, 240))
	height = max(1, min(height, 100))
	var buf watchBuffer
	state := strings.ToUpper(v.Health)
	if v.Stale {
		state += " / STALE"
	}
	if v.Refreshing {
		state += " / refreshing"
	}
	fmt.Fprintf(&buf, "NodeX | %s | %s\n", watchText(profile), state)
	last := "never"
	if !v.LastSuccess.IsZero() {
		last = v.LastSuccess.Local().Format("15:04:05")
	}
	fmt.Fprintf(&buf, "Last successful refresh: %s | next delay: %s\n", last, v.NextDelay)
	if v.Stale {
		fmt.Fprintln(&buf, "WARNING: cached readings; current health is not established.")
	}
	if v.Changes != "" {
		fmt.Fprintf(&buf, "Changed: %s\n", v.Changes)
	}
	if v.Snapshot == nil {
		fmt.Fprintln(&buf, "Waiting for a usable snapshot...")
	} else {
		s := statusWatchSnapshot(v.Snapshot)
		fmt.Fprint(&buf, "Nodes: ")
		for i, n := range s.NodesDetail {
			if i > 0 {
				fmt.Fprint(&buf, ", ")
			}
			fmt.Fprintf(&buf, "%s=%s", watchText(n.Name), watchText(n.Status))
		}
		fmt.Fprintln(&buf)
		_ = writeStatus(&Context{Writer: output.NewSanitizingWriter(&buf), Opts: Options{Output: output.FormatTable}}, s)
		if s.Sections["tasks"] != "" {
			fmt.Fprintln(&buf, "\nActive/recent tasks (sample, up to 5 per node):")
			for _, task := range s.RecentTasks {
				fmt.Fprintf(&buf, "  %s  %s  %s  %s\n", watchText(task.Node), watchText(task.Type), watchText(task.State), watchText(task.Status))
			}
		}
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	maxLines := max(0, height-2)
	truncated := len(lines) > maxLines
	if truncated {
		lines = lines[:maxLines]
	}
	footer := "q: quit | Ctrl+C: interrupt | read-only"
	if truncated {
		footer = "... more: use status --output json | q: quit"
	}
	lines = append(lines, footer)
	for i, line := range lines {
		line = watchText(line)
		if len(line) > width-1 {
			line = line[:width-1]
		}
		lines[i] = line + "\x1b[K"
	}
	return "\x1b[H" + strings.Join(lines, "\r\n") + "\x1b[J"
}

// ASCII framing has predictable cell width on Windows, Unix and SSH. No
// provider-controlled control/escape/newline can become terminal framing.
func watchText(s string) string {
	s = redact.String(output.SanitizeTerminal(s))
	return strings.Map(func(r rune) rune {
		if r < 32 || r > 126 {
			return '?'
		}
		return r
	}, s)
}

// Sanitize fields before the shared renderer adds its own newlines. Sanitizing
// the finished frame alone would let a remote name inject extra dashboard rows.
func statusWatchSnapshot(original *statusOverview) *statusOverview {
	s := *original
	s.Cluster, s.Version = watchText(s.Cluster), watchText(s.Version)
	s.NodesDetail = append([]statusNode(nil), s.NodesDetail...)
	for i := range s.NodesDetail {
		s.NodesDetail[i].Name = watchText(s.NodesDetail[i].Name)
		s.NodesDetail[i].Status = watchText(s.NodesDetail[i].Status)
	}
	s.Storage = append([]statusStorage(nil), s.Storage...)
	for i := range s.Storage {
		e := &s.Storage[i]
		e.Name, e.Node, e.Type, e.Status = watchText(e.Name), watchText(e.Node), watchText(e.Type), watchText(e.Status)
		if e.Identity != nil {
			id := *e.Identity
			id.Namespace, id.Reason = watchText(id.Namespace), watchText(id.Reason)
			e.Identity = &id
		}
	}
	if s.HA != nil {
		ha := *s.HA
		ha.Status = watchText(ha.Status)
		s.HA = &ha
	}
	return &s
}
