package parser

import (
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/APRSCN/aprsutils"
	"github.com/APRSCN/aprsutils/utils"
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

	if utils.StringLen(dstCall) != 6 {
		return "", errors.New("dstCall has to be 6 characters")
	}
	if utils.StringLen(body) < 8 {
		return "", errors.New("packet data field is too short")
	}

	re1 := aprsutils.CompiledRegexps.Get(`^[0-9A-Z]{3}[0-9L-Z]{3}$`)
	if !re1.MatchString(dstCall) {
		return "", errors.New("invalid dstCall")
	}

	re2 := aprsutils.CompiledRegexps.Get(`^[&-\x7f][&-a][\x1c-\x7f]{2}[\x1c-\x7d][\x1c-\x7f][\x21-\x7e][/\\0-9A-Z]`)
	if !re2.MatchString(body) {
		return "", errors.New("invalid data format")
	}

	p.Symbol = []string{string([]rune(body)[6]), string([]rune(body)[7])}

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
	re3 := aprsutils.CompiledRegexps.Get(`^\d+( *)$`)
	matches := re3.FindStringSubmatch(tempDstCall)
	if matches == nil {
		return "", errors.New("invalid latitude ambiguity")
	}

	posAmbiguity := utils.StringLen(matches[1])
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
	latMinutesStr := strings.ReplaceAll(string([]rune(tempDstCall)[2:4])+"."+string([]rune(tempDstCall)[4:6]), " ", "0")
	latMinutes, err := strconv.ParseFloat(latMinutesStr, 64)
	if err != nil {
		return "", errors.New("invalid latitude minutes format")
	}

	latDegrees, _ := strconv.Atoi(string([]rune(tempDstCall)[0:2]))
	latitude := float64(latDegrees) + (latMinutes / 60.0)

	// Determine the sign N/S
	if []rune(dstCall)[3] <= 0x4c {
		latitude = -latitude
	}

	p.Lat = latitude

	// Parse message bits
	mBits := aprsutils.CompiledRegexps.Get("[0-9L]").ReplaceAllString(string([]rune(dstCall)[0:3]), "0")
	mBits = aprsutils.CompiledRegexps.Get("[P-Z]").ReplaceAllString(mBits, "1")
	mBits = aprsutils.CompiledRegexps.Get("[A-K]").ReplaceAllString(mBits, "2")

	p.MBits = mBits

	// Resolve message type
	if strings.Contains(mBits, "2") {
		mTypeKey := strings.ReplaceAll(mBits, "2", "1")
		p.MType = MtypeTableCustom[mTypeKey]
	} else {
		p.MType = MtypeTableStd[mBits]
	}

	// Parse longitude
	lonF64, _ := strconv.ParseFloat(string([]rune(body)[0]), 64)
	longitude := lonF64 - 28
	if []rune(dstCall)[4] >= 0x50 {
		longitude += 100
	}
	if longitude >= 180 && longitude <= 189 {
		longitude -= 80
	} else if longitude >= 190 && longitude <= 199 {
		longitude -= 190
	}

	// Long minutes
	lngF641, _ := strconv.ParseFloat(string([]rune(body)[1]), 64)
	lngMinutes := lngF641 - 28.0
	if lngMinutes >= 60 {
		lngMinutes -= 60
	}

	// + (long hundredths of minutes)
	lngF642, _ := strconv.ParseFloat(string([]rune(body)[2]), 64)
	lngMinutes += (lngF642 - 28.0) / 100.0

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
	if []rune(dstCall)[5] >= 0x50 {
		longitude = -longitude
	}

	p.Lon = longitude

	// Parse speed and course
	speedF64, _ := strconv.ParseFloat(string([]rune(body)[3]), 64)
	speed := (speedF64 - 28) * 10
	courseF644, _ := strconv.ParseFloat(string([]rune(body)[4]), 64)
	course := courseF644 - 28
	quotient := int(course / 10.0)
	course -= float64(quotient * 10)
	courseF645, _ := strconv.ParseFloat(string([]rune(body)[5]), 64)
	course = course*100 + courseF645 - 28
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

	if utils.StringLen(body) > 8 {
		body = string([]rune(body)[8:])

		// Check for optional 2 or 5 channel telemetry
		re4 := aprsutils.CompiledRegexps.Get(`^('[0-9a-f]{10}|` + "`" + `[0-9a-f]{4})(.*)$`)
		matches := re4.FindStringSubmatch(body)
		if matches != nil && len(matches) >= 3 {
			hexData, remainingBody := matches[1], matches[2]
			hexData = string([]rune(hexData)[1:])

			channels := utils.StringLen(hexData) / 2

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

		re5 := aprsutils.CompiledRegexps.Get(`^(.*)([!-{]{3})}(.*)$`)
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
