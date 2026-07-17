package redact

import (
	"strings"
	"testing"
)

// FuzzStringNeverLeaksPVEToken verifies that a PVE API token secret embedded
// in arbitrary surrounding text never survives redaction.
func FuzzStringNeverLeaksPVEToken(f *testing.F) {
	f.Add("", "")
	f.Add("Authorization: ", "\nnext line")
	f.Add("error from server: ", " (status 401)")
	f.Add("prefix\x00binary", "\x1b[31msuffix")
	f.Fuzz(func(t *testing.T, prefix, suffix string) {
		const secret = "eefc7031-1111-2222-3333-fuzzsecretval" // #nosec G101 -- fictional test value
		input := prefix + " PVEAPIToken=root@pam!monitor=" + secret + suffix
		out := String(input)
		if strings.Contains(out, secret) {
			t.Errorf("PVE token secret leaked: %q -> %q", input, out)
		}
	})
}

// FuzzStringNeverLeaksPBSToken verifies that a PBS API token secret (colon
// separator) embedded in arbitrary surrounding text never survives redaction.
func FuzzStringNeverLeaksPBSToken(f *testing.F) {
	f.Add("", "")
	f.Add("Authorization: ", "\nnext line")
	f.Add("error from server: ", " (status 401)")
	f.Add("prefix\x00binary", "\x1b[31msuffix")
	f.Fuzz(func(t *testing.T, prefix, suffix string) {
		const secret = "aa1b9535-4444-5555-6666-fuzzsecretval" // #nosec G101 -- fictional test value
		input := prefix + " PBSAPIToken=backup@pbs!reader:" + secret + suffix
		out := String(input)
		if strings.Contains(out, secret) {
			t.Errorf("PBS token secret leaked: %q -> %q", input, out)
		}
	})
}

// FuzzStringNeverLeaksBareProxmoxToken verifies the bare token grammar
// (user@realm!tokenid=secret and user@realm!tokenid:secret) is redacted
// without the Authorization scheme prefix.
func FuzzStringNeverLeaksBareProxmoxToken(f *testing.F) {
	f.Add("=", "", "")
	f.Add(":", "log: ", " trailing")
	f.Add(":", "", "\twith\ttabs")
	f.Fuzz(func(t *testing.T, sep, prefix, suffix string) {
		if sep != "=" && sep != ":" {
			t.Skip()
		}
		const secret = "0badc0de-7777-8888-9999-fuzzsecretval" // #nosec G101 -- fictional test value
		input := prefix + " automation@pve!maint" + sep + secret + suffix
		out := String(input)
		if strings.Contains(out, secret) {
			t.Errorf("bare token secret leaked (sep %q): %q -> %q", sep, input, out)
		}
	})
}
