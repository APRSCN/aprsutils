package aprsutils

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// ToDecimal transfers Base91 to decimal
func ToDecimal(text string) (int, error) {
	if text == "" {
		return 0, nil
	}

	text = strings.TrimLeft(text, "!")
	result := 0

	for i := 0; i < len(text); i++ {
		char := text[i]
		if char <= 0x20 || char >= 0x7c {
			return 0, errors.New("invalid character in sequence")
		}

		value := int(char) - 33
		for j := 0; j < len(text)-1-i; j++ {
			value *= 91
		}
		result += value
	}

	return result, nil
}

// FromDecimal transfer decimal to Base91
func FromDecimal(number int, width ...int) (string, error) {
	w := 1
	if len(width) > 0 {
		w = width[0]
		if w < 0 {
			return "", errors.New("width must be non-negative")
		}
	}

	if number < 0 {
		return "", errors.New("expected number to be positive integer")
	}

	if number == 0 {
		return strings.Repeat("!", max(1, w)), nil
	}

	var builder strings.Builder
	temp := number

	for temp > 0 {
		remainder := temp % 91
		temp = temp / 91
		builder.WriteRune(rune(33 + remainder))
	}

	runes := []rune(builder.String())
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}

	result := string(runes)
	result = strings.TrimLeft(result, "!")

	if utf8.RuneCountInString(result) < w {
		result = strings.Repeat("!", w-utf8.RuneCountInString(result)) + result
	}

	return result, nil
}
