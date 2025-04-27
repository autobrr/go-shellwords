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
		// On Windows, backslash is literal in this context (post-parsing)
		if r == '\\' {
			buf.WriteRune(r) // Append the backslash itself
			continue
		} else if r == '$' {
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
					// Malformed variable expansion, treat '$' literally
					buf.WriteRune('$')
					if rs[i-1] == '{' { // If it was ${
						buf.WriteRune('{')
					}
					// Reset i to process characters after '$' or '${'
					i = p - 1
					if rs[i] == '{' {
						i-- // Adjust if it was ${
					}
				} else {
					buf.WriteString(getenv(string(rs[p:i])))
					if i < len(rs) && rs[i] != '}' { // If loop broke on non-brace char
						i-- // Re-process the breaking character
					}
				}

			} else { // Simple $VAR form
				p := i
				for ; i < len(rs); i++ {
					r = rs[i]
					if !unicode.IsLetter(r) && r != '_' && !unicode.IsDigit(r) {
						break
					}
				}
				if i > p {
					buf.WriteString(getenv(string(rs[p:i])))
					i-- // Re-process the character that ended the variable name
				} else {
					// Just a '$' followed by non-variable character, treat '$' literally
					buf.WriteRune('$')
					i-- // Re-process the character after '$'
				}
			}
		} else {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}
