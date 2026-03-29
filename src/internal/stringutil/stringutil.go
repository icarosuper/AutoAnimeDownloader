package stringutil

import (
	"regexp"
	"strings"
)

var specialCharsPattern = regexp.MustCompile(`[^a-z0-9\s]`)

// RemoveSpecialCharacters normalizes a string to lowercase with only letters, digits, and spaces.
func RemoveSpecialCharacters(s string) string {
	s = strings.ToLower(s)
	s = specialCharsPattern.ReplaceAllString(s, "")
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimSpace(s)
	return s
}
