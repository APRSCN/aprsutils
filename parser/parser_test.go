package parser

import (
	"math"
	"testing"
)

// approx reports whether got is within tol of want.
func approx(got, want, tol float64) bool {
	return math.Abs(got-want) <= tol
}

func TestParseHeader(t *testing.T) {
	p, err := Parse("OH2RDP-1>BEACON-15,OH2RDG*,WIDE,qAS,N5CAL-1:>status text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.From != "OH2RDP-1" {
		t.Errorf("From = %q, want OH2RDP-1", p.From)
	}
	if p.To != "BEACON-15" {
		t.Errorf("To = %q, want BEACON-15", p.To)
	}
	wantPath := []string{"OH2RDG*", "WIDE", "qAS", "N5CAL-1"}
	if len(p.Path) != len(wantPath) {
		t.Fatalf("Path = %v, want %v", p.Path, wantPath)
	}
	for i := range wantPath {
		if p.Path[i] != wantPath[i] {
			t.Errorf("Path[%d] = %q, want %q", i, p.Path[i], wantPath[i])
		}
	}
}

func TestParseUncompressedPosition(t *testing.T) {
	// Known-good from aprsc t/30parser-filter.t: matches r/60.4752/25.0947/1
	p, err := Parse("OH2RDP-1>BEACON-15,OH2RDG*,WIDE:!6028.51N/02505.68E#PHG7220 should pass")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.HasPosition {
		t.Fatal("HasPosition = false, want true")
	}
	if !approx(p.Lat, 60.4752, 0.001) {
		t.Errorf("Lat = %f, want ~60.4752", p.Lat)
	}
	if !approx(p.Lon, 25.0947, 0.001) {
		t.Errorf("Lon = %f, want ~25.0947", p.Lon)
	}
	if !p.PacketType.Has(TypePosition) {
		t.Errorf("PacketType missing TypePosition: %b", p.PacketType)
	}
	if p.PHG == "" {
		t.Errorf("expected PHG to be parsed")
	}
}

func TestParseCompressedPosition(t *testing.T) {
	// From aprsc t/30parser-filter.t COMPRESSED packet.
	p, err := Parse("OH2RDP-1>BEACON-15:!I0-X;T_Wv&{-Aigate testing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Format != "compressed" {
		t.Errorf("Format = %q, want compressed", p.Format)
	}
	if !p.HasPosition {
		t.Error("HasPosition = false, want true")
	}
	// Coordinates should be plausible (within global bounds, non-zero).
	if p.Lat < -90 || p.Lat > 90 || p.Lon < -180 || p.Lon > 180 {
		t.Errorf("lat/lon out of range: %f %f", p.Lat, p.Lon)
	}
}

func TestParseMicE(t *testing.T) {
	// From aprsc t/30parser-filter.t: latitude 47.93283, longitude 12.93733.
	p, err := Parse("OX8AAA>T7UU97,qAR,N5CAL-1:`(T4l!u>/]\"83}=")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Format != "mic-e" {
		t.Errorf("Format = %q, want mic-e", p.Format)
	}
	if !approx(p.Lat, 47.93283, 0.01) {
		t.Errorf("Lat = %f, want ~47.93283", p.Lat)
	}
	if !approx(p.Lon, 12.93733, 0.01) {
		t.Errorf("Lon = %f, want ~12.93733", p.Lon)
	}
	if !p.PacketType.Has(TypePosition) {
		t.Errorf("PacketType missing TypePosition")
	}
}

func TestParseMessage(t *testing.T) {
	p, err := Parse("WU2Z>APRS,TCPIP*,qAC,FOURTH::WU2Z     :Testing{003")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.PacketType.Has(TypeMessage) {
		t.Errorf("PacketType missing TypeMessage")
	}
	if p.Addressee != "WU2Z" {
		t.Errorf("Addressee = %q, want WU2Z", p.Addressee)
	}
	if p.MessageText != "Testing" {
		t.Errorf("MessageText = %q, want Testing", p.MessageText)
	}
	if p.MsgNo != "003" {
		t.Errorf("MsgNo = %q, want 003", p.MsgNo)
	}
}

