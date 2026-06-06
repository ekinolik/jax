package confluence

import (
	"fmt"
	"regexp"
	"strings"
)

var tickerPattern = regexp.MustCompile(`^[A-Z0-9\-]{1,6}$`)

// NormalizeTicker uppercases and trims a ticker symbol.
func NormalizeTicker(ticker string) string {
	return strings.ToUpper(strings.TrimSpace(ticker))
}

// ValidateTicker rejects empty or path-like ticker values.
func ValidateTicker(ticker string) error {
	ticker = NormalizeTicker(ticker)
	if ticker == "" {
		return fmt.Errorf("empty ticker")
	}
	if !tickerPattern.MatchString(ticker) {
		return fmt.Errorf("invalid ticker %q", ticker)
	}
	return nil
}
