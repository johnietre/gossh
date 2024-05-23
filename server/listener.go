package server

import (
	"errors"
	"io"
	"net"
	"sync/atomic"

	"github.com/johnietre/gossh/common"
	utils "github.com/johnietre/utils/go"
)

type Listener struct {
	net.Listener
	httpChan, tcpChan chan net.Conn
	errVal            *utils.AValue[utils.ErrorValue]
}

func Listen(ntwk, addr string) (*Listener, error) {
	ln, err := net.Listen(ntwk, addr)
	if err != nil {
		return nil, err
	}
	return &Listener{
		Listener: ln,
		httpChan: make(chan net.Conn, 128),
		tcpChan:  make(chan net.Conn, 128),
		errVal:   utils.NewAValue(utils.ErrorValue{}),
	}, nil
}

func (l *Listener) Run() error {
	for {
		conn, err := l.Accept()
		if err != nil {
			println(err.Error())
			swapped := l.errVal.CompareAndSwap(
				utils.ErrorValue{},
				utils.NewErrorValue(err),
			)
			if swapped {
				close(l.httpChan)
				close(l.tcpChan)
			}
			return err
		}
		go l.handle(conn)
	}
}

func (l *Listener) handle(c net.Conn) {
	defer func() {
		// NOTE: Lazy way to handle possiblity of sending on closed chan
		if r := recover(); r != nil {
			c.Close()
		}
	}()
	var buf [8]byte
	if !noTcp {
		if _, err := io.ReadFull(c, buf[:]); err != nil {
			// TODO?
			c.Close()
			return
		}
	} else {
		l.httpChan <- c
		return
	}
	if common.IsTcpInitial(buf[:]) {
		l.tcpChan <- c
	} else {
		l.httpChan <- newHttpConn(c, buf[:])
	}
}

func (l *Listener) Close() error {
	err := l.Listener.Close()
	storeErr := err
	if err == nil {
		storeErr = errors.New("listener closed")
	}
	swapped := l.errVal.CompareAndSwap(
		utils.ErrorValue{},
		utils.NewErrorValue(storeErr),
	)
	if swapped {
		close(l.httpChan)
		close(l.tcpChan)
	}
	return err
}

func (l *Listener) Http() *HttpListener {
	return &HttpListener{
		ln: l,
		ch: l.httpChan,
	}
}

func (l *Listener) Tcp() *TcpListener {
	return &TcpListener{
		ln: l,
		ch: l.tcpChan,
	}
}

type TcpListener struct {
	ln *Listener
	ch chan net.Conn
}

func (tl *TcpListener) Accept() (net.Conn, error) {
	conn, ok := <-tl.ch
	if !ok {
		errVal := tl.ln.errVal.Load()
		if errVal.Error == nil {
			// TODO?
			errVal.Error = errors.New("listener closed")
		}
		return nil, errVal.Error
	}
	return conn, nil
}

func (tl *TcpListener) Close() error {
	return tl.ln.Close()
}

func (tl *TcpListener) Addr() net.Addr {
	return tl.ln.Addr()
}

type HttpListener struct {
	ln *Listener
	ch chan net.Conn
}

func (hl *HttpListener) Accept() (net.Conn, error) {
	conn, ok := <-hl.ch
	if !ok {
		errVal := hl.ln.errVal.Load()
		if errVal.Error == nil {
			// TODO?
			errVal.Error = errors.New("listener closed")
		}
		return nil, errVal.Error
	}
	return conn, nil
}

func (hl *HttpListener) Close() error {
	return hl.ln.Close()
}

func (hl *HttpListener) Addr() net.Addr {
	return hl.ln.Addr()
}

type httpConn struct {
	initial         *utils.Mutex[[]byte]
	finishedInitial atomic.Bool

	net.Conn
}

func newHttpConn(conn net.Conn, initial []byte) *httpConn {
	return &httpConn{
		initial: utils.NewMutex(initial),
		Conn:    conn,
	}
}

func (hc *httpConn) Read(b []byte) (n int, err error) {
	if !hc.finishedInitial.Load() {
		hc.initial.Apply(func(ip *[]byte) {
			initial := *ip
			if len(initial) == 0 {
				hc.finishedInitial.Store(true)
				return
			}
			n = copy(b, initial)
			b, *ip = b[n:], initial[n:]
			if len(initial) == n {
				*ip = nil
				hc.finishedInitial.Store(true)
			}
		})
		if len(b) == 0 {
			return
		}
	}
	nn, err := hc.Conn.Read(b)
	return n + nn, err
}
