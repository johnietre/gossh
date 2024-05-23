package client

import (
	"encoding/binary"
	"io"
	"log"
	"net/http"
	"os"
	"path"

	"github.com/johnietre/gossh/common"
	utils "github.com/johnietre/utils/go"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var (
	inheritThisEnv bool
	envFile        string

	proc = &common.Process{}
)

func getProcsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "procs",
		Short: "Run procs client",
		// FIXME: add more
		Long: "Perform actions with the procs on the gossh server",
		Run: func(cmd *cobra.Command, args []string) {
		},
	}
	cmd.AddCommand(getAddProcCmd())
	return cmd
}

func getSignalProcCmd() *cobra.Command {
	// TODO
	cmd := &cobra.Command{
		Use:     "signal [ADDR]",
		Aliases: []string{"s"},
		Short:   "Signal a process on the server",
		Long:    "", // TODO
	}
	return cmd
}

func getAddProcCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "add [ADDR] [OPTIONS] -- <CMD>",
		Aliases: []string{"a"},
		Short:   "Start a new process on the server",
		Long:    "Starts a new process on the gossh server.",
		Run: func(cmd *cobra.Command, args []string) {
			progStart := cmd.ArgsLenAtDash()
			if progStart == -1 {
				cmd.ErrOrStderr().Write([]byte(`Missing program after "--"`))
				if err := cmd.Usage(); err != nil {
					log.Print("Error printing usage: ", err)
				}
				os.Exit(1)
			}

			flags := cmd.Flags()
			addr := flags.Arg(0)
			if addr == "" {
				cmd.ErrOrStderr().Write([]byte("Missing address to connect to"))
				if err := cmd.Usage(); err != nil {
					log.Fatal("Error printing usage: ", err)
				}
				return
			}
			if envFile := must(flags.GetString("envfile")); envFile != "" {
				envMap, err := godotenv.Read(envFile)
				if err != nil {
					log.Fatal("Error reading envfile: ", err)
				}
				if l := len(envMap); l != 0 {
					proc.Env = append(make([]string, l), proc.Env...)
					i := 0
					for k, v := range envMap {
						proc.Env[i] = k + "=" + v
						i++
					}
				}
			}
			if must(flags.GetBool("thisenv")) {
				proc.Env = append(os.Environ(), proc.Env...)
			}
			pipe := must(flags.GetBool("pipe"))
			if pipe {
				proc.Stdout, proc.Stderr = common.ProcPipe, common.ProcPipe
				proc.Stdin = common.ProcPipe
			}
			proc.Program, proc.Args = args[progStart], args[progStart+1:]

			// TODO

			if useHttp && !pipe {
				body, err := encodeJsonBuf(proc)
				if err != nil {
					log.Fatal("Error serializing process: ", err)
				}
				password = handlePasswordErr(getPassword())
				req := newReq(http.MethodPost, path.Join(addr, "procs"), body)
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					log.Fatal("Error sending request: ", err)
				}
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					log.Print("SUCCESS")
				} else {
					log.Print("Received non-200 status: ", resp.StatusCode)
				}
				bytes, err := io.ReadAll(resp.Body)
				if err != nil {
					log.Fatal("Error reading response body: ", err)
				}
				os.Stdout.Write(bytes)
				if resp.StatusCode != http.StatusOK {
					os.Exit(1)
				}
				return
			}
			// Connect
			conn, err := dialConn(addr, common.TcpProcs)
			defer conn.Close()
			if err != nil {
				log.Fatal("Error connecting: ", err)
			}
			// Send password
			password = handlePasswordErr(getPassword())
			gotPassword = true
			if err := sendPassword(conn, nil); err != nil {
				log.Fatal("Error connecting: ", err)
			}
			// Send intent
			if _, err := conn.Write([]byte{common.HeaderAddProc}); err != nil {
				log.Fatal("Error sending intent: ", err)
			}
			// Send proc
			buf, err := encodeJsonBuf(proc)
			if err != nil {
				log.Fatal("Error serializing process: ", err)
			}
			_, err = utils.WriteAll(
				conn,
				binary.LittleEndian.AppendUint64(nil, uint64(buf.Len())),
			)
			if err != nil {
				log.Fatal("Error sending process: ", err)
			}
			if _, err := utils.WriteAll(conn, buf.Bytes()); err != nil {
				log.Fatal("Error sending process: ", err)
			}
			buf.Reset()

			if pipe {
				// Run as SSH
				runSsh(addr, conn, nil)
				return
			}
			// Get response
			b := make([]byte, 1)
			if _, err := conn.Read(b); err != nil {
				log.Fatal("Error reading response: ", err)
			}
			if b[0] != common.RespOk {
				log.Print("Received non-Ok response: ", b[0])
				errMsg, err := readErrResp(conn)
				if err != nil {
					log.Fatal("Error reading error response: ", err)
				}
				log.Fatal(string(errMsg))
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&proc.Name, "name", "", "Name of process")
	flags.StringVar(
		&proc.Dir, "dir", "",
		"Directory on server to start process in (empty means whatever the server sets the procs working directory as)",
	)
	flags.StringArrayVarP(
		&proc.Env, "env", "e", nil,
		"Environment variables to use in process (should be in KEY=VALUE format)",
	)
	flags.BoolVar(
		&proc.InheritEnv,
		"useenv",
		false,
		"Inherit the environment of the server",
	)
	flags.BoolVar(
		&inheritThisEnv,
		"thisenv",
		false,
		"Inherit the environment of this machine",
	)
	flags.StringVar(&envFile, "envfile", "", "Path to .env file")
	flags.Bool("pipe", false, "Pipe stdin/stdout/stderr to this machine")
	return cmd
}
