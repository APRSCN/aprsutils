package parser

import (
	"errors"
	"strings"

	"github.com/APRSCN/aprsutils"
)

// parseHeader parses header of APRS packet
func (p *Parsed) parseHeader(head string) error {
	// Split fromCall and path
	fromCall, path, ok := SplitOnce(head, ">")
	if !ok {
		return errors.New("invalid packet header")
	}

	// Check fromCall
	if !(1 <= StringLen(fromCall) && StringLen(fromCall) <= 9) ||
		!aprsutils.CompiledRegexps.Get(`(?i)^[a-z0-9]{0,9}(-[a-z0-9]{1,8})?$`).MatchString(fromCall) {
		return errors.New("fromCallsign is invalid")
	}

	// Split paths
	paths := strings.Split(path, ",")
	if len(paths) < 1 {
		return errors.New("no toCallsign in header")
	}

	// Check toCall
	if StringLen(paths[0]) == 0 {
		return errors.New("no toCallsign in header")
	}

	toCall := paths[0]
	paths = paths[1:]

	// Validate callsign
	ok = aprsutils.ValidateCallsign(toCall)
	if !ok {
		return errors.New("invalid toCallsign in header")
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
		if !aprsutils.CompiledRegexps.Get(`(?i)^[A-Z0-9\-]{1,9}\*?$`).MatchString(pa) {
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
	// Get type
	packetType := string([]rune(body)[0:1])
	body = string([]rune(body)[1:])

	if StringLen(body) == 0 && packetType != ">" {
		return errors.New("packet body is empty after packet type character")
	}

	// Check formats
	for _, f := range unsupportedFormats {
		if packetType == f {
			return errors.New("packet type is unsupported")
		}
	}

	// Match type
	switch packetType {
	// 3rd party traffic
	case "}":
		err := p.parseThirdParty(body)
		if err != nil {
			return err
		}
	// Invalid
	case ",":
		p.parseInvalid(body)
	// User defined
	case "{":
		p.parseUserDefined(body)
	// Status report
	case ">":
		p.parseStatus(body)
	// Mic-E packet
	case "`":
		fallthrough
	case "‘":
		fallthrough
	case "'":
		_, err := p.parseMicE(p.To, body)
		if err != nil {
			return err
		}
	// Message packet
	case ":":
		p.parseMessage(body)
	// Positionless weather report
	case "_":
		_, err := p.parseWeather(body)
		if err != nil {
			return err
		}
	// Position report (regular or compressed)
	case "!":
		fallthrough
	case "=":
		fallthrough
	case "/":
		fallthrough
	case "@":
		fallthrough
	case ";":
		err := p.parsePosition(packetType, body)
		if err != nil {
			return err
		}
	default:
		// Position report (regular or compressed)
		if pos := strings.Index(body, "!"); pos >= 0 && pos < 40 {
			err := p.parsePosition(packetType, body)
			if err != nil {
				return err
			}
		} else {
			// Invalid
			p.parseInvalid(body)
		}
	}

	return nil
}
