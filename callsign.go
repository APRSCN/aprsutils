package aprsutils

import (
	"github.com/APRSCN/aprsutils/utils"
	"github.com/ghinknet/regexp"
)

// ValidateCallsign checks whether a callsign is valid
func ValidateCallsign(callsign string) bool {
	// Match
	return (1 <= utils.StringLen(callsign) && utils.StringLen(callsign) <= 9) &&
		regexp.MustCompile(`(?i)^[a-z0-9]{0,9}(-[a-z0-9]{1,8})?$`).MatchString(callsign)
}
