package engine

import (
	"os/exec"
	"strings"
)

func Cmd(engine Engine, host Host, args ...string) *exec.Cmd {
	cmdArgs := append(hostToArgs(host), args...)
	return exec.Command(engine.binary, cmdArgs...)
}

func ComposeCmd(engine Engine, host Host, composeFile string, args ...string) *exec.Cmd {
	composeArgs := append([]string{"compose", "-f", composeFile}, args...)
	cmdArgs := append(hostToArgs(host), composeArgs...)
	return exec.Command(engine.binary, cmdArgs...)
}

func String(cmd *exec.Cmd) string {
	return strings.Join(cmd.Args, " ")
}

func hostToArgs(h Host) []string {
	if h.value == "" {
		return nil
	}
	return []string{"-H", h.value}
}
