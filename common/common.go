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
	AddrEnvName     = "GOSSH_ADDR"
	PasswordEnvName = "GOSSH_PASSWORD"
)

// HTTP specific
const (
	HttpPasswordHeader = "Gossh-Password"
)

// Initial TCP specific
const (
	TcpUnknown byte = 0
	TcpSsh     byte = 1
	TcpProcs   byte = 2
	TcpFiles   byte = 3
)

// Stream responses
const (
	RespUnknown byte = 0
	RespOk      byte = 1
	RespStop    byte = 2
	RespError   byte = 3

	RespErr                byte = 128
	RespErrNotExist        byte = 129
	RespErrPasswordInvalid byte = 130
	RespErrPasswordError   byte = 131
)

// SSH specific
const (
	HeaderNewSsh  byte = 1
	HeaderJoinSsh byte = 2

	ActionResize byte = 3
)

// Procs specific
const (
	HeaderGetProc  byte = 1
	HeaderGetProcs byte = 2
	HeaderAddProc  byte = 3
)

// Files specific
const (
	HeaderSendFiles byte = 1
	HeaderRecvFiles byte = 2
)

func TcpInitial(what byte) []byte {
	return []byte{
		255, 254, 253,
		'g', 'o', 's', 's', 'h',
		what,
	}
}

func IsTcpInitial(b []byte) bool {
	return len(b) >= 8 && bytes.Equal(b[:8], TcpInitial(0)[:8])
	//return len(b) >= 9 && bytes.Equal(b[:9], TcpInitial(b[8]))
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

const (
	ProcPipe = "|"
)

type Process struct {
	Id         uint64   `json:"id,omitempty"`
	Name       string   `json:"name"`
	Program    string   `json:"program"`
	Args       []string `json:"args,omitempty"`
	Start      int64    `json:"start,omitempty"`
	Dir        string   `json:"dir,omitempty"`
	Env        []string `json:"env,omitempty"`
	InheritEnv bool     `json:"inheritEnv,omitempty"`
	Stdout     string   `json:"stdout"`
	Stderr     string   `json:"stderr"`
	Stdin      string   `json:"stdin"`

	cmd        *exec.Cmd
	procs      *utils.RWMutex[Procs]
	closedChan chan utils.Unit
	err        *utils.AValue[utils.ErrorValue]
}

func (p *Process) CmdEnv() []string {
	if p.cmd == nil {
		return []string{}
	}
	return p.cmd.Env
}

func (p *Process) PopulateCmd() *exec.Cmd {
	if p.cmd != nil {
		return p.cmd
	}
	p.cmd = exec.Command(p.Program, p.Args...)
	p.cmd.Dir = p.Dir
	if p.InheritEnv {
		p.cmd.Env = os.Environ()
	}
	p.cmd.Env = append(p.cmd.Env, p.Env...)
	p.closedChan = make(chan utils.Unit)
	p.err = utils.NewAValue(utils.ErrorValue{})
	return p.cmd
}

func (p *Process) Run(procs *utils.RWMutex[Procs]) error {
	p.PopulateCmd()
	p.procs = procs
	// Clear env so it isn't sent every time it's serialized
	p.Env = nil

	p.Start = time.Now().Unix()
	if err := p.cmd.Start(); err != nil {
		return err
	}
	go p.watch()
	return nil
}

func (p *Process) Wait() error {
	<-p.closedChan
	return p.err.Load().Error
}

func (p *Process) watch() {
	err := p.cmd.Wait()
	p.err.Store(utils.NewErrorValue(err))
	close(p.closedChan)
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
