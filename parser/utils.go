package parser

import "strings"

// SplitOnce splits strings when sep symbol first appear
func SplitOnce(s string, sep string) (string, string, bool) {
	split := strings.SplitN(s, sep, 2)
	if len(split) != 2 {
		return "", "", false
	}
	return split[0], split[1], true
}

// StringLen returns length of string
func StringLen(s string) int {
	return len([]rune(s))
}

// isDigit checks whether a string only contains numbers
func isDigit(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
