package cli

import "testing"

func TestNormalizeFirewallCIDR(t *testing.T) {
	for _, tt := range []struct{ input, want string }{
		{"203.0.113.7/32", "203.0.113.7/32"},
		{"203.0.113.7", "203.0.113.7/32"},
		{"2001:db8::1/128", "2001:db8::1/128"},
		{"2001:db8::1", "2001:db8::1/128"},
		{"198.51.100.0/24", "198.51.100.0/24"},
	} {
		if got := normalizeFirewallCIDR(tt.input); got != tt.want {
			t.Errorf("normalizeFirewallCIDR(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
