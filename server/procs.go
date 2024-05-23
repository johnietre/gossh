package server

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"sync/atomic"
	"syscall"

	"github.com/johnietre/gossh/common"
	utils "github.com/johnietre/utils/go"
)

var (
	procs  = utils.NewRWMutex[common.Procs](common.Procs{})
	procId atomic.Uint64
)

func nextProcId(procs []*common.Process) uint64 {
	for {
		id := procId.Add(1)
		if id == 0 {
			procId.Add(1)
		}
		ok := true
		for _, proc := range procs {
			if proc.Id == id {
				ok = false
				break
			}
		}
		if ok {
			return id
		}
	}
}

func getProc(id uint64, getEnv bool) (proc *common.Process) {
	procs.RApply(func(pp *common.Procs) {
		for _, p := range *pp {
			if p.Id == id {
				if getEnv {
					p.Env = p.CmdEnv()
				}
				proc = p
				return
			}
		}
	})
	return
}

func getAndSendProcs(f func(common.Procs) error) error {
	pp := procs.RLock()
	defer procs.RUnlock()
	return f(*pp)
}

func addProc(proc *common.Process) error {
	if proc.Dir == "" {
		proc.Dir = procsDir
	}
	procs.Apply(func(pp *common.Procs) {
		proc.Id = nextProcId(*pp)
	})
	return proc.Run(procs)
}

func signalProc(id uint64, signal syscall.Signal) (err error) {
	procs.RApply(func(pp *common.Procs) {
		for _, proc := range *pp {
			if proc.Id == id {
				err = proc.Signal(signal)
				return
			}
		}
	})
	return
}

func handleProcsConn(conn net.Conn) {
	defer conn.Close()
	var buf [1]byte

	if _, err := conn.Read(buf[:1]); err != nil {
		return
	}
	switch buf[0] {
	case common.HeaderGetProc:
		handleProcsConnGetProc(conn)
	case common.HeaderGetProcs:
		handleProcsConnGetProcs(conn)
	case common.HeaderAddProc:
		handleProcsConnAddProc(conn)
	default:
		// TODO
	}
}

func handleProcsConnGetProc(conn net.Conn) {
}

func handleProcsConnGetProcs(conn net.Conn) {
}

func handleProcsConnAddProc(conn net.Conn) {
	buf := make([]byte, 8)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return
	}
	lr := &io.LimitedReader{
		R: conn,
		N: int64(binary.LittleEndian.Uint64(buf)),
	}
	proc := &common.Process{}
	if err := json.NewDecoder(lr).Decode(proc); err != nil {
		writeConnRespMsg(conn, common.RespErr, err.Error())
		return
	}
	cmd := proc.PopulateCmd()
	if proc.Stdout == common.ProcPipe &&
		proc.Stderr == common.ProcPipe &&
		proc.Stdin == common.ProcPipe {
		// TODO: Send conn resp
		wg := handleSshConnCmd(
			conn,
			cmd,
			func() error {
				return addProc(proc)
			},
			proc.Wait,
		)
		wg.Wait()
		return
	}

	if err := addProc(proc); err != nil {
		writeConnRespMsg(conn, common.RespErr, err.Error())
		return
	}
	bytes, err := json.Marshal(proc)
	if err != nil {
		writeConnRespMsg(conn, common.RespErr, err.Error())
		return
	}
	_, err = utils.WriteAll(
		conn,
		binary.LittleEndian.AppendUint64(
			[]byte{common.RespOk},
			uint64(len(bytes)),
		),
	)
	if err != nil {
		return
	}
	utils.WriteAll(conn, bytes)
}
