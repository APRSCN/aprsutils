package parser

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// Const
const (
	windMultiplier = 0.44704
	rainMultiplier = 0.254
)

var keyMap = map[byte]string{
	'g': "windGust",
	'c': "windDirection",
	't': "temperature",
	'S': "windSpeed",
	'r': "rain1h",
	'p': "rain24h",
	'P': "rainSinceMidnight",
	'h': "humidity",
	'b': "pressure",
	'l': "luminosity",
	'L': "luminosity",
	's': "snow",
	'#': "rainRaw",
}

var valMap = map[byte]func(string) float64{
	'g': func(x string) float64 {
		val, _ := strconv.Atoi(x)
		return float64(val) * windMultiplier
	},
	'c': func(x string) float64 {
		val, _ := strconv.Atoi(x)
		return float64(val)
	},
	'S': func(x string) float64 {
		val, _ := strconv.Atoi(x)
		return float64(val) * windMultiplier
	},
	't': func(x string) float64 {
		val, _ := strconv.ParseFloat(x, 64)
		return (val - 32) / 1.8
	},
	'r': func(x string) float64 {
		val, _ := strconv.Atoi(x)
		return float64(val) * rainMultiplier
	},
	'p': func(x string) float64 {
		val, _ := strconv.Atoi(x)
		return float64(val) * rainMultiplier
	},
	'P': func(x string) float64 {
		val, _ := strconv.Atoi(x)
		return float64(val) * rainMultiplier
	},
	'h': func(x string) float64 {
		val, _ := strconv.Atoi(x)
		if val == 0 {
			return 100
		}
		return float64(val)
	},
	'b': func(x string) float64 {
		val, _ := strconv.ParseFloat(x, 64)
		return val / 10
	},
	'l': func(x string) float64 {
		val, _ := strconv.Atoi(x)
		return float64(val + 1000)
	},
	'L': func(x string) float64 {
		val, _ := strconv.Atoi(x)
		return float64(val)
	},
	's': func(x string) float64 {
		val, _ := strconv.ParseFloat(x, 64)
		return val * 25.4
	},
	'#': func(x string) float64 {
		val, _ := strconv.Atoi(x)
		return float64(val)
	},
}

// parseWeatherData parses weather data from APRS packet
func (p *Parsed) parseWeatherData(body string) string {
	re1 := regexp.MustCompile(`^([0-9]{3})/([0-9]{3})`)
	body = re1.ReplaceAllString(body, "c${1}s${2}")
	body = strings.Replace(body, "s", "S", 1)

	re2 := regexp.MustCompile(`^([cSgtrpPlLs#][0-9\-. ]{3}|h[0-9. ]{2}|b[0-9. ]{5})+`)
	dataMatch := re2.FindString(body)

	if dataMatch != "" {
		data := dataMatch
		body = body[len(data):]

		re3 := regexp.MustCompile(`([cSgtrpPlLs#]\d{3}|t-\d{2}|h\d{2}|b\d{5}|s\.\d{2}|s\d\.\d)`)
		matches := re3.FindAllString(data, -1)

		for _, match := range matches {
			if len(match) < 2 {
				continue
			}

			keyChar := match[0]
			valueStr := match[1:]

			valueStr = strings.ReplaceAll(valueStr, " ", "")

			if keyFunc, ok := valMap[keyChar]; ok {
				if keyName, ok := keyMap[keyChar]; ok {
					p.Weather[keyName] = keyFunc(valueStr)
				}
			}
		}
	}

	return body
}

// parseWeather parses weather data from APRS packet
func (p *Parsed) parseWeather(body string) (string, error) {
	re := regexp.MustCompile(`^(\d{8})c[. \d]{3}s[. \d]{3}g[. \d]{3}t[. \d]{3}`)
	match := re.FindStringSubmatch(body)

	if match == nil {
		return "", errors.New("invalid positionless weather report format")
	}

	comment := p.parseWeatherData(body[8:])

	p.Comment = strings.Trim(comment, " ")

	return "", nil
}
