package ipc

import (
	"encoding/gob"
	"errors"
	"flag"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

func IsRunning(socketFile string) bool {
	if fileExists(socketFile) {
		if !ping(socketFile) {
			log.Println("txlogger is not running, removing stale socket file")
			if err := os.Remove(socketFile); err != nil {
				log.Printf("failed to remove stale socket file: %v", err)
			}
		} else {
			log.Println("txlogger is running, sending show request over socket")
			sendShow(socketFile)
			return true
		}
	}
	return false
}

func sendShow(socketFile string) {
	c, err := net.Dial("unix", socketFile)
	if err != nil {
		var nErr *net.OpError
		if errors.As(err, &nErr) {
			if nErr.Op == "dial" {
				log.Println("txlogger is not running")
				return
			}
		}
		log.Println("failed to send show request:", err)
		return
	}
	defer c.Close()
	enc := gob.NewEncoder(c)
	if filename := flag.Arg(0); filename != "" {
		err = enc.Encode(Message{Type: "open", Data: filename})
	} else {
		err = enc.Encode(Message{Type: "open", Data: ""})
	}
	if err != nil {
		log.Println(err)
	}
}

type Router map[string]CommandHandler
type CommandHandler func(string) *Message

type Message struct {
	Type string
	Data string
}

func (m Message) String() string {
	return m.Type + ": " + m.Data
}

type Server struct {
	quit      chan struct{}
	l         net.Listener
	r         Router
	closeOnce sync.Once
}

func NewServer(router Router, socketFile string) (*Server, error) {
	srv := &Server{
		quit: make(chan struct{}),
		r:    router,
	}

	var err error
	srv.l, err = net.Listen("unix", socketFile)
	if err != nil {
		return nil, err
	}

	go ipcHandler(srv)

	return srv, nil
}

func (srv *Server) Close() {
	srv.closeOnce.Do(func() {
		close(srv.quit)
		srv.l.Close()
	})
}

func ipcHandler(srv *Server) {
	for {
		conn, err := srv.l.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			log.Println(err)
			return
		}
		go handleConn(conn, srv.r)
	}
}

func handleConn(conn net.Conn, r Router) {
	defer conn.Close()
	gb := gob.NewDecoder(conn)
	ge := gob.NewEncoder(conn)

	var msg Message
	err := gb.Decode(&msg)
	if err != nil {
		if err == io.EOF {
			return
		}
		log.Println(err)
		return
	}

	log.Println(msg)

	handler, ok := r[msg.Type]
	if ok {
		if msg := handler(msg.Data); msg != nil {
			if err := ge.Encode(*msg); err != nil {
				log.Println(err)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func ping(socketFile string) bool {
	c, err := net.Dial("unix", socketFile)
	if err != nil {
		var nErr *net.OpError
		if errors.As(err, &nErr) {
			if nErr.Op == "dial" {
				return false
			}
		}
		log.Println(err)
		return false
	}
	defer c.Close()

	gdec := gob.NewDecoder(c)
	gb := gob.NewEncoder(c)

	err = gb.Encode(Message{Type: "ping", Data: ""})
	if err != nil {
		log.Println(err)
		return false
	}

	var msg Message
	err = gdec.Decode(&msg)
	if err != nil {
		log.Println(err)
		return false
	}

	if msg.Type == "pong" {
		return true
	}

	return false
}

func fileExists(name string) bool {
	_, err := os.Stat(name)
	return !os.IsNotExist(err)
}
