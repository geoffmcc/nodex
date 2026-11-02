package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/config"
	"golang.org/x/crypto/ssh"
)

func TestSSHKeyInitAndExistingSetup(t *testing.T) {
	isolateConfigAndHome(t)
	var out, errout bytes.Buffer
	run := func(args ...string) error {
		out.Reset()
		errout.Reset()
		return Run(context.Background(), args, &out, &errout)
	}
	if err := run("--non-interactive", "init"); err != nil {
		t.Fatal(err)
	}
	path, _ := dedicatedSSHKeyPath()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("implicit noninteractive generation")
	}
	configPath, _ := config.ConfigPath()
	before, _ := os.ReadFile(configPath)
	if err := run("--non-interactive", "setup", "--generate-ssh-key"); err == nil {
		t.Fatal("unencrypted generation was not explicit")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("failed setup created key")
	}
	if err := run("--non-interactive", "setup", "--generate-ssh-key", "--ssh-key-no-passphrase"); err != nil {
		t.Fatal(err)
	}
	private, _ := os.ReadFile(path)
	key, err := ssh.ParsePrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errout.String(), ssh.FingerprintSHA256(key.PublicKey())) {
		t.Fatal("missing fingerprint")
	}
	if bytes.Contains(out.Bytes(), private) || bytes.Contains(errout.Bytes(), private) {
		t.Fatal("private key leaked")
	}
	if err := run("--non-interactive", "setup", "--generate-ssh-key"); err != nil {
		t.Fatal(err)
	}
	afterKey, _ := os.ReadFile(path)
	afterConfig, _ := os.ReadFile(configPath)
	if !bytes.Equal(private, afterKey) || !bytes.Equal(before, afterConfig) {
		t.Fatal("existing key or config replaced")
	}
	if runtime.GOOS != "windows" {
		for name, mode := range map[string]os.FileMode{path: 0600, filepath.Dir(path): 0700} {
			info, _ := os.Stat(name)
			if info.Mode().Perm() != mode {
				t.Fatalf("unsafe mode %s: %v", name, info.Mode())
			}
		}
	}
}

func TestSSHKeyExplicitInit(t *testing.T) {
	isolateConfigAndHome(t)
	var out, errout bytes.Buffer
	if err := Run(context.Background(), []string{"--non-interactive", "init", "--generate-ssh-key", "--ssh-key-no-passphrase"}, &out, &errout); err != nil {
		t.Fatal(err)
	}
	path, _ := dedicatedSSHKeyPath()
	if _, exists, err := inspectDedicatedSSHKey(path); err != nil || !exists {
		t.Fatalf("key missing: %v", err)
	}
}

func TestSSHKeyEncryptedPairAndMismatch(t *testing.T) {
	isolateConfigAndHome(t)
	path, _ := dedicatedSSHKeyPath()
	pass := []byte("test-only passphrase")
	if _, err := createDedicatedSSHKey(path, pass); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if _, err := ssh.ParsePrivateKey(data); err == nil {
		t.Fatal("private key is not protected")
	}
	if _, err := ssh.ParsePrivateKeyWithPassphrase(data, pass); err != nil {
		t.Fatal(err)
	}
	if _, exists, err := inspectDedicatedSSHKey(path); err != nil || !exists {
		t.Fatalf("encrypted inspection: %v", err)
	}
	if _, err := createDedicatedSSHKey(path, nil); err == nil {
		t.Fatal("existing identity overwritten")
	}
	if err := os.WriteFile(path+".pub", []byte("invalid public key"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := inspectDedicatedSSHKey(path); err == nil {
		t.Fatal("mismatched identity accepted")
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(data, after) {
		t.Fatal("private key changed")
	}
}

func TestSSHKeyRefusesUnsafeAndIncompleteIdentities(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix modes and symlinks")
	}
	for _, kind := range []string{"missing-public", "private-symlink", "public-symlink", "directory-symlink", "loose-private", "loose-directory"} {
		t.Run(kind, func(t *testing.T) {
			isolateConfigAndHome(t)
			path, _ := dedicatedSSHKeyPath()
			if _, err := createDedicatedSSHKey(path, nil); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "missing-public":
				if err := os.Remove(path + ".pub"); err != nil {
					t.Fatal(err)
				}
			case "private-symlink", "public-symlink":
				target := path
				if kind == "public-symlink" {
					target += ".pub"
				}
				if err := os.Rename(target, target+".saved"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target+".saved", target); err != nil {
					t.Fatal(err)
				}
			case "directory-symlink":
				dir := filepath.Dir(path)
				if err := os.Rename(dir, dir+".saved"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(dir+".saved", dir); err != nil {
					t.Fatal(err)
				}
			case "loose-private":
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			case "loose-directory":
				if err := os.Chmod(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
			}
			if _, _, err := inspectDedicatedSSHKey(path); err == nil {
				t.Fatal("unsafe identity accepted")
			}
		})
	}
}

func TestSSHKeyInteractiveInitUsesSharedReader(t *testing.T) {
	for _, create := range []bool{false, true} {
		t.Run(fmt.Sprint(create), func(t *testing.T) {
			isolateConfigAndHome(t)
			original := osIn
			t.Cleanup(func() { osIn = original })
			answer := "n\n"
			if create {
				answer = "y\nn\n"
			}
			osIn = func() io.Reader { return strings.NewReader("proxmox\nhttps://pve.example.test:8006\n\nlab\n" + answer) }
			var out, errout bytes.Buffer
			if err := Run(context.Background(), []string{"init"}, &out, &errout); err != nil {
				t.Fatal(err)
			}
			path, _ := dedicatedSSHKeyPath()
			_, err := os.Stat(path)
			if create && err != nil {
				t.Fatal(err)
			}
			if !create && !os.IsNotExist(err) {
				t.Fatal("declined key created")
			}
		})
	}
}

func TestSSHKeyFlagValidationAndStructuredOutput(t *testing.T) {
	isolateConfigAndHome(t)
	var out, errout bytes.Buffer
	for _, args := range [][]string{
		{"--non-interactive", "init", "--ssh-key-no-passphrase"},
		{"--non-interactive", "setup", "--ssh-key-no-passphrase"},
		{"--non-interactive", "init", "--generate-ssh-key=false"},
	} {
		if err := Run(context.Background(), args, &out, &errout); err == nil {
			t.Fatalf("accepted invalid flags: %v", args)
		}
	}
	if err := Run(context.Background(), []string{"--non-interactive", "--output", "json", "setup", "--generate-ssh-key", "--ssh-key-no-passphrase"}, &out, &errout); err != nil {
		t.Fatal(err)
	}
	var result map[string]string
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["fingerprint"] == "" || result["ssh_key_file"] == "" {
		t.Fatal("missing key metadata")
	}
}

func TestSSHKeyPassphraseRequiresTerminal(t *testing.T) {
	var errout bytes.Buffer
	if _, err := readSSHKeyPassphrase(&Context{Stdin: strings.NewReader("do-not-read"), ErrW: &errout}); err == nil {
		t.Fatal("passphrase accepted without terminal")
	}
}
