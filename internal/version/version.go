package version

import (
	"runtime/debug"
	"strings"
)

// These variables are set at build time via -ldflags.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
	GoVersion = "unknown"
)

// Info contains resolved version metadata for display.
type Info struct {
	Version   string
	Commit    string
	BuildDate string
	GoVersion string
	Dirty     bool
}

// Current returns version metadata, preferring explicit ldflags and falling
// back to Go build information when installed with `go install`.
func Current() Info {
	base := Info{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
		GoVersion: GoVersion,
	}
	if base.Version != "dev" {
		return base
	}
	info, _ := debug.ReadBuildInfo()
	return resolve(base, info)
}

func resolve(base Info, build *debug.BuildInfo) Info {
	out := base
	if out.Version != "dev" {
		return out
	}
	if build == nil {
		return out
	}
	if build.Main.Version != "" && build.Main.Version != "(devel)" {
		out.Version = build.Main.Version
	}
	if out.GoVersion == "unknown" && build.GoVersion != "" {
		out.GoVersion = build.GoVersion
	}
	settings := buildSettings(build)
	commitFromBuild := false
	if out.Commit == "unknown" {
		if revision := settings["vcs.revision"]; revision != "" {
			out.Commit = revision
			commitFromBuild = true
		} else if revision := pseudoVersionRevision(out.Version); revision != "" {
			out.Commit = revision
			commitFromBuild = true
		}
	}
	out.Dirty = settings["vcs.modified"] == "true"
	if out.Dirty && commitFromBuild {
		out.Commit += "-dirty"
	}
	return out
}

func pseudoVersionRevision(version string) string {
	version = strings.TrimSuffix(version, "+incompatible")
	parts := strings.Split(version, "-")
	if len(parts) != 3 {
		return ""
	}
	base, pseudo, revision := parts[0], parts[1], parts[2]
	if !isHexRevision(revision) || !strings.HasPrefix(base, "v") {
		return ""
	}
	if isPseudoVersionTimestamp(pseudo) {
		if !isMajorZeroBase(base) {
			return ""
		}
		return revision
	}
	if strings.HasPrefix(pseudo, "0.") && isPseudoVersionTimestamp(strings.TrimPrefix(pseudo, "0.")) && isSemanticVersionBase(base) {
		return revision
	}
	if idx := strings.LastIndex(pseudo, ".0."); idx > 0 && isPseudoVersionTimestamp(pseudo[idx+3:]) && isSemanticVersionBase(base) {
		return revision
	}
	return ""
}

func isMajorZeroBase(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 3 || parts[1] != "0" || parts[2] != "0" {
		return false
	}
	return isNumericVersionPart(strings.TrimPrefix(parts[0], "v"))
}

func isSemanticVersionBase(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return false
	}
	parts[0] = strings.TrimPrefix(parts[0], "v")
	for _, part := range parts {
		if !isNumericVersionPart(part) {
			return false
		}
	}
	return true
}

func isNumericVersionPart(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isPseudoVersionTimestamp(s string) bool {
	if len(s) != len("20060102150405") {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isHexRevision(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

func buildSettings(info *debug.BuildInfo) map[string]string {
	settings := make(map[string]string, len(info.Settings))
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}
	return settings
}
