//go:build windows

package runtime

import (
	"os"
	"os/exec"
)

// setSysProcAttr is a no-op on Windows.
// Windows does not support POSIX process groups; the exec.CommandContext
// cancellation and os.Process.Kill() are sufficient to stop the server process.
func setSysProcAttr(cmd *exec.Cmd) {}

// killProcessGroup on Windows falls back to a plain process kill.
// Windows has no POSIX process group concept, so we kill the root process only.
// Child processes started by llama-server (if any) will be cleaned up by the OS
// when the parent exits because they share the same Job Object under Windows.
func killProcessGroup(p *os.Process) {
	_ = p.Kill()
}
