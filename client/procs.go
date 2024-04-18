package client

import (
	"log"
	_ "net/http"
	"os"

	"github.com/johnietre/gossh/common"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var (
	useHttp, inheritThisEnv bool
	envFile                 string

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
	psflags := cmd.PersistentFlags()
	psflags.BoolVar(&useHttp, "http", false, "Use HTTP rather than TCP")
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
		Use:     "add [ADDR] -- <CMD>",
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
			proc.Program, proc.Args = args[progStart], args[progStart+1:]
			// TODO
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
	return cmd
}
