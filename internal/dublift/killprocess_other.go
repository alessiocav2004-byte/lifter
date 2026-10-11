//go:build !unix

package dublift

import (
	"errors"
	"os"
)

func killProcess(p *os.Process) error {
	if p == nil {
		return errors.New("os: process not initialized")
	}
	return p.Kill()
}
