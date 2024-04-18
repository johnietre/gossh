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
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	useWs bool

	gotPassword bool = false
	termState   *term.State
)

func getSshCmd() *cobra.Command {
	addr := os.Getenv(common.AddrEnvName)
	cmd := &cobra.Command{
		Use:   fmt.Sprintf("ssh [ADDR (default: %s)]", addr),
		Short: "Run SSH client",
		Long:  "Connect to a gossh instance acting as an SSH server. The address can either be passed as a CLI arg or is gotten from the value of the " + common.AddrEnvName + " environment variable.",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			addr = cmd.Flags().Arg(0)
			if addr == "" {
				cmd.ErrOrStderr().Write([]byte("Missing address to connect to"))
				if err := cmd.Usage(); err != nil {
					log.Fatal("Error printing usage: ", err)
				}
				return
			}
			runSsh(addr)
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&useWs, "ws", false, "Use WebSocket instead of plain TCP")
	return cmd
}

func runSsh(addr string) {
	conn, other := connectSsh(addr)

	ws, err := pty.GetsizeFull(os.Stdin)
	if err != nil {
		log.Fatal("Error getting terminal size: ", err)
	}
	if _, err := other.Write(common.WinsizeToBytes(nil, ws)); err != nil {
		log.Fatal("Error sending terminal size: ", err)
	}
	winchCh := make(chan os.Signal, 1)
	signal.Notify(winchCh, syscall.SIGWINCH)
	go sshWatchWinSize(other, winchCh)

	log.Print("\n===Connected===")
	log.Print()

	termState, err = term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatal("\nError setting terminal: ", err)
	}

	go sshStdinToConn(conn)
	ret := sshConnToStdout(conn)
	log.Print("\n===Disconnected===")
	os.Exit(ret)
}

func sshConnToStdout(conn net.Conn) int {
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
	return ret
}

func sshStdinToConn(conn net.Conn) {
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
}

func sshWatchWinSize(other net.Conn, winchCh chan os.Signal) {
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
}

func connectMainConn(addr string, idBuf []byte) net.Conn {
	conn, err := connectConn(addr)
	if err != nil {
		log.Fatal("Error connecting: ", err)
	}
	if _, err := conn.Write([]byte{common.HeaderNew}); err != nil {
		log.Fatal("Error connecting: ", err)
	}
	if n, err := conn.Read(idBuf[1:]); err != nil {
		log.Fatal("Error getting ID: ", err)
	} else if n != 8 {
		log.Fatal("Expected 8 bytes for ID, got ", n)
	}
	return conn
}

func connectOtherConn(addr string, idBuf []byte) net.Conn {
	other, err := connectConn(addr)
	if err != nil {
		log.Fatal("Error connecting: ", err)
	}
	idBuf[0] = common.HeaderJoin
	if _, err := other.Write(idBuf[:]); err != nil {
		log.Fatal("Error sending ID: ", err)
	}
	return other
}

func connectSsh(addr string) (net.Conn, net.Conn) {
	var idBuf [9]byte
	conn := connectMainConn(addr, idBuf[:])
	other := connectOtherConn(addr, idBuf[:])

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

	return conn, other
}

func connectConn(addr string) (conn net.Conn, err error) {
	closeConn := utils.NewT(true)
	defer func() {
		if *closeConn && conn != nil {
			conn.Close()
		}
	}()

	conn, err = dialConn(addr)
	if err != nil {
		return nil, err
	}
	// NOTE: Refactor?
	if !gotPassword {
		if password, err = getPassword(); err != nil {
			log.Fatal("Error reading password: ", err)
		}
		gotPassword = true
	}

	// Send password
	if _, err := conn.Write([]byte{byte(len(password))}); err != nil {
		return nil, err
	}
	if _, err := utils.WriteAll(conn, password); err != nil {
		return nil, err
	}
	// Check password response
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
