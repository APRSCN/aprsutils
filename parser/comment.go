package parser

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/APRSCN/aprsutils"
)

// parseComment parses comment from APRS packet
func (p *Parsed) parseComment(body string) string {
	body = p.parseDataExtensions(body)

	body = p.parseCommentAltitude(body)

	body = p.parseCommentTelemetry(body)

	body = p.parseDAO(body)

	if len(body) > 0 && body[0] == '/' {
		body = body[1:]
	}

	p.Comment = strings.Trim(body, " ")
	return body
}

// parseDataExtensions parses data extensions from APRS packet
func (p *Parsed) parseDataExtensions(body string) string {
	// Course speed bearing nrq
	// Page 27 of the spec
	// Format: 111/222/333/444text
	pattern1 := `^([0-9 \.]{3})/([0-9 \.]{3})`
	re1 := regexp.MustCompile(pattern1)
	matches := re1.FindStringSubmatch(body)

	if matches != nil && len(matches) >= 3 {
		cse, spd := matches[1], matches[2]
		body = body[7:]

		if isDigit(cse) && cse != "000" {
			cseInt, _ := strconv.Atoi(cse)
			if cseInt >= 1 && cseInt <= 360 {
				p.Course = float64(cseInt)
			} else {
				p.Course = 0
			}
		}

		if isDigit(spd) && spd != "000" {
			spdInt, _ := strconv.Atoi(spd)
			p.Speed = float64(spdInt) * 1.852
		}

		// DF Report format
		// Page 29 of teh spec
		pattern2 := `^/([0-9 \.]{3})/([0-9 \.]{3})`
		re2 := regexp.MustCompile(pattern2)
		matches2 := re2.FindStringSubmatch(body)

		if matches2 != nil && len(matches2) >= 3 {
			// cse=000 means stations is fixed, Page 29 of the spec
			if cse == "000" {
				p.Course = 0
			}

			brg, nrq := matches2[1], matches2[2]
			body = body[8:]

			if isDigit(brg) {
				brgInt, _ := strconv.Atoi(brg)
				p.Bearing = brgInt
			}

			if isDigit(nrq) {
				nrqInt, _ := strconv.Atoi(nrq)
				p.NRQ = nrqInt
			}
		}
	} else {
		// PHG format: PHGabcd....
		// RHGR format: RHGabcdr/....
		pattern3 := `^(PHG(\d[\x30-\x7e]\d\d)([0-9A-Z]\/)?)`
		re3 := regexp.MustCompile(pattern3)
		matches3 := re3.FindStringSubmatch(body)

		if matches3 != nil && len(matches3) >= 4 {
			ext, phg, phgr := matches3[1], matches3[2], matches3[3]
			body = body[len(ext):]

			power, _ := strconv.Atoi(string(phg[0]))
			phgPower := math.Pow(float64(power), 2)

			height := (10 * math.Pow(2, float64(int(phg[1])-0x30))) * 0.3048

			gain, _ := strconv.Atoi(string(phg[2]))
			phgGain := math.Pow(10, float64(gain)/10.0)

			p.PHG = phg
			p.PHGPower = phgPower
			p.PHGHeight = height
			p.PHGGain = phgGain

			phgDir, _ := strconv.Atoi(string(phg[3]))
			var direction string
			if phgDir == 0 {
				direction = "omni"
			} else if phgDir == 9 {
				direction = "invalid"
			} else {
				direction = strconv.Itoa(45 * phgDir)
			}
			p.PHGDir = direction

			phgRange := math.Sqrt(2*(height/0.3048)*
				math.Sqrt((phgPower/10.0)*
					(phgGain/2.0))) * 1.60934
			p.PHGRange = phgRange

			if phgr != "" {
				p.PHG = phg + string(phgr[0])
				rate, _ := strconv.ParseInt(string(phgr[0]), 16, 64)
				p.PHGRate = int(rate)
			}
		} else {
			pattern4 := `^RNG(\d{4})`
			re4 := regexp.MustCompile(pattern4)
			matches4 := re4.FindStringSubmatch(body)

			if matches4 != nil && len(matches4) >= 2 {
				rng := matches4[1]
				body = body[7:]
				rngInt, _ := strconv.Atoi(rng)
				p.RNG = float64(rngInt) * 1.609344
			}
		}
	}

	return body
}

// parseCommentAltitude parses comment altitude from APRS packet
func (p *Parsed) parseCommentAltitude(body string) string {
	pattern := `^(.*?)/A=(\-\d{5}|\d{6})(.*)$`
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(body)

	if matches != nil && len(matches) >= 4 {
		body = matches[1] + matches[3]
		altitude, _ := strconv.Atoi(matches[2])
		p.Altitude = float64(altitude) * 0.3048
	}

	return body
}

// parseDAO parses DAO from APRS packet
func (p *Parsed) parseDAO(body string) string {
	pattern := `^(.*)\!([\x21-\x7b])([\x20-\x7b]{2})\!(.*?)$`
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(body)

	if matches != nil && len(matches) >= 5 {
		body, daobyte, dao, rest := matches[1], matches[2], matches[3], matches[4]
		body += rest

		p.DAODatumByte = strings.ToUpper(daobyte)
		latOffset, lonOffset := 0.0, 0.0

		if daobyte == "W" && isDigit(dao) {
			dao0, _ := strconv.Atoi(string(dao[0]))
			dao1, _ := strconv.Atoi(string(dao[1]))
			latOffset = float64(dao0) * 0.001 / 60
			lonOffset = float64(dao1) * 0.001 / 60
		} else if daobyte == "w" && !strings.Contains(dao, " ") {
			latBase91, _ := aprsutils.ToDecimal(string(dao[0]))
			lonBase91, _ := aprsutils.ToDecimal(string(dao[1]))
			latOffset = (float64(latBase91) / 91.0) * 0.01 / 60
			lonOffset = (float64(lonBase91) / 91.0) * 0.01 / 60
		}

		if p.Lat >= 0 {
			p.Lat = p.Lat + latOffset
		} else {
			p.Lat = p.Lat - latOffset
		}

		if p.Lon >= 0 {
			p.Lon = p.Lon + lonOffset
		} else {
			p.Lon = p.Lon - lonOffset
		}
	}

	return body
}
