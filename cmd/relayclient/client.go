package main

import (
	"bytes"
	"encoding/gob"
	"io"
	"log"
	"net"
	"time"

	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/server"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
	c, err := net.Dial("tcp", "localhost:7777")
	if err != nil {
		log.Println(err)
		return
	}
	defer c.Close()

	log.Println("Connected to server")

	enc := gob.NewEncoder(c)

	time.Sleep(100 * time.Millisecond)

	err = enc.Encode(&server.Message{Type: server.MessageTypeRequest, Data: []byte("hello")})
	if err != nil {
		log.Println(err)
		return
	}

	var btn *widget.Button
	btn = widget.NewButton("Quit", func() {
		btn.SetText("Quitting...")
	})

	go func() {
		dec := gob.NewDecoder(c)
		for {
			var msg *server.Message
			err := dec.Decode(&msg)
			if err != nil {
				if err == io.EOF {
					log.Println("Server closed connection")
					return
				}
				log.Println(err)
				return
			}
			log.Println("Server:", msg)
			switch {
			case bytes.EqualFold(msg.Data, []byte("ping")):
				err = enc.Encode(&server.Message{Type: server.MessageTypeRequest, Data: []byte("pong")})
				if err != nil {
					log.Println(err)
					return
				}
			}
		}
	}()

	time.Sleep(10 * time.Second)

	err = enc.Encode(&server.Message{Type: server.MessageTypeRequest, Data: []byte("quit")})
	if err != nil {
		log.Println(err)
		return
	}
	time.Sleep(1 * time.Second)

	c.Close()
}
