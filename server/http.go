package server

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"

	chi "github.com/go-chi/chi/v5"
	"github.com/johnietre/gossh/common"
	utils "github.com/johnietre/utils/go"
	webs "golang.org/x/net/websocket"
)

func runHttp(ln net.Listener) error {
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				pwd := []byte(r.Header.Get(common.HttpPasswordHeader))
				if ok, err := checkPassword(pwd); !ok {
					http.Error(w, "password incorrect", http.StatusUnauthorized)
				} else if err != nil {
					log.Print("error checking password: ", err)
					http.Error(
						w,
						"Error checking password",
						http.StatusInternalServerError,
					)
				} else {
					next.ServeHTTP(w, r)
				}
			})
		})
		r.Get("/procs/{id}", getProcHandler)
		r.Get("/procs", getProcsHandler)
		r.Post("/procs", addProcHandler)
		r.Post("/procs/{id}/signal", signalProcHandler)
	})
	r.Handle("/ssh/ws", webs.Handler(sshWsHandler))
	http.Handle("/", r)

	return http.Serve(ln, nil)
}

func sshWsHandler(ws *webs.Conn) {
	handle(ws)
}

var (
	procs  = utils.NewRWMutex[common.Procs](common.Procs{})
	procId atomic.Uint64
)

func nextProcId(procs []*common.Process) uint64 {
	for {
		id := procId.Add(1)
		if id == 0 {
			procId.Add(1)
		}
		ok := true
		for _, proc := range procs {
			if proc.Id == id {
				ok = false
				break
			}
		}
		if ok {
			return id
		}
	}
}

func getProcsHandler(w http.ResponseWriter, r *http.Request) {
	p := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(p, "/")
	if len(parts) == 2 {
		r.URL.Path = parts[1]
		getProcHandler(w, r)
		return
	} else if len(parts) != 1 {
		http.Error(w, "bad path", http.StatusNotFound)
		return
	}
	procs.RApply(func(pp *common.Procs) {
		if err := json.NewEncoder(w).Encode(*pp); err != nil {
			// TODO
		}
	})
}

func getProcHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := getId(w, r)
	if !ok {
		return
	}
	envVal := strings.ToLower(r.URL.Query().Get("env"))
	getEnv := envVal == "1" || envVal == "true"
	procs.RApply(func(pp *common.Procs) {
		for _, proc := range *pp {
			if proc.Id == id {
				if getEnv {
					proc.Env = proc.CmdEnv()
				}
				if err := json.NewEncoder(w).Encode(proc); err != nil {
					// TODO
				}
				return
			}
		}
		http.Error(w, "no process with ID "+r.URL.Path, http.StatusNotFound)
	})
}

func addProcHandler(w http.ResponseWriter, r *http.Request) {
	proc := &common.Process{}
	if err := json.NewDecoder(r.Body).Decode(proc); err != nil {
		// TODO
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	procs.Apply(func(pp *common.Procs) {
		proc.Id = nextProcId(*pp)
	})
	proc.Run(procs)
	if err := json.NewEncoder(w).Encode(proc); err != nil {
		// TODO
	}
}

func signalProcHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := getId(w, r)
	if !ok {
		return
	}
	signalStr := r.URL.Query().Get("signal")
	signalInt, err := strconv.Atoi(signalStr)
	if err != nil {
		return
	}
	signal := syscall.Signal(signalInt)
	procs.RApply(func(pp *common.Procs) {
		for _, proc := range *pp {
			if proc.Id == id {
				if err := proc.Signal(signal); err != nil {
					// TODO: Error code?
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
				return
			}
		}
	})
}

func getId(w http.ResponseWriter, r *http.Request) (uint64, bool) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid ID", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}
