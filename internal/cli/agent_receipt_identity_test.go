package cli

import (
	"testing"

	"github.com/geoffmcc/nodex/internal/agent"
)

func TestSameExecutionIdentityIgnoresUnrecordedSSHSettings(t *testing.T) {
	receipt := agent.ExecutionContext{
		Profile: "lab", Provider: "proxmox", Endpoint: "https://pve.example:8006",
		TLSCAFile: "/etc/nodex/lab-ca.pem",
	}
	current := receipt
	current.SSHHost = "10.0.0.10"
	current.SSHUser = "root"
	current.SSHKeyFile = "/home/user/.ssh/lab"
	current.SSHPort = 22
	if !sameExecutionIdentity(receipt, current) {
		t.Fatal("unrecorded SSH settings should not change identity for an API-only receipt")
	}

	changedEndpoint := current
	changedEndpoint.Endpoint = "https://other.example:8006"
	if sameExecutionIdentity(receipt, changedEndpoint) {
		t.Fatal("different provider endpoint must not match")
	}

	changedCA := current
	changedCA.TLSCAFile = "/etc/nodex/other-ca.pem"
	if sameExecutionIdentity(receipt, changedCA) {
		t.Fatal("different TLS CA must not match")
	}

	sshBoundReceipt := receipt
	sshBoundReceipt.SSHHost = "10.0.0.10"
	if !sameExecutionIdentity(sshBoundReceipt, current) {
		t.Fatal("matching recorded SSH identity should match")
	}
	changedSSH := current
	changedSSH.SSHHost = "10.0.0.11"
	if sameExecutionIdentity(sshBoundReceipt, changedSSH) {
		t.Fatal("different recorded SSH host must not match")
	}
}
