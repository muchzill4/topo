package command

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func SSHKeyGen(ctx context.Context, keyType string, keyPath string, targetHost string) *exec.Cmd {
	sshKeyGenArgs := []string{"-t", keyType, "-f", keyPath, "-C", targetHost}
	return exec.CommandContext(ctx, "ssh-keygen", sshKeyGenArgs...)
}

func WrapInLoginShell(cmd string) string {
	escaped := shellEscapeForDoubleQuotes(cmd)
	return fmt.Sprintf(`/bin/sh -c "exec ${SHELL:-/bin/sh} -l -c \"%s\""`, escaped)
}

func BinaryLookupCommand(bin string) (string, error) {
	if err := ValidateBinaryName(bin); err != nil {
		return "", err
	}

	return fmt.Sprintf("command -v %s", bin), nil
}

func QuoteArg(s string) string {
	if s == "" {
		return "''"
	}

	if strings.ContainsAny(s, " \t\n\"'\\$`") {
		return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
	}

	return s
}

func shellEscapeForDoubleQuotes(s string) string {
	repl := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\\\"`,
		`$`, `\\\$`,
		"`", `\\\`+"`",
	)
	return repl.Replace(s)
}
