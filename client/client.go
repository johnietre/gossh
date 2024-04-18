// TODO: Send password

package client

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/johnietre/gossh/common"
	utils "github.com/johnietre/utils/go"
	"github.com/spf13/cobra"
	webs "golang.org/x/net/websocket"
	"golang.org/x/term"
)

var (
	password []byte
)

func GetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "client",
		Short:   "Use gossh client",
		Long:    "Run gossh client to connect to a gossh server instance to connect to gossh SSH or do stuff with gossh procs.",
		Aliases: []string{"c"},
		//Run: runClient,
	}
	cmd.AddCommand(getSshCmd(), getProcsCmd())
	psflags := cmd.PersistentFlags()
	psflags.BoolVar(
		&envPwd, "envpwd", false,
		fmt.Sprintf(
			"Use password set by value of %s environment variable",
			common.PasswordEnvName,
		),
	)
	return cmd
}

var (
	envPwd bool
)

func getPassword() (pwd []byte, err error) {
	if envPwd {
		pwd = []byte(os.Getenv(common.PasswordEnvName))
	} else {
		fmt.Print("Password: ")
		pwd, err = term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
	}
	// FIXME: Check to make sure length constraint is correct
	if len(pwd) > 70 {
		err = fmt.Errorf("password too long")
	}
	return
}

func dialConn(addr string) (net.Conn, error) {
	if useWs {
		return webs.Dial("ws://"+addr+"/ssh/ws", "", "http://localhost/")
	}
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	if _, err := utils.WriteAll(conn, common.TcpInitial()); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func must[T any](t T, err error) T {
	if err != nil {
		log.Fatal(err)
	}
	return t
}
