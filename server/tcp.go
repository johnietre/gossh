package server

import (
	"encoding/binary"
	"io"
	"net"

	"github.com/johnietre/gossh/common"
	"github.com/johnietre/utils/go"
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
	shouldClose := utils.NewT(true)
	defer utils.DeferClose(shouldClose, conn)

	var buf [1]byte
	if _, err := conn.Read(buf[:1]); err != nil {
		return
	}

	if !checkTcpPassword(conn) {
		return
	}

	*shouldClose = false
	switch buf[0] {
	case common.TcpSsh:
		handleSshConn(conn)
	case common.TcpFiles:
		handleFilesConn(conn)
	case common.TcpProcs:
		handleProcsConn(conn)
	default:
		// TODO
		*shouldClose = true
	}
}

func checkTcpPassword(conn net.Conn) bool {
	var buf [8]byte
	// Read password
	if _, err := conn.Read(buf[:1]); err != nil {
		return false
	}
	pwdLen := int(buf[0])
	pwdBytes := make([]byte, pwdLen)
	if _, err := io.ReadFull(conn, pwdBytes); err != nil {
		return false
	}
	if ok, err := checkPassword(pwdBytes); err != nil {
		conn.Write([]byte{common.RespErrPasswordError})
		return false
	} else if !ok {
		conn.Write([]byte{common.RespErrPasswordInvalid})
		return false
	}
	if _, err := conn.Write([]byte{common.RespOk}); err != nil {
		return false
	}
	return true
}

func writeConnRespMsg(conn net.Conn, what byte, msg string) error {
	if len(msg) > 1<<16-1 {
		msg = msg[:1<<16-1]
	}
	bytes := append(
		binary.LittleEndian.AppendUint16([]byte{what}, uint16(len(msg))),
		msg...,
	)
	_, err := utils.WriteAll(conn, bytes)
	return err
}
