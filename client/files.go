package client

import (
	"encoding/binary"
	"fmt"
	"net"
	"path/filepath"

	//"fmt"
	"io"
	"log"
	"os"

	"github.com/johnietre/gossh/common"
	utils "github.com/johnietre/utils/go"
	"github.com/spf13/cobra"
)

func getFilesCmd() *cobra.Command {
	cmd := &cobra.Command{
    //Use:   "files <ADDR:/path/to/file> </path/to/local>",
    Use:   "files <SOURCE...> <TARGET>",
		Short: "Run SSH client",
		Long:  "Connect to a gossh instance acting as an SSH server. The address can either be passed as a CLI arg or is gotten from the value of the " + common.AddrEnvName + " environment variable.",
		Args:  cobra.MinimumNArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
      //addr := cmd.Flags().Arg(0)
      recursive, _ := cmd.Flags().GetBool("r")
      sources, target := args[:len(args)-1], args[len(args)-1]
      checkPaths(sources, recursive)
			runFiles(target, sources...)
		},
	}
	flags := cmd.Flags()
  flags.BoolP("", "r", false, "Allow directories")
	flags.BoolVar(&useWs, "ws", false, "Use WebSocket instead of plain TCP")
	return cmd
}

// TODO: Directories
// TODO: Send files

func runFiles(target string, sources ...string) {
  addr := getPathAddr(target)
  if addr != "" {
    return
  }
  addr = getPathAddr(sources[0])
  if addr == "" {
    log.Fatal("Missing address")
  }
  for _, source := range sources[1:] {
    if a := getPathAddr(source); a != addr {
      log.Fatalf("Expected address '%s', found '%s'", addr, a)
    }
  }
}

func runFilesToServer(addr string, targetPath string, sources ...string) {
  conn, err := dialConn(addr, common.TcpFiles)
  if err != nil {
    log.Fatal("Error connecting: ", err)
  }
  defer conn.Close()

  // Send intent and get response
  if _, err := conn.Write([]byte{common.HeaderSendFiles}); err != nil {
    log.Fatal("Error sending intent: ", err)
  }
  buf := make([]byte, 1)
  if _, err := conn.Read(buf); err != nil {
    log.Fatal("Error reading intent response: ", err)
  } else if buf[0] != common.RespOk {
    // TODO
  }

  // Send target path and get response
  buf = encodePath(targetPath)
  if _, err := utils.WriteAll(conn, buf); err != nil {
    log.Fatal("Error sending target: ", err)
  }
  if _, err := conn.Read(buf[:1]); err != nil {
    log.Fatal("Error reading target response: ", err)
  } else if buf[0] != common.RespOk {
    // TODO
  }

  // Send files
  for _, source := range sources {
    f, err := os.Open(source)
    if err != nil {
      log.Fatalf("Error opening '%s': %v", source, err)
    }
    info, err := f.Stat()
    if err != nil {
      log.Fatalf("Error getting info for %s: %v", source, err)
    }
    if info.IsDir() {
      f.Close()
      if err := sendDir(conn, source); err != nil {
        log.Fatal(err)
      }
    } else {
      if err := sendFile(conn, f, source); err != nil {
        log.Fatal(err)
      }
    }
    if _, err := conn.Read(buf[:1]); err != nil {
      // TODO
      log.Fatal("Error reading sending response: ", err)
    } else if buf[0] != common.RespOk {
      // TODO
    }
  }
}

func sendDir(conn net.Conn, source string) error {
  return filepath.WalkDir(source, func(path string, d os.DirEntry, err error) error {
    if err != nil {
      return err
    }
    if d.IsDir() {
      // TODO?
      return nil
    }
    path = filepath.Join(source, path)
    f, err := os.Open(path)
    if err != nil {
      return fmt.Errorf("Error opening %s: %v", path, err)
    }
    return sendFile(conn, f, path)
  })
}

func sendFile(conn net.Conn, f *os.File, path string) error {
  info, err := f.Stat()
  if err != nil {
    return fmt.Errorf("Error getting info for %s: %v", path, err)
  }
  // Send size
  sizeBytes := binary.LittleEndian.AppendUint64(nil, uint64(info.Size()))
  if _, err := utils.WriteAll(conn, sizeBytes); err != nil {
    return fmt.Errorf("Error sending size of %s: %v", path, err)
  }
  // Send path
  if _, err := utils.WriteAll(conn, encodePath(path)); err != nil {
    return fmt.Errorf("Error sending name %s: %v", path, err)
  }
  // Send file
  if _, err := io.CopyN(conn, f, info.Size()); err != nil {
    return fmt.Errorf("Error sending %s: %v", path, err)
  }
  return nil
}

func runFilesFromServer(addr string, target string, sourcePaths ...string) {
  conn, err := dialConn(addr, common.TcpFiles)
  if err != nil {
    log.Fatal("Error connecting: ", err)
  }
  defer conn.Close()

  // Send path len
  pathLen := uint16(len(path))
  buf := append(
    binary.LittleEndian.AppendUint16(nil, pathLen),
    path...,
  )
  if _, err := utils.WriteAll(conn, buf); err != nil {
    log.Fatal("Error sending path: ", err)
  }
  // Get size
  buf = make([]byte, 8)
  if _, err := io.ReadFull(conn, buf); err != nil {
    log.Fatal("Error getting size: ", err)
  }
  // TODO: check size is ok
  // Send resp
  if _, err := conn.Write([]byte{common.RespOk}); err != nil {
    log.Fatal("Error writing response: ", err)
  }
  // Wait for resp
  if _, err := conn.Read(buf[:1]); err != nil {
    log.Fatal("Error reading response: ", err)
  }
  switch buf[0] {
  case common.RespOk:
  case common.ErrNotExist:
    log.Fatal("Path on server does not exist")
  case common.ErrOther:
    readErrOther(conn, "Server error: ")
  default:
    log.Fatal("Received unknown response: ", buf[0])
  }
  // TODO
}

func readErrOther(r io.Reader, prompt string) {
  buf := make([]byte, 2)
  if _, err := io.ReadFull(r, buf); err != nil {
    log.Fatal("Error reading error: ", err)
  }
  l := int(binary.LittleEndian.Uint16(buf))
  buf = make([]byte, l)
  if _, err := io.ReadFull(r, buf); err != nil {
    log.Fatal("Error reading error: ", err)
  }
  log.Fatal(prompt, string(buf))
}

func checkPaths(paths []string, recursive bool) {
  nameMap := make(map[string]string)
  for _, path := range paths {
    info, err := os.Lstat(path)
    if err != nil {
      log.Fatal("Error reading path:", err)
    }
    if !recursive && info.IsDir() {
      log.Fatalf("'%s' is a directory", path)
    }
    name := filepath.Base(path)
    if other := nameMap[name]; other != "" {
      log.Fatalf("Duplicate names: '%s' and '%s'", other, path)
    }
    nameMap[name] = path
  }
}

func getPathAddr(target string) string {
  return ""
}

func encodePath(path string) []byte {
  return append(
    binary.LittleEndian.AppendUint16(nil, uint16(len(path))),
    path...,
  )
}
