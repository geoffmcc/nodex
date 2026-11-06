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
	taskWarningCache := newStatusTaskWarningCache()
	return watchStatus(ctx, terminal, name, statusColorEnabled(cmdCtx), interval, timeout, func(ctx context.Context) *statusOverview {
		return collectStatusSnapshotWithTaskWarningCache(ctx, prov, true, taskWarningCache)
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
	Snapshot     *statusOverview
	Previous     *statusOverview
	Attempt      time.Time
	LastSuccess  time.Time
	Health       string
	HealthDetail string
	Stale        bool
	Refreshing   bool
	Changes      string
	NextDelay    time.Duration
	Color        bool
}

const (
	watchKeyUp byte = 0x80 + iota
	watchKeyDown
	watchKeyPageUp
	watchKeyPageDown
)

// decodeWatchKey recognizes common terminal escape sequences as well as the
// native keys translated by the Windows console reader.
type watchKeyDecoder struct{ state byte }

func (d *watchKeyDecoder) feed(key byte) byte {
	if key == 27 {
		d.state = 1
		return 0
	}
	switch d.state {
	case 1:
		if key == '[' || key == 'O' {
			d.state = 2
			return 0
		}
	case 2:
		switch key {
		case 'A':
			d.state = 0
			return watchKeyUp
		case 'B':
			d.state = 0
			return watchKeyDown
		case '5', '6':
			d.state = key
			return 0
		}
	case '5', '6':
		state := d.state
		d.state = 0
		if key == '~' {
			if state == '5' {
				return watchKeyPageUp
			}
			return watchKeyPageDown
		}
		return 0
	}
	if d.state != 0 {
		d.state = 0
		return 0
	}
	return key
}

// One collector runs at a time while input and resize events remain responsive.
// Both collector and input reader are joined before restoring the terminal.
func watchStatus(parent context.Context, terminal watchTerminal, profile string, color bool, interval, timeout time.Duration, collect func(context.Context) *statusOverview) (result error) {
	defer func() { result = errors.Join(result, terminal.End()) }()
	if err := terminal.Begin(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(parent)
	inputResult := make(chan error, 1)
	keys := make(chan byte, 32)
	go func() {
		defer cancel()
		var decoder watchKeyDecoder
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
			key = decoder.feed(key)
			switch key {
			case watchKeyUp, watchKeyDown, watchKeyPageUp, watchKeyPageDown, 'j', 'k':
				select {
				case keys <- key:
				default:
					// Never let a burst of movement keys delay quit.
				}
			}
		}
	}()
	defer func() { cancel(); result = errors.Join(result, <-inputResult) }()
	view := statusWatchView{Health: "unknown", NextDelay: interval, Color: color}
	offset := 0
	lastWidth, lastHeight := 0, 0
	draw := func() error {
		width, height, err := terminal.Size()
		if err != nil {
			return err
		}
		lastWidth, lastHeight = width, height
		frame, clamped := renderStatusWatchAt(profile, view, width, height, offset)
		offset = clamped
		return terminal.Draw(frame)
	}
	scroll := func(key byte) error {
		page := max(1, min(lastHeight, 100)-4)
		switch key {
		case watchKeyUp, 'k':
			offset--
		case watchKeyDown, 'j':
			offset++
		case watchKeyPageUp:
			offset -= page
		case watchKeyPageDown:
			offset += page
		}
		return draw()
	}
	resize := time.NewTicker(250 * time.Millisecond)
	defer resize.Stop()
	redrawIfResized := func() error {
		w, h, err := terminal.Size()
		if err != nil {
			return err
		}
		if w != lastWidth || h != lastHeight {
			return draw()
		}
		return nil
	}
	failures := 0
	for ctx.Err() == nil {
		view.Refreshing = true
		view.Changes = ""
		if err := draw(); err != nil {
			return err
		}
		cycle, cancelCycle := context.WithTimeout(ctx, timeout)
		collected := make(chan *statusOverview, 1)
		go func() { collected <- collect(cycle) }()
		var next *statusOverview
		waiting := true
		for waiting {
			select {
			case next = <-collected:
				waiting = false
			case key := <-keys:
				if err := scroll(key); err != nil {
					cancelCycle()
					<-collected
					return err
				}
			case <-resize.C:
				if err := redrawIfResized(); err != nil {
					cancelCycle()
					<-collected
					return err
				}
			case <-ctx.Done():
				cancelCycle()
				<-collected
				return parent.Err()
			}
		}
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
		view.HealthDetail = "refresh failed"
		if next != nil {
			view.Health, view.HealthDetail = statusHealthDetail(next)
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
			view.HealthDetail = "refresh timed out"
		}
		view.NextDelay = statusPollDelay(interval, failures)
		if err := draw(); err != nil {
			return err
		}
		timer := time.NewTimer(view.NextDelay)
		waiting = true
		for waiting {
			select {
			case <-ctx.Done():
				timer.Stop()
				waiting = false
			case <-timer.C:
				waiting = false
			case key := <-keys:
				if err := scroll(key); err != nil {
					timer.Stop()
					return err
				}
			case <-resize.C:
				if err := redrawIfResized(); err != nil {
					timer.Stop()
					return err
				}
			}
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
	} else if !reflect.DeepEqual(guestStates(previous.VMDetails), guestStates(next.VMDetails)) {
		changes = append(changes, "VM identity/state")
	}
	if previous.CTsRunning != next.CTsRunning || previous.CTsStopped != next.CTsStopped || previous.CTsUnknown != next.CTsUnknown {
		changes = append(changes, "container counts")
	} else if !reflect.DeepEqual(guestStates(previous.ContainerDetails), guestStates(next.ContainerDetails)) {
		changes = append(changes, "container identity/state")
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

func guestStates(guests []statusGuest) map[string]string {
	states := make(map[string]string, len(guests))
	for _, guest := range guests {
		states[guest.ID] = guest.Name + "\x00" + guest.Node + "\x00" + guest.Status
	}
	return states
}

// A bounded buffer prevents large inventories from generating an unbounded
// terminal frame. Structured one-shot output still exposes the full inventory.
type watchBuffer struct {
	bytes.Buffer
	truncated bool
}

func (b *watchBuffer) WriteString(s string) (int, error) { return b.Write([]byte(s)) }

func (b *watchBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := 1024*1024 - b.Len()
	if len(p) > remaining {
		b.truncated = true
	}
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.Buffer.Write(p)
	}
	return n, nil
}

func renderStatusWatch(profile string, v statusWatchView, width, height int) string {
	frame, _ := renderStatusWatchAt(profile, v, width, height, 0)
	return frame
}

func renderStatusWatchAt(profile string, v statusWatchView, width, height, offset int) (string, int) {
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
	if v.HealthDetail != "" {
		state += " (" + watchText(v.HealthDetail) + ")"
	}
	if v.Snapshot != nil && len(v.Snapshot.Warnings) > 0 {
		state += fmt.Sprintf(" / %d warning(s)", len(v.Snapshot.Warnings))
	}
	if v.Snapshot != nil && v.Snapshot.StorageCapacity != nil {
		for _, group := range v.Snapshot.StorageCapacity.Groups {
			if statusPBSMinorDifference(group, v.Snapshot.Storage) {
				state += " / PBS views differ slightly"
				break
			}
		}
	}
	if v.Snapshot != nil {
		alerts := 0
		for _, task := range v.Snapshot.RecentTasks {
			status := strings.TrimSpace(task.Status)
			if strings.EqualFold(task.State, "stopped") && status != "" && !strings.EqualFold(status, "OK") {
				alerts++
			}
		}
		if alerts > 0 {
			state += fmt.Sprintf(" / %d sampled task alert(s)", alerts)
		}
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
	} else if v.Snapshot != nil {
		// Keep the viewport stable when the transient change notice clears.
		fmt.Fprintln(&buf)
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
		_ = writeStatusTableWithGuestLimit(output.NewSanitizingWriter(&buf), s, 0)
		if s.Sections["tasks"] != "" {
			fmt.Fprintln(&buf, "\nActive/recent tasks (sample, up to 5 per node):")
			for _, task := range s.RecentTasks {
				state := task.State
				if strings.EqualFold(state, "stopped") && strings.EqualFold(task.Status, "OK") {
					state = "completed"
				}
				guest := statusTaskGuestLabel(task, s)
				fmt.Fprintf(&buf, "  %s  %s  %s  %s  %s\n", watchText(task.Node), watchText(task.Type), watchText(guest), watchText(state), watchText(task.Status))
				if detail := s.TaskWarningDetails[task.UPID]; detail != "" {
					fmt.Fprintf(&buf, "    %s\n", watchText(detail))
				}
			}
		}
	}
	if buf.truncated {
		_, _ = buf.Buffer.WriteString("\n[Output limit reached; use list commands or status --output json.]\n")
	}
	all := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	headerCount := min(2, max(0, height-2), len(all))
	header, body := all[:headerCount], all[headerCount:]
	rows := max(0, height-headerCount-1)
	offset = max(0, min(offset, max(0, len(body)-rows)))
	end := min(len(body), offset+rows)
	lines := append([]string{}, header...)
	lines = append(lines, body[offset:end]...)
	start := 0
	if end > offset {
		start = offset + 1
	}
	footer := fmt.Sprintf("q: quit | read-only | lines %d-%d/%d", start, end, len(body))
	if len(body) <= rows {
		footer += " | all visible"
	} else {
		footer += " | arrows/j/k/PgUp/PgDn"
	}
	if buf.truncated {
		footer = "q: quit | read-only | output limit reached | scroll for details"
	}
	lines = append(lines, footer)
	for i, line := range lines {
		line = watchText(line)
		if len(line) > width-1 {
			line = line[:width-1]
		}
		line = statusColorText(line, v.Color)
		lines[i] = line + "\x1b[K"
	}
	return "\x1b[H" + strings.Join(lines, "\r\n") + "\x1b[J", offset
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
	s.VMDetails = append([]statusGuest(nil), s.VMDetails...)
	for i := range s.VMDetails {
		s.VMDetails[i].ID = watchText(s.VMDetails[i].ID)
		s.VMDetails[i].Name = watchText(s.VMDetails[i].Name)
		s.VMDetails[i].Node = watchText(s.VMDetails[i].Node)
		s.VMDetails[i].Status = watchText(s.VMDetails[i].Status)
	}
	s.ContainerDetails = append([]statusGuest(nil), s.ContainerDetails...)
	for i := range s.ContainerDetails {
		s.ContainerDetails[i].ID = watchText(s.ContainerDetails[i].ID)
		s.ContainerDetails[i].Name = watchText(s.ContainerDetails[i].Name)
		s.ContainerDetails[i].Node = watchText(s.ContainerDetails[i].Node)
		s.ContainerDetails[i].Status = watchText(s.ContainerDetails[i].Status)
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
	if original.TaskWarningDetails != nil {
		s.TaskWarningDetails = make(map[string]string, len(original.TaskWarningDetails))
		for upid, detail := range original.TaskWarningDetails {
			s.TaskWarningDetails[watchText(upid)] = watchText(detail)
		}
	}
	if s.HA != nil {
		ha := *s.HA
		ha.Status = watchText(ha.Status)
		s.HA = &ha
	}
	return &s
}
