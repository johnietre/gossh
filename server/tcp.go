package server

import (
	"encoding/binary"
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

func runTcp(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go handle(conn)
	}
}

func handle(conn net.Conn) {
	closeConn := utils.NewT(true)
	go func() {
		if *closeConn {
			conn.Close()
		}
	}()

	var buf [8]byte

	// Read password
	if _, err := conn.Read(buf[:1]); err != nil {
		return
	}
	pwdLen := int(buf[0])
	pwdBytes := make([]byte, pwdLen)
	if _, err := io.ReadFull(conn, pwdBytes); err != nil {
		return
	}
	if ok, err := checkPassword(pwdBytes); !ok {
		conn.Write([]byte{1})
		return
	} else if err != nil {
		conn.Write([]byte{2})
		return
	}
	if _, err := conn.Write([]byte{0}); err != nil {
		return
	}

	if _, err := conn.Read(buf[:1]); err != nil {
		return
	}
	if buf[0] == common.HeaderNew {
		*closeConn = false
		go handleOOB(conn)
		return
	} else if buf[0] != common.HeaderJoin {
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
	ch := ich.(chan net.Conn)
	ch <- conn
	close(ch)
	*closeConn = false
}

// out-of-band
func handleOOB(conn net.Conn) {
	defer conn.Close()
	var other net.Conn

	id := idCounter.Add(1)
	idBytes := binary.LittleEndian.AppendUint64(nil, id)
	if _, err := conn.Write(idBytes); err != nil {
		return
	}
	ch := make(chan net.Conn, 1)
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
		log.Print("Error starting bash: ", err)
	}
	defer pf.Close()
	defer tf.Close()

	cmd := exec.Command("bash")
	cmd.Dir = dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = tf, tf, tf
	f, err := pty.StartWithSize(cmd, &sz)
	if err != nil || f == nil {
		conn.Write([]byte(err.Error()))
		log.Print("Error starting: ", err)
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
	if err := cmd.Wait(); err != nil {
		log.Print("Error waiting: ", err)
	}
}

type Interceptor struct {
	conn net.Conn
	io.ReadWriter
}

func (p *Interceptor) Read(b []byte) (int, error) {
	return p.ReadWriter.Read(b)
}

func (p *Interceptor) Write(b []byte) (int, error) {
	return p.ReadWriter.Write(b)
}
