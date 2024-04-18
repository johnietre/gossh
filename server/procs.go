package server

import (
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
