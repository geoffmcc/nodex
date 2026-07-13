package version

import (
	"testing"
)

func TestParseSemVerSimple(t *testing.T) {
	sv, err := ParseSemVer("1.2.3")
	if err != nil {
		t.Fatalf("ParseSemVer: %v", err)
	}
	if sv.Major != 1 || sv.Minor != 2 || sv.Patch != 3 {
		t.Fatalf("SemVer = %+v", sv)
	}
	if sv.Prerelease != "" || sv.BuildMeta != "" {
		t.Fatalf("unexpected prerelease/build: %+v", sv)
	}
}

func TestParseSemVerVPrefix(t *testing.T) {
	sv, err := ParseSemVer("v1.2.3")
	if err != nil {
		t.Fatalf("ParseSemVer: %v", err)
	}
	if sv.Major != 1 || sv.Minor != 2 || sv.Patch != 3 {
		t.Fatalf("SemVer = %+v", sv)
	}
}

func TestParseSemVerPrerelease(t *testing.T) {
	sv, err := ParseSemVer("1.2.3-alpha.1")
	if err != nil {
		t.Fatalf("ParseSemVer: %v", err)
	}
	if sv.Prerelease != "alpha.1" {
		t.Fatalf("Prerelease = %q", sv.Prerelease)
	}
}

func TestParseSemVerBuildMeta(t *testing.T) {
	sv, err := ParseSemVer("1.2.3+build.123")
	if err != nil {
		t.Fatalf("ParseSemVer: %v", err)
	}
	if sv.BuildMeta != "build.123" {
		t.Fatalf("BuildMeta = %q", sv.BuildMeta)
	}
}

func TestParseSemVerPrereleaseAndBuild(t *testing.T) {
	sv, err := ParseSemVer("1.2.3-rc.1+build.456")
	if err != nil {
		t.Fatalf("ParseSemVer: %v", err)
	}
	if sv.Prerelease != "rc.1" || sv.BuildMeta != "build.456" {
		t.Fatalf("SemVer = %+v", sv)
	}
}

func TestParseSemVerInvalid(t *testing.T) {
	tests := []string{
		"",
		"1",
		"1.2",
		"1.2.3.4",
		"a.b.c",
		"1.2.x",
	}
	for _, v := range tests {
		t.Run(v, func(t *testing.T) {
			if _, err := ParseSemVer(v); err == nil {
				t.Fatalf("expected error for %q", v)
			}
		})
	}
}

func TestCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "2.0.0", -1},
		{"2.0.0", "1.0.0", 1},
		{"1.1.0", "1.2.0", -1},
		{"1.2.0", "1.1.0", 1},
		{"1.0.1", "1.0.2", -1},
		{"1.0.2", "1.0.1", 1},
		{"1.0.0-alpha", "1.0.0-alpha", 0},
		{"1.0.0-alpha", "1.0.0-beta", -1},
		{"1.0.0-beta", "1.0.0-alpha", 1},
		{"1.0.0-alpha", "1.0.0", -1},
		{"1.0.0", "1.0.0-alpha", 1},
		{"1.0.0-alpha", "1.0.0-alpha.1", -1},
		{"1.0.0-alpha.1", "1.0.0-alpha", 1},
		{"1.0.0-alpha.1", "1.0.0-alpha.beta", -1},
		{"1.0.0-alpha.beta", "1.0.0-alpha.1", 1},
		{"1.0.0-1", "1.0.0-2", -1},
		{"1.0.0-2", "1.0.0-1", 1},
		{"1.0.0-1", "1.0.0-alpha", -1},
		{"1.0.0-alpha", "1.0.0-1", 1},
	}
	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			got, err := Compare(tt.a, tt.b)
			if err != nil {
				t.Fatalf("Compare(%q, %q): %v", tt.a, tt.b, err)
			}
			if got != tt.want {
				t.Fatalf("Compare(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCompareInvalid(t *testing.T) {
	if _, err := Compare("invalid", "1.0.0"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := Compare("1.0.0", "invalid"); err == nil {
		t.Fatal("expected error")
	}
}

func TestSemVerString(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1.2.3", "1.2.3"},
		{"v1.2.3", "1.2.3"},
		{"1.2.3-alpha.1", "1.2.3-alpha.1"},
		{"1.2.3+build.456", "1.2.3+build.456"},
		{"1.2.3-rc.1+build.789", "1.2.3-rc.1+build.789"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			sv, err := ParseSemVer(tt.input)
			if err != nil {
				t.Fatalf("ParseSemVer: %v", err)
			}
			if got := sv.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSemVerCompareMethod(t *testing.T) {
	a := &SemVer{Major: 1, Minor: 0, Patch: 0}
	b := &SemVer{Major: 2, Minor: 0, Patch: 0}
	if a.Compare(b) != -1 {
		t.Fatal("expected -1")
	}
	if b.Compare(a) != 1 {
		t.Fatal("expected 1")
	}
	if a.Compare(a) != 0 {
		t.Fatal("expected 0")
	}
}
