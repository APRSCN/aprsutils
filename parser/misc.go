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
	p.ID = string([]rune(body)[0])
	p.Type = string([]rune(body)[1])
	p.Body = string([]rune(body)[2:])
	return body
}

// parseStatus parses status packet
func (p *Parsed) parseStatus(body string) string {
	p.Format = "status"
	p.Status = strings.Trim(body, " ")
	return body
}
