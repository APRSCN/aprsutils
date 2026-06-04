package client

import (
	"net"
	"strings"
	"testing"
	"time"
)

// TestUDPSubmitDatagram verifies that a UDP-mode client prefixes the login
// line to each datagram (UDP submit format), as expected by an APRS-IS server.
func TestUDPSubmitDatagram(t *testing.T) {
	// Local UDP server to capture the datagram.
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	defer func(pc net.PacketConn) { _ = pc.Close() }(pc)

	addr := pc.LocalAddr().(*net.UDPAddr)

	received := make(chan string, 1)
	go func() {
		buf := make([]byte, 2048)
		_ = pc.SetReadDeadline(time.Now().Add(3 * time.Second))
		n, _, err := pc.ReadFrom(buf)
		if err != nil {
			received <- ""
			return
		}
		received <- string(buf[:n])
	}()

	c := NewClient(
		"TEST", "29939",
		Fullfeed, UDP,
		"127.0.0.1", addr.Port,
		WithSoftwareAndVersion("udpaprstester", "1.0"),
	)
	if err := c.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	pkt := "TEST>UDAPRS,TCPIP*:>udp packet content"
	if err := c.SendPacket(pkt); err != nil {
		t.Fatalf("send: %v", err)
	}

	select {
	case got := <-received:
		if got == "" {
			t.Fatal("no datagram received")
		}
		// Must contain the login line then the packet.
		wantLogin := "user TEST pass 29939 vers udpaprstester 1.0"
		if !strings.HasPrefix(got, wantLogin) {
			t.Errorf("datagram missing login prefix; got %q", got)
		}
		if !strings.Contains(got, pkt) {
			t.Errorf("datagram missing packet; got %q", got)
		}
		// Lines must be CRLF-terminated.
		if !strings.Contains(got, "\r\n") {
			t.Errorf("datagram not CRLF terminated; got %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for datagram")
	}
}

// TestProtocolAccessor verifies the UDP protocol is recorded.
func TestProtocolAccessor(t *testing.T) {
	c := NewClient("N0CALL", "", Fullfeed, UDP, "example.com", 10152)
	if c.Protocol() != UDP {
		t.Errorf("Protocol() = %q, want udp", c.Protocol())
	}
}
