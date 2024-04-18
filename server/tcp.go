package server

import (
	"net"
)

func runTcp(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go handleSshConn(conn)
	}
}
