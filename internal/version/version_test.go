package version

import (
	"runtime/debug"
	"testing"
)

func TestResolvePreservesCompleteLdflagsMetadata(t *testing.T) {
	got := resolve(Info{
		Version:   "v0.1.0",
		Commit:    "ld-commit",
		BuildDate: "2026-07-13T01:00:00Z",
		GoVersion: "go1.25.12",
	}, buildInfo("v0.2.0", "go1.26.0", setting("vcs.revision", "build-commit"), setting("vcs.modified", "true")))

	if got.Version != "v0.1.0" || got.Commit != "ld-commit" || got.BuildDate != "2026-07-13T01:00:00Z" || got.GoVersion != "go1.25.12" {
		t.Fatalf("metadata = %+v", got)
	}
	if got.Dirty {
		t.Fatalf("Dirty = true, want false when ldflags metadata is complete")
	}
}

func TestResolveUsesGoInstallStyleModuleVersionFallback(t *testing.T) {
	got := resolve(defaultInfo(), buildInfo(
		"v0.0.0-20260713015102-c043e8392bebd",
		"go1.25.12",
		setting("vcs.revision", "c043e8392bebdebfa5391a9bf46e29bfad93a98f"),
		setting("vcs.modified", "false"),
	))

	if got.Version != "v0.0.0-20260713015102-c043e8392bebd" {
		t.Fatalf("Version = %q", got.Version)
	}
	if got.GoVersion != "go1.25.12" {
		t.Fatalf("GoVersion = %q", got.GoVersion)
	}
	if got.Commit != "c043e8392bebdebfa5391a9bf46e29bfad93a98f" {
		t.Fatalf("Commit = %q", got.Commit)
	}
	if got.BuildDate != "unknown" {
		t.Fatalf("BuildDate = %q, want unknown", got.BuildDate)
	}
	if got.Dirty {
		t.Fatalf("Dirty = true, want false")
	}
}

func TestResolveHandlesLocalDevelBuildInfo(t *testing.T) {
	got := resolve(defaultInfo(), buildInfo(
		"(devel)",
		"go1.25.12",
		setting("vcs.revision", "local-revision"),
		setting("vcs.modified", "false"),
	))

	if got.Version != "dev" {
		t.Fatalf("Version = %q, want dev", got.Version)
	}
	if got.GoVersion != "go1.25.12" || got.Commit != "local-revision" {
		t.Fatalf("metadata = %+v", got)
	}
}

func TestResolveHandlesAbsentVCSSettings(t *testing.T) {
	got := resolve(defaultInfo(), buildInfo("(devel)", "go1.25.12"))

	if got.Version != "dev" || got.GoVersion != "go1.25.12" || got.Commit != "unknown" || got.BuildDate != "unknown" {
		t.Fatalf("metadata = %+v", got)
	}
	if got.Dirty {
		t.Fatalf("Dirty = true, want false")
	}
}

func TestResolveHandlesUnavailableBuildInfo(t *testing.T) {
	got := resolve(defaultInfo(), nil)

	if got.Version != "dev" || got.GoVersion != "unknown" || got.Commit != "unknown" || got.BuildDate != "unknown" {
		t.Fatalf("metadata = %+v", got)
	}
}

func TestResolveMarksDirtyVCSState(t *testing.T) {
	got := resolve(defaultInfo(), buildInfo(
		"(devel)",
		"go1.25.12",
		setting("vcs.revision", "dirty-revision"),
		setting("vcs.modified", "true"),
	))

	if !got.Dirty {
		t.Fatalf("Dirty = false, want true")
	}
	if got.Commit != "dirty-revision-dirty" {
		t.Fatalf("Commit = %q, want dirty suffix", got.Commit)
	}
}

func TestResolveLdflagsOverrideBuildInfoFallback(t *testing.T) {
	got := resolve(Info{
		Version:   "dev",
		Commit:    "ld-commit",
		BuildDate: "unknown",
		GoVersion: "go-ldflags",
	}, buildInfo(
		"v0.0.0-20260713015102-c043e8392bebd",
		"go1.25.12",
		setting("vcs.revision", "build-commit"),
		setting("vcs.modified", "true"),
	))

	if got.Version != "v0.0.0-20260713015102-c043e8392bebd" || got.Commit != "ld-commit" || got.GoVersion != "go-ldflags" {
		t.Fatalf("metadata = %+v", got)
	}
	if !got.Dirty {
		t.Fatalf("Dirty = false, want true")
	}
	if got.BuildDate != "unknown" {
		t.Fatalf("metadata = %+v", got)
	}
}

func defaultInfo() Info {
	return Info{Version: "dev", Commit: "unknown", BuildDate: "unknown", GoVersion: "unknown"}
}

func buildInfo(version, goVersion string, settings ...debug.BuildSetting) *debug.BuildInfo {
	return &debug.BuildInfo{
		GoVersion: goVersion,
		Main:      debug.Module{Path: "github.com/geoffmcc/nodex", Version: version},
		Settings:  settings,
	}
}

func setting(key, value string) debug.BuildSetting {
	return debug.BuildSetting{Key: key, Value: value}
}
