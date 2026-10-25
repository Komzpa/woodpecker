//go:build darwin

package local

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func checkSandbox() error {
	_, err := os.Stat("/usr/bin/sandbox-exec")
	if err != nil {
		return fmt.Errorf("required macOS sandbox unavailable: %w", err)
	}
	return nil
}

func sandboxPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return strconv.Quote(path)
}

// The profile confines writes and hides other checkouts and the host's homes.
// This is filesystem isolation, not a VM or a separate network namespace.
func sandboxProfile(state *workflowState) (string, error) {
	binary, err := os.Executable()
	if err != nil {
		return "", err
	}
	var p strings.Builder
	p.WriteString(`(version 1)
(deny default)
(allow file-read*)
(allow process* signal sysctl-read mach-lookup ipc-posix-shm* network*)
(allow file-read* file-write* file-ioctl (literal "/dev/null") (literal "/dev/zero") (literal "/dev/random") (literal "/dev/urandom"))
`)
	// xcrun resolves this account-specific cache path without consulting TMPDIR.
	if out, err := exec.Command("/usr/bin/getconf", "DARWIN_USER_TEMP_DIR").Output(); err == nil {
		if tmp := strings.TrimSpace(string(out)); filepath.IsAbs(tmp) {
			fmt.Fprintf(&p, "(allow file* (subpath %s))\n", sandboxPath(tmp))
		}
	}
	// Deny all siblings before allowing only the current workflow back.
	for _, denied := range []string{"/Users", filepath.Dir(state.baseDir)} {
		fmt.Fprintf(&p, "(deny file-read* (subpath %s))\n(deny file* (subpath %s))\n", sandboxPath(denied), sandboxPath(denied))
	}
	fmt.Fprintf(&p, "(allow file-read* (subpath %s))\n", sandboxPath(filepath.Dir(binary)))
	if state.pluginGitBinary != "" {
		fmt.Fprintf(&p, "(allow file-read* (literal %s))\n", sandboxPath(state.pluginGitBinary))
	}
	for _, allowed := range []string{state.baseDir, state.workspaceDir, state.homeDir} {
		fmt.Fprintf(&p, "(allow file-read* (subpath %s))\n(allow file* (subpath %s))\n", sandboxPath(allowed), sandboxPath(allowed))
	}
	return p.String(), nil
}

func sandboxCommand(ctx context.Context, original *exec.Cmd, state *workflowState) (*exec.Cmd, error) {
	if err := checkSandbox(); err != nil {
		return nil, err
	}
	profile, err := sandboxProfile(state)
	if err != nil {
		return nil, err
	}
	args := append([]string{"-p", profile, original.Path}, original.Args[1:]...)
	cmd := newCmd(ctx, "/usr/bin/sandbox-exec", args...)
	cmd.Env, cmd.Dir, cmd.Stdin = original.Env, original.Dir, original.Stdin
	return cmd, nil
}
