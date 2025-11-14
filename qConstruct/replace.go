package qConstruct

import (
	"errors"
	"strings"

	"github.com/APRSCN/aprsutils/utils"
)

// Replace path in the APRS packet
func Replace(packet string, toCall string, newPath []string) (string, error) {
	// Split head and body
	head, _, ok := utils.SplitOnce(packet, ":")
	if !ok {
		return "", errors.New("packet has no body")
	}

	// Check body
	if utils.StringLen(head) == 0 {
		return "", errors.New("packet head is empty")
	}

	// Split fromCall and path
	_, path, ok := utils.SplitOnce(head, ">")
	if !ok {
		return "", errors.New("invalid packet header")
	}

	// Replace
	packet = strings.Replace(
		packet, path,
		strings.Join(append([]string{toCall}, newPath...), ","),
		-1,
	)

	return packet, nil
}
