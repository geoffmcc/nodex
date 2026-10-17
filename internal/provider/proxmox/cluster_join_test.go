package proxmox

import (
	"strings"
	"testing"
)

func TestBuildClusterJoinSSHArgsPinsHostAndForwardsAgent(t *testing.T) {
	fingerprint := strings.Repeat("AA:", 31) + "AA"
	args := buildClusterJoinSSHArgs("pve-join.example", "root", "/keys/join", 2222, "/keys/known_hosts", "10.0.0.10", fingerprint, true)
	joined := strings.Join(args, " ")
	for _, want := range []string{"-T", "-A", "-i /keys/join", "-p 2222", "StrictHostKeyChecking=yes", "UserKnownHostsFile=/keys/known_hosts", "root@pve-join.example", "pvecm add '10.0.0.10'", "--fingerprint '" + fingerprint + "'", "--use_ssh 1"} {
		if !strings.Contains(joined, want) {
			t.Errorf("SSH args %q do not contain %q", joined, want)
		}
	}
	if strings.Contains(strings.ToLower(joined), "password") {
		t.Fatalf("cluster join SSH args contain a password field: %q", joined)
	}
}

func TestBuildClusterJoinSSHArgsDefaultsPortAndDoesNotForwardMissingAgent(t *testing.T) {
	args := buildClusterJoinSSHArgs("10.0.0.11", "root", "/keys/join", 0, "", "10.0.0.10", strings.Repeat("BB:", 31)+"BB", false)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-p 22") || strings.Contains(joined, "-A") {
		t.Fatalf("SSH args = %q; expected port 22 and no agent forwarding", joined)
	}
}

func TestClusterJoinAddressValidationRejectsShellSyntax(t *testing.T) {
	for _, address := range []string{"10.0.0.10;id", "peer name", "-oProxyCommand=evil"} {
		if safeClusterNodeAddress(address) {
			t.Errorf("safeClusterNodeAddress(%q) = true", address)
		}
	}
	for _, host := range []string{"10.0.0.11", "pve-join.example", "2001:db8::11"} {
		if !safeClusterNodeAddress(host) {
			t.Errorf("safeClusterNodeAddress(%q) = false", host)
		}
	}
}

func TestBuildClusterInitSSHArgsPinsHostAndQuotesArguments(t *testing.T) {
	args := buildClusterInitSSHArgs("10.0.0.10", "root", "/keys/pve", 22, "/keys/known_hosts", "lab-cluster", "10.0.0.10")
	joined := strings.Join(args, " ")
	for _, want := range []string{"StrictHostKeyChecking=yes", "UserKnownHostsFile=/keys/known_hosts", "root@10.0.0.10", "pvecm create 'lab-cluster' --link0 '10.0.0.10'"} {
		if !strings.Contains(joined, want) {
			t.Errorf("SSH args %q do not contain %q", joined, want)
		}
	}
}

func TestClusterNamePatternRejectsShellSyntax(t *testing.T) {
	for _, name := range []string{"lab;id", "lab cluster", "-bad"} {
		if clusterNamePattern.MatchString(name) {
			t.Errorf("clusterNamePattern accepts unsafe name %q", name)
		}
	}
}
