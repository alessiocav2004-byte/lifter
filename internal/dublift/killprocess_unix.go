//go:build unix

package dublift

import (
	"errors"
	"os"
	"syscall"
)

// killProcess sends SIGKILL with kill(2) instead of os.Process.Kill.
//
// Since Go 1.23, os.Process.Kill signals through pidfd_send_signal when the
// runtime believes pidfds work. Some Android app seccomp policies trap that
// syscall even though Go's probe reported pidfds as usable (on Android the
// probe ignores SIGSYS, see golang/go#69065). The first Kill then raises
// SIGSYS and terminates the whole server; seen on Fire OS 7. kill(2) is
// allowed everywhere.
func killProcess(p *os.Process) error {
	if p == nil || p.Pid <= 0 {
		return errors.New("os: process not initialized")
	}
	err := syscall.Kill(p.Pid, syscall.SIGKILL)
	if err == syscall.ESRCH {
		return os.ErrProcessDone
	}
	return err
}
