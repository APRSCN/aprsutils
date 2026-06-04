package parser

import "strings"

// parseInvalid parses invalid APRS packet
func (p *Parsed) parseInvalid(body string) string {
	p.Format = "invalid"
	p.Body = body
	return body
}

// parseUserDefined parses user defined APRS packet
func (p *Parsed) parseUserDefined(body string) string {
	p.Format = "user-defined"
	runes := []rune(body)
	// Body always has at least one rune here (guaranteed by parseBody), but the
	// type byte may be missing on malformed packets — guard the slice accesses.
	if len(runes) >= 1 {
		p.ID = string(runes[0])
	}
	if len(runes) >= 2 {
		p.Type = string(runes[1])
		p.Body = string(runes[2:])
	} else {
		p.Body = ""
	}
	return body
}

// parseStatus parses status packet
func (p *Parsed) parseStatus(body string) string {
	p.Format = "status"
	p.Status = strings.Trim(body, " ")
	return body
}
