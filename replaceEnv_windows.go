//go:build windows
// +build windows

package shellwords

import (
	"bytes"
	"os"
	"unicode"
)

// replaceEnv for Windows: Treats backslash as a literal character.
func replaceEnv(getenv func(string) string, s string) string {
	if getenv == nil {
		getenv = os.Getenv
	}

	var buf bytes.Buffer
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == '$' {
			i++
			if i == len(rs) {
				buf.WriteRune(r)
				break
			}
			if rs[i] == '{' { // Changed 0x7b to '{' for clarity
				i++
				p := i
				for ; i < len(rs); i++ {
					r = rs[i]
					// Need to handle escaped '}' within variable expansion?
					// Assuming simple variable names for now.
					if r == '}' || (!unicode.IsLetter(r) && r != '_' && !unicode.IsDigit(r)) {
						break
					}
				}
				if i == p || (i < len(rs) && rs[i] != '}') { // Check if variable name is empty or closing brace is missing
					// Malformed: empty or no closing brace
					buf.WriteRune('$')
					buf.WriteRune('{')
					i = p - 1 // Reprocess chars after ${
				} else {
					// Need to handle escapes *within* the variable name before getenv?
					// Current logic passes raw name including potential escapes.
					buf.WriteString(getenv(string(rs[p:i])))
					if i < len(rs) && rs[i] != '}' {
						i-- // Reprocess breaking char
					}
				}
			} else { // Simple $VAR
				p := i
				for ; i < len(rs); i++ {
					r = rs[i]
					if !unicode.IsLetter(r) && r != '_' && !unicode.IsDigit(r) {
						break
					}
				}
				if i > p {
					buf.WriteString(getenv(string(rs[p:i])))
					i-- // Reprocess ending char
				} else {
					buf.WriteRune('$') // Treat '$' literally
					i--                // Reprocess char after '$'
				}
			}
		} else {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}
