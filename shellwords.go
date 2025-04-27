package shellwords

import (
	"errors"
	"runtime"
	"strings"
)

var (
	ParseEnv      bool = false
	ParseBacktick bool = false
)

func isSpace(r rune) bool {
	switch r {
	case ' ', '\t', '\r', '\n':
		return true
	}
	return false
}

type Parser struct {
	ParseEnv      bool
	ParseBacktick bool
	Position      int
	Dir           string

	// If ParseEnv is true, use this for getenv.
	// If nil, use os.Getenv.
	Getenv func(string) string

	// Internal flag to indicate if this is a recursive parse call
	// used after environment variable expansion on Windows.
	IsInnerParse bool
}

func NewParser() *Parser {
	return &Parser{
		ParseEnv:      ParseEnv,
		ParseBacktick: ParseBacktick,
		Position:      0,
		Dir:           "",
	}
}

type argType int

const (
	argNo argType = iota
	argSingle
	argQuoted
)

func (p *Parser) Parse(line string) ([]string, error) {
	args := []string{}
	buf := ""
	var escaped, doubleQuoted, singleQuoted, backQuote, dollarQuote bool
	backtick := ""

	pos := -1
	got := argNo

	i := -1
loop:
	for _, r := range line {
		i++
		if escaped {
			if r == 't' {
				r = '\t'
			}
			if r == 'n' {
				r = '\n'
			}
			buf += string(r)
			escaped = false
			got = argSingle
			continue
		}

		// Determine if the current rune should be treated as an escape character.
		// On Windows, during an inner parse (after env var expansion), treat backslash literally.
		isPlatformEscapeChar := isEscapeRune(r)
		treatAsEscape := isPlatformEscapeChar && !(p.IsInnerParse && runtime.GOOS == "windows")

		if treatAsEscape {
			if singleQuoted { // Inside single quotes, escape char is literal (except for '\'')
				buf += string(r)
			} else {
				escaped = true // Mark the next character as escaped
			}
			continue
		}

		// If it's not an escape char (or we're treating it literally), handle spaces etc.
		if isSpace(r) {
			if singleQuoted || doubleQuoted || backQuote || dollarQuote {
				buf += string(r)
				backtick += string(r)
			} else if got != argNo {
				// Argument finished. Process it.
				argToAppend := buf
				if p.ParseEnv {
					// Apply environment variable expansion *before* potential inner parse
					expandedArg := replaceEnv(p.Getenv, buf)
					if got == argSingle {
						// If the original arg was unquoted, and env expansion happened,
						// we need to re-parse the result in case expansion introduced spaces,
						// but treat backslashes literally during this inner parse on Windows.
						parser := &Parser{
							ParseEnv:      false, // Don't re-expand env vars
							ParseBacktick: false, // Don't run backticks
							Position:      0,
							Dir:           p.Dir,
							Getenv:        p.Getenv, // Pass original Getenv
							IsInnerParse:  true,     // Mark as inner parse
						}
						strs, err := parser.Parse(expandedArg)
						if err != nil {
							return nil, err // Propagate error from inner parse
						}
						// Append the results of the inner parse
						args = append(args, strs...)
						// Reset buf and got, skip appending the original/expanded arg below
						buf = ""
						got = argNo
						continue // Skip the final append for this case
					} else {
						// If the original arg was quoted, use the expanded result directly
						argToAppend = expandedArg
					}
				}
				// Append the final argument (original, or expanded if quoted)
				if got != argNo { // Check got again as inner parse might have reset it
					args = append(args, argToAppend)
				}
				buf = ""
				got = argNo
			}
			continue
		}

		switch r {
		case '`':
			if !singleQuoted && !doubleQuoted && !dollarQuote {
				if p.ParseBacktick {
					if backQuote {
						out, err := shellRun(backtick, p.Dir)
						if err != nil {
							return nil, err
						}
						buf = buf[:len(buf)-len(backtick)] + out
					}
					backtick = ""
					backQuote = !backQuote
					continue
				}
				backtick = ""
				backQuote = !backQuote
			}
		case ')':
			if !singleQuoted && !doubleQuoted && !backQuote {
				if p.ParseBacktick {
					if dollarQuote {
						out, err := shellRun(backtick, p.Dir)
						if err != nil {
							return nil, err
						}
						buf = buf[:len(buf)-len(backtick)-2] + out
					}
					backtick = ""
					dollarQuote = !dollarQuote
					continue
				}
				backtick = ""
				dollarQuote = !dollarQuote
			}
		case '(':
			if !singleQuoted && !doubleQuoted && !backQuote {
				if !dollarQuote && strings.HasSuffix(buf, "$") {
					dollarQuote = true
					buf += "("
					continue
				} else {
					return nil, errors.New("invalid command line string")
				}
			}
		case '"':
			if !singleQuoted && !dollarQuote {
				if doubleQuoted {
					got = argQuoted
				}
				doubleQuoted = !doubleQuoted
				continue
			}
		case '\'':
			if !doubleQuoted && !dollarQuote {
				if singleQuoted {
					got = argQuoted
				}
				singleQuoted = !singleQuoted
				continue
			}
		case ';', '&', '|', '<', '>':
			if !(escaped || singleQuoted || doubleQuoted || backQuote || dollarQuote) {
				if r == '>' && len(buf) > 0 {
					if c := buf[0]; '0' <= c && c <= '9' {
						i -= 1
						got = argNo
					}
				}
				pos = i
				break loop
			}
		}

		got = argSingle
		buf += string(r)
		if backQuote || dollarQuote {
			backtick += string(r)
		}
	}

	if got != argNo {
		if p.ParseEnv {
			if got == argSingle {
				parser := &Parser{ParseEnv: false, ParseBacktick: false, Position: 0, Dir: p.Dir}
				strs, err := parser.Parse(replaceEnv(p.Getenv, buf))
				if err != nil {
					return nil, err
				}
				args = append(args, strs...)
			} else {
				args = append(args, replaceEnv(p.Getenv, buf))
			}
		} else {
			args = append(args, buf)
		}
	}

	if escaped || singleQuoted || doubleQuoted || backQuote || dollarQuote {
		return nil, errors.New("invalid command line string")
	}

	p.Position = pos

	return args, nil
}

func (p *Parser) ParseWithEnvs(line string) (envs []string, args []string, err error) {
	_args, err := p.Parse(line)
	if err != nil {
		return nil, nil, err
	}
	envs = []string{}
	args = []string{}
	parsingEnv := true
	for _, arg := range _args {
		if parsingEnv && isEnv(arg) {
			envs = append(envs, arg)
		} else {
			if parsingEnv {
				parsingEnv = false
			}
			args = append(args, arg)
		}
	}
	return envs, args, nil
}

func isEnv(arg string) bool {
	return len(strings.Split(arg, "=")) == 2
}

func Parse(line string) ([]string, error) {
	return NewParser().Parse(line)
}

func ParseWithEnvs(line string) (envs []string, args []string, err error) {
	return NewParser().ParseWithEnvs(line)
}
