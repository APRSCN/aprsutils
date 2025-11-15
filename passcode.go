package aprsutils

import (
	"strings"

	"github.com/APRSCN/aprsutils/utils"
)

const key = 0x73e2 // This is the key for the data

// Passcode calculates passcode of the callsign
func Passcode(callsign string) int {
	// Trim SSID
	parts := strings.SplitN(callsign, "-", 2)
	rootCall := parts[0]

	// Transfer to upper case
	if utils.StringLen(rootCall) > 8 {
		rootCall = rootCall[:8]
	}
	rootCall = strings.ToUpper(rootCall)

	hash := key // Initialize with the key value
	data := []byte(rootCall)
	length := len(data)

	for i := 0; i+1 < length; i += 2 {
		hash ^= int(data[i]) << 8 // XOR high byte with accumulated hash
		hash ^= int(data[i+1])    // XOR low byte with accumulated hash
	}

	return hash & 0x7fff
}
