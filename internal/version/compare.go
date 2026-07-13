package version

import (
	"fmt"
	"strconv"
	"strings"
)

type SemVer struct {
	Major      int64
	Minor      int64
	Patch      int64
	Prerelease string
	BuildMeta  string
}

func (s *SemVer) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d.%d.%d", s.Major, s.Minor, s.Patch)
	if s.Prerelease != "" {
		b.WriteByte('-')
		b.WriteString(s.Prerelease)
	}
	if s.BuildMeta != "" {
		b.WriteByte('+')
		b.WriteString(s.BuildMeta)
	}
	return b.String()
}

func (s *SemVer) Compare(other *SemVer) int {
	if d := compareInt(s.Major, other.Major); d != 0 {
		return d
	}
	if d := compareInt(s.Minor, other.Minor); d != 0 {
		return d
	}
	if d := compareInt(s.Patch, other.Patch); d != 0 {
		return d
	}
	return comparePrerelease(s.Prerelease, other.Prerelease)
}

func ParseSemVer(v string) (*SemVer, error) {
	v = strings.TrimPrefix(v, "v")
	plusIdx := strings.IndexByte(v, '+')
	var buildMeta string
	if plusIdx >= 0 {
		buildMeta = v[plusIdx+1:]
		v = v[:plusIdx]
	}
	dashIdx := strings.IndexByte(v, '-')
	var prerelease string
	if dashIdx >= 0 {
		prerelease = v[dashIdx+1:]
		v = v[:dashIdx]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid semver %q: must be MAJOR.MINOR.PATCH", v)
	}
	major, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid major version %q: %w", parts[0], err)
	}
	minor, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid minor version %q: %w", parts[1], err)
	}
	patch, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid patch version %q: %w", parts[2], err)
	}
	return &SemVer{
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		Prerelease: prerelease,
		BuildMeta:  buildMeta,
	}, nil
}

func Compare(a, b string) (int, error) {
	va, err := ParseSemVer(a)
	if err != nil {
		return 0, err
	}
	vb, err := ParseSemVer(b)
	if err != nil {
		return 0, err
	}
	return va.Compare(vb), nil
}

func compareInt(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func comparePrerelease(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" {
		return 1
	}
	if b == "" {
		return -1
	}
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	for i := 0; i < len(aParts) && i < len(bParts); i++ {
		d := comparePrereleasePart(aParts[i], bParts[i])
		if d != 0 {
			return d
		}
	}
	if len(aParts) < len(bParts) {
		return -1
	}
	if len(aParts) > len(bParts) {
		return 1
	}
	return 0
}

func comparePrereleasePart(a, b string) int {
	aNum, aIsNum := isNumeric(a)
	bNum, bIsNum := isNumeric(b)
	if aIsNum && bIsNum {
		return compareInt(aNum, bNum)
	}
	if aIsNum {
		return -1
	}
	if bIsNum {
		return 1
	}
	return strings.Compare(a, b)
}

func isNumeric(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}
