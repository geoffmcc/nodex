package cli

import (
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/geoffmcc/nodex/internal/output"
)

var statusColorRules = []struct {
	pattern *regexp.Regexp
	color   string
}{
	{regexp.MustCompile(`(?i)\bWARN(?:ING)?S?:`), "\x1b[33m"},
	{regexp.MustCompile(`(?i)\b(healthy|online|running|available|completed|ok)\b`), "\x1b[32m"},
	{regexp.MustCompile(`(?i)\b(unknown|degraded|warning|unresolved|not established|paused)\b`), "\x1b[33m"},
	{regexp.MustCompile(`(?i)\b(stopped|unreachable|unavailable|offline|failed|error)\b`), "\x1b[31m"},
}

func statusColorEnabled(cmdCtx *Context) bool {
	if cmdCtx == nil {
		return false
	}
	noColorEnv, dumbTerm := statusColorEnvEnabled()
	return statusColorPolicy(cmdCtx.Opts.NoColor, cmdCtx.Opts.Output == output.FormatTable, isTTY(cmdCtx.InteractiveOut), noColorEnv, dumbTerm)
}

func statusColorText(text string, enabled bool) string {
	if !enabled {
		return text
	}
	lines := strings.SplitAfter(text, "\n")
	for i, line := range lines {
		body := strings.TrimSuffix(line, "\n")
		if state, start, end, ok := statusGuestRowState(body); ok {
			if color := colorForStatus(state); color != "" {
				body = body[:start] + color + body[start:end] + "\x1b[0m" + body[end:]
			}
			lines[i] = body + strings.TrimPrefix(line, body)
			continue
		}
		for _, rule := range statusColorRules {
			body = rule.pattern.ReplaceAllStringFunc(body, func(match string) string {
				return rule.color + match + "\x1b[0m"
			})
		}
		lines[i] = body + strings.TrimPrefix(line, strings.TrimSuffix(line, "\n"))
	}
	return strings.Join(lines, "")
}

func statusGuestRowState(line string) (string, int, int, bool) {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return "", 0, 0, false
	}
	if _, err := strconv.Atoi(fields[0]); err != nil {
		return "", 0, 0, false
	}
	stateIndex := len(fields) - 1
	if strings.HasPrefix(fields[stateIndex], "[") && strings.HasSuffix(fields[stateIndex], "]") {
		stateIndex--
	}
	if stateIndex < 2 || colorForStatus(fields[stateIndex]) == "" {
		return "", 0, 0, false
	}
	start := strings.LastIndex(line, fields[stateIndex])
	if start < 0 {
		return "", 0, 0, false
	}
	return fields[stateIndex], start, start + len(fields[stateIndex]), true
}

func colorForStatus(state string) string {
	switch strings.ToLower(state) {
	case "healthy", "online", "running", "available", "completed", "ok":
		return "\x1b[32m"
	case "unknown", "degraded", "warning", "unresolved", "paused":
		return "\x1b[33m"
	case "stopped", "unreachable", "unavailable", "offline", "failed", "error":
		return "\x1b[31m"
	default:
		return ""
	}
}

func statusColorPolicy(noColor, table, tty, noColorEnv, dumbTerm bool) bool {
	return !noColor && table && tty && !noColorEnv && !dumbTerm
}

func statusColorEnvEnabled() (noColor, dumbTerm bool) {
	_, noColor = os.LookupEnv("NO_COLOR")
	dumbTerm = strings.EqualFold(os.Getenv("TERM"), "dumb")
	return noColor, dumbTerm
}
