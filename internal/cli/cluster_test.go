package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestClusterCommandsAreRegistered(t *testing.T) {
	cluster, ok := GetCommand("cluster")
	if !ok {
		t.Fatal("cluster command is not registered")
	}
	for _, name := range []string{"init", "join"} {
		if _, ok := cluster.sub[name]; !ok {
			t.Fatalf("cluster command missing %s subcommand", name)
		}
	}
}

func TestClusterInitRequiresExpertBeforeConnecting(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"cluster", "init", "lab", "10.0.0.10"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "expert") {
		t.Fatalf("error = %v, want expert-mode refusal", err)
	}
	if strings.Contains(stdout.String()+stderr.String(), "10.0.0.10") {
		t.Log("target is present in the safety description")
	}
}

func TestClusterInitNonInteractiveRequiresExactTargetConfirmation(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{
		"--non-interactive", "--expert", "--yes", "--force",
		"cluster", "init", "lab", "10.0.0.10",
	}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "confirmation") {
		t.Fatalf("error = %v, want fail-closed confirmation refusal", err)
	}
}

func TestClusterJoinNeverAcceptsPasswordArgument(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{
		"--non-interactive", "--expert", "--yes", "--force", "--confirm-target", "10.0.0.20",
		"cluster", "join", "10.0.0.20", strings.Repeat("AA:", 31) + "AA",
		"password=not-a-password-argument",
	}, &stdout, &stderr)
	if err == nil {
		t.Fatal("join accepted an extra password argument")
	}
	if strings.Contains(err.Error(), "not-a-password-argument") {
		t.Fatal("password argument was echoed in the error")
	}
}
