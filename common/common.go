package common

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/creack/pty"
	utils "github.com/johnietre/utils/go"
)

const (
	HttpPasswordHeader = "Gossh-Password"
)

const (
	ActionResize byte = 3
)

const (
	HeaderNew  byte = 1
	HeaderJoin byte = 2
)

const (
	PasswordOk      byte = 1
	PasswordInvalid byte = 2
	PasswordError   byte = 3
)

func TcpInitial() []byte {
	return []byte{
		255, 254, 253,
		'g', 'o', 's', 's', 'h',
	}
}

func IsTcpInitial(b []byte) bool {
	return len(b) == 8 && bytes.Equal(b, TcpInitial())
}

func WinsizeFromBytes(b []byte) pty.Winsize {
	return pty.Winsize{
		Rows: binary.LittleEndian.Uint16(b[:2]),
		Cols: binary.LittleEndian.Uint16(b[2:4]),
		X:    binary.LittleEndian.Uint16(b[4:6]),
		Y:    binary.LittleEndian.Uint16(b[6:]),
	}
}

func WinsizeToBytes(b []byte, ws *pty.Winsize) []byte {
	if b == nil {
		b = make([]byte, 8)
	}
	binary.LittleEndian.PutUint16(b[:2], ws.Rows)
	binary.LittleEndian.PutUint16(b[2:4], ws.Cols)
	binary.LittleEndian.PutUint16(b[4:6], ws.X)
	binary.LittleEndian.PutUint16(b[6:], ws.Y)
	return b
}

type Procs = []*Process

type Process struct {
	Id         uint64   `json:"id"`
	Name       string   `json:"name"`
	Program    string   `json:"program"`
	Args       []string `json:"args"`
	Start      int64    `json:"start"`
	Dir        string   `json:"dir"`
	Env        []string `json:"env"`
	InheritEnv bool     `json:"inheritEnv"`

	cmd   *exec.Cmd
	procs *utils.RWMutex[Procs]
}

func (p *Process) CmdEnv() []string {
	if p.cmd == nil {
		return []string{}
	}
	return p.cmd.Env
}

func (p *Process) Run(procs *utils.RWMutex[Procs]) error {
	p.procs = procs
	p.cmd = exec.Command(p.Program, p.Args...)
	p.cmd.Dir = p.Dir
	if p.InheritEnv {
		p.cmd.Env = os.Environ()
	}
	p.cmd.Env = append(p.cmd.Env, p.Env...)
	// Clear env so it isn't sent every time it's serialized
	p.Env = nil

	p.Start = time.Now().Unix()
	if err := p.cmd.Start(); err != nil {
		return err
	}
	go p.watch()
	return nil
}

func (p *Process) watch() {
	// TODO: err
	p.cmd.Wait()
	p.procs.Apply(func(pp *Procs) {
		for i, proc := range *pp {
			if proc.Id == p.Id {
				*pp = append((*pp)[:i], (*pp)[i+1:]...)
				return
			}
		}
	})
}

func (p *Process) Signal(sig syscall.Signal) error {
	if p.cmd == nil {
		return fmt.Errorf("command not started")
	} else if p.cmd.Process == nil {
		return fmt.Errorf("no attached process")
	}
	return p.cmd.Process.Signal(sig)
}
