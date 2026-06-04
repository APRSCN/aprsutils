package parser

import (
	"strings"

	"go.gh.ink/regexp"
	"go.gh.ink/toolbox/expr"

	"github.com/APRSCN/aprsutils/utils"
)

// Message sub-format regexps, compiled once at package load.
var (
	reBulletin     = regexp.MustCompile(`(?i)^BLN([0-9])([a-z0-9_ \-]{5}):(.{0,67})`)
	reAnnouncement = regexp.MustCompile(`^BLN([A-Z])([a-zA-Z0-9_ \-]{5}):(.{0,67})`)
	reAddressed    = regexp.MustCompile(`^([a-zA-Z0-9_ \-]{9}):(.*)$`)
	// NEW reply-ack ack/rej: ackMM}AA
	reAckRejReply = regexp.MustCompile(`^(ack|rej)([A-Za-z0-9]{2})}([A-Za-z0-9]{2})?$`)
	// Standard ack/rej (aprs101.pdf ch.14): ack12345
	reAckRej = regexp.MustCompile(`^(ack|rej)([A-Za-z0-9]{1,5})$`)
	// NEW message format trailer: text...{MM}AA
	reMsgNoReply = regexp.MustCompile(`{([A-Za-z0-9]{2})}([A-Za-z0-9]{2})?$`)
	// Old message format trailer: text...{msgNo
	reMsgNo = regexp.MustCompile(`{([A-Za-z0-9]{1,5})$`)
)

// parseMessage parses a message (":") body, populating the relevant Parsed
// fields and Format. APRS supports two message formats:
//   - the standard format described in aprs101.pdf
//   - the 1999 reply-ack addendum (http://www.aprs.org/aprs11/replyacks.txt)
//
// A message (ack/rej or a text body) may carry no message number, an
// old-format number (1..5 chars), or a new-format number (2 chars) with an
// optional trailing free ack number.
func (p *Parsed) parseMessage(body string) {
	switch {
	// Bulletin: BLN<digit><id>:text
	case matchN(reBulletin, body, 4):
		m := reBulletin.FindStringSubmatch(body)
		identifier := strings.TrimRight(m[2], " ")
		p.Format = expr.Ternary(identifier != "", "group-bulletin", "bulletin")
		p.MessageText = strings.Trim(m[3], " ")
		p.BID = m[1]
		p.Identifier = identifier

	// Announcement: BLN<letter><id>:text
	case matchN(reAnnouncement, body, 4):
		m := reAnnouncement.FindStringSubmatch(body)
		p.Format = "announcement"
		p.MessageText = strings.Trim(m[3], " ")
		p.AID = m[1]
		p.Identifier = strings.TrimRight(m[2], " ")

	// Addressed message: <9-char addressee>:body
	case matchN(reAddressed, body, 3):
		m := reAddressed.FindStringSubmatch(body)
		p.Addressee = strings.TrimRight(m[1], " ")
		p.parseAddressedMessage(m[2])
	}
}

// matchN reports whether re matches body with at least n submatch groups
// (including the full match at index 0).
func matchN(re *regexp.Regexp, body string, n int) bool {
	m := re.FindStringSubmatch(body)
	return m != nil && len(m) >= n
}

// parseAddressedMessage parses the part following the leading
// "<addressee>:" of a message packet, setting Format and the message/ack
// fields.
func (p *Parsed) parseAddressedMessage(body string) {
	// Telemetry configuration (PARM/UNIT/EQNS/BITS) is itself an addressed
	// message; parseTelemetryConfig sets Format="telemetry-message" when it
	// matches. Only fall back to a plain "message" when it did not.
	if _, err := p.parseTelemetryConfig(body); err == nil && p.Format == "telemetry-message" {
		return
	}

	p.Format = "message"

	switch {
	// NEW reply-ack ack/rej: ackMM}AA
	case matchN(reAckRejReply, body, 3):
		m := reAckRejReply.FindStringSubmatch(body)
		p.Response = m[1]
		p.MsgNo = m[2]
		if len(m) >= 4 && m[3] != "" {
			p.AckMsgNo = m[3]
		}

	// Standard ack/rej: ack12345
	case matchN(reAckRej, body, 3):
		m := reAckRej.FindStringSubmatch(body)
		p.Response = m[1]
		p.MsgNo = m[2]

	// NEW message format with trailing {MM}AA
	case matchN(reMsgNoReply, body, 2):
		m := reMsgNoReply.FindStringSubmatch(body)
		ackMsgNo := ""
		if len(m) >= 3 {
			ackMsgNo = m[2]
		}
		removeLen := 4 + utils.StringLen(ackMsgNo) // {MM} + AA
		p.MessageText = trimTrailer(body, removeLen)
		p.MsgNo = m[1]
		if ackMsgNo != "" {
			p.AckMsgNo = ackMsgNo
		}

	// Old message format with trailing {msgNo
	case matchN(reMsgNo, body, 2):
		m := reMsgNo.FindStringSubmatch(body)
		removeLen := 1 + utils.StringLen(m[1]) // { + msgNo
		p.MessageText = trimTrailer(body, removeLen)
		p.MsgNo = m[1]

	// Plain message text (no message number).
	default:
		p.MessageText = strings.Trim(body, " ")
	}
}

// trimTrailer removes the trailing removeLen runes (the message-number
// trailer) from body and trims surrounding spaces. If body is shorter than
// removeLen it is returned trimmed unchanged.
func trimTrailer(body string, removeLen int) string {
	if utils.StringLen(body) < removeLen {
		return strings.Trim(body, " ")
	}
	return strings.Trim(string([]rune(body)[:utils.StringLen(body)-removeLen]), " ")
}
