package parser

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ghinknet/regexp"

	"github.com/APRSCN/aprsutils/utils"
)

// config provides parser config options
type config struct {
	disableToCallsignValidate bool
}

// Option provides a basic option type
type Option func(*config)

// WithDisableToCallsignValidate disables to callsign validate
func WithDisableToCallsignValidate() Option {
	return func(p *config) {
		p.disableToCallsignValidate = true
	}
}

func Parse(packet string, options ...Option) (Parsed, error) {
	// Create config
	conf := &config{
		disableToCallsignValidate: false,
	}

	// Apply options
	for _, opt := range options {
		opt(conf)
	}

	// Create result
	parsed := &Parsed{}

	// Save raw packet
	parsed.Raw = packet

	// Check packet content
	if packet == "" {
		return *parsed, errors.New("packet is empty")
	}

	// Trim
	packet = strings.Trim(packet, "\r\n")

	// Split head and body
	head, body, ok := utils.SplitOnce(packet, ":")
	if !ok {
		return *parsed, errors.New("packet has no body")
	}

	// Check body
	if utils.StringLen(head) == 0 || utils.StringLen(body) == 0 {
		return *parsed, errors.New("packet head or body is empty")
	}

	// Parse head
	err := parsed.parseHeader(head, conf)
	if err != nil {
		return *parsed, err
	}

	// Parse body
	err = parsed.parseBody(body)
	if err != nil {
		return *parsed, err
	}

	return *parsed, nil
}

// parseTimeStamp parses timestamp from APRS packet
func (p *Parsed) parseTimeStamp(packetType string, body string) (string, error) {
	// Check body length
	if utils.StringLen(body) < 7 {
		return body, errors.New("invalid timestamp format")
	}
	// Match
	matches := regexp.MustCompile(`^((\d{6})(.))$`).FindStringSubmatch(string([]rune(body)[0:7]))
	if matches == nil || len(matches) < 4 {
		return body, nil
	}

	rawts, ts, form := matches[1], matches[2], matches[3]
	utc := time.Now().UTC()
	timestamp := 0

	if !(packetType == ">" && form != "z") {
		body = string([]rune(body)[7:])

		var timeStr string
		var err error

		switch form {
		case "h":
			// Zulu hhmmss format
			timeStr = fmt.Sprintf("%d%02d%02d%s", utc.Year(), utc.Month(), utc.Day(), ts)
			timestamp, err = parseTimeString(timeStr, "20060102150405")
		case "z", "/":
			// Zulu ddhhmm format
			// '/' local ddhhmm format
			timeStr = fmt.Sprintf("%d%02d%s%02d", utc.Year(), utc.Month(), ts, 0)
			timestamp, err = parseTimeString(timeStr, "20060102150405")
		default:
			timestamp = 0
		}

		if err != nil {
			timestamp = 0
		}
	}

	p.RawTimestamp = rawts
	p.Timestamp = timestamp

	return body, nil
}

func parseTimeString(timeStr, layout string) (int, error) {
	t, err := time.Parse(layout, timeStr)
	if err != nil {
		return 0, err
	}
	return int(t.Unix()), nil
}
