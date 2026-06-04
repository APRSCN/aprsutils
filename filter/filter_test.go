package filter

import (
	"testing"

	"github.com/APRSCN/aprsutils/parser"
)

// mustParse parses a packet for tests, failing on error.
func mustParse(t *testing.T, raw string) *parser.Parsed {
	t.Helper()
	p, err := parser.Parse(raw, parser.WithDisableToCallsignValidate())
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", raw, err)
	}
	return &p
}

// fixedCtx is a test Context with optional client and station positions.
type fixedCtx struct {
	client   *Position
	stations map[string]Position
}

func (c fixedCtx) ClientPosition() (Position, bool) {
	if c.client == nil {
		return Position{}, false
	}
	return *c.client, true
}

func (c fixedCtx) StationPosition(call string) (Position, bool) {
	p, ok := c.stations[call]
	return p, ok
}

func TestRangeFilter(t *testing.T) {
	f := Compile("r/60.4752/25.0947/1")
	// In range
	pass := mustParse(t, "OH2RDP-1>BEACON,OH2RDG*,WIDE,qAS,N5CAL-1:!6028.51N/02505.68E#")
	if !f.Match(pass, nil) {
		t.Error("expected packet within 1km to pass r/ filter")
	}
	// Out of range (62.58N)
	drop := mustParse(t, "OH2RDP-1>BEACON,OH2RDG*,WIDE,qAS,N5CAL-1:!6258.51N/02505.68E#")
	if f.Match(drop, nil) {
		t.Error("expected packet far away to be dropped by r/ filter")
	}
}

func TestPrefixFilter(t *testing.T) {
	f := Compile("p/OH/G")
	for _, c := range []struct {
		raw  string
		want bool
	}{
		{"OH0TES>APRS,qAR,N5CAL-1:!6028.51N/02505.68E#", true},
		{"G0TES>APRS,qAR,N5CAL-1:!6028.51N/02505.68E#", true},
		{"N0TES>APRS,qAR,N5CAL-1:!6028.51N/02505.68E#", false},
	} {
		got := f.Match(mustParse(t, c.raw), nil)
		if got != c.want {
			t.Errorf("p/OH/G match %q = %v, want %v", c.raw, got, c.want)
		}
	}
}

func TestBuddyFilterExactVsWildcard(t *testing.T) {
	// b/ is exact (not prefix) for non-wildcard entries; trailing '*' is wild.
	f := Compile("b/OH0TES/OH2TES b/OH7*")
	for _, c := range []struct {
		raw  string
		want bool
	}{
		{"OH0TES>APRS,qAR,N5CAL-1:!6028.51N/02505.68E#", true},    // exact
		{"OH0TES-9>APRS,qAR,N5CAL-1:!6028.51N/02505.68E#", false}, // not prefix-like
		{"G0TES>APRS,qAR,N5CAL-1:!6028.51N/02505.68E#", false},
		{"OH7TES>APRS,qAR,N5CAL-1:!6028.51N/02505.68E#", true}, // wildcard
	} {
		got := f.Match(mustParse(t, c.raw), nil)
		if got != c.want {
			t.Errorf("buddy match %q = %v, want %v", c.raw, got, c.want)
		}
	}
}

func TestObjectFilter(t *testing.T) {
	f := Compile("o/OBJ1/OBJ2/ISS/PRE*")
	for _, c := range []struct {
		raw  string
		want bool
	}{
		{"SRC>APRS,qAR,N5CAL-1:;OBJ2     *090902z6010.78N/02451.11E-Object 2", true},
		{"SRC>APRS,qAR,N5CAL-1:;PREFIX   *090902z6010.78N/02451.11E-prefix", true},
		{"G0TES>APRS,qAR,N5CAL-1:!6028.51N/02505.68E#", false},
		// Items should match object filter too.
		{"SRC>APRS,qAR,N5CAL-1:)OBJ1!4903.50N/07201.75WA", true},
	} {
		got := f.Match(mustParse(t, c.raw), nil)
		if got != c.want {
			t.Errorf("object match %q = %v, want %v", c.raw, got, c.want)
		}
	}
}

