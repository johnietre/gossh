package client

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"github.com/johnietre/gossh/common"
	utils "github.com/johnietre/utils/go"
	"golang.org/x/term"
)

var (
	termState *term.State
)

func connectConn(addr string) (conn net.Conn, err error) {
	closeConn := utils.NewT(true)
	defer func() {
		if *closeConn && conn != nil {
			conn.Close()
		}
	}()

	conn, err = net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	if _, err := utils.WriteAll(conn, common.TcpInitial()); err != nil {
		return nil, err
	}
	if _, err := conn.Write([]byte{byte(len(password))}); err != nil {
		return nil, err
	}
	if _, err := utils.WriteAll(conn, password); err != nil {
		return nil, err
	}
	var buf [1]byte
	if _, err := conn.Read(buf[:]); err != nil {
		return nil, err
	}
	switch buf[0] {
	case common.PasswordOk:
	case common.PasswordInvalid:
		return nil, fmt.Errorf("password incorrect")
	case common.PasswordError:
		return nil, fmt.Errorf("password server error")
	default:
		return nil, fmt.Errorf("received unknown password response: %d", buf[0])
	}
	*closeConn = false
	return conn, nil
}

// TODO: Refactor?
func runSsh(addr string) {
	conn, err := connectConn(addr)
	if err != nil {
		log.Fatal("Error connecting: ", err)
	}
	if _, err := conn.Write([]byte{common.HeaderNew}); err != nil {
		log.Fatal("Error connecting: ", err)
	}
	var idBuf [9]byte
	if n, err := conn.Read(idBuf[1:]); err != nil {
		log.Fatal("Error getting ID: ", err)
	} else if n != 8 {
		log.Fatal("Expected 8 bytes for ID, got ", n)
	}

	other, err := connectConn(addr)
	if err != nil {
		log.Fatal("Error connecting: ", err)
	}
	idBuf[0] = common.HeaderJoin
	if _, err := other.Write(idBuf[:]); err != nil {
		log.Fatal("Error sending ID: ", err)
	}

	if n, err := conn.Read(idBuf[:8]); err != nil {
		log.Fatal("Error reading response: ", err)
	} else if n != 8 {
		log.Fatal("Expected 8 bytes, got ", n)
	}
	if n, err := other.Read(idBuf[:8]); err != nil {
		log.Fatal("Error reading response: ", err)
	} else if n != 8 {
		log.Fatal("Expected 8 bytes, got ", n)
	}

	ws, err := pty.GetsizeFull(os.Stdin)
	if err != nil {
		log.Fatal("Error getting terminal size: ", err)
	}
	if _, err := other.Write(common.WinsizeToBytes(nil, ws)); err != nil {
		log.Fatal("Error sending terminal size: ", err)
	}
	winchCh := make(chan os.Signal, 1)
	signal.Notify(winchCh, syscall.SIGWINCH)
	go func() {
		var buf [9]byte
		buf[0] = common.ActionResize
		for range winchCh {
			ws, err := pty.GetsizeFull(os.Stdin)
			if err != nil {
				log.Fatal("Error getting terminal size: ", err)
			}
			common.WinsizeToBytes(buf[1:], ws)
			if _, err := other.Write(buf[:]); err != nil {
				log.Fatal("Error sending terminal size: ", err)
			}
		}
	}()

	log.Print("\n===Connected===")
	log.Print()

	termState, err = term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatal("\nError setting terminal: ", err)
	}

	go func() {
		ret, buf := 0, [1024]byte{}
		for {
			n, err := os.Stdin.Read(buf[:])
			if err != nil {
				if err != io.EOF {
					log.Print("\nError reading stdin: ", err)
					ret = 1
				}
				break
			}
			if _, err := conn.Write(buf[:n]); err != nil {
				log.Print("\nError writing: ", err)
				ret = 1
				break
			}
		}
		term.Restore(int(os.Stdin.Fd()), termState)
		os.Exit(ret)
	}()
	ret, buf := 0, make([]byte, 1<<15)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Print("\nError reading: ", err)
				ret = 1
			}
			break
		}
		os.Stdout.Write(buf[:n])
	}
	term.Restore(int(os.Stdin.Fd()), termState)
	log.Print("\n===Disconnected===")
	os.Exit(ret)
}
