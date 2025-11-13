package parser

import (
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/APRSCN/aprsutils"
)

var MtypeTableStd = map[string]string{
	"111": "M0: Off Duty",
	"110": "M1: En Route",
	"101": "M2: In Service",
	"100": "M3: Returning",
	"011": "M4: Committed",
	"010": "M5: Special",
	"001": "M6: Priority",
	"000": "Emergency",
}

var MtypeTableCustom = map[string]string{
	"111": "C0: Custom-0",
	"110": "C1: Custom-1",
	"101": "C2: Custom-2",
	"100": "C3: Custom-3",
	"011": "C4: Custom-4",
	"010": "C5: Custom-5",
	"001": "C6: Custom-6",
	"000": "Emergency",
}

// parseMicE parses MIC-E data from APRS packet
func (p *Parsed) parseMicE(dstCall string, body string) (string, error) {
	p.Format = "mic-e"

	parts := strings.Split(dstCall, "-")
	dstCall = parts[0]

	if len(dstCall) != 6 {
		return "", errors.New("dstCall has to be 6 characters")
	}
	if len(body) < 8 {
		return "", errors.New("packet data field is too short")
	}

	re1 := regexp.MustCompile(`^[0-9A-Z]{3}[0-9L-Z]{3}$`)
	if !re1.MatchString(dstCall) {
		return "", errors.New("invalid dstCall")
	}

	re2 := regexp.MustCompile(`^[&-\x7f][&-a][\x1c-\x7f]{2}[\x1c-\x7d][\x1c-\x7f][\x21-\x7e][/\\0-9A-Z]`)
	if !re2.MatchString(body) {
		return "", errors.New("invalid data format")
	}

	p.Symbol = []string{string(body[6]), string(body[7])}

	// Parse latitude
	// The routine translates each character into a lat digit as described in
	// 'Mic-E Destination Address Field Encoding' table
	tempDstCall := ""
	for _, i := range dstCall {
		c := byte(i)
		if c == 'K' || c == 'L' || c == 'Z' { // 空格
			tempDstCall += " "
		} else if c > 76 { // P-Y
			tempDstCall += string(c - 32)
		} else if c > 57 { // A-J
			tempDstCall += string(c - 17)
		} else { // 0-9
			tempDstCall += string(c)
		}
	}

	// Determine position ambiguity
	re3 := regexp.MustCompile(`^\d+( *)$`)
	matches := re3.FindStringSubmatch(tempDstCall)
	if matches == nil {
		return "", errors.New("invalid latitude ambiguity")
	}

	posAmbiguity := len(matches[1])
	p.PosAmbiguity = posAmbiguity

	tempDstCallRunes := []rune(tempDstCall)
	if posAmbiguity > 0 {
		if posAmbiguity >= 4 {
			tempDstCallRunes[2] = '3'
		} else {
			tempDstCallRunes[6-posAmbiguity] = '5'
		}
	}
	tempDstCall = string(tempDstCallRunes)

	// Adjust the coordinates be in center of ambiguity box
	latMinutesStr := strings.ReplaceAll(tempDstCall[2:4]+"."+tempDstCall[4:6], " ", "0")
	latMinutes, err := strconv.ParseFloat(latMinutesStr, 64)
	if err != nil {
		return "", errors.New("invalid latitude minutes format")
	}

	latDegrees, _ := strconv.Atoi(tempDstCall[0:2])
	latitude := float64(latDegrees) + (latMinutes / 60.0)

	// Determine the sign N/S
	if dstCall[3] <= 0x4c {
		latitude = -latitude
	}

	p.Lat = latitude

	// Parse message bits
	mBits := regexp.MustCompile("[0-9L]").ReplaceAllString(dstCall[0:3], "0")
	mBits = regexp.MustCompile("[P-Z]").ReplaceAllString(mBits, "1")
	mBits = regexp.MustCompile("[A-K]").ReplaceAllString(mBits, "2")

	p.MBits = mBits

	// Resolve message type
	if strings.Contains(mBits, "2") {
		mTypeKey := strings.ReplaceAll(mBits, "2", "1")
		p.MType = MtypeTableCustom[mTypeKey]
	} else {
		p.MType = MtypeTableStd[mBits]
	}

	// Parse longitude
	longitude := float64(body[0]) - 28
	if dstCall[4] >= 0x50 {
		longitude += 100
	}
	if longitude >= 180 && longitude <= 189 {
		longitude -= 80
	} else if longitude >= 190 && longitude <= 199 {
		longitude -= 190
	}

	// Long minutes
	lngMinutes := float64(body[1]) - 28.0
	if lngMinutes >= 60 {
		lngMinutes -= 60
	}

	// + (long hundredths of minutes)
	lngMinutes += (float64(body[2]) - 28.0) / 100.0

	// Apply position ambiguity
	// Routines adjust longitude to center of the ambiguity box
	if posAmbiguity == 4 {
		lngMinutes = 30
	} else if posAmbiguity == 3 {
		lngMinutes = (math.Floor(lngMinutes/10) + 0.5) * 10
	} else if posAmbiguity == 2 {
		lngMinutes = math.Floor(lngMinutes) + 0.5
	} else if posAmbiguity == 1 {
		lngMinutes = (math.Floor(lngMinutes*10) + 0.5) / 10.0
	} else if posAmbiguity != 0 {
		return "", errors.New("Unsupported position ambiguity: " + strconv.Itoa(posAmbiguity))
	}

	longitude += lngMinutes / 60.0

	// Apply E/W sign
	if dstCall[5] >= 0x50 {
		longitude = -longitude
	}

	p.Lon = longitude

	// Parse speed and course
	speed := (float64(body[3]) - 28) * 10
	course := float64(body[4]) - 28
	quotient := int(course / 10.0)
	course -= float64(quotient * 10)
	course = course*100 + float64(body[5]) - 28
	speed += float64(quotient)

	if speed >= 800 {
		speed -= 800
	}
	if course >= 400 {
		course -= 400
	}

	speed *= 1.852
	p.Speed = speed
	p.Course = course

	if len(body) > 8 {
		body = body[8:]

		// Check for optional 2 or 5 channel telemetry
		re4 := regexp.MustCompile(`^('[0-9a-f]{10}|` + "`" + `[0-9a-f]{4})(.*)$`)
		matches := re4.FindStringSubmatch(body)
		if matches != nil && len(matches) >= 3 {
			hexData, remainingBody := matches[1], matches[2]
			hexData = hexData[1:]

			channels := len(hexData) / 2

			hexInt, err := strconv.ParseInt(hexData, 16, 64)
			if err != nil {
				return "", errors.New("invalid telemetry hex data")
			}

			telemetry := make([]int, channels)
			for i := 0; i < channels; i++ {
				telemetry[channels-1-i] = int(hexInt >> uint(8*i) & 255)
			}

			p.TelemetryMicE = telemetry
			body = remainingBody
		}

		re5 := regexp.MustCompile(`^(.*)([!-{]{3})}(.*)$`)
		matches = re5.FindStringSubmatch(body)
		if matches != nil && len(matches) >= 4 {
			bodyPart, altitude, extra := matches[1], matches[2], matches[3]
			altitudeBase91, err := aprsutils.ToDecimal(altitude)
			if err != nil {
				return "", err
			}
			altitudeValue := altitudeBase91 - 10000
			p.Altitude = float64(altitudeValue)
			body = bodyPart + extra
		}

		body = p.parseCommentTelemetry(body)

		body = p.parseDAO(body)

		p.Comment = strings.Trim(body, " ")
	}

	return "", nil
}
