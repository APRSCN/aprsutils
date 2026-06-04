package parser

import (
	"errors"
	"strings"

	"go.gh.ink/regexp"

	"github.com/APRSCN/aprsutils"
	"github.com/APRSCN/aprsutils/utils"
)

// parseHeader parses header of APRS packet
func (p *Parsed) parseHeader(head string, conf *config) error {
	// Split fromCall and path
	fromCall, path, ok := utils.SplitOnce(head, ">")
	if !ok {
		return errors.New("invalid packet header")
	}

	// Check fromCall
	if !(1 <= utils.StringLen(fromCall) && utils.StringLen(fromCall) <= 9) ||
		!regexp.MustCompile(`(?i)^[a-z0-9]{0,9}(-[a-z0-9]{1,8})?$`).MatchString(fromCall) {
		return errors.New("fromCallsign is invalid")
	}

	// Split paths
	paths := strings.Split(path, ",")
	if len(paths) < 1 {
		return errors.New("no toCallsign in header")
	}

	// Check toCall
	if utils.StringLen(paths[0]) == 0 {
		return errors.New("no toCallsign in header")
	}

	toCall := paths[0]
	paths = paths[1:]

	// Validate callsign
	if !conf.disableToCallsignValidate {
		if ok = aprsutils.ValidateCallsign(toCall); !ok {
			return errors.New("invalid toCallsign in header")
		}
	}

	// Remove space path
	i := 0
	for _, pa := range paths {
		if strings.TrimSpace(pa) != "" {
			paths[i] = pa
			i++
		}
	}
	paths = paths[:i]

	// Check callsign in paths
	for _, pa := range paths {
		if !regexp.MustCompile(`(?i)^[A-Z0-9\-]{1,9}\*?$`).MatchString(pa) {
			return errors.New("invalid callsign in path")
		}
	}

	// Save result
	p.From = fromCall
	p.To = toCall
	p.Path = paths

	return nil
}

// parseBody parses body of APRS packet
func (p *Parsed) parseBody(body string) error {
	// Get type (first rune)
	runes := []rune(body)
	if len(runes) == 0 {
		return errors.New("packet body is empty")
	}
	packetType := string(runes[0:1])
	body = string(runes[1:])

	// Only status reports may have an empty payload (e.g. ">").
	if utils.StringLen(body) == 0 && packetType != ">" {
		return errors.New("packet body is empty after packet type character")
	}

	// Reject formats we explicitly do not decode.
	if _, ok := unsupportedFormats[packetType]; ok {
		p.parseInvalid(body)
		return errors.New("packet type is unsupported")
	}

	// Match type
	switch packetType {
	// 3rd party traffic
	case "}":
		if err := p.parseThirdParty(body); err != nil {
			return err
		}
		p.PacketType |= TypeThirdParty
	// Invalid
	case ",":
		p.parseInvalid(body)
	// User defined
	case "{":
		p.parseUserDefined(body)
		p.PacketType |= TypeUserDef
	// Status report
	case ">":
		p.parseStatus(body)
		p.PacketType |= TypeStatus
	// Query
	case "?":
		p.parseQuery(body)
		p.PacketType |= TypeQuery
	// Telemetry report (Tnnn or T#nnn)
	case "T":
		p.parseTelemetryReport(body)
		p.PacketType |= TypeTelemetry
	// Raw NMEA / GPS sentence
	case "$":
		p.parseNMEA(body)
		p.PacketType |= TypeNMEA
	// Item report
	case ")":
		if err := p.parseItem(body); err != nil {
			return err
		}
		p.PacketType |= TypeItem
	// Mic-E packet
	case "`", "‘", "'":
		if _, err := p.parseMicE(p.To, body); err != nil {
			return err
		}
		p.PacketType |= TypePosition
	// Message packet
	case ":":
		p.parseMessage(body)
		p.PacketType |= TypeMessage
		if p.Format == "bulletin" || p.Format == "group-bulletin" || p.Format == "announcement" {
			p.PacketType |= TypeBulletin
		}
	// Positionless weather report ("_" classic, "#"/"*" raw)
	case "_", "#", "*":
		if _, err := p.parseWeather(body); err != nil {
			return err
		}
		p.PacketType |= TypeWeather
	// Object report
	case ";":
		if err := p.parsePosition(packetType, body); err != nil {
			return err
		}
		p.PacketType |= TypeObject
	// Position report (regular or compressed)
	case "!", "=", "/", "@":
		if err := p.parsePosition(packetType, body); err != nil {
			return err
		}
		p.PacketType |= TypePosition
	default:
		// Some clients omit the leading data-type char; if an embedded '!'
		// appears early, treat the body as a position report.
		if pos := strings.Index(body, "!"); pos >= 0 && pos < 40 {
			if err := p.parsePosition(packetType, body); err != nil {
				return err
			}
			p.PacketType |= TypePosition
		} else {
			p.parseInvalid(body)
		}
	}

	// Mark presence of a usable position fix for position-aware filters.
	if len(p.Symbol) == 2 {
		p.HasPosition = true
	}

	// Weather data also implies a weather type even on positioned reports.
	if len(p.Weather) > 0 {
		p.PacketType |= TypeWeather
	}

	// CWOP weather (Citizen Weather Observer Program): CW####/DW####/...
	// callsigns or APRSWXNET path. Tracked separately so t/w can exclude it
	// and t/c can select it.
	if p.PacketType.Has(TypeWeather) && isCWOP(p) {
		p.PacketType |= TypeCWOP
	}

	// NWS detection: messages/objects whose addressee/identifier/source look
	// like National Weather Service broadcasts.
	if p.PacketType.Has(TypeMessage|TypeObject) && isNWS(p) {
		p.PacketType |= TypeNWS
	}

	return nil
}

// cwopCallRe matches CWOP station callsigns: two letters from C..F (the
// CWOP-assigned ranges) followed by 4+ digits, e.g. CW1234, DW5678, EW0001.
var cwopCallRe = regexp.MustCompile(`(?i)^[CDEFGH]W\d{3,}$`)

// isCWOP reports whether a (weather) packet originates from a CWOP station.
func isCWOP(p *Parsed) bool {
	if cwopCallRe.MatchString(p.From) {
		return true
	}
	// CWOP traffic is frequently relayed via the APRSWXNET destination/path.
	if p.To == "APRSWXNET" {
		return true
	}
	for _, hop := range p.Path {
		if hop == "APRSWXNET" {
			return true
		}
	}
	return false
}

// isNWS heuristically detects National Weather Service broadcasts.
func isNWS(p *Parsed) bool {
	if strings.HasPrefix(p.Addressee, "NWS") || strings.HasPrefix(p.Identifier, "NWS") {
		return true
	}
	if strings.HasPrefix(p.From, "NWS") {
		return true
	}
	return false
}
