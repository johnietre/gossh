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

func handleSshConn(conn net.Conn) (wg *sync.WaitGroup) {
	cw := &connWait{Conn: conn, WaitGroup: &sync.WaitGroup{}}
	cw.Add(1)
	wg = cw.WaitGroup
	closeConn := utils.NewT(true)
	defer func() {
		if *closeConn {
			wg.Done()
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
	if ok, err := checkPassword(pwdBytes); err != nil {
		conn.Write([]byte{common.PasswordError})
		return
	} else if !ok {
		conn.Write([]byte{common.PasswordInvalid})
		return
	}
	if _, err := conn.Write([]byte{common.PasswordOk}); err != nil {
		return
	}

	// Get the header to see what kind of connection it is
	if _, err := conn.Read(buf[:1]); err != nil {
		return
	}
	if buf[0] == common.HeaderNewSsh {
		*closeConn = false
		go handleOOB(cw)
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
func handleOOB(cw *connWait) {
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
		log.Print("Error starting bash: ", err)
	}
	defer pf.Close()
	defer tf.Close()

	cmd := exec.Command("bash")
	cmd.Dir = sshDir
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
