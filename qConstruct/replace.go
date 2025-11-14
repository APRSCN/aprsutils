package qConstruct

import (
	"strings"

	"github.com/APRSCN/aprsutils/parser"
)

// Replace path in the APRS packet
func Replace(packet string, parsed parser.Parsed, to []string) string {
	fromStr := strings.Join(parsed.Path, ",") + ":"
	toStr := strings.Join(to, ",") + ":"

	// With no path
	if strings.Contains(packet, parsed.To+":") {
		toStr = "," + toStr
	}

	return strings.Replace(packet, fromStr, toStr, 1)
}
