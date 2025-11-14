package parser

import (
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/APRSCN/aprsutils"
	"github.com/APRSCN/aprsutils/utils"
)

// parsePosition parses position format APRS packet
func (p *Parsed) parsePosition(packetType string, body string) error {
	// Check format
	if !strings.Contains("!=/@;", packetType) {
		packetType = "!"
		_, body, _ = utils.SplitOnce(body, "!")
	}

	// Attempt to parse object report format
	if packetType == ";" {
		matches := aprsutils.CompiledRegexps.Get(`^([ -~]{9})(\*|_)`).FindStringSubmatch(body)
		if matches != nil && len(matches) >= 3 {
			name := matches[1]
			flag := matches[2]

			p.ObjectName = name
			p.Alive = flag == "*"

			body = string([]rune(body)[10:])
		} else {
			return errors.New("invalid format")
		}
	} else {
		p.MessageCapable = strings.Contains("@=", packetType)
	}

	// Decode timestamp
	if strings.Contains("/@;", packetType) {
		var err error
		body, err = p.parseTimeStamp(packetType, body)
		if err != nil {
			return err
		}
	}
	if utils.StringLen(body) == 0 && p.Timestamp != 0 {
		return errors.New("invalid timestamp format")
	}

	// Decode body
	var err error
	if aprsutils.CompiledRegexps.Get(`^[0-9\s]{4}\.[0-9\s]{2}[NS].[0-9\s]{5}\.[0-9\s]{2}[EW]`).MatchString(body) {
		body, err = p.parseNormal(body)
		if err != nil {
			return err
		}
	} else {
		body, err = p.parseCompressed(body)
		if err != nil {
			return err
		}
	}

	// Check for weather info
	if p.Symbol[0] == "_" {
		// Attempt to parse winddir/speed
		// Page 92 of the spec
		body = p.parseDataExtensions(body)

		body = p.parseWeatherData(body)
	} else {
		body = p.parseComment(body)
	}

	// Object
	if packetType == ";" {
		p.ObjectFormat = p.Format
		p.Format = "object"
	}

	return nil
}

// parseCompressed parses compressed APRS packet
func (p *Parsed) parseCompressed(body string) (string, error) {
	// Attempt to parse as compressed position report
	// Check length
	if len(body) < 13 {
		return body, errors.New("invalid compressed format")
	}

	// Set format
	p.Format = "compressed"

	compressed := string([]rune(body)[:13])
	body = string([]rune(body)[13:])

	symbolTable := string([]rune(compressed)[0])
	symbol := string([]rune(compressed)[9])

	base91Lat, err := aprsutils.ToDecimal(string([]rune(compressed)[1:5]))
	if err != nil {
		return body, err
	}
	base91Lon, err := aprsutils.ToDecimal(string([]rune(compressed)[5:9]))
	if err != nil {
		return body, err
	}

	latitude := 90 - (float64(base91Lat) / 380926)
	longitude := -180 + (float64(base91Lon) / 190463)

	c1 := int(compressed[10]) - 33
	s1 := int(compressed[11]) - 33
	ctype := int(compressed[12]) - 33

	if c1 == -1 {
		if ctype&0x20 == 0x20 {
			p.GPSFixStatus = true
		} else {
			p.GPSFixStatus = false
		}
	}

	if c1 == -1 || s1 == -1 {
		// Do nothing
	} else if ctype&0x18 == 0x10 {
		p.Altitude = math.Pow(1.002, float64(c1*91+s1)) * 0.3048
	} else if c1 >= 0 && c1 <= 89 {
		course := 360
		if c1 != 0 {
			course = c1 * 4
		}
		speed := (math.Pow(1.08, float64(s1)) - 1) * 1.852 // From knts To kph

		p.Course = float64(course)
		p.Speed = speed
	} else if c1 == 90 {
		p.RadioRange = (2 * math.Pow(1.08, float64(s1))) * 1.609344
	}

	p.Symbol = []string{symbol, symbolTable}
	p.Lon = longitude
	p.Lat = latitude
	
	return body, nil
}

// parseNormal parses normal APRS packet
func (p *Parsed) parseNormal(body string) (string, error) {
	pattern := `^(\d{2})([0-9 ]{2}\.[0-9 ]{2})([NnSs])([\/\\0-9A-Z])` +
		`(\d{3})([0-9 ]{2}\.[0-9 ]{2})([EeWw])([\x21-\x7e])(.*)$`

	re := aprsutils.CompiledRegexps.Get(pattern)
	matches := re.FindStringSubmatch(body)

	if matches == nil || len(matches) < 10 {
		return body, nil
	}

	p.Format = "uncompressed"

	latDeg := matches[1]
	latMin := matches[2]
	latDir := matches[3]
	symbolTable := matches[4]
	lonDeg := matches[5]
	lonMin := matches[6]
	lonDir := matches[7]
	symbol := matches[8]
	remainingBody := matches[9]

	posAmbiguity := strings.Count(latMin, " ")
	if posAmbiguity != strings.Count(lonMin, " ") {
		return body, errors.New("latitude and longitude ambiguity mismatch")
	}
	p.PosAmbiguity = posAmbiguity

	if posAmbiguity >= 4 {
		latMin = "30"
		lonMin = "30"
	} else {
		latMin = strings.Replace(latMin, " ", "5", 1)
		lonMin = strings.Replace(lonMin, " ", "5", 1)
	}

	latDegInt, err := strconv.Atoi(latDeg)
	if err != nil {
		return body, errors.New("invalid latitude degrees")
	}
	if latDegInt > 89 || latDegInt < 0 {
		return body, errors.New("latitude is out of range (0-90 degrees)")
	}

	lonDegInt, err := strconv.Atoi(lonDeg)
	if err != nil {
		return body, errors.New("invalid longitude degrees")
	}
	if lonDegInt > 179 || lonDegInt < 0 {
		return body, errors.New("longitude is out of range (0-180 degrees)")
	}

	// From DDMM.MM to decimal
	latMinFloat, err := strconv.ParseFloat(strings.TrimSpace(latMin), 64)
	if err != nil {
		return body, errors.New("invalid latitude minutes")
	}
	latitude := float64(latDegInt) + (latMinFloat / 60.0)

	lonMinFloat, err := strconv.ParseFloat(strings.TrimSpace(lonMin), 64)
	if err != nil {
		return body, errors.New("invalid longitude minutes")
	}
	longitude := float64(lonDegInt) + (lonMinFloat / 60.0)

	if strings.Contains("Ss", string(latDir[0])) {
		latitude *= -1
	}
	if strings.Contains("Ww", string(lonDir[0])) {
		longitude *= -1
	}

	// Save result
	p.Symbol = []string{symbol, symbolTable}
	p.Lon = longitude
	p.Lat = latitude

	return remainingBody, nil
}
