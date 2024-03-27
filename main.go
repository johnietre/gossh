package main

import (
	"os"

	"github.com/johnietre/gossh/client"
	"github.com/johnietre/gossh/server"
)

func main() {
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
