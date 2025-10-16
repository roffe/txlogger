package ipc

import (
	"encoding/gob"
	"errors"
	"flag"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"github.com/roffe/txlogger/pkg/windows"
)

func CreateIPCRouter(mw *windows.MainWindow) Router {
	return Router{
		"ping": func(data string) *Message {
			return &Message{Type: "pong", Data: ""}
		},
		"open": func(filename string) *Message {
			fyne.DoAndWait(mw.Window.RequestFocus)
			if strings.HasSuffix(filename, ".bin") {
				mw.LoadSymbolsFromFile(filename)
			}
			if isLogfile(filename) {
				f, err := os.Open(filename)
				if err != nil {
					mw.Error(err)
				}
				defer f.Close()
				sz := mw.Canvas().Size()
				mw.LoadLogfile(filename, f, fyne.Position{X: sz.Width / 2, Y: sz.Height / 2})
			}
			return nil
		},
	}
}

var logfileExtensions = [...]string{".t5l", ".t7l", ".t8l", ".csv"}

func isLogfile(name string) bool {
	filename := strings.ToLower(name)
	for _, ext := range logfileExtensions {
		if strings.HasSuffix(filename, ext) {
			return true
		}
	}
	return false
}

func sendShow() {
	c, err := dial()
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

func NewServer(router Router) (*Server, error) {
	srv := &Server{
		quit: make(chan struct{}),
		r:    router,
	}

	var err error
	srv.l, err = listen()
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

func ping() bool {
	c, err := dial()
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
