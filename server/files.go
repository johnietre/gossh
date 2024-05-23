package server

import (
	"encoding/binary"
	"io"
	"net"
	"os"

	"github.com/johnietre/gossh/common"
	utils "github.com/johnietre/utils/go"
)

func handleFilesConn(conn net.Conn) {
  defer conn.Close()
  // Get intent
  buf := make([]byte, 2)
  if _, err := conn.Read(buf[:1]); err != nil {
    return
  }
  switch buf[0] {
  case common.HeaderSendFiles:
    handleFilesRecvClientFiles(conn)
  case common.HeaderRecvFiles:
    handleFilesSendClientFiles(conn)
  default:
    return
  }

  pathLen := int(binary.LittleEndian.Uint16(buf))
  // Get path
  buf = make([]byte, pathLen)
  if _, err := io.ReadFull(conn, buf); err != nil {
    return
  }
  path := string(buf)
  // Check to make sure path exists
  info, err := os.Stat(path)
  if err != nil {
    writeErr(conn, err)
    return
  }
  // Send file size
  size := uint64(info.Size())
  sizeBytes := binary.LittleEndian.AppendUint64(nil, size)
  if _, err := utils.WriteAll(conn, sizeBytes); err != nil {
    return
  }
  // Wait for ok
  if _, err := conn.Read(buf[:1]); err != nil {
    return
  }
  if buf[0] != common.RespOk {
    // TODO: Send something?
    return
  }
  // Open and send file
  f, err := os.Open(path)
  if err != nil {
    writeErr(conn, err)
  }
  defer f.Close()
  if _, err := conn.Write([]byte{common.RespOk}); err != nil {
    return
  }
  io.CopyN(conn, f, int64(size))
}

func writeErr(w io.Writer, err error) error {
  errCode, errMsg := byte(0), ""
  if os.IsNotExist(err) {
    errCode = common.ErrNotExist
  } else {
    errCode, errMsg = common.ErrOther, err.Error()
  }
  if len(errMsg) > 1<<16 - 1 {
    errMsg = errMsg[:1<<16 - 1]
  }
  buf := append(
    append(
      []byte{errCode},
      binary.LittleEndian.AppendUint16(nil, uint16(len(errMsg)))...,
    ),
    errMsg...,
  )
  _, err = w.Write(buf)
  return err
}

func handleFilesRecvClientFiles(conn net.Conn) {
  // Send response
  if _, err := conn.Write([]byte{common.RespOk}); err != nil {
    return
  }

  // Get target path and send response
  buf := make([]byte, 2)
  if _, err := io.ReadFull(conn, buf); err != nil {
    return
  }
  buf = make([]byte, binary.LittleEndian.Uint16(buf))
  if _, err := io.ReadFull(conn, buf); err != nil {
    return
  }
  target := string(buf)
  // Make sure the path exists
  f, err := os.OpenFile(target, os.O_RDONLY|os.O_CREATE, 0644)
  if err != nil {
    if os.IsNotExist(err) {
      conn.Write([]byte{common.ErrNotExist})
    } else {
      conn.Write(append([]byte{common.ErrNotExist}, err.Error()...))
    }
    return
  }
  f.Close()
  if _, err := conn.Write([]byte{common.RespOk}); err != nil {
    return
  }

  //
}

func handleFilesSendClientFiles(conn net.Conn) {
}
