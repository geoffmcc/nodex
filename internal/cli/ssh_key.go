package cli

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/credentials"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

type sshKeyOptions struct{ generate, noPassphrase bool }

func (o sshKeyOptions) validate() error {
	if o.noPassphrase && !o.generate {
		return fmt.Errorf("--ssh-key-no-passphrase requires --generate-ssh-key")
	}
	return nil
}

func dedicatedSSHKeyPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ssh", "id_ed25519"), nil
}

// setupSSHKey only touches Nodex's own identity, never ambient SSH keys or trust.
func setupSSHKey(cmdCtx *Context, reader *bufio.Reader, opts sshKeyOptions) error {
	if err := opts.validate(); err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}
	if cmdCtx.Opts.NonInteractive && !opts.generate {
		return nil
	}
	path, err := dedicatedSSHKeyPath()
	if err != nil {
		return app.NewExitError(err, app.ExitConfig)
	}
	fingerprint, exists, err := inspectDedicatedSSHKey(path)
	if err != nil {
		return app.NewExitError(err, app.ExitCredential)
	}
	if exists {
		fmt.Fprintf(cmdCtx.ErrW, "Existing Nodex SSH key preserved: %s (%s)\n", path, fingerprint)
		return nil
	}
	if !opts.generate {
		answer, err := setupPrompt(reader, cmdCtx.ErrW, "Create a dedicated Nodex SSH key for SSH operations? [y/N] ")
		if err != nil {
			return err
		}
		if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
			return nil
		}
	}
	if cmdCtx.Opts.NonInteractive && !opts.noPassphrase {
		return app.NewExitError(fmt.Errorf("unattended key creation requires --ssh-key-no-passphrase; use interactive setup for a protected key"), app.ExitUsage)
	}
	var passphrase []byte
	if !opts.noPassphrase {
		answer, err := setupPrompt(reader, cmdCtx.ErrW, "Protect the SSH key with a passphrase? [Y/n] ")
		if err != nil {
			return err
		}
		if !strings.EqualFold(answer, "n") && !strings.EqualFold(answer, "no") {
			passphrase, err = readSSHKeyPassphrase(cmdCtx)
			if err != nil {
				return app.NewExitError(err, app.ExitCredential)
			}
			defer clear(passphrase)
		}
	}
	fingerprint, err = createDedicatedSSHKey(path, passphrase)
	if err != nil {
		return app.NewExitError(err, app.ExitCredential)
	}
	fmt.Fprintf(cmdCtx.ErrW, "Created Nodex SSH key: %s (%s)\nPublic key: %s.pub\nAuthorize the public key on each managed host, verify its SSH host fingerprint, and configure ssh_user, ssh_key_file and known_hosts for the relevant profile or inventory host. Protected keys require ssh-agent for unattended use.\n", path, fingerprint, path)
	return nil
}

func readSSHKeyPassphrase(cmdCtx *Context) ([]byte, error) {
	input, ok := cmdCtx.Stdin.(*os.File)
	if !ok || !term.IsTerminal(int(input.Fd())) {
		return nil, fmt.Errorf("SSH key passphrase requires a terminal; use --ssh-key-no-passphrase only for an intentionally unencrypted key")
	}
	fmt.Fprint(cmdCtx.ErrW, "SSH key passphrase: ")
	first, err := term.ReadPassword(int(input.Fd()))
	fmt.Fprintln(cmdCtx.ErrW)
	if err != nil {
		clear(first)
		return nil, err
	}
	if len(first) == 0 {
		return nil, fmt.Errorf("passphrase must not be empty")
	}
	fmt.Fprint(cmdCtx.ErrW, "Confirm SSH key passphrase: ")
	second, err := term.ReadPassword(int(input.Fd()))
	fmt.Fprintln(cmdCtx.ErrW)
	defer clear(second)
	if err != nil {
		clear(first)
		return nil, err
	}
	if !bytes.Equal(first, second) {
		clear(first)
		return nil, fmt.Errorf("passphrases do not match")
	}
	return first, nil
}

