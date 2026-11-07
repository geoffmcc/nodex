package ansible

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Exercise the embedded callback with a real Ansible process and a local,
// read-only playbook. CI requires Ansible so this check cannot silently skip.
func TestAnsibleCallbackIntegration(t *testing.T) {
	if os.Getenv("NODEX_REQUIRE_ANSIBLE") != "1" {
		t.Skip("set NODEX_REQUIRE_ANSIBLE=1 to run the Ansible integration check")
	}
	executable, err := exec.LookPath("ansible-playbook")
	if err != nil {
		t.Fatal("ansible-playbook is required for this integration check")
	}
	dir := t.TempDir()
	callbackDir := filepath.Join(dir, "callbacks")
	if err := os.Mkdir(callbackDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(callbackDir, "nodex_json.py"), nodexJSONCallback, 0o600); err != nil {
		t.Fatal(err)
	}
	playbook := filepath.Join(dir, "playbook.yml")
	contents := "- hosts: localhost\n  gather_facts: false\n  tasks:\n    - name: callback fixture\n      ansible.builtin.debug:\n        msg: nodex-callback-ok\n"
	if err := os.WriteFile(playbook, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dir, "ansible.cfg")
	if err := os.WriteFile(config, []byte("[defaults]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-i", "localhost,", "-c", "local", playbook)
	cmd.Env = append(os.Environ(), "ANSIBLE_CONFIG="+config, "ANSIBLE_STDOUT_CALLBACK=nodex_json", "ANSIBLE_CALLBACK_PLUGINS="+callbackDir, "ANSIBLE_LOCAL_TEMP="+filepath.Join(dir, "tmp"), "ANSIBLE_NOCOLOR=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ansible-playbook failed: %v\n%s", err, out)
	}
	var result struct {
		Schema   int    `json:"schema"`
		Contract string `json:"contract"`
		Plays    []struct {
			Tasks []struct {
				Hosts map[string]struct {
					Msg    string `json:"msg"`
					Failed bool   `json:"failed"`
				} `json:"hosts"`
			} `json:"tasks"`
		} `json:"plays"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("invalid callback JSON: %v\n%s", err, out)
	}
	if result.Schema != 1 || result.Contract != "nodex.ansible.task-results.v1" || len(result.Plays) != 1 || len(result.Plays[0].Tasks) != 1 {
		t.Fatalf("unexpected callback structure: %s", out)
	}
	host, ok := result.Plays[0].Tasks[0].Hosts["localhost"]
	if !ok || host.Failed || host.Msg != "nodex-callback-ok" {
		t.Fatalf("unexpected callback host result: %s", out)
	}
}
