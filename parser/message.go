package parser

import (
	"regexp"
	"strings"
)

// parseMessage parses message from APRS packet
func (p *Parsed) parseMessage(body string) string {
	for {
		re1 := regexp.MustCompile(`(?i)^BLN([0-9])([a-z0-9_ \-]{5}):(.{0,67})`)
		matches1 := re1.FindStringSubmatch(body)
		if matches1 != nil && len(matches1) >= 4 {
			bid, identifier, text := matches1[1], matches1[2], matches1[3]
			identifier = strings.TrimRight(identifier, " ")

			mformat := "bulletin"
			if identifier != "" {
				mformat = "group-bulletin"
			}

			p.Format = mformat
			p.MessageText = strings.Trim(text, " ")
			p.BID = bid
			p.Identifier = identifier
			break
		}

		re2 := regexp.MustCompile(`^BLN([A-Z])([a-zA-Z0-9_ \-]{5}):(.{0,67})`)
		matches2 := re2.FindStringSubmatch(body)
		if matches2 != nil && len(matches2) >= 4 {
			aid, identifier, text := matches2[1], matches2[2], matches2[3]
			identifier = strings.TrimRight(identifier, " ")

			p.Format = "announcement"
			p.MessageText = strings.Trim(text, " ")
			p.AID = aid
			p.Identifier = identifier
			break
		}

		re3 := regexp.MustCompile(`^([a-zA-Z0-9_ \-]{9}):(.*)$`)
		matches3 := re3.FindStringSubmatch(body)
		if matches3 == nil || len(matches3) < 3 {
			break
		}

		addressee, remainingBody := matches3[1], matches3[2]
		p.Addressee = strings.TrimRight(addressee, " ")
		body = remainingBody

		remainingBody, _ = p.parseTelemetryConfig(body)

		p.Format = "message"

		/*
		 APRS supports two different message formats:
		 - the standard format which is described in 'aprs101.pdf':
		   http://www.aprs.org/doc/APRS101.PDF
		 - an addendum from 1999 which introduces a new format:
		   http://www.aprs.org/aprs11/replyacks.txt

		 A message (ack/rej as well as a standard msg text body) can either have:
		 - no message number at all
		 - a message number in the old format (1..5 characters / digits)
		 - a message number in the new format (2 characters / digits) without trailing 'ack msg no'
		 - a message number in the new format with trailing 'free ack msg no' (2 characters / digits)
		*/

		// ack / rej
		// ---------------------------
		// NEW REPLAY-ACK
		// Format: :AAAABBBBC:ackMM}AA
		re4 := regexp.MustCompile(`^(ack|rej)([A-Za-z0-9]{2})}([A-Za-z0-9]{2})?$`)
		matches4 := re4.FindStringSubmatch(body)
		if matches4 != nil && len(matches4) >= 3 {
			p.Response = matches4[1]
			p.MsgNo = matches4[2]
			if len(matches4) >= 4 && matches4[3] != "" {
				p.AckMsgNo = matches4[3]
			}
			break
		}

		// ack/rej standard format as per aprs101.pdf chapter 14
		// Format: :AAAABBBBC:ack12345
		re5 := regexp.MustCompile(`^(ack|rej)([A-Za-z0-9]{1,5})$`)
		matches5 := re5.FindStringSubmatch(body)
		if matches5 != nil && len(matches5) >= 3 {
			p.Response = matches5[1]
			p.MsgNo = matches5[2]
			break
		}

		// Regular message body parser
		// ---------------------------
		p.MessageText = strings.Trim(body, " ")

		// Check for ACKs
		// New message format: http://www.aprs.org/aprs11/replyacks.txt
		// Format: :AAAABBBBC:text.....{MM}AA
		re6 := regexp.MustCompile(`{([A-Za-z0-9]{2})}([A-Za-z0-9]{2})?$`)
		matches6 := re6.FindStringSubmatch(body)
		if matches6 != nil && len(matches6) >= 2 {
			msgNo := matches6[1]
			ackMsgNo := ""
			if len(matches6) >= 3 {
				ackMsgNo = matches6[2]
			}

			removeLen := 4 + len(ackMsgNo) // {MM} + AA
			if len(body) >= removeLen {
				p.MessageText = strings.Trim(body[:len(body)-removeLen], " ")
			}
			p.MsgNo = msgNo
			if ackMsgNo != "" {
				p.AckMsgNo = ackMsgNo
			}
			break
		}

		// Old message format - see aprs101.pdf.
		// Search for: msgNo present
		re7 := regexp.MustCompile(`{([A-Za-z0-9]{1,5})$`)
		matches7 := re7.FindStringSubmatch(body)
		if matches7 != nil && len(matches7) >= 2 {
			msgNo := matches7[1]
			removeLen := 1 + len(msgNo) // { + msgNo
			if len(body) >= removeLen {
				p.MessageText = strings.Trim(body[:len(body)-removeLen], " ")
			}
			p.MsgNo = msgNo
			break
		}

		break
	}

	return ""
}
