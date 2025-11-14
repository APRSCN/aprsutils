package qConstruct

import "strings"

// Replace path in the APRS packet
func Replace(packet string, from []string, to []string) string {
	fromStr := strings.Join(from, ",")
	toStr := strings.Join(to, ",")

	return strings.Replace(packet, fromStr, toStr, 1)
}
