package server

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"syscall"

	chi "github.com/go-chi/chi/v5"
	"github.com/johnietre/gossh/common"
	webs "golang.org/x/net/websocket"
)

func runHttp(ln net.Listener) error {
	r := chi.NewRouter()
	if noProcs {
		r.HandleFunc("/procs/", func(w http.ResponseWriter, r *http.Request) {
			// FIXME: Status code
			http.Error(w, "Server is not running procs", http.StatusNotFound)
		})
	} else {
		r.Group(func(r chi.Router) {
			r.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					println("ok")
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
	}
	if noSsh {
		r.HandleFunc("/ssh/", func(w http.ResponseWriter, r *http.Request) {
			// FIXME: Status code
			http.Error(w, "Server is not running ssh", http.StatusNotFound)
		})
	} else {
		r.Handle("/ssh/ws", webs.Handler(sshWsHandler))
	}

	return http.Serve(ln, r)
}

func sshWsHandler(ws *webs.Conn) {
	handleSshConn(ws).Wait()
	return
}

func getProcsHandler(w http.ResponseWriter, r *http.Request) {
	err := getAndSendProcs(func(procs common.Procs) error {
		return json.NewEncoder(w).Encode(procs)
	})
	if err != nil {
		// TODO
	}
}

func getProcHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := getId(w, r)
	if !ok {
		return
	}
	envVal := strings.ToLower(r.URL.Query().Get("env"))
	getEnv := envVal == "1" || envVal == "true"
	proc := getProc(id, getEnv)
	if proc == nil {
		http.Error(w, "no process with ID "+r.URL.Path, http.StatusNotFound)
		return
	}
	if err := json.NewEncoder(w).Encode(proc); err != nil {
		// TODO
	}
}

func addProcHandler(w http.ResponseWriter, r *http.Request) {
	proc := &common.Process{}
	if err := json.NewDecoder(r.Body).Decode(proc); err != nil {
		// TODO
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := addProc(proc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
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
	if err := signalProc(id, signal); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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
