package monitor

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

type fakeICMPConn struct {
	local       net.Addr
	writeAddr   net.Addr
	writePacket []byte
	read        func([]byte) (int, net.Addr, error)
	deadline    time.Time
}

func (c *fakeICMPConn) Close() error                  { return nil }
func (c *fakeICMPConn) LocalAddr() net.Addr           { return c.local }
func (c *fakeICMPConn) SetDeadline(t time.Time) error { c.deadline = t; return nil }
func (c *fakeICMPConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	c.writeAddr, c.writePacket = addr, append([]byte(nil), b...)
	return len(b), nil
}
func (c *fakeICMPConn) ReadFrom(b []byte) (int, net.Addr, error) { return c.read(b) }

func TestICMPEchoUsesUDPAddressesAndValidatesReply(t *testing.T) {
	previous := listenICMP
	t.Cleanup(func() { listenICMP = previous })
	conn := &fakeICMPConn{local: &net.UDPAddr{IP: net.IPv4zero, Port: 43123}}
	conn.read = func(dst []byte) (int, net.Addr, error) {
		request, err := icmp.ParseMessage(ipv4.ICMPTypeEcho.Protocol(), conn.writePacket)
		if err != nil {
			t.Fatalf("parse request: %v", err)
		}
		echo := request.Body.(*icmp.Echo)
		if echo.ID != 43123 {
			t.Errorf("request id = %d, want kernel socket id 43123", echo.ID)
		}
		if _, ok := conn.writeAddr.(*net.UDPAddr); !ok {
			t.Errorf("destination type = %T, want *net.UDPAddr", conn.writeAddr)
		}
		wrongMessage := icmp.Message{Type: ipv4.ICMPTypeEchoReply, Body: &icmp.Echo{ID: echo.ID, Seq: (echo.Seq + 1) & 0xffff, Data: echo.Data}}
		wrong, _ := wrongMessage.Marshal(nil)
		copy(dst, wrong)
		return len(wrong), &net.UDPAddr{IP: net.ParseIP("192.0.2.1")}, nil
	}
	listenICMP = func(network, address string) (icmpPacketConn, error) {
		if network != "udp4" || address != "0.0.0.0" {
			t.Errorf("listen args = %q %q", network, address)
		}
		return conn, nil
	}
	// First deliver the mismatched response from the wrong peer, then a valid
	// response. Neither mismatch may satisfy the request.
	reads := 0
	conn.read = func(dst []byte) (int, net.Addr, error) {
		request, _ := icmp.ParseMessage(ipv4.ICMPTypeEcho.Protocol(), conn.writePacket)
		echo := request.Body.(*icmp.Echo)
		reads++
		replyEcho := &icmp.Echo{ID: echo.ID, Seq: echo.Seq, Data: echo.Data}
		peer := &net.UDPAddr{IP: net.ParseIP("127.0.0.1")}
		switch reads {
		case 1:
			replyEcho.ID++
		case 2:
			replyEcho.Seq++
		case 3:
			replyEcho.Data = []byte("unrelated")
		case 4:
			peer = &net.UDPAddr{IP: net.ParseIP("192.0.2.1")}
		}
		message := icmp.Message{Type: ipv4.ICMPTypeEchoReply, Body: replyEcho}
		if reads == 5 {
			message.Code = 1
		}
		packet, _ := message.Marshal(nil)
		copy(dst, packet)
		return len(packet), peer, nil
	}
	if err := icmpEcho(context.Background(), net.IPAddr{IP: net.ParseIP("127.0.0.1")}); err != nil {
		t.Fatalf("icmpEcho: %v", err)
	}
	if reads != 6 {
		t.Fatalf("accepted a mismatched reply; reads=%d", reads)
	}
}

