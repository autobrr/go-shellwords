//go:build !windows
// +build !windows

package shellwords

import (
	"bytes"
	"os"
	"unicode"
)

// replaceEnv for POSIX-like systems: Handles backslash escaping.
func replaceEnv(getenv func(string) string, s string) string {
	if getenv == nil {
		getenv = os.Getenv
	}

	var buf bytes.Buffer
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == '\\' { // Original POSIX escape handling
			i++
			if i == len(rs) {
				buf.WriteRune('\\') // Keep trailing backslash
				break
			}
			buf.WriteRune(rs[i]) // Write the escaped character
			continue
		} else if r == '$' {
			i++
			if i == len(rs) {
				buf.WriteRune(r)
				break
			}
			if rs[i] == '{' { // Changed 0x7b to '{'
				i++
				p := i
				for ; i < len(rs); i++ {
					r = rs[i]
					if r == '\\' { // Handle escaped chars within ${VAR}
						i++
						if i == len(rs) {
							// Malformed: ended with escape in var name
							buf.WriteString("${") // Treat literally
							buf.WriteString(string(rs[p : i-1]))
							i = len(rs) // Force loop end
							break
						}
						continue // Skip escaped char, process next
					}
					if r == '}' || (!unicode.IsLetter(r) && r != '_' && !unicode.IsDigit(r)) {
						break
					}
				}
				if i == p || (i < len(rs) && rs[i] != '}') {
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
					// Escapes are handled by the main `if r == '\\'` block above
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
