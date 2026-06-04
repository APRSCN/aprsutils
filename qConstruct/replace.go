package qConstruct

import (
	"errors"
	"strings"

	"github.com/APRSCN/aprsutils/utils"
)

// Replace rewrites the path in the header of an APRS packet, leaving the body
// untouched. It rebuilds the header as "fromCall>toCall,newPath..." so that
// path-like text appearing in the body/comment is never accidentally modified.
func Replace(packet string, toCall string, newPath []string) (string, error) {
	// Split head and body on the FIRST colon.
	head, body, ok := utils.SplitOnce(packet, ":")
	if !ok {
		return "", errors.New("packet has no body")
	}

	// Check head
	if utils.StringLen(head) == 0 {
		return "", errors.New("packet head is empty")
	}

	// Split fromCall and path
	fromCall, _, ok := utils.SplitOnce(head, ">")
	if !ok {
		return "", errors.New("invalid packet header")
	}

	// Rebuild header: fromCall>toCall[,newPath...]
	var b strings.Builder
	b.WriteString(fromCall)
	b.WriteString(">")
	b.WriteString(toCall)
	for _, p := range newPath {
		b.WriteString(",")
		b.WriteString(p)
	}
	b.WriteString(":")
	b.WriteString(body)

	return b.String(), nil
}
