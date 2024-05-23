package server

import (
	"net"

	"github.com/johnietre/gossh/common"
)

func runTcp(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
    go handleTcp(conn)
	}
}

func handleTcp(conn net.Conn) {
  var b [1]byte
  if _, err := conn.Read(b[:]); err != nil {
    conn.Close()
    return
  }
  switch b[0] {
  case common.TcpSsh:
    handleSshConn(conn)
  case common.TcpFiles:
    handleFilesConn(conn)
  case common.TcpProcs:
    // TODO
  default:
    // TODO
    conn.Close()
  }
}
