package aprsutils

// ValidateCallsign checks whether a callsign is valid
func ValidateCallsign(callsign string) bool {
	// Match
	pattern := `^([A-Z0-9]{1,6})(-(\d{1,2}))?$`
	re := CompiledRegexps.Get(pattern)
	matches := re.FindStringSubmatch(callsign)

	return matches != nil
}
