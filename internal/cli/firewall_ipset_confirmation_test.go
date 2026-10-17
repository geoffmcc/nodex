package cli

import "testing"

func TestFirewallIPSetEntryConfirmationTarget(t *testing.T) {
	tests := []struct {
		name string
		cidr string
		want string
	}{
		{name: "documentation IPv4 range", cidr: "10.0.0.0/8", want: "myipset-10.0.0.0-8"},
		{name: "IPv4 host", cidr: "192.0.2.1/32", want: "myipset-192.0.2.1-32"},
		{name: "IPv6 range", cidr: "2001:db8::/32", want: "myipset-2001:db8::-32"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firewallIPSetEntryConfirmationTarget("myipset", tt.cidr); got != tt.want {
				t.Fatalf("confirmation target = %q, want %q", got, tt.want)
			}
		})
	}
}
