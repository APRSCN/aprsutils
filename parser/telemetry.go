package parser

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/APRSCN/aprsutils"
)

// TelemetryData is the struct for telemetry data
type TelemetryData struct {
	Seq  int
	Vals []int
	Bits string
}

// parseCommentTelemetry parses comment telemetry from APRS packet
func (p *Parsed) parseCommentTelemetry(text string) string {
	pattern := `^(.*?)\|([!-{]{4,14})\|(.*)$`
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(text)

	if matches != nil && len(matches) >= 4 && len(matches[2])%2 == 0 {
		text, telemetry, post := matches[1], matches[2], matches[3]
		text += post

		temp := make([]int, 7)
		for i := 0; i < 7 && i*2+2 <= len(telemetry); i++ {
			temp[i], _ = aprsutils.ToDecimal(telemetry[i*2 : i*2+2])
		}

		telemetryData := TelemetryData{
			Seq:  temp[0],
			Vals: temp[1:6],
		}

		if temp[6] != 0 {
			bits := temp[6] & 0xFF
			binaryStr := ""
			for i := 0; i < 8; i++ {
				if bits&(1<<uint(i)) != 0 {
					binaryStr += "1"
				} else {
					binaryStr += "0"
				}
			}
			telemetryData.Bits = binaryStr
		}

		p.Telemetry = telemetryData
	}

	return text
}

// parseTelemetryConfig parses telemetry config from APRS packet
func (p *Parsed) parseTelemetryConfig(body string) (string, error) {
	pattern := `^(PARM|UNIT|EQNS|BITS)\.(.*)$`
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(body)

	if matches != nil && len(matches) >= 3 {
		form, body := matches[1], matches[2]

		p.Format = "telemetry-message"

		switch form {
		case "PARM", "UNIT":
			vals := strings.Split(strings.TrimRight(body, " "), ",")
			if len(vals) > 13 {
				vals = vals[:13]
			}

			for _, val := range vals {
				if len(val) > 20 {
					return body, errors.New("incorrect format of " + form + " (name too long?)")
				}
			}

			defvals := make([]string, 13)
			for i := range defvals {
				if i < len(vals) {
					defvals[i] = vals[i]
				} else {
					defvals[i] = ""
				}
			}

			if form == "PARM" {
				p.TPARM = defvals
			} else {
				p.TUNIT = defvals
			}
		case "EQNS":
			eqns := strings.Split(strings.TrimRight(body, " "), ",")
			if len(eqns) > 15 {
				eqns = eqns[:15]
			}

			teqns := make([]float64, 15)
			for i := 0; i < 5; i++ {
				teqns[i*3] = 0.0
				teqns[i*3+1] = 1.0
				teqns[i*3+2] = 0.0
			}

			for idx, val := range eqns {
				if val == "" {
					continue
				}

				if !regexp.MustCompile(`^[-]?\d*\.?\d+$`).MatchString(val) {
					return body, errors.New("value at " + strconv.Itoa(idx+1) + " is not a number in " + form)
				}

				if intVal, err := strconv.Atoi(val); err == nil {
					teqns[idx] = float64(intVal)
				} else {
					floatVal, err := strconv.ParseFloat(val, 64)
					if err != nil {
						teqns[idx] = 0.0
					} else {
						teqns[idx] = floatVal
					}
				}
			}

			groupedEqns := make([][]float64, 5)
			for i := 0; i < 5; i++ {
				groupedEqns[i] = teqns[i*3 : (i+1)*3]
			}

			p.TEQNS = groupedEqns

		case "BITS":
			pattern := `^([01]{8}),(.{0,23})$`
			re := regexp.MustCompile(pattern)
			matches := re.FindStringSubmatch(strings.TrimRight(body, " "))
			if matches == nil || len(matches) < 3 {
				return body, errors.New("incorrect format of " + form + " (title too long?)")
			}

			bits, title := matches[1], matches[2]
			p.TBITS = bits
			p.Title = strings.Trim(title, " ")
		}
	}

	return body, nil
}
