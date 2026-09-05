package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/config"
)

func TestProfileDiagnosePermissionsReportsMissingCredential(t *testing.T) {
	isolateConfigAndHome(t)
	if err := config.Write(&config.Config{
		Version:        config.CurrentSchemaVersion,
		CurrentProfile: "lab",
		Profiles: map[string]config.Profile{
			"lab": {Provider: config.ProviderProxmox, Endpoint: "https://pve.example.test:8006"},
		},
	}); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"--output", "json", "profile", "diagnose-permissions", "lab"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("diagnose-permissions: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, `"name": "credential"`) || !strings.Contains(out, `"status": "missing"`) {
		t.Fatalf("missing credential check not reported: %s", out)
	}
	if strings.Contains(out, "PVEAPIToken") || strings.Contains(out, "secret") {
		t.Fatalf("diagnostic output exposed credential material: %s", out)
	}
}
