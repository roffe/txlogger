package relayserver

import (
	"encoding/gob"
	"log"
	"net"
	"sync"

	symbol "github.com/roffe/ecusymbol"
)

const (
	SERVER_HOST = "relay.txlogger.com:9000"
)

func init() {
	gob.Register(LogValues{})
	gob.Register(&DataRequest{})
	gob.Register([]*symbol.Symbol{})

}

type Server struct {
	Sessions  map[string][]*Client
	sessionMu sync.Mutex
}

func New() *Server {
	return &Server{
		Sessions: make(map[string][]*Client),
	}
}

func (s *Server) Run(listenAddr string) error {
	if listenAddr == "" {
		listenAddr = ":9000"
	}
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("listen error: %v", err)
	}
	log.Println("Server listening on", listenAddr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("accept error: %v", err)
			continue
		}
		log.Printf("connection from %s", conn.RemoteAddr().String())
		client := &Client{
			conn:        conn,
			dec:         gob.NewDecoder(conn),
			enc:         gob.NewEncoder(conn),
			sendChan:    make(chan Message, 10),
			recvChan:    make(chan Message, 10),
			recevierMap: make(map[RelayMessageType]chan Message),
		}
		go client.sendHandler()
		go client.receiveHandler()
		go s.handle(client)
	}
}

func (s *Server) SendToSession(c *Client, sessionID string, msg Message) {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	clients, exists := s.Sessions[sessionID]
	if !exists {
		log.Printf("No clients in session %s to send message", sessionID)
		return
	}
	for _, client := range clients {
		if client != c {
			if err := client.Send(msg); err != nil {
				log.Printf("Error sending message to client %s: %v", client.conn.RemoteAddr().String(), err)
			}
		}
	}
}

func (s *Server) AddClient(client *Client, sessionID string) {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	log.Printf("Adding client to session %s", sessionID)
	s.Sessions[sessionID] = append(s.Sessions[sessionID], client)
}

func (s *Server) RemoveClient(client *Client, sessionID string) {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	clients := s.Sessions[sessionID]
	for i, c := range clients {
		if c == client {
			log.Printf("Removing client from session: %s", sessionID)
			s.Sessions[sessionID] = append(clients[:i], clients[i+1:]...)
			break
		}
	}
	if len(s.Sessions[sessionID]) == 0 {
		delete(s.Sessions, sessionID)
	}
}

func (s *Server) handle(c *Client) {
	defer log.Println("exit handle()!!")
	defer c.conn.Close()
	var sessionID string
	for msg := range c.recvChan {
		switch msg.Kind {
		case MsgTypeJoinSession:
			sessId := msg.Body.(string)
			s.AddClient(c, sessId)
			defer s.RemoveClient(c, sessId)
			sessionID = sessId
		default:
			//log.Printf("Received %s from %s", msg.Kind.String(), c.conn.RemoteAddr().String())
			s.SendToSession(c, sessionID, msg)
		}
	}
}
