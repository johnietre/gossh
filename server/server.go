package server

import (
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/johnietre/gossh/common"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
)

var (
	procsDir, sshDir              string
	noSsh, noProcs, noTcp, noHttp bool

	passwordHash []byte
	hasPassword  bool
)

func GetCmd() *cobra.Command {
	addr := os.Getenv(common.AddrEnvName)
	cmd := &cobra.Command{
		Use:     fmt.Sprintf("server [ADDR (default: %s)]", addr),
		Aliases: []string{"s"},
		Short:   "Start gossh server",
		Long: `Start a gossh server which can be used to start a gossh ssh server and/or a gossh procs server.
Both are started by default and can be opted out of using flags. Acceptance of plain TCP or HTTP connections can also be opted out of.
The address can either be passed as a CLI arg or is gotten from the value of the ` + common.AddrEnvName + ` environment variable.
The password, if desired, can be set using the ` + common.PasswordEnvName + ` environment variable.`,
		Run: func(cmd *cobra.Command, args []string) {
			if procsDir == "" {
				if sshDir != "" {
					procsDir = sshDir
				} else {
					procsDir = "."
				}
			}
			if sshDir == "" {
				sshDir = procsDir
			}
			if noSsh && noProcs {
				log.Fatal("Must start at least one type of server (SSH, Procs, etc.)")
			} else if noTcp && noHttp {
				log.Fatal("Must allow at least one type of connection (TCP, HTTP, etc.)")
			}
			if l := len(args); l == 1 {
				addr = args[0]
			} else if l != 0 {
				// TODO
			}
			if addr == "" {
				cmd.ErrOrStderr().Write([]byte("Missing address to run on"))
				if err := cmd.Usage(); err != nil {
					log.Fatal("Error printing usage: ", err)
				}
				return
			}

			runServer(addr)
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(
		&procsDir, "dir", "d", "",
		"Directory to start processes in. If empty, uses current dir. If empty and --sdir is set, uses SSH directory",
	)
	flags.StringVarP(
		&sshDir, "sdir", "D", "",
		"Directory to start SSH connections in. Follows same rules as --dir",
	)
	flags.BoolVar(&noSsh, "nossh", false, "Don't start SSH server")
	flags.BoolVar(&noProcs, "noprocs", false, "Don't start procs server")
	flags.BoolVar(&noTcp, "notcp", false, "Don't allow plain TCP connections, must be HTTP(s)")
	flags.BoolVar(&noHttp, "nohttp", false, "Don't allow HTTP requests/connections")
	return cmd
}

func runServer(addr string) {
	password := os.Getenv(common.PasswordEnvName)
	if password != "" {
		err := os.Setenv(common.PasswordEnvName, "")
		if err != nil {
			log.Fatal("error resetting password envvar: ", err)
		}
		passwordHash, err = bcrypt.GenerateFromPassword(
			[]byte(password),
			bcrypt.DefaultCost,
		)
		if err != nil {
			log.Fatal("error setting password: ", err)
		}
		hasPassword = true
	}

	ln, err := Listen("tcp", addr)
	if err != nil {
		log.Fatal("Error listening: ", err)
	}
	defer ln.Close()
	go func() {
		err := ln.Run()
		if err != nil {
			log.Print("Error running: ", err)
		}
	}()
	if !noSsh {
		log.Printf("Using %s as SSH starting directory", sshDir)
	}
	if !noProcs {
		log.Printf("Using %s as procs working directory", procsDir)
	}
	log.Print("Listening on ", addr)

	var wg sync.WaitGroup
	if !noTcp {
		wg.Add(1)
		go func() {
			if err := runTcp(ln.Tcp()); err != nil {
				log.Print("Error running TCP: ", err)
			}
			wg.Done()
		}()
	}
	if !noHttp {
		wg.Add(1)
		go func() {
			if err := runHttp(ln.Http()); err != nil {
				log.Print("Error running HTTP: ", err)
			}
			wg.Done()
		}()
	}
	wg.Wait()
}

func checkPassword(pwd []byte) (bool, error) {
	if !hasPassword {
		return len(pwd) == 0, nil
	}
	err := bcrypt.CompareHashAndPassword(passwordHash, pwd)
	if err == bcrypt.ErrMismatchedHashAndPassword || err == bcrypt.ErrHashTooShort {
		return false, nil
	}
	return err == nil, err
}
