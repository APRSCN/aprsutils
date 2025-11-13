package parser

// parseThirdParty parses third-party data from APRS packet
func (p *Parsed) parseThirdParty(body string) error {
	p.Format = "thirdparty"

	parsed, err := Parse(body)
	if err != nil {
		return err
	}

	p.SubPacket = &parsed

	return nil
}