// TestTypeFilterNegation reproduces aprsc t/34filter-type-symbol.t: t/poimq -t/nw.
func TestTypeFilterNegation(t *testing.T) {
	f := Compile("t/poimq -t/nw")
	login := "N5CAL-1"
	cases := []struct {
		raw  string
		want bool
		desc string
	}{
		{"ST1>APRS,qAR," + login + ":>status type drop", false, "status drop"},
		{"ST2>APRS,qAR," + login + ":!6028.51N/02505.68E# type pos", true, "position pass"},
		{"ST3>APRS,qAR," + login + ":T#931,113,000,000,000,000,00000000", false, "telemetry drop"},
		{"ST4>APRS,qAR," + login + ":;OBJ1     *090902z6010.78N/02451.11E-Object 1", true, "object pass"},
		{"ST5>APRS,qAR," + login + ":{Q1qwerty", false, "user-defined drop"},
		{"ST6>APRS,qAR," + login + ":)OBJ1!4903.50N/07201.75WA", true, "item pass"},
		// Weather packet with position: matches t/p but dropped by -t/w.
		{"ST10>APRS,qAR," + login + ":@100857z5241.73N/00611.14E_086/002g008t064r000p000P000h63b10102L810.DsIP", false, "wx drop via -t/w"},
	}
	for _, c := range cases {
		got := f.Match(mustParse(t, c.raw), nil)
		if got != c.want {
			t.Errorf("%s: match %q = %v, want %v", c.desc, c.raw, got, c.want)
		}
	}
}

func TestTypeFilterSecondHalf(t *testing.T) {
	f := Compile("t/stunwc")
	login := "N5CAL-1"
	cases := []struct {
		raw  string
		want bool
		desc string
	}{
		{"ST11>APRS,qAR," + login + ":>status pass", true, "status pass"},
		{"ST12>APRS,qAR," + login + ":!6028.51N/02505.68E# pos drop", false, "position drop"},
		{"ST13>APRS,qAR," + login + ":T#931,113,000,000,000,000,00000000", true, "telemetry pass"},
		{"ST14>APRS,qAR," + login + ":;OBJ1     *090902z6010.78N/02451.11E-Object 1", false, "object drop"},
		{"ST15>APRS,qAR," + login + ":{Q1qwerty", true, "user-defined pass"},
		{"ST16>APRS,qAR," + login + ":)OBJ1!4903.50N/07201.75WA", false, "item drop"},
	}
	for _, c := range cases {
		got := f.Match(mustParse(t, c.raw), nil)
		if got != c.want {
			t.Errorf("%s: match %q = %v, want %v", c.desc, c.raw, got, c.want)
		}
	}
}

func TestAreaFilter(t *testing.T) {
	// Box covering Northern Europe roughly.
	f := Compile("a/62/24/58/26")
	in := mustParse(t, "SRC>APRS,qAR,N5CAL-1:!6028.51N/02505.68E#")
	if !f.Match(in, nil) {
		t.Error("expected packet inside box to pass a/ filter")
	}
	out := mustParse(t, "SRC>APRS,qAR,N5CAL-1:!4028.51N/02505.68E#")
	if f.Match(out, nil) {
		t.Error("expected packet outside box to be dropped")
	}
}

// TestTypeFilterCWOP verifies that t/c selects CWOP weather while t/w excludes
// it (aprsc semantics).
func TestTypeFilterCWOP(t *testing.T) {
	// A CWOP station (CW####) sending a positionless weather report.
	cwop := mustParse(t, "CW1234>APRS,qAR,N5CAL-1:_10090556c220s004g005t077r000p000P000h50b09900")
	// A regular weather station.
	wx := mustParse(t, "OH2WX>APRS,qAR,N5CAL-1:_10090556c220s004g005t077r000p000P000h50b09900")

	cFilter := Compile("t/c")
	wFilter := Compile("t/w")

	if !cFilter.Match(cwop, nil) {
		t.Error("t/c should match a CWOP station")
	}
	if cFilter.Match(wx, nil) {
		t.Error("t/c should NOT match a non-CWOP weather station")
	}
	if wFilter.Match(cwop, nil) {
		t.Error("t/w should NOT match a CWOP station")
	}
	if !wFilter.Match(wx, nil) {
		t.Error("t/w should match a regular weather station")
	}
}

func TestMyRangeFilter(t *testing.T) {
	f := Compile("m/100")
	pkt := mustParse(t, "SRC>APRS,qAR,N5CAL-1:!6028.51N/02505.68E#")

	// No client position known -> no match.
	if f.Match(pkt, fixedCtx{}) {
		t.Error("m/ should not match when client position unknown")
	}
	// Client near the packet -> match.
	ctx := fixedCtx{client: &Position{Lat: 60.5, Lon: 25.1}}
	if !f.Match(pkt, ctx) {
		t.Error("m/ should match within range of client position")
	}
	// Client far away -> no match.
	far := fixedCtx{client: &Position{Lat: 0, Lon: 0}}
	if f.Match(pkt, far) {
		t.Error("m/ should not match when client far away")
	}
}

