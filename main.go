package main

import (
	"log"

	"github.com/johnietre/gossh/client"
	"github.com/johnietre/gossh/server"
	"github.com/spf13/cobra"
)

func main() {
	log.SetFlags(0)
	rootCmd := cobra.Command{
		Use: "gossh",
	}
	rootCmd.AddCommand(client.GetCmd(), server.GetCmd())
	cobra.CheckErr(rootCmd.Execute())
}

/*
func main() {
  rootCmd := cobra.Command{
  }
  rootCmd.AddCommand(client.GetClientCmd())
	if len(os.Args) == 1 {
		// TODO
	}
	arg := os.Args[1]
	os.Args = append(os.Args[:1], os.Args[2:]...)
	os.Args[0] += " " + arg
	switch arg {
	case "client":
		client.RunClient()
	case "server":
		server.RunServer()
	}
}
*/
