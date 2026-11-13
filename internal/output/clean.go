package output

import (
	"strings"
	"unicode"
)

// Clean makes text that somebody else wrote (an error message from the API,
// a file name in a job, text read off a photo) safe to print on a terminal,
// which acts on the escape sequences in what it shows: it removes the escape
// character and every other control character except newline and tab.
func Clean(s string) string {
	clean := true
	for _, r := range s {
		if isControl(r) {
			clean = false
			break
		}
	}
	if clean {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if !isControl(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isControl(r rune) bool {
	if r == '\n' || r == '\t' {
		return false
	}
	// C0, DEL and C1 controls, and the invalid-UTF-8 replacement is kept.
	return unicode.IsControl(r)
}

// OneLine is Clean with line breaks and tabs turned into spaces, for a cell
// of a table.
func OneLine(s string) string {
	return strings.NewReplacer("\n", " ", "\t", " ").Replace(Clean(s))
}
