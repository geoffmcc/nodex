package client

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/transport/httpclient"
	"github.com/gorilla/websocket"
)

func TestVMConsoleUsesTermProxyAndRelaysWebsocket(t *testing.T) {
	const ticket = "ticket-that-must-not-leak"
	var gotParams url.Values
	upgrader := websocket.Upgrader{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/node1/qemu/100/termproxy":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			gotParams = r.Form
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"data":{"port":"5900","ticket":%q}}`, ticket)
		case "/api2/json/nodes/node1/qemu/100/vncwebsocket":
			if r.URL.Query().Get("vncticket") != ticket {
				t.Errorf("vncticket = %q, want ticket", r.URL.Query().Get("vncticket"))
			}
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Errorf("Upgrade: %v", err)
				return
			}
			defer conn.Close()
			_ = conn.WriteMessage(websocket.BinaryMessage, []byte("welcome"))
			_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	caPath := writeServerCA(t, server.Certificate())
	caOpt, err := httpclient.WithCACert(caPath)
	if err != nil {
		t.Fatalf("WithCACert: %v", err)
	}
	httpsURL := server.URL
	c := &Client{
		endpoint: httpsURL,
		baseURL:  server.URL + DefaultAPIPath,
		client:   httpclient.New(caOpt),
	}

	var output strings.Builder
	input := &oneByteThenBlock{data: []byte("input"), release: make(chan struct{})}
	if err := c.VMConsole(context.Background(), "node1", 100, input, &output); err != nil {
		t.Fatalf("VMConsole: %v", err)
	}
	close(input.release)
	if output.String() != "welcome" {
		t.Fatalf("console output = %q, want welcome", output.String())
	}
	if gotParams.Get("serial") != "serial0" || gotParams.Get("websocket") != "" {
		t.Fatalf("termproxy params = %v, want serial=serial0 without websocket", gotParams)
	}
}

func TestConsoleRejectsInvalidArguments(t *testing.T) {
	c := &Client{}
	for _, test := range []struct {
		name string
		node string
		vmid int
		in   io.Reader
		out  io.Writer
	}{
		{name: "node", vmid: 1, in: strings.NewReader("x"), out: io.Discard},
		{name: "vmid", node: "node", in: strings.NewReader("x"), out: io.Discard},
		{name: "streams", node: "node", vmid: 1, out: io.Discard},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := c.VMConsole(context.Background(), test.node, test.vmid, test.in, test.out); err == nil {
				t.Fatal("VMConsole succeeded with invalid arguments")
			}
		})
	}
}

func writeServerCA(t *testing.T, cert *x509.Certificate) string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "ca-*.pem")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if err := pem.Encode(file, &pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return file.Name()
}

type oneByteThenBlock struct {
	data    []byte
	release chan struct{}
}

func (r *oneByteThenBlock) Read(p []byte) (int, error) {
	if len(r.data) > 0 {
		n := copy(p, r.data)
		r.data = r.data[n:]
		return n, nil
	}
	<-r.release
	return 0, io.EOF
}
