//go:build windows
// +build windows

package server

import (
	"os"
	"os/exec"

	"github.com/creack/pty"
)

func startWithSize(
	cmd *exec.Cmd,
	sz *pty.Winsize,
	start func() error,
) (*os.File, error) {
	return nil, pty.ErrUnsupported
}