func inspectDedicatedSSHKey(path string) (string, bool, error) {
	dir, err := os.Lstat(filepath.Dir(path))
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !dir.IsDir() || dir.Mode()&os.ModeSymlink != 0 {
		return "", false, fmt.Errorf("Nodex SSH directory must be a real directory")
	}
	if err := credentials.CheckSecretDirectory(filepath.Dir(path)); err != nil {
		return "", false, fmt.Errorf("unsafe Nodex SSH directory: %w", err)
	}
	// An existing directory with an incomplete identity is a recovery condition.
	for _, name := range []string{path, path + ".pub"} {
		info, err := os.Lstat(name)
		if err != nil {
			return "", false, fmt.Errorf("incomplete Nodex SSH identity: %w", err)
		}
		if !info.Mode().IsRegular() {
			return "", false, fmt.Errorf("Nodex SSH key files must be regular files")
		}
	}
	if err := credentials.CheckSecretFilePermissions(path); err != nil {
		return "", false, err
	}
	f, err := credentials.OpenSecretFile(path)
	if err != nil {
		return "", false, err
	}
	private, err := io.ReadAll(io.LimitReader(f, 1<<20))
	closeErr := f.Close()
	if err = errors.Join(err, closeErr); err != nil {
		clear(private)
		return "", false, err
	}
	if len(private) == 1<<20 {
		clear(private)
		return "", false, fmt.Errorf("Nodex SSH key exceeds size limit")
	}
	defer clear(private)
	parsed, err := ssh.ParseRawPrivateKey(private)
	var public ssh.PublicKey
	var protected *ssh.PassphraseMissingError
	if errors.As(err, &protected) {
		public = protected.PublicKey
	} else if err != nil {
		return "", false, fmt.Errorf("invalid Nodex SSH private key")
	} else {
		key, ok := parsed.(*ed25519.PrivateKey)
		if !ok {
			return "", false, fmt.Errorf("Nodex SSH identity must be Ed25519")
		}
		public, err = ssh.NewPublicKey(key.Public())
		if err != nil {
			return "", false, err
		}
	}
	if public == nil || public.Type() != ssh.KeyAlgoED25519 {
		return "", false, fmt.Errorf("Nodex SSH identity must be Ed25519")
	}
	data, err := os.ReadFile(path + ".pub")
	if err != nil {
		return "", false, err
	}
	published, _, _, rest, err := ssh.ParseAuthorizedKey(data)
	if err != nil || len(bytes.TrimSpace(rest)) != 0 || !bytes.Equal(published.Marshal(), public.Marshal()) {
		return "", false, fmt.Errorf("Nodex SSH public key does not match private key")
	}
	return ssh.FingerprintSHA256(public), true, nil
}

func createDedicatedSSHKey(path string, passphrase []byte) (string, error) {
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	defer clear(private)
	var block *pem.Block
	if len(passphrase) > 0 {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(private, "nodex", passphrase)
	} else {
		block, err = ssh.MarshalPrivateKey(private, "nodex")
	}
	if err != nil {
		return "", err
	}
	defer clear(block.Bytes)
	public, err := ssh.NewPublicKey(pub)
	if err != nil {
		return "", err
	}
	if _, err := config.EnsureDir(); err != nil {
		return "", err
	}
	// Claim a new private directory exclusively; never overwrite an existing identity.
	dir := filepath.Dir(path)
	if err := os.Mkdir(dir, 0700); err != nil {
		return "", fmt.Errorf("create Nodex SSH directory (existing identities are never replaced): %w", err)
	}
	if err := credentials.CheckSecretDirectory(dir); err != nil {
		_ = os.Remove(dir)
		return "", err
	}
	success := false
	defer func() {
		if !success {
			_ = os.Remove(path + ".pub")
			_ = os.Remove(path)
			_ = os.Remove(dir)
		}
	}()
	for _, file := range []struct {
		name string
		data []byte
		mode os.FileMode
	}{
		{path, pem.EncodeToMemory(block), 0600}, {path + ".pub", ssh.MarshalAuthorizedKey(public), 0644},
	} {
		f, err := os.OpenFile(file.name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, file.mode)
		if err != nil {
			return "", err
		}
		if file.name == path {
			if err := credentials.ProtectSecretFile(f); err != nil {
				_ = f.Close()
				clear(file.data)
				return "", err
			}
		}
		_, writeErr := f.Write(file.data)
		syncErr := f.Sync()
		closeErr := f.Close()
		clear(file.data)
		if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
			return "", err
		}
	}
	success = true
	return ssh.FingerprintSHA256(public), nil
}
