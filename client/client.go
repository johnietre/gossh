// TODO: Send password

package client

import (
	"log"
	"os"

	"golang.org/x/term"
)

var (
	password []byte
)

func RunClient() {
	log.SetFlags(0)

	if len(os.Args) < 2 {
		log.Fatal("Usage: gossh client <COMMAND>")
	}

	// TODO: Args (e.g., inherit local and/or remote environs)
	arg := os.Args[1]
	os.Args = append(os.Args[:1], os.Args[2:]...)
	os.Args[0] += " " + arg
	// Current arg[0] should be "gossh client [arg]"
	switch arg {
	case "ssh":
		if len(os.Args) == 2 {
			log.Fatal("Usage: gossh client ssh <ADDR>")
		}
		addr := os.Args[1]
		runSsh(addr)
	case "proc":
		runProcs()
	default:
		log.Fatal("Unknown command: ", arg)
	}

}

func getPassword() ([]byte, error) {
	return term.ReadPassword(int(os.Stdin.Fd()))
}
