package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/geoffmcc/nodex/internal/output"
)

// parseGlobal scans the full argument list, extracting global flags wherever
// they appear while separating the command path from the handler region.
// Unlike flag.FlagSet it does not stop at the first non-flag token, so flags
// may be interspersed with the command path and handler arguments.
//
// It returns:
//   - opts:      the accumulated global options (defaults applied and values
//     validated);
//   - helpPath:  a non-nil path when a -h/--help/-help token was found (the
//     scan stops at that point);
//   - remaining: the command path followed by the handler argument region,
//     with handler-owned flags preserved verbatim;
//   - err:       a usage error for unknown flags, invalid values, or a
//     missing global value.
func parseGlobal(args []string) (opts Options, helpPath []string, remaining []string, err error) {
	opts.Timeout = 30 * time.Second
	outFmt := ""
	var path []string
	var region []string
	pathFinal := false
	literal := false

	for i := 0; i < len(args); i++ {
		tok := args[i]

		if literal {
			region = append(region, tok)
			continue
		}
		if tok == "--" {
			if len(path) == 0 && !pathFinal && len(region) == 0 {
				literal = true
				continue
			}
			literal = true
			region = append(region, tok)
			continue
		}
		if !strings.HasPrefix(tok, "-") || tok == "-" {
			if pathFinal {
				region = append(region, tok)
				continue
			}
			if len(path) == 0 {
				path = append(path, tok)
				continue
			}
			cur := resolveCommandPath(path)
			if cur != nil && cur.sub != nil && cur.sub[tok] != nil {
				path = append(path, tok)
				continue
			}
			pathFinal = true
			region = append(region, tok)
			continue
		}

		trimmed := strings.TrimLeft(tok, "-")
		name, inline, hasInline := strings.Cut(trimmed, "=")
		if name == "" {
			region = append(region, tok)
			continue
		}

		// Help short-circuits whatever the scan was building.
		if name == "help" || name == "h" {
			helpPath = append(helpPath, path...)
			return opts, helpPath, region, nil
		}

		// Handler-owned flags pass through to the handler verbatim.
		if commandFlagSet(path, region).owns(name, hasInline) {
			if !pathFinal {
				cur := resolveCommandPath(path)
				if cur == nil || cur.run == nil {
					return opts, helpPath, region, fmt.Errorf("unknown flag: %s", tok)
				}
				pathFinal = true
			}
			region = append(region, tok)
			continue
		}

		if isGlobalValue(name) {
			var value string
			if hasInline {
				value = inline
			} else {
				var ok bool
				value, ok = nextArg(args, i)
				if !ok {
					return opts, helpPath, region, fmt.Errorf("flag needs an argument: %s", tok)
				}
				i++
			}
			switch name {
			case "profile":
				opts.Profile = value
			case "confirm-target":
				opts.ConfirmTarget = value
			case "output":
				outFmt = value
			case "timeout":
				d, perr := time.ParseDuration(value)
				if perr != nil {
					return opts, helpPath, region, fmt.Errorf("invalid value %q for flag -timeout: %w", value, perr)
				}
				if d <= 0 {
					return opts, helpPath, region, fmt.Errorf("timeout must be greater than zero")
				}
				opts.Timeout = d
			case "limit":
				n, perr := strconv.Atoi(value)
				if perr != nil {
					return opts, helpPath, region, fmt.Errorf("invalid value %q for flag -limit: %w", value, perr)
				}
				if n < 0 {
					return opts, helpPath, region, fmt.Errorf("limit must be non-negative")
				}
				opts.Limit = n
			}
			continue
		}

		if isGlobalBool(name) {
			value := true
			if hasInline {
				v, perr := strconv.ParseBool(inline)
				if perr != nil {
					return opts, helpPath, region, fmt.Errorf("invalid value %q for flag -%s: %w", inline, name, perr)
				}
				value = v
			}
			switch name {
			case "no-color":
				opts.NoColor = value
			case "non-interactive":
				opts.NonInteractive = value
			case "quiet":
				opts.Quiet = value
			case "verbose":
				opts.Verbose = value
			case "debug":
				opts.Debug = value
			case "yes":
				opts.Yes = value
			case "force":
				opts.Force = value
			case "wait":
				opts.Wait = value
			case "expert":
				opts.Expert = value
			case "all":
				opts.All = value
			case "password-stdin":
				opts.PasswordStdin = value
			}
			continue
		}

		return opts, helpPath, region, fmt.Errorf("unknown flag: %s", tok)
	}

	if outFmt != "" {
		switch strings.ToLower(outFmt) {
		case "table":
			opts.Output = output.FormatTable
		case "json":
			opts.Output = output.FormatJSON
		case "yaml":
			opts.Output = output.FormatYAML
		default:
			return opts, helpPath, region, fmt.Errorf("invalid output format: %s (use table, json, or yaml)", outFmt)
		}
	} else {
		opts.Output = output.DefaultFormat()
	}

	remaining = append(append([]string{}, path...), region...)
	return opts, helpPath, remaining, nil
}

// nextArg returns args[i+1] and true when the next element exists.
func nextArg(args []string, i int) (string, bool) {
	if i+1 >= len(args) {
		return "", false
	}
	return args[i+1], true
}

func resolveCommandPath(path []string) *command {
	if len(path) == 0 {
		return nil
	}
	cur, ok := commands[path[0]]
	if !ok {
		return nil
	}
	for _, p := range path[1:] {
		if cur.sub == nil {
			return nil
		}
		cur, ok = cur.sub[p]
		if !ok {
			return nil
		}
	}
	return cur
}