func TestParseStatus(t *testing.T) {
	p, err := Parse("OH2RDP-1>BEACON-15,qAS,N5CAL-1:>Net Control Center")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.PacketType.Has(TypeStatus) {
		t.Errorf("PacketType missing TypeStatus")
	}
	if p.Status != "Net Control Center" {
		t.Errorf("Status = %q", p.Status)
	}
}

func TestParseObject(t *testing.T) {
	p, err := Parse("SRC>APRS,qAR,N5CAL-1:;OBJ1     *090902z6010.78N/02451.11E-Object 1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.PacketType.Has(TypeObject) {
		t.Errorf("PacketType missing TypeObject")
	}
	if p.ObjectName != "OBJ1" {
		t.Errorf("ObjectName = %q, want OBJ1", p.ObjectName)
	}
	if !p.Alive {
		t.Errorf("Alive = false, want true")
	}
}

func TestParseItem(t *testing.T) {
	p, err := Parse("SRC>APRS,qAR,N5CAL-1:)OBJ1!4903.50N/07201.75WA")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.PacketType.Has(TypeItem) {
		t.Errorf("PacketType missing TypeItem: %b", p.PacketType)
	}
	if p.ObjectName != "OBJ1" {
		t.Errorf("ObjectName = %q, want OBJ1", p.ObjectName)
	}
	if !p.Alive {
		t.Errorf("Alive = false, want true")
	}
}

func TestParseThirdParty(t *testing.T) {
	p, err := Parse("SRC>APRS,qAR,N5CAL-1:}OH2RDP-1>BEACON,TCPIP*:>inner status")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.PacketType.Has(TypeThirdParty) {
		t.Errorf("PacketType missing TypeThirdParty")
	}
	if p.SubPacket == nil {
		t.Fatal("SubPacket is nil")
	}
	if p.SubPacket.From != "OH2RDP-1" {
		t.Errorf("SubPacket.From = %q, want OH2RDP-1", p.SubPacket.From)
	}
}

func TestParseTelemetryReport(t *testing.T) {
	p, err := Parse("SRC>APRS,qAR,N5CAL-1:T#005,199,000,255,073,123,01101001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.PacketType.Has(TypeTelemetry) {
		t.Errorf("PacketType missing TypeTelemetry")
	}
	if p.Telemetry.Seq != 5 {
		t.Errorf("Telemetry.Seq = %d, want 5", p.Telemetry.Seq)
	}
	if p.Telemetry.Bits != "01101001" {
		t.Errorf("Telemetry.Bits = %q, want 01101001", p.Telemetry.Bits)
	}
	if len(p.Telemetry.Vals) != 5 {
		t.Errorf("Telemetry.Vals len = %d, want 5", len(p.Telemetry.Vals))
	}
}

func TestParseQuery(t *testing.T) {
	p, err := Parse("SRC>APRS,qAR,N5CAL-1:?APRS?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.PacketType.Has(TypeQuery) {
		t.Errorf("PacketType missing TypeQuery")
	}
}

// Robustness: malformed packets must never panic and should return an error
// (or a parsed-but-invalid result) rather than crashing the server.
func TestParseMalformedNoPanic(t *testing.T) {
	cases := []string{
		"",
		":",
		">",
		"A>B:",
		"A>B:{",
		"A>B:{X",
		"A>B:)",
		"A>B:;",
		"A>B:`",
		"A>B:!",
		"A>B:T",
		"A>B:?",
		"A>B:$",
		"A>B:_",
		"NOCOLON",
		">NOHEADER:test",
		"A>B:" + string([]byte{0x00, 0x01, 0x02}),
		"VERY-LONG-CALLSIGN-THAT-EXCEEDS>B:!test",
	}
	for _, c := range cases {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Parse(%q) panicked: %v", c, r)
				}
			}()
			_, _ = Parse(c, WithDisableToCallsignValidate())
		}()
	}
}

func TestParseUnsupportedFormat(t *testing.T) {
	_, err := Parse("SRC>APRS,qAR,N5CAL-1:<station capabilities")
	if err == nil {
		t.Error("expected error for unsupported format '<'")
	}
}
