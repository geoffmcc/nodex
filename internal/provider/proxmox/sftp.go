package proxmox

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// sftpClient is the minimal SFTP surface the provider needs. The injectable
// dialer keeps the download path testable without a real SSH server.
type sftpClient interface {
	Open(path string) (io.ReadCloser, error)
	Close() error
}

// sftpDialer opens an SFTP connection. Tests replace it with a fake.
type sftpDialer func(ctx context.Context, host, user, keyFile string, port int) (sftpClient, error)

// realSFTPDial authenticates to host:port as user with the private key in
// keyFile and returns an SFTP client.
func realSFTPDial(ctx context.Context, host, user, keyFile string, port int) (sftpClient, error) {
	if port < 1 || port > 65535 {
		port = 22
	}
	pem, err := os.ReadFile(keyFile) // #nosec G304 -- keyFile comes from the validated credential configuration for the selected profile.
	if err != nil {
		return nil, fmt.Errorf("read ssh key %s: %w", keyFile, err)
	}
	signer, err := ssh.ParsePrivateKey(pem)
	if err != nil {
		return nil, fmt.Errorf("parse ssh key %s: %w", keyFile, err)
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}

	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // #nosec G106 -- the SFTP host is the operator-selected profile endpoint; host-key pinning is intentionally deferred and tracked for a future hardening pass.
		Timeout:         15 * time.Second,
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ssh handshake with %s: %w", addr, err)
	}
	sshClient := ssh.NewClient(sshConn, chans, reqs)

	client, err := sftp.NewClient(sshClient)
	if err != nil {
		_ = sshClient.Close()
		return nil, fmt.Errorf("open sftp session with %s: %w", addr, err)
	}
	return &sftpAdapter{Client: client}, nil
}

// sftpAdapter adapts *sftp.Client to the narrow sftpClient interface so fakes
// are trivial in tests.
type sftpAdapter struct {
	*sftp.Client
}

// Open returns the remote file as a readable stream.
func (a *sftpAdapter) Open(path string) (io.ReadCloser, error) {
	return a.Client.Open(path)
}

// downloadViaSFTP streams remotePath from the SFTP server into w. It runs the
// copy in a goroutine so a cancelled or timed-out context aborts the transfer.
func downloadViaSFTP(ctx context.Context, host, user, keyFile string, port int, remotePath string, w io.Writer, dial sftpDialer) error {
	client, err := dial(ctx, host, user, keyFile, port)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	f, err := client.Open(remotePath)
	if err != nil {
		return fmt.Errorf("open remote file %s: %w", remotePath, err)
	}
	defer func() { _ = f.Close() }()

	copied := make(chan error, 1)
	go func() {
		_, err := io.Copy(w, f)
		copied <- err
	}()

	select {
	case err := <-copied:
		if err != nil {
			return fmt.Errorf("download %s: %w", remotePath, err)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("download %s: %w", remotePath, ctx.Err())
	}
}
