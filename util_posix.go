//go:build !windows
// +build !windows

package shellwords

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func shellRun(line, dir string) (string, error) {
	var shell string
	if shell = os.Getenv("SHELL"); shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell, "-c", line)
	if dir != "" {
		cmd.Dir = dir
	}
	b, err := cmd.Output()
	if err != nil {
		if eerr, ok := err.(*exec.ExitError); ok {
			b = eerr.Stderr
		}
		return "", fmt.Errorf("%s: %w", string(b), err)
	}
	return strings.TrimSpace(string(b)), nil
}

func isEscapeRune(r rune) bool {
	return r == '\\'
}
