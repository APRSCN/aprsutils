package parser

import (
	"errors"
	"strconv"
	"strings"

	"go.gh.ink/regexp"

	"github.com/APRSCN/aprsutils/utils"
)

// itemNameRe matches an APRS item name (3-9 chars) followed by the live flag.
//
// Item format (aprs101.pdf ch. 11):
//
//	)DDDDDDDDD!....   where the name is 3-9 chars and the flag is '!' (live) or '_' (killed)
var itemNameRe = regexp.MustCompile(`^([\x20-\x7e]{3,9})(!|_)`)

// parseItem parses an APRS item report ( ')' data type ).
func (p *Parsed) parseItem(body string) error {
	matches := itemNameRe.FindStringSubmatch(body)
	if len(matches) < 3 {
		p.parseInvalid(body)
		return errors.New("invalid item format")
	}

	name := strings.TrimRight(matches[1], " ")
	flag := matches[2]

	p.ObjectName = name
	p.Alive = flag == "!"

	// Remaining payload after name + flag is a position report (compressed or
	// uncompressed) optionally followed by a comment.
	rest := string([]rune(body)[utils.StringLen(matches[1])+1:])

	// Reuse the position decoder. We feed type "!" so it decodes position only.
	if err := p.parsePosition("!", rest); err != nil {
		// Items may legitimately be position-less in malformed feeds; keep the
		// name but flag the format rather than failing the whole packet.
		p.Format = "item"
		return nil
	}

	p.ObjectFormat = p.Format
	p.Format = "item"
	return nil
}

// parseQuery parses an APRS general query ( '?' data type ).
func (p *Parsed) parseQuery(body string) string {
	p.Format = "query"
	// The query type is the text up to a comma or end of line, e.g. "?APRS?".
	q := strings.TrimSpace(body)
	q = strings.TrimSuffix(q, "?")
	p.Body = q
	return body
}

// telemetryReportRe matches a telemetry data report: T#nnn or Tnnn style.
//
//	T#005,199,000,255,073,123,01101001
var telemetryReportRe = regexp.MustCompile(`^#?(\d{1,3}|MIC)\s*,?(.*)$`)

// parseTelemetryReport parses an APRS telemetry data report ( 'T' data type ).
func (p *Parsed) parseTelemetryReport(body string) string {
	p.Format = "telemetry"

	matches := telemetryReportRe.FindStringSubmatch(body)
	if matches == nil {
		p.Body = body
		return body
	}

	seq := matches[1]
	fields := strings.Split(matches[2], ",")

	// Sequence number (non-numeric "MIC" stays 0).
	if n, err := strconv.Atoi(seq); err == nil {
		p.Telemetry.Seq = n
	}

	// Up to 5 analogue channels followed by an 8-bit digital field.
	vals := make([]int, 0, 5)
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		// The trailing all-binary token is the digital bit field.
		if len(f) == 8 && isBinaryString(f) {
			p.Telemetry.Bits = f
			continue
		}
		if v, err := strconv.Atoi(f); err == nil {
			vals = append(vals, v)
		} else if fv, err := strconv.ParseFloat(f, 64); err == nil {
			vals = append(vals, int(fv))
		}
	}
	p.Telemetry.Vals = vals
	return body
}

// parseNMEA records a raw NMEA / GPS sentence ( '$' data type ).
//
// We do not attempt full GPRMC/GPGGA decoding here (that can be layered on
// later); we keep the raw body and mark the format so type filters work.
func (p *Parsed) parseNMEA(body string) string {
	p.Format = "nmea"
	p.Body = body
	return body
}

// isBinaryString reports whether s consists solely of '0'/'1'.
func isBinaryString(s string) bool {
	for _, r := range s {
		if r != '0' && r != '1' {
			return false
		}
	}
	return s != ""
}
