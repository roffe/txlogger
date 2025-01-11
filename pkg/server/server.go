package server

import (
	"bytes"
	"context"
	"encoding/gob"
	"io"
	"log"
	"net"
	"time"

	"golang.org/x/sync/errgroup"
)

type Server struct {
	l           net.Listener
	connections map[string]*Client
}

type Client struct {
	c net.Conn
}

func New() (*Server, error) {
	s := &Server{
		connections: make(map[string]*Client),
	}
	if err := s.listen(); err != nil {
		return nil, err
	}
	go s.run()
	return s, nil
}

func (s *Server) listen() error {
	var err error
	s.l, err = net.Listen("tcp", ":7777")
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) run() {
	for {
		c, err :=
			s.l.Accept()
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
				continue
			}
			log.Println("Failed to accept connection:", err.Error())
			return
		}
		log.Println("New connection from", c.RemoteAddr())
		s.handleConnection(c)
	}
}

func (s *Server) Close() error {
	return s.l.Close()
}

func (s *Server) handleConnection(c net.Conn) {
	if _, found := s.connections[c.RemoteAddr().String()]; found {
		log.Println("Client already connected")
		c.Close()
		return
	}
	client := &Client{c: c}
	s.connections[c.RemoteAddr().String()] = client
	go s.handleClient(client)
}

func (s *Server) handleClient(c *Client) {
	defer func() {
		log.Println("Client disconnected")
		c.c.Close()
		delete(s.connections, c.c.RemoteAddr().String())
	}()

	enc := gob.NewEncoder(c.c)
	dec := gob.NewDecoder(c.c)

	// errg, ctx := errgroup.WithContext(context.Background())

	cctx, cancel := context.WithCancel(context.Background())

	errg, ctx := errgroup.WithContext(cctx)

	errg.Go(func() error {
		defer cancel()
		for {
			var msg *Message
			err := dec.Decode(&msg)
			if err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
			log.Println("Received message:", msg)
			if bytes.EqualFold(msg.Data, []byte("quit")) {
				return nil
			}
		}
	})

	errg.Go(func() error {
		defer cancel()
		t := time.NewTicker(3 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Println("Client context done")
				return nil
			case <-t.C:
				enc.Encode(&Message{Type: MessageTypeRequest, Data: []byte("ping")})
			}

		}
	})

	//errg.Go(func() error {
	//	for {
	//
	//	}
	//})

	if err := errg.Wait(); err != nil {
		log.Println("Error handling client:", err)
	}
}
