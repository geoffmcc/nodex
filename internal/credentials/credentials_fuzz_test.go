package credentials

import (
	"testing"
)

// FuzzParseCredentialRefStrict tests credential reference parsing at the
// trust boundary. Credential refs come from configuration files and CLI
// arguments. The parser must reject path traversal, absolute paths, and
// other potentially dangerous inputs without panicking.
func FuzzParseCredentialRefStrict(f *testing.F) {
	// Seed corpus.
	seeds := []string{
		// Valid refs.
		"myprofile",
		"keyring:myprofile",
		"env:prod",
		"file:staging",
		// Invalid refs - path traversal.
		"",
		":name",
		"file:",
		"file:../secret",
		"file:..\\secret",
		"file:/tmp/secret",
		"file:C:\\secret",
		`file:\\server\share`,
		"file:café",
		"keyring:some:complex:name",
		// Edge cases.
		"file:::name",
		"unknown:name",
		"file: -bad",
		"file:_bad",
		"env:has spaces",
		"file:too!long@name#here$",
		// Extremely long inputs.
		"file:" + string(make([]byte, 200)),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, ref string) {
		if len(ref) > 2048 {
			return
		}

		// Must never panic.
		backend, name, err := ParseCredentialRefStrict(ref)

		// If no error, the result must be consistent.
		if err == nil {
			if backend == "" || name == "" {
				t.Errorf("ParseCredentialRefStrict(%q) returned empty backend=%q or name=%q without error", ref, backend, name)
			}
		}
	})
}

// FuzzValidateName tests credential name validation with arbitrary inputs.
func FuzzValidateName(f *testing.F) {
	seeds := []string{
		"valid",
		"test-profile",
		"profile_1",
		"",
		"-bad",
		"_bad",
		"has spaces",
		"../escape",
		"C:\\windows",
		"/etc/passwd",
		"valid:but-has-colon",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, name string) {
		if len(name) > 2048 {
			return
		}
		_ = ValidateName(name)
	})
}
