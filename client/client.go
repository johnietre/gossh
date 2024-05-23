// TODO: Send password

package client

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/johnietre/gossh/common"
	utils "github.com/johnietre/utils/go"
	"github.com/spf13/cobra"
	webs "golang.org/x/net/websocket"
	"golang.org/x/term"
)

var (
	password          []byte
	useHttp, insecure bool
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
	psflags.BoolVar(
		&useHttp, "http", false,
		"Use HTTP (websocket when applicable) rather than TCP",
	)
	psflags.BoolVar(
		&insecure, "insecure", true,
		"Use insecure connection",
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

func handlePasswordErr(pwd []byte, err error) []byte {
	if err != nil {
		log.Fatal("Error reading password: ", err)
	}
	return pwd
}

func dialConn(addr string, what byte) (net.Conn, error) {
	if useHttp {
		if insecure {
			addr = "ws://" + addr
		} else {
			addr = "wss://" + addr
		}
		return webs.Dial(addr, "", "http://localhost/")
	}
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	if _, err := utils.WriteAll(conn, common.TcpInitial(what)); err != nil {
		conn.Close()
		// TODO
		return nil, err
	}
	return conn, nil
}

func newReq(method string, urlStr string, body io.Reader) *http.Request {
	if insecure {
		urlStr = "http://" + urlStr
	} else {
		urlStr = "https://" + urlStr
	}
	req, err := http.NewRequest(method, urlStr, body)
	if err != nil {
		log.Fatal("Error creating request: ", err)
	}
	req.Header.Set(common.HttpPasswordHeader, string(password))
	return req
}

func sendPassword(conn net.Conn, pwd []byte) error {
	if pwd == nil {
		pwd = password
	}
	// Send password
	if _, err := conn.Write([]byte{byte(len(pwd))}); err != nil {
		return err
	}
	if _, err := utils.WriteAll(conn, pwd); err != nil {
		return err
	}
	// Check password response
	var buf [1]byte
	if _, err := conn.Read(buf[:]); err != nil {
		return err
	}
	switch buf[0] {
	case common.RespOk:
		return nil
	case common.RespErrPasswordInvalid:
		return errIncorrectPassword
	case common.RespErrPasswordError:
		return fmt.Errorf("password server error")
	default:
		return fmt.Errorf("received unknown password response: %d", buf[0])
	}
}

func readErrResp(conn net.Conn) ([]byte, error) {
	buf := make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return nil, err
	}
	buf = make([]byte, binary.LittleEndian.Uint16(buf))
	_, err := io.ReadFull(conn, buf)
	return buf, err
}

func encodeJsonBuf(v any) (*bytes.Buffer, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return bytes.NewBuffer(b), nil
}

var (
	errIncorrectPassword = fmt.Errorf("password incorrect")
)

func must[T any](t T, err error) T {
	if err != nil {
		log.Fatal(err)
	}
	return t
}
