package version

import "runtime/debug"

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
		}
	}
	out.Dirty = settings["vcs.modified"] == "true"
	if out.Dirty && commitFromBuild {
		out.Commit += "-dirty"
	}
	return out
}

func buildSettings(info *debug.BuildInfo) map[string]string {
	settings := make(map[string]string, len(info.Settings))
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}
	return settings
}
