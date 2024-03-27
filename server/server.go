package server

import (
	"flag"
	"log"
	"os"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

const (
	passwordEnvName = "GOSSH_PASSWORD"
)

var (
	dir string

	passwordHash []byte
	hasPassword  bool
)

func RunServer() {
	log.SetFlags(0)

	addr := flag.String("addr", "127.0.0.1:2222", "Address to run on")
	flag.StringVar(&dir, "dir", ".", "Directory to start processes in")
	flag.Parse()

	password := os.Getenv(passwordEnvName)
	if password != "" {
		err := os.Setenv(passwordEnvName, "")
		if err != nil {
			log.Fatal("error resetting password envvar: ", err)
		}
		passwordHash, err = bcrypt.GenerateFromPassword(
			[]byte(password),
			bcrypt.DefaultCost,
		)
		if err != nil {
			log.Fatal("error setting password: ", err)
		}
		hasPassword = true
	}

	ln, err := Listen("tcp", *addr)
	if err != nil {
		log.Fatal("Error listening: ", err)
	}
	defer ln.Close()
	go func() {
		err := ln.Run()
		if err != nil {
			log.Print("Error running: ", err)
		}
	}()
	log.Print("Listening on ", *addr)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		if err := runTcp(ln.Tcp()); err != nil {
			log.Print("Error running: ", err)
		}
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		if err := runHttp(ln.Http()); err != nil {
			log.Print("Error running: ", err)
		}
		wg.Done()
	}()
	wg.Wait()
}

func checkPassword(pwd []byte) (bool, error) {
	if !hasPassword {
		return len(pwd) == 0, nil
	}
	err := bcrypt.CompareHashAndPassword(passwordHash, pwd)
	if err == bcrypt.ErrMismatchedHashAndPassword || err == bcrypt.ErrHashTooShort {
		return true, nil
	}
	return false, err
}
