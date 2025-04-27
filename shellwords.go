package shellwords

import (
	"errors"
	"runtime" // Added import
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

		// Original escape handling logic restored, with Windows modification
		if isEscapeRune(r) { // Check if the rune is the platform's escape character
			if singleQuoted {
				// Inside single quotes, the escape character is literal
				buf += string(r)
			} else {
				// Outside single quotes:
				// Windows: '\' only escapes the *next* character if it's a double quote '"'. Otherwise, it's literal.
				// POSIX: '\' always escapes the next character.
				isWindows := runtime.GOOS == "windows" // Check OS (import "runtime")
				if isWindows {
					// Look ahead: Check if next char exists and is '"'
					if i+1 < len(line) && line[i+1] == '"' {
						escaped = true // Treat '\' as escape for the upcoming '"'
					} else {
						buf += string(r) // Treat '\' as literal
					}
				} else {
					// POSIX behavior: always escape the next character
					escaped = true
				}
			}
			continue
		}

		// If it wasn't the escape rune, handle spaces etc.
		if isSpace(r) {
			if singleQuoted || doubleQuoted || backQuote || dollarQuote {
				buf += string(r)
				backtick += string(r)
			} else if got != argNo {
				// Argument finished. Process it (original logic restored).
				if p.ParseEnv {
					if got == argSingle {
						// Re-parse unquoted args after expansion (original logic)
						parser := &Parser{ParseEnv: false, ParseBacktick: false, Position: 0, Dir: p.Dir, Getenv: p.Getenv}
						strs, err := parser.Parse(replaceEnv(p.Getenv, buf))
						if err != nil {
							return nil, err
						}
						args = append(args, strs...)
					} else {
						// Append quoted args after expansion
						args = append(args, replaceEnv(p.Getenv, buf))
					}
				} else {
					// Append arg without expansion
					args = append(args, buf)
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

	// Process the last argument (original logic restored)
	if got != argNo {
		if p.ParseEnv {
			if got == argSingle {
				parser := &Parser{ParseEnv: false, ParseBacktick: false, Position: 0, Dir: p.Dir, Getenv: p.Getenv}
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
