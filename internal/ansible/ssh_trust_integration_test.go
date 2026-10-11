package ansible

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestOpenSSHRejectsUnknownAndChangedKeysAcrossSSHAndTransfer(t *testing.T) {
	sshd, err := exec.LookPath("sshd")
	if err != nil {
		t.Skip("sshd is unavailable; CI's ssh-hostkey integration job installs OpenSSH server")
	}
	ansiblePlaybook, err := exec.LookPath("ansible-playbook")
	if err != nil {
		t.Skip("ansible-playbook is unavailable")
	}
	if safetyErr := checkExecutableSafety(ansiblePlaybook); safetyErr != nil {
		// Runner images can put a world-writable pipx shim ahead of the
		// distro-provided ansible-core on PATH. Prefer the first candidate that
		// passes the same safety gate so this SSH trust test still runs where a
		// safe ansible-playbook is installed; otherwise the environment cannot
		// exercise the probe and the test is skipped rather than failed.
		if alternative, ok := safeAnsiblePlaybookCandidate(); ok {
			ansiblePlaybook = alternative
		} else {
			t.Skipf("no safely executable ansible-playbook available: %v", safetyErr)
		}
	}
	ssh, err := exec.LookPath("ssh")
	if err != nil {
		t.Skip("OpenSSH client is unavailable")
	}
	sftp, err := exec.LookPath("sftp")
	if err != nil {
		t.Skip("OpenSSH sftp client is unavailable")
	}
	scp, err := exec.LookPath("scp")
	if err != nil {
		t.Skip("OpenSSH scp client is unavailable")
	}
	keygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen is unavailable")
	}
	account, err := user.Current()
	if err != nil {
		t.Skipf("cannot resolve local SSH test account: %v", err)
	}
	root := t.TempDir()
	// Control sockets live in their own short-prefix temp directory: the Unix
	// socket path limit is ~108 bytes and OpenSSH appends a random suffix, so a
	// ControlPath under t.TempDir() (which embeds the long test name) overflows.
	controlDir, err := os.MkdirTemp("", "nodex-ctl-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(controlDir) })
	controlPath := filepath.Join(controlDir, "s")
	clientKey := filepath.Join(root, "client")
	firstHostKey := filepath.Join(root, "host-first")
	secondHostKey := filepath.Join(root, "host-second")
	for _, key := range []string{clientKey, firstHostKey, secondHostKey} {
		if output, err := exec.Command(keygen, "-q", "-t", "ed25519", "-N", "", "-f", key).CombinedOutput(); err != nil { // #nosec G204 -- ssh-keygen is resolved via exec.LookPath; arguments are test-controlled
			t.Fatalf("ssh-keygen: %v\n%s", err, output)
		}
	}
	clientPub, err := os.ReadFile(clientKey + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	knownHosts := filepath.Join(root, "known_hosts")
	permissiveConfig := filepath.Join(root, "config")
	if err := os.WriteFile(permissiveConfig, []byte("Host *\n  StrictHostKeyChecking no\n  UserKnownHostsFile /dev/null\n  ControlMaster yes\n  ControlPath "+controlPath+"\n  ControlPersist 10m\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "config"), []byte("Host *\n  StrictHostKeyChecking no\n  UserKnownHostsFile /dev/null\n  ControlMaster yes\n  ControlPath "+controlPath+"\n  ControlPersist 10m\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	port, err := unusedTCPPort()
	if err != nil {
		t.Fatal(err)
	}
	portText := strconv.Itoa(port)
	serverConfig := filepath.Join(root, "sshd_config")
	writeServerConfig := func(hostKey string) {
		t.Helper()
		config := fmt.Sprintf("Port %s\nListenAddress 127.0.0.1\nHostKey %s\nPidFile %s\nAuthorizedKeysFile %s\nStrictModes no\nPasswordAuthentication no\nKbdInteractiveAuthentication no\nPubkeyAuthentication yes\nPermitRootLogin yes\nUsePAM no\nSubsystem sftp internal-sftp\nAllowUsers %s\nLogLevel ERROR\n", portText, hostKey, filepath.Join(root, "sshd.pid"), filepath.Join(root, "authorized_keys"), account.Username)
		if err := os.WriteFile(serverConfig, []byte(config), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "authorized_keys"), clientPub, 0o600); err != nil { // #nosec G703 -- path is derived from the test's private temporary directory
		t.Fatal(err)
	}
	writeKnownHost := func(keyPath string) {
		t.Helper()
		pub, readErr := os.ReadFile(keyPath + ".pub")
		if readErr != nil {
			t.Fatal(readErr)
		}
		fields := strings.Fields(string(pub))
		if len(fields) < 2 {
			t.Fatalf("invalid host public key file")
		}
		entry := fmt.Sprintf("[127.0.0.1]:%s %s %s\n", portText, fields[0], fields[1])
		if err := os.WriteFile(knownHosts, []byte(entry), 0o600); err != nil { // #nosec G703 -- knownHosts is a path inside the test's private temporary directory
			t.Fatal(err)
		}
	}
	startServer := func(hostKey string) func() {
		t.Helper()
		writeServerConfig(hostKey)
		ctx, cancel := context.WithCancel(context.Background())
		cmd := exec.CommandContext(ctx, sshd, "-D", "-e", "-f", serverConfig) // #nosec G204 -- sshd is resolved via exec.LookPath; arguments are test-controlled
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Start(); err != nil {
			cancel()
			t.Fatalf("start sshd: %v", err)
		}
		ready := false
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			conn, dialErr := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", portText), 50*time.Millisecond)
			if dialErr == nil {
				_ = conn.Close()
				ready = true
				break
			}
			if cmd.ProcessState != nil {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if !ready {
			cancel()
			_ = cmd.Wait()
			t.Fatalf("sshd did not become ready: %s", stderr.String())
		}
		return func() {
			cancel()
			_ = cmd.Wait()
		}
	}

	localFile := filepath.Join(root, "transfer.txt")
	if err := os.WriteFile(localFile, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	remoteFile := filepath.Join(account.HomeDir, "nodex-ssh-trust-"+strconv.Itoa(port)+".txt")
	defer os.Remove(remoteFile)
	probe := func(tool string) error {
		opts := strings.Fields(strictSSHOptions(knownHosts))
		switch tool {
		case "ssh":
			args := make([]string, 0, 7+len(opts)+2)
			args = append(args, "-F", permissiveConfig, "-p", portText, "-i", clientKey, "-oIdentitiesOnly=yes")
			args = append(args, opts...)
			args = append(args, account.Username+"@127.0.0.1", "true")
			return exec.Command(ssh, args...).Run() // #nosec G204 -- ssh is resolved via exec.LookPath; arguments are test-controlled
		case "sftp":
			args := make([]string, 0, 7+len(opts)+3)
			args = append(args, "-F", permissiveConfig, "-P", portText, "-i", clientKey, "-oIdentitiesOnly=yes")
			args = append(args, opts...)
			args = append(args, "-b", "/dev/null", account.Username+"@127.0.0.1")
			return exec.Command(sftp, args...).Run() // #nosec G204 -- sftp is resolved via exec.LookPath; arguments are test-controlled
		case "scp":
			args := make([]string, 0, 7+len(opts)+2)
			args = append(args, "-F", permissiveConfig, "-P", portText, "-i", clientKey, "-oIdentitiesOnly=yes")
			args = append(args, opts...)
			args = append(args, localFile, account.Username+"@127.0.0.1:"+remoteFile)
			return exec.Command(scp, args...).Run() // #nosec G204 -- scp is resolved via exec.LookPath; arguments are test-controlled
		default:
			return fmt.Errorf("unknown probe %s", tool)
		}
	}

	stopServer := startServer(firstHostKey)
	if err := os.WriteFile(knownHosts, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	unknownResult, unknownErr := runStrictAnsibleProbe(ansiblePlaybook, port, account.Username, clientKey, knownHosts)
	if unknownErr != nil || unknownResult == nil || unknownResult.Success {
		t.Fatalf("Ansible accepted unknown host key: result=%+v error=%v", unknownResult, unknownErr)
	}
	for _, tool := range []string{"ssh", "sftp", "scp"} {
		if err := probe(tool); err == nil {
			t.Errorf("%s accepted an unknown host key", tool)
		}
	}
	writeKnownHost(firstHostKey)
	knownResult, knownErr := runStrictAnsibleProbe(ansiblePlaybook, port, account.Username, clientKey, knownHosts)
	if knownErr != nil || knownResult == nil || !knownResult.Success {
		t.Fatalf("Ansible rejected configured known host: result=%+v error=%v", knownResult, knownErr)
	}
	for _, tool := range []string{"ssh", "sftp", "scp"} {
		if err := probe(tool); err != nil {
			t.Errorf("%s rejected the configured known host key: %v", tool, err)
		}
	}
	master := exec.Command(ssh, "-F", permissiveConfig, "-p", portText, "-i", clientKey, "-oIdentitiesOnly=yes", "-MNf", account.Username+"@127.0.0.1") // #nosec G204 -- ssh is resolved via exec.LookPath; arguments are test-controlled
	if output, err := master.CombinedOutput(); err != nil {
		t.Fatalf("create pre-existing permissive SSH control connection: %v\n%s", err, output)
	}
	// The permissive config sets ControlMaster=yes; a "-O" client command must
	// override it to "no", otherwise OpenSSH skips the mux socket and attempts a
	// fresh connection (no identity) that the server rejects with publickey.
	controlOpt := "-oControlMaster=no"
	defer func() {
		_ = exec.Command(ssh, "-F", permissiveConfig, "-p", portText, controlOpt, "-O", "exit", account.Username+"@127.0.0.1").Run() // #nosec G204 -- ssh is resolved via exec.LookPath; arguments are test-controlled
	}()
	if output, err := exec.Command(ssh, "-F", permissiveConfig, "-p", portText, controlOpt, "-O", "check", account.Username+"@127.0.0.1").CombinedOutput(); err != nil { // #nosec G204 -- ssh is resolved via exec.LookPath; arguments are test-controlled
		t.Fatalf("verify permissive control connection exists: %v\n%s", err, output)
	}
	stopServer()

	stopServer = startServer(secondHostKey)
	defer stopServer()
	changedResult, changedErr := runStrictAnsibleProbe(ansiblePlaybook, port, account.Username, clientKey, knownHosts)
	if changedErr != nil || changedResult == nil || changedResult.Success {
		t.Fatalf("Ansible accepted changed host key or reused a permissive control connection: result=%+v error=%v", changedResult, changedErr)
	}
	for _, tool := range []string{"ssh", "sftp", "scp"} {
		if err := probe(tool); err == nil {
			t.Errorf("%s accepted a changed host key", tool)
		}
	}
}

func runStrictAnsibleProbe(executable string, port int, username, keyPath, knownHosts string) (*RunResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	host := HostSpec{Name: "fixture", Address: "127.0.0.1", Port: port, User: username, KeyFile: keyPath, KnownHostsFile: knownHosts}
	return (&Runner{Exe: executable, Timeout: 25 * time.Second}).Run(ctx, RunRequest{Operation: "verify-host", Hosts: []HostSpec{host}})
}

// safeAnsiblePlaybookCandidate returns the first ansible-playbook on PATH that
// passes the production executable-safety gate. It lets this integration test
// avoid a world-writable shim that a runner image may place earlier on PATH,
// while still refusing every candidate the production code would refuse.
func safeAnsiblePlaybookCandidate() (string, bool) {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, "ansible-playbook")
		info, err := os.Stat(candidate) // #nosec G703 -- candidate is a PATH-derived fixture lookup, not untrusted input
		if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
			continue
		}
		if checkExecutableSafety(candidate) == nil {
			return candidate, true
		}
	}
	return "", false
}

func unusedTCPPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}
