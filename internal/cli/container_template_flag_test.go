package cli

import (
	"context"
	"strings"
	"testing"
)

// --- container template indicator (finding F11) ---
//
// /cluster/resources reports the template flag for lxc rows as well as qemu
// rows. The container mapper dropped it, so a container template was
// indistinguishable from an ordinary container in list/show output, while the
// VM equivalent (finding evidence 128) already exposed it.

// TestContainerListReportsTemplateFlag covers template vs ordinary discovery in
// the inventory JSON: a template container must carry "template": true while an
// ordinary one must not claim the flag.
func TestContainerListReportsTemplateFlag(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	t.Run("template", func(t *testing.T) {
		e2eContainerInventoryTemplateOverride = true
		t.Cleanup(func() { e2eContainerInventoryTemplateOverride = false })

		var out, errOut strings.Builder
		if err := Run(context.Background(), []string{"--output", "json", "container", "list"}, &out, &errOut); err != nil {
			t.Fatalf("container list: %v (stderr: %s)", err, errOut.String())
		}
		if !strings.Contains(out.String(), `"template": true`) {
			t.Fatalf("template container must be identified, got: %s", out.String())
		}
	})

	t.Run("ordinary", func(t *testing.T) {
		e2eContainerInventoryTemplateOverride = false

		var out, errOut strings.Builder
		if err := Run(context.Background(), []string{"--output", "json", "container", "list"}, &out, &errOut); err != nil {
			t.Fatalf("container list: %v (stderr: %s)", err, errOut.String())
		}
		if strings.Contains(out.String(), "template") {
			t.Fatalf("ordinary container must not report a template flag, got: %s", out.String())
		}
	})
}

// TestContainerShowReportsTemplateFlagInTable pins the table rendering: the
// indicator must be visible in the single-container view as well as the JSON.
func TestContainerShowReportsTemplateFlagInTable(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	e2eContainerInventoryTemplateOverride = true
	t.Cleanup(func() { e2eContainerInventoryTemplateOverride = false })

	var out, errOut strings.Builder
	if err := Run(context.Background(), []string{"--output", "table", "container", "show", "e2e-node/200"}, &out, &errOut); err != nil {
		t.Fatalf("container show: %v (stderr: %s)", err, errOut.String())
	}
	got := out.String()
	if !strings.Contains(got, "TEMPLATE") || !strings.Contains(got, "yes") {
		t.Fatalf("table must show the template indicator, got: %s", got)
	}
}

// TestContainerShowOmitsTemplateForOrdinaryGuest: an ordinary container must
// report "no" rather than being mistaken for a template.
func TestContainerShowOmitsTemplateForOrdinaryGuest(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	e2eContainerInventoryTemplateOverride = false

	var out, errOut strings.Builder
	if err := Run(context.Background(), []string{"--output", "json", "container", "show", "e2e-node/200"}, &out, &errOut); err != nil {
		t.Fatalf("container show: %v (stderr: %s)", err, errOut.String())
	}
	if strings.Contains(out.String(), "template") {
		t.Fatalf("ordinary container must not report a template flag, got: %s", out.String())
	}
}
