package qConstruct

import (
	"testing"

	"github.com/APRSCN/aprsutils/parser"
)

const (
	testServer = "TESTING"
	testLogin  = "N5CAL-1"
)

// run is a helper that parses a raw packet, runs QConstruct with the given
// config, and returns the resulting comma-joined path (after the toCall) plus
// the drop/loop status.
func run(t *testing.T, raw string, cfg *QConfig) (path string, drop bool, loop bool) {
	t.Helper()
	p, err := parser.Parse(raw, parser.WithDisableToCallsignValidate())
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	res, err := QConstruct(p, cfg)
	if err != nil {
		t.Fatalf("QConstruct %q: %v", raw, err)
	}
	return res.GetPathString(), res.ShouldDrop, res.IsLoop
}

// verifiedClientCfg models a standard verified inbound client connection
// (originated_by_client is decided inside QConstruct via From == ClientLogin).
func verifiedCfg(from string) *QConfig {
	return &QConfig{
		ServerLogin:    testServer,
		ClientLogin:    testLogin,
		ConnectionType: ConnectionVerified,
		IsVerified:     true,
	}
}

func TestQVerifiedExistingQUnchanged(t *testing.T) {
	// Packet already carrying a valid qAS,otherserver from a forwarded path
	// must keep its q construct (not originated by client).
	// GetPathString returns the via-path only (excluding the toCall), which is
	// what the server re-prepends via Replace.
	path, drop, loop := run(t,
		"SRC>DST,DIGI1,DIGI2*,qAS,IGATE:>status", verifiedCfg("SRC"))
	if drop || loop {
		t.Fatalf("unexpected drop/loop: %v/%v", drop, loop)
	}
	if path != "DIGI1,DIGI2*,qAS,IGATE" {
		t.Errorf("path = %q, want DIGI1,DIGI2*,qAS,IGATE", path)
	}
}

func TestQVerifiedForwardNoQAddsQAS(t *testing.T) {
	// Forwarded packet (not from login) without a q construct gets ,qAS,login.
	path, drop, _ := run(t,
		"SRC>DST,DIGI1,DIGI2*:>status", verifiedCfg("SRC"))
	if drop {
		t.Fatal("unexpected drop")
	}
	if path != "DIGI1,DIGI2*,qAS,"+testLogin {
		t.Errorf("path = %q, want DIGI1,DIGI2*,qAS,%s", path, testLogin)
	}
}

func TestQDropQAZ(t *testing.T) {
	_, drop, _ := run(t,
		"SRCCALL>DST,DIGI1*,qAZ,"+testLogin+":>status", verifiedCfg("SRCCALL"))
	if !drop {
		t.Error("qAZ packet should be dropped")
	}
}

func TestQDropServerLoginLoop(t *testing.T) {
	// Server's own call appearing after the q construct => loop, drop.
	_, drop, loop := run(t,
		"SRCCALL>DST,DIGI1*,qAR,"+testServer+":>status", verifiedCfg("SRCCALL"))
	if !drop || !loop {
		t.Errorf("server-login-in-path should drop+loop, got drop=%v loop=%v", drop, loop)
	}
}

func TestQDropDuplicateCallsign(t *testing.T) {
	// Duplicate callsign after q construct => loop, drop.
	_, drop, loop := run(t,
		"SRCCALL>DST,DIGI1*,qAI,FOOBAR,ASDF,ASDF,BARFOO:>status", verifiedCfg("SRCCALL"))
	if !drop || !loop {
		t.Errorf("duplicate callsign should drop+loop, got drop=%v loop=%v", drop, loop)
	}
}

func TestQDuplicateCaseSensitive(t *testing.T) {
	// aprsc treats the duplicate check as case-sensitive: ASDF vs asdf are NOT
	// duplicates and must not be dropped.
	_, drop, _ := run(t,
		"SRCCALL>DST,DIGI1*,qAI,FOOBAR,ASDF,asdf,BARFOO:>status", verifiedCfg("SRCCALL"))
	if drop {
		t.Error("case-differing callsigns must NOT be treated as duplicates")
	}
}

// TestQOriginatedByClient reproduces aprsc t/20qconstr-verified.t: a validated
// client sending its OWN packet on an inbound port has its digipeater path
// discarded and replaced with TCPIP*,qAC,SERVER.
func TestQOriginatedByClient(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"N5CAL-1>DST:>status", "TCPIP*,qAC,TESTING"},
		{"N5CAL-1>DST,TCPIP*:>status", "TCPIP*,qAC,TESTING"},
		{"N5CAL-1>DST,qAR,N5CAL-1:>status", "TCPIP*,qAC,TESTING"},
		{"N5CAL-1>DST,WIDE2-2,qAR,N5CAL-1:>status", "TCPIP*,qAC,TESTING"},
	}
	for _, c := range cases {
		path, drop, loop := run(t, c.raw, verifiedCfg("N5CAL-1"))
		if drop || loop {
			t.Errorf("%s: unexpected drop=%v loop=%v", c.raw, drop, loop)
			continue
		}
		if path != c.want {
			t.Errorf("%s => %q, want %q", c.raw, path, c.want)
		}
	}
}

// TestDisallowOtherQProtocols verifies that, when enabled, a packet whose
// q-construct uses a protocol id other than the configured one is dropped,
// while the accepted id passes through.
func TestDisallowOtherQProtocols(t *testing.T) {
	cfg := func() *QConfig {
		return &QConfig{
			ServerLogin:            testServer,
			ClientLogin:            testLogin,
			ConnectionType:         ConnectionOutboundServer,
			IsVerified:             true,
			QProtocolID:            "A",
			DisallowOtherProtocols: true,
		}
	}

	// qZX uses a non-"A" protocol id -> dropped.
	_, drop, _ := run(t, "N5CAL-1>DST,qZX,SRV:>status", cfg())
	if !drop {
		t.Error("packet with qZ construct should be dropped when other protocols are disallowed")
	}

	// qAR uses the accepted "A" protocol id -> not dropped on this basis.
	_, drop, loop := run(t, "N5CAL-1>DST,qAR,SRV:>status", cfg())
	if drop || loop {
		t.Errorf("packet with qAR should not be dropped (drop=%v loop=%v)", drop, loop)
	}

	// With the policy off, the qZ packet is not dropped for protocol reasons.
	offCfg := cfg()
	offCfg.DisallowOtherProtocols = false
	_, drop, _ = run(t, "N5CAL-1>DST,qZX,SRV:>status", offCfg)
	if drop {
		t.Error("packet with qZ should not be dropped when policy is off")
	}
}