func TestICMPEchoRejectsUnrelatedPayloadUntilDeadline(t *testing.T) {
	previous := listenICMP
	t.Cleanup(func() { listenICMP = previous })
	conn := &fakeICMPConn{local: &net.UDPAddr{IP: net.IPv4zero, Port: 43124}}
	listenICMP = func(string, string) (icmpPacketConn, error) { return conn, nil }
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	reads := 0
	conn.read = func(dst []byte) (int, net.Addr, error) {
		reads++
		if reads == 1 {
			request, _ := icmp.ParseMessage(ipv4.ICMPTypeEcho.Protocol(), conn.writePacket)
			echo := request.Body.(*icmp.Echo)
			message := icmp.Message{Type: ipv4.ICMPTypeEchoReply, Body: &icmp.Echo{ID: echo.ID, Seq: echo.Seq, Data: []byte("unrelated")}}
			packet, _ := message.Marshal(nil)
			copy(dst, packet)
			return len(packet), &net.UDPAddr{IP: net.ParseIP("127.0.0.1")}, nil
		}
		<-ctx.Done()
		return 0, nil, os.ErrDeadlineExceeded
	}
	if err := icmpEcho(ctx, net.IPAddr{IP: net.ParseIP("127.0.0.1")}); err == nil {
		t.Fatal("unrelated reply must not be accepted")
	}
	if reads != 2 {
		t.Fatalf("unrelated payload unexpectedly satisfied probe; reads=%d", reads)
	}
}

func TestICMPEchoCancellationInterruptsRead(t *testing.T) {
	previous := listenICMP
	t.Cleanup(func() { listenICMP = previous })
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	conn := &fakeICMPConn{local: &net.UDPAddr{IP: net.IPv4zero, Port: 43126}}
	conn.read = func([]byte) (int, net.Addr, error) {
		close(started)
		<-ctx.Done()
		return 0, nil, os.ErrDeadlineExceeded
	}
	listenICMP = func(string, string) (icmpPacketConn, error) { return conn, nil }
	done := make(chan error, 1)
	go func() { done <- icmpEcho(ctx, net.IPAddr{IP: net.ParseIP("127.0.0.1")}) }()
	<-started
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled probe unexpectedly succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled probe did not interrupt packet read")
	}
}

func TestICMPEchoPermissionFailureIsUnsupported(t *testing.T) {
	previous := listenICMP
	t.Cleanup(func() { listenICMP = previous })
	listenICMP = func(string, string) (icmpPacketConn, error) { return nil, os.ErrPermission }
	if err := icmpEcho(context.Background(), net.IPAddr{IP: net.ParseIP("127.0.0.1")}); !errors.Is(err, errICMPUnavailable) {
		t.Fatalf("permission error = %v, want unsupported ICMP", err)
	}
}

func TestICMPLoopbackRealIPv4AndIPv6(t *testing.T) {
	tests := []struct {
		name string
		addr string
	}{
		{name: "ipv4", addr: "127.0.0.1"},
		{name: "ipv6", addr: "::1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			err := probeICMP(ctx, tt.addr)
			if errors.Is(err, errICMPUnavailable) {
				t.Skipf("kernel does not provide unprivileged ICMP sockets for %s: %v", tt.name, err)
			}
			if err != nil {
				t.Fatalf("real loopback echo to %s failed: %v", tt.addr, err)
			}
		})
	}
}

func TestICMPEchoValidatesIPv6AddressFamily(t *testing.T) {
	previous := listenICMP
	t.Cleanup(func() { listenICMP = previous })
	conn := &fakeICMPConn{local: &net.UDPAddr{IP: net.IPv6unspecified, Port: 43125}}
	conn.read = func(dst []byte) (int, net.Addr, error) {
		request, err := icmp.ParseMessage(ipv6.ICMPTypeEchoRequest.Protocol(), conn.writePacket)
		if err != nil {
			return 0, nil, err
		}
		echo := request.Body.(*icmp.Echo)
		message := icmp.Message{Type: ipv6.ICMPTypeEchoReply, Body: &icmp.Echo{ID: echo.ID, Seq: echo.Seq, Data: echo.Data}}
		packet, _ := message.Marshal(nil)
		copy(dst, packet)
		return len(packet), &net.UDPAddr{IP: net.ParseIP("::1")}, nil
	}
	listenICMP = func(network, address string) (icmpPacketConn, error) {
		if network != "udp6" || address != "::" {
			t.Errorf("listen args = %q %q, want udp6 ::", network, address)
		}
		return conn, nil
	}
	if err := icmpEcho(context.Background(), net.IPAddr{IP: net.ParseIP("::1")}); err != nil {
		t.Fatalf("IPv6 icmpEcho: %v", err)
	}
	if _, ok := conn.writeAddr.(*net.UDPAddr); !ok {
		t.Fatalf("IPv6 destination type = %T, want *net.UDPAddr", conn.writeAddr)
	}
}
