package ansible

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const priorPolicyFixture = "# Managed by Nodex: unattended security updates\nAPT::Periodic::Unattended-Upgrade \"0\";\n"
const appliedPolicyFixture = "# Managed by Nodex: unattended security updates\nAPT::Periodic::Unattended-Upgrade \"1\";\n"

type policyPlaybookFixture struct {
	root, playbook, inventory, config string
	policyPath, backupRoot            string
	timerStatePath, failMarkerPath    string
	executable                        string
}

func TestEmbeddedSecurityPolicyPlaybooksAgainstIsolatedAnsibleFixture(t *testing.T) {
	executable, err := exec.LookPath("ansible-playbook")
	if err != nil {
		t.Skip("Ansible is not installed; CI runs this integration with ansible-core")
	}
	apply, err := Lookup("apply-security-policy")
	if err != nil {
		t.Fatal(err)
	}
	restore, err := Lookup("restore-security-policy")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("first application absent paths and restore absent prior file", func(t *testing.T) {
		f := newPolicyPlaybookFixture(t, executable)
		if err := f.run(t, apply.Playbook(), f.applyVars(false, "", false)); err != nil {
			t.Fatalf("first apply against absent files/directories: %v", err)
		}
		if got := readFixtureFile(t, f.policyPath); got != appliedPolicyFixture {
			t.Fatalf("applied policy = %q", got)
		}
		state := f.readTimerState(t)
		if !state.Enabled || state.State != "started" {
			t.Fatalf("apply timer state = %+v, want enabled and started", state)
		}
		vars := f.restoreVars(false, "", checksum(appliedPolicyFixture), "not-found", "not-found", false)
		if err := f.run(t, restore.Playbook(), vars); err != nil {
			t.Fatalf("restore absent prior policy: %v", err)
		}
		if _, err := os.Lstat(f.policyPath); !os.IsNotExist(err) {
			t.Fatalf("new policy file remains after restore: %v", err)
		}
		state = f.readTimerState(t)
		if state.Enabled || state.State != "stopped" {
			t.Fatalf("restored timer state = %+v, want disabled and stopped", state)
		}
	})

	t.Run("existing managed file backup and successful prior file/timer restore", func(t *testing.T) {
		f := newPolicyPlaybookFixture(t, executable)
		writeFixtureFile(t, f.policyPath, priorPolicyFixture)
		if err := f.run(t, apply.Playbook(), f.applyVars(true, checksum(priorPolicyFixture), false)); err != nil {
			t.Fatalf("apply with existing managed file: %v", err)
		}
		backup := filepath.Join(f.backupRoot, "fixture-plan", "99nodex-unattended-security")
		if got := readFixtureFile(t, backup); got != priorPolicyFixture {
			t.Fatalf("retained backup = %q", got)
		}
		vars := f.restoreVars(true, checksum(priorPolicyFixture), checksum(appliedPolicyFixture), "disabled", "inactive", false)
		if err := f.run(t, restore.Playbook(), vars); err != nil {
			t.Fatalf("restore prior managed file: %v", err)
		}
		if got := readFixtureFile(t, f.policyPath); got != priorPolicyFixture {
			t.Fatalf("restored policy = %q, want original content", got)
		}
		state := f.readTimerState(t)
		if state.Enabled || state.State != "stopped" {
			t.Fatalf("restored timer state = %+v, want disabled and stopped", state)
		}
	})

	t.Run("symlink and unexpected policy file type refused", func(t *testing.T) {
		t.Run("symlink", func(t *testing.T) {
			f := newPolicyPlaybookFixture(t, executable)
			target := filepath.Join(f.root, "external-policy")
			writeFixtureFile(t, target, "administrator content\n")
			if err := os.Symlink(target, f.policyPath); err != nil {
				t.Fatal(err)
			}
			if err := f.run(t, apply.Playbook(), f.applyVars(false, "", false)); err == nil {
				t.Fatal("apply accepted a symlink policy path")
			}
			if got := readFixtureFile(t, target); got != "administrator content\n" {
				t.Fatalf("symlink target changed to %q", got)
			}
		})
		t.Run("directory", func(t *testing.T) {
			f := newPolicyPlaybookFixture(t, executable)
			if err := os.Mkdir(f.policyPath, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := f.run(t, apply.Playbook(), f.applyVars(false, "", false)); err == nil {
				t.Fatal("apply accepted a directory at the managed file path")
			}
		})
	})

	t.Run("backup conflict blocks apply before managed file write", func(t *testing.T) {
		f := newPolicyPlaybookFixture(t, executable)
		backup := filepath.Join(f.backupRoot, "fixture-plan")
		if err := os.MkdirAll(backup, 0o700); err != nil {
			t.Fatal(err)
		}
		writeFixtureFile(t, filepath.Join(backup, "99nodex-unattended-security"), "occupied\n")
		if err := f.run(t, apply.Playbook(), f.applyVars(false, "", false)); err == nil {
			t.Fatal("apply overwrote a pre-existing backup")
		}
		if _, err := os.Lstat(f.policyPath); !os.IsNotExist(err) {
			t.Fatalf("policy was written despite backup conflict: %v", err)
		}
	})

	t.Run("backup symlinks and special objects are refused", func(t *testing.T) {
		t.Run("backup root symlink", func(t *testing.T) {
			f := newPolicyPlaybookFixture(t, executable)
			if err := os.MkdirAll(filepath.Dir(f.backupRoot), 0o700); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(f.root, "backup-target")
			if err := os.Mkdir(target, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, f.backupRoot); err != nil {
				t.Fatal(err)
			}
			if err := f.run(t, apply.Playbook(), f.applyVars(false, "", false)); err == nil {
				t.Fatal("apply accepted a symlink backup root")
			}
		})
		t.Run("plan directory symlink", func(t *testing.T) {
			f := newPolicyPlaybookFixture(t, executable)
			if err := os.MkdirAll(f.backupRoot, 0o700); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(f.root, "backup-target")
			if err := os.Mkdir(target, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(f.backupRoot, "fixture-plan")); err != nil {
				t.Fatal(err)
			}
			if err := f.run(t, apply.Playbook(), f.applyVars(false, "", false)); err == nil {
				t.Fatal("apply accepted a symlink plan backup directory")
			}
		})
		t.Run("backup file symlink", func(t *testing.T) {
			f := newPolicyPlaybookFixture(t, executable)
			backupDir := filepath.Join(f.backupRoot, "fixture-plan")
			if err := os.MkdirAll(backupDir, 0o700); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(f.root, "external-backup")
			writeFixtureFile(t, target, "external backup\n")
			if err := os.Symlink(target, filepath.Join(backupDir, "99nodex-unattended-security")); err != nil {
				t.Fatal(err)
			}
			if err := f.run(t, apply.Playbook(), f.applyVars(false, "", false)); err == nil {
				t.Fatal("apply accepted a symlink backup file")
			}
			if got := readFixtureFile(t, target); got != "external backup\n" {
				t.Fatalf("external backup target changed: %q", got)
			}
		})
		t.Run("backup file directory", func(t *testing.T) {
			f := newPolicyPlaybookFixture(t, executable)
			backupDir := filepath.Join(f.backupRoot, "fixture-plan")
			if err := os.MkdirAll(filepath.Join(backupDir, "99nodex-unattended-security"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := f.run(t, apply.Playbook(), f.applyVars(false, "", false)); err == nil {
				t.Fatal("apply accepted a directory at the backup file path")
			}
		})
	})

	t.Run("partial apply remains recoverable", func(t *testing.T) {
		f := newPolicyPlaybookFixture(t, executable)
		if err := f.run(t, apply.Playbook(), f.applyVars(false, "", true)); err == nil {
			t.Fatal("injected timer failure did not fail the Ansible play")
		}
		if got := readFixtureFile(t, f.policyPath); got != appliedPolicyFixture {
			t.Fatalf("partial apply policy = %q, want the write before timer failure", got)
		}
		if err := f.run(t, restore.Playbook(), f.restoreVars(false, "", checksum(appliedPolicyFixture), "disabled", "inactive", false)); err != nil {
			t.Fatalf("restore after partial apply: %v", err)
		}
		if _, err := os.Lstat(f.policyPath); !os.IsNotExist(err) {
			t.Fatalf("partial apply was not restored: %v", err)
		}
	})

	t.Run("interrupted restore resumes idempotently", func(t *testing.T) {
		f := newPolicyPlaybookFixture(t, executable)
		writeFixtureFile(t, f.policyPath, priorPolicyFixture)
		if err := f.run(t, apply.Playbook(), f.applyVars(true, checksum(priorPolicyFixture), false)); err != nil {
			t.Fatalf("apply: %v", err)
		}
		vars := f.restoreVars(true, checksum(priorPolicyFixture), checksum(appliedPolicyFixture), "enabled", "active", true)
		if err := f.run(t, restore.Playbook(), vars); err == nil {
			t.Fatal("injected interrupted restore unexpectedly succeeded")
		}
		if got := readFixtureFile(t, f.policyPath); got != priorPolicyFixture {
			t.Fatalf("restore did not complete file step before interruption: %q", got)
		}
		vars["nodex_policy_current_checksum"] = checksum(priorPolicyFixture)
		vars["nodex_test_fail_once"] = false
		if err := f.run(t, restore.Playbook(), vars); err != nil {
			t.Fatalf("retry restore: %v", err)
		}
		if got := readFixtureFile(t, f.policyPath); got != priorPolicyFixture {
			t.Fatalf("retry changed restored file: %q", got)
		}
		state := f.readTimerState(t)
		if !state.Enabled || state.State != "started" {
			t.Fatalf("recovered timer state = %+v, want enabled and started", state)
		}
	})

	t.Run("restore refuses a symlinked retained backup", func(t *testing.T) {
		f := newPolicyPlaybookFixture(t, executable)
		writeFixtureFile(t, f.policyPath, priorPolicyFixture)
		if err := f.run(t, apply.Playbook(), f.applyVars(true, checksum(priorPolicyFixture), false)); err != nil {
			t.Fatalf("apply: %v", err)
		}
		backup := filepath.Join(f.backupRoot, "fixture-plan", "99nodex-unattended-security")
		if err := os.Remove(backup); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(f.root, "external-restore-backup")
		writeFixtureFile(t, target, priorPolicyFixture)
		if err := os.Symlink(target, backup); err != nil {
			t.Fatal(err)
		}
		vars := f.restoreVars(true, checksum(priorPolicyFixture), checksum(appliedPolicyFixture), "disabled", "inactive", false)
		if err := f.run(t, restore.Playbook(), vars); err == nil {
			t.Fatal("restore followed a symlinked retained backup")
		}
		if got := readFixtureFile(t, f.policyPath); got != appliedPolicyFixture {
			t.Fatalf("policy changed despite unsafe backup refusal: %q", got)
		}
	})
}

func newPolicyPlaybookFixture(t *testing.T, executable string) *policyPlaybookFixture {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	library := filepath.Join(root, "library")
	localTmp := filepath.Join(root, "local-tmp")
	for _, dir := range []string{library, localTmp} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	packageModule := `#!/usr/bin/python3
from ansible.module_utils.basic import AnsibleModule
m = AnsibleModule(argument_spec={"name": {"type": "str"}, "state": {"type": "str"}, "update_cache": {"type": "bool", "default": False}})
m.exit_json(changed=False, fixture_package_state="retained")
`
	timerModule := `#!/usr/bin/python3
import json, os
from ansible.module_utils.basic import AnsibleModule
m = AnsibleModule(argument_spec={"name": {"type": "str", "required": True}, "enabled": {"type": "bool", "required": True}, "state": {"type": "str", "required": True}, "state_path": {"type": "str", "required": True}, "marker_path": {"type": "str", "required": True}, "fail_once": {"type": "bool", "default": False}})
p = m.params
if p["fail_once"] and not os.path.exists(p["marker_path"]):
    with open(p["marker_path"], "w", encoding="utf-8") as f: f.write("failed once")
    m.fail_json(msg="injected isolated timer interruption")
with open(p["state_path"], "w", encoding="utf-8") as f: json.dump({"enabled": p["enabled"], "state": p["state"]}, f)
m.exit_json(changed=True, fixture_timer_state="recorded")
`
	if err := os.WriteFile(filepath.Join(library, "nodex_fixture_package.py"), []byte(packageModule), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(library, "nodex_fixture_systemd.py"), []byte(timerModule), 0o700); err != nil {
		t.Fatal(err)
	}
	ansibleConfig := fmt.Sprintf("[defaults]\nhost_key_checking = True\nretry_files_enabled = False\nlibrary = %s\n", library)
	configPath := filepath.Join(root, "ansible.cfg")
	if err := os.WriteFile(configPath, []byte(ansibleConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	inventory := filepath.Join(root, "inventory.ini")
	if err := os.WriteFile(inventory, []byte("[all]\nfixture ansible_connection=local\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := &policyPlaybookFixture{
		root: root, inventory: inventory, config: configPath, executable: executable,
		policyPath:     filepath.Join(root, "etc", "apt", "apt.conf.d", "99nodex-unattended-security"),
		backupRoot:     filepath.Join(root, "var", "lib", "nodex", "security-policy-backups"),
		timerStatePath: filepath.Join(root, "timer-state.json"), failMarkerPath: filepath.Join(root, "timer-failed-once"),
	}
	if err := os.MkdirAll(filepath.Dir(f.policyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	f.playbook = filepath.Join(root, "playbook.yml")
	return f
}

func (f *policyPlaybookFixture) applyVars(beforeExists bool, beforeChecksum string, failOnce bool) map[string]any {
	return map[string]any{
		"nodex_policy_plan_id": "fixture-plan", "nodex_policy_config_path": f.policyPath,
		"nodex_security_policy_backup_root": f.backupRoot, "nodex_policy_before_exists": beforeExists,
		"nodex_policy_before_checksum": beforeChecksum, "nodex_policy_config": appliedPolicyFixture,
		"nodex_test_state_path": f.timerStatePath, "nodex_test_marker_path": f.failMarkerPath,
		"nodex_test_fail_once": failOnce,
	}
}

func (f *policyPlaybookFixture) restoreVars(beforeExists bool, beforeChecksum string, currentChecksum, timerEnabled, timerActive string, failOnce bool) map[string]any {
	return map[string]any{
		"nodex_policy_plan_id": "fixture-plan", "nodex_policy_config_path": f.policyPath,
		"nodex_security_policy_backup_root": f.backupRoot, "nodex_policy_before_exists": beforeExists,
		"nodex_policy_before_checksum": beforeChecksum, "nodex_policy_current_exists": true,
		"nodex_policy_current_checksum": currentChecksum, "nodex_policy_before_timer_enabled": timerEnabled,
		"nodex_policy_before_timer_active": timerActive,
		"nodex_test_state_path":            f.timerStatePath, "nodex_test_marker_path": f.failMarkerPath,
		"nodex_test_fail_once": failOnce,
	}
}

func (f *policyPlaybookFixture) run(t *testing.T, embedded string, extra map[string]any) error {
	t.Helper()
	playbook := strings.ReplaceAll(embedded, "become: true", "become: false")
	playbook = strings.ReplaceAll(playbook, "/var/lib/nodex", filepath.Join(f.root, "var", "lib", "nodex"))
	playbook = strings.ReplaceAll(playbook, "ansible.builtin.apt:", "nodex_fixture_package:")
	playbook = strings.ReplaceAll(playbook, "      ansible.builtin.systemd:\n", "      nodex_fixture_systemd:\n        state_path: \"{{ nodex_test_state_path }}\"\n        marker_path: \"{{ nodex_test_marker_path }}\"\n        fail_once: \"{{ nodex_test_fail_once | default(false) }}\"\n")
	playbook = strings.ReplaceAll(playbook, "        owner: root\n", "")
	playbook = strings.ReplaceAll(playbook, "        group: root\n", "")
	if strings.Contains(embedded, "ansible.builtin.systemd:") && !strings.Contains(playbook, "nodex_fixture_systemd:") {
		t.Fatal("isolated fixture did not replace the timer side effect")
	}
	if strings.Contains(embedded, "ansible.builtin.apt:") && !strings.Contains(playbook, "nodex_fixture_package:") {
		t.Fatal("isolated fixture did not replace the package side effect")
	}
	if err := os.WriteFile(f.playbook, []byte(playbook), 0o600); err != nil {
		return err
	}
	extraVars, err := json.Marshal(extra)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, f.executable, "-i", f.inventory, "--extra-vars", string(extraVars), f.playbook) // #nosec G204 -- f.executable is fixture-validated; inventory/playbook are private temp-dir paths
	cmd.Env = append(os.Environ(), "ANSIBLE_CONFIG="+f.config, "ANSIBLE_LOCAL_TEMP="+filepath.Join(f.root, "local-tmp"), "ANSIBLE_HOST_KEY_CHECKING=True", "ANSIBLE_RETRY_FILES_ENABLED=False")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ansible-playbook: %w\n%s", err, output)
	}
	return nil
}

func (f *policyPlaybookFixture) readTimerState(t *testing.T) struct {
	Enabled bool   `json:"enabled"`
	State   string `json:"state"`
} {
	t.Helper()
	var state struct {
		Enabled bool   `json:"enabled"`
		State   string `json:"state"`
	}
	data, err := os.ReadFile(f.timerStatePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	return state
}

func writeFixtureFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFixtureFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func checksum(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}
