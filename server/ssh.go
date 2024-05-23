package server

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/creack/pty"
	"github.com/johnietre/gossh/common"
	utils "github.com/johnietre/utils/go"
)

var (
	connChans sync.Map
	idCounter atomic.Uint64
)

func handleSshConn(conn net.Conn) (wg *sync.WaitGroup) {
	cmd := exec.Command(shell)
	cmd.Dir = sshDir
	return handleSshConnCmd(conn, cmd, cmd.Start, cmd.Wait)
}

func handleSshConnCmd(
	conn net.Conn,
	cmd *exec.Cmd,
	start, wait func() error,
) (wg *sync.WaitGroup) {
	cw := &connWait{Conn: conn, WaitGroup: &sync.WaitGroup{}}
	cw.Add(1)
	wg = cw.WaitGroup
	closeConn := utils.NewT(true)
	defer utils.DeferClose(closeConn, conn)
	defer utils.DeferFunc(closeConn, wg.Done)

	var buf [8]byte

	// Get the header to see what kind of connection it is
	if _, err := conn.Read(buf[:1]); err != nil {
		return
	}
	if buf[0] == common.HeaderNewSsh {
		*closeConn = false
		go handleOOB(cw, cmd, start, wait)
		return
	} else if buf[0] != common.HeaderJoinSsh {
		return
	}
	if n, err := conn.Read(buf[:]); err != nil || n != 8 {
		return
	}
	id := binary.LittleEndian.Uint64(buf[:])
	ich, loaded := connChans.LoadAndDelete(id)
	if !loaded {
		return
	}
	ch := ich.(chan *connWait)
	ch <- cw
	close(ch)
	*closeConn = false
	return
}

type connWait struct {
	net.Conn
	*sync.WaitGroup
}

// out-of-band
func handleOOB(
	cw *connWait,
	cmd *exec.Cmd,
	start, wait func() error,
) {
	conn := cw.Conn
	defer cw.Done()
	defer conn.Close()
	var other *connWait

	id := idCounter.Add(1)
	idBytes := binary.LittleEndian.AppendUint64(nil, id)
	if _, err := conn.Write(idBytes); err != nil {
		return
	}
	ch := make(chan *connWait, 1)
	connChans.Store(id, ch)
	timer := time.NewTimer(time.Second * 10)
	select {
	case <-timer.C:
		_, loaded := connChans.LoadAndDelete(id)
		// TODO: Send timed out?
		if loaded {
			return
		} else if other, _ = <-ch; other == nil {
			return
		}
	case other = <-ch:
		if !timer.Stop() {
			<-timer.C
		}
	}
	defer other.Done()
	defer other.Close()

	if _, err := conn.Write(idBytes); err != nil {
		return
	} else if _, err := other.Write(idBytes); err != nil {
		return
	}
	if n, err := other.Read(idBytes); err != nil || n != 8 {
		return
	}
	sz := common.WinsizeFromBytes(idBytes)

	pf, tf, err := pty.Open()
	if err != nil || pf == nil || tf == nil {
		if err == nil {
			err = fmt.Errorf("files returned were nil")
		}
		conn.Write([]byte(err.Error()))
		log.Print("Error starting program: ", err)
		return
	}
	defer pf.Close()
	defer tf.Close()

	cmd.Stdin, cmd.Stdout, cmd.Stderr = tf, tf, tf
	f, err := startWithSize(cmd, &sz, start)
	if err != nil || f == nil {
		if err == nil {
			err = fmt.Errorf("file returned was nil")
		}
		conn.Write([]byte(err.Error()))
		log.Printf("Error starting %s: %v", cmd.Path, err)
		return
	}
	defer f.Close()
	pty.Setsize(tf, &sz)

	go io.Copy(conn, pf)
	go io.Copy(pf, conn)
	go func() {
		var buf [128]byte
		for {
			_, err := other.Read(buf[:1])
			if err != nil {
				break
			}
			if buf[0] == common.ActionResize {
				n, err := other.Read(buf[:8])
				if err != nil || n != 8 {
					// TODO
					break
				}
				ws := common.WinsizeFromBytes(buf[:8])
				if err := pty.Setsize(tf, &ws); err != nil {
					// TODO
				}
			}
		}
		other.Close()
	}()
	if err := wait(); err != nil {
		log.Print("Error waiting: ", err)
	}
}
