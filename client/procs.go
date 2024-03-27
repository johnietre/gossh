package client

import (
	"flag"
	_ "net/http"
)

func runProcs() {
	flag.Uint("signal", 0, "Signal to send")
	flag.Uint64("id", 0, "ID to get")
	flag.Parse()
	// TODO
}