func TestFriendRangeFilter(t *testing.T) {
	f := Compile("f/OH2RDP-1/100")
	pkt := mustParse(t, "SRC>APRS,qAR,N5CAL-1:!6028.51N/02505.68E#")
	ctx := fixedCtx{stations: map[string]Position{
		"OH2RDP-1": {Lat: 60.5, Lon: 25.1},
	}}
	if !f.Match(pkt, ctx) {
		t.Error("f/ should match within range of friend position")
	}
	if f.Match(pkt, fixedCtx{}) {
		t.Error("f/ should not match when friend position unknown")
	}
}

func TestEntryFilter(t *testing.T) {
	f := Compile("e/N5CAL-1")
	// Entry station is the call right after the q-construct.
	pkt := mustParse(t, "SRC>APRS,OH2RDG*,WIDE,qAR,N5CAL-1:!6028.51N/02505.68E#")
	if !f.Match(pkt, nil) {
		t.Error("e/ should match entry station after q-construct")
	}
	other := mustParse(t, "SRC>APRS,qAR,OTHER-1:!6028.51N/02505.68E#")
	if f.Match(other, nil) {
		t.Error("e/ should not match different entry station")
	}
}

func TestDigiFilter(t *testing.T) {
	f := Compile("d/OH2RDG")
	// Used digi has a trailing '*'.
	used := mustParse(t, "SRC>APRS,OH2RDG*,WIDE,qAR,N5CAL-1:!6028.51N/02505.68E#")
	if !f.Match(used, nil) {
		t.Error("d/ should match used digipeater")
	}
	unused := mustParse(t, "SRC>APRS,OH2RDG,WIDE,qAR,N5CAL-1:!6028.51N/02505.68E#")
	if f.Match(unused, nil) {
		t.Error("d/ should not match un-used digipeater (no '*')")
	}
}

func TestQConstructFilter(t *testing.T) {
	f := Compile("q/C")
	qac := mustParse(t, "SRC>APRS,TCPIP*,qAC,SERVER:!6028.51N/02505.68E#")
	if !f.Match(qac, nil) {
		t.Error("q/C should match qAC packet")
	}
	qar := mustParse(t, "SRC>APRS,qAR,N5CAL-1:!6028.51N/02505.68E#")
	if f.Match(qar, nil) {
		t.Error("q/C should not match qAR packet")
	}
}

func TestUnprotoFilter(t *testing.T) {
	f := Compile("u/APRS")
	pkt := mustParse(t, "SRC>APRS,qAR,N5CAL-1:!6028.51N/02505.68E#")
	if !f.Match(pkt, nil) {
		t.Error("u/APRS should match destination APRS")
	}
	pkt2 := mustParse(t, "SRC>APZTLE,qAR,N5CAL-1:!6028.51N/02505.68E#")
	if f.Match(pkt2, nil) {
		t.Error("u/APRS should not match destination APZTLE")
	}
}

func TestGroupFilter(t *testing.T) {
	f := Compile("g/WU2Z")
	msg := mustParse(t, "SRC>APRS,qAR,N5CAL-1::WU2Z     :hello{1")
	if !f.Match(msg, nil) {
		t.Error("g/WU2Z should match message addressed to WU2Z")
	}
}

func TestSymbolFilter(t *testing.T) {
	// Primary table '/' with car symbol '>'.
	// Uncompressed format: lat[N/S]<table>lon[E/W]<symbol>
	f := Compile("s/>")
	pkt := mustParse(t, "SRC>APRS,qAR,N5CAL-1:!6028.51N/02505.68E>car")
	if !f.Match(pkt, nil) {
		t.Errorf("s/> should match primary-table '>' symbol (got symbol %v)", pkt.Symbol)
	}
}

func TestEmptyAndNegationOnly(t *testing.T) {
	if Compile("").Match(mustParse(t, "SRC>APRS,qAR,N5CAL-1:>hi"), nil) {
		t.Error("empty filter should match nothing")
	}
	// Negation only: still matches nothing (no positive spec).
	if Compile("-t/m").Match(mustParse(t, "SRC>APRS,qAR,N5CAL-1:>hi"), nil) {
		t.Error("negation-only filter should match nothing")
	}
}
