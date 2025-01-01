package ipc

import "net"

type Message struct {
	Type string
	Data string
}

func (m Message) String() string {
	return m.Type + ": " + m.Data
}

type Server struct {
	quit chan struct{}
	l    net.Listener
}

type CommandHandler func(string)

func NewServer(router map[string]CommandHandler) (*Server, error) {
	srv := &Server{
		quit: make(chan struct{}),
	}

	var err error
	srv.l, err = net.Listen("unix", "txlogger.sock")
	if err != nil {

	}
	go srv.start()

	return srv, nil
}

func (s *Server) start() {

}
