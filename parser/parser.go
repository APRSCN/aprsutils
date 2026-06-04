package parser

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"go.gh.ink/regexp"

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
	parsed := new(Parsed)

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
	if err := parsed.parseHeader(head, conf); err != nil {
		return *parsed, err
	}

	// Parse body
	if err := parsed.parseBody(body); err != nil {
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
	local := time.Now()
	timestamp := 0

	if !(packetType == ">" && form != "z") {
		body = string([]rune(body)[7:])

		var timeStr string
		var err error

		switch form {
		case "h":
			// Zulu hhmmss format (UTC).
			timeStr = fmt.Sprintf("%d%02d%02d%s", utc.Year(), utc.Month(), utc.Day(), ts)
			timestamp, err = parseTimeStringIn(timeStr, "20060102150405", time.UTC)
		case "z":
			// Zulu ddhhmm format (UTC): ts is DDHHMM, seconds = 00.
			timeStr = fmt.Sprintf("%d%02d%s00", utc.Year(), utc.Month(), ts)
			timestamp, err = parseTimeStringIn(timeStr, "20060102150405", time.UTC)
		case "/":
			// Local ddhhmm format: interpret in the host's local timezone
			// (this is what the '/' form denotes per the APRS spec).
			timeStr = fmt.Sprintf("%d%02d%s00", local.Year(), local.Month(), ts)
			timestamp, err = parseTimeStringIn(timeStr, "20060102150405", time.Local)
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

// parseTimeStringIn parses timeStr in the given location and returns a Unix
// timestamp.
func parseTimeStringIn(timeStr, layout string, loc *time.Location) (int, error) {
	t, err := time.ParseInLocation(layout, timeStr, loc)
	if err != nil {
		return 0, err
	}
	return int(t.Unix()), nil
}
