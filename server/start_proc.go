//go:build !windows
// +build !windows

// An almost direct copy of the code in https://github.com/creack/pty
package server

import (
	"os"
	"os/exec"
	"syscall"

	ptypkg "github.com/creack/pty"
)

func startWithSize(
	cmd *exec.Cmd,
	sz *ptypkg.Winsize,
	start func() error,
) (*os.File, error) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true
	cmd.SysProcAttr.Setctty = true
	return startWithAttrs(cmd, sz, cmd.SysProcAttr, start)
}

func startWithAttrs(
	c *exec.Cmd,
	sz *ptypkg.Winsize,
	attrs *syscall.SysProcAttr,
	start func() error,
) (*os.File, error) {
	pty, tty, err := ptypkg.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tty.Close() }() // Best effort.

	if sz != nil {
		if err := ptypkg.Setsize(pty, sz); err != nil {
			_ = pty.Close() // Best effort.
			return nil, err
		}
	}
	if c.Stdout == nil {
		c.Stdout = tty
	}
	if c.Stderr == nil {
		c.Stderr = tty
	}
	if c.Stdin == nil {
		c.Stdin = tty
	}

	c.SysProcAttr = attrs

	if err := start(); err != nil {
		_ = pty.Close() // Best effort.
		return nil, err
	}
	return pty, err
}
