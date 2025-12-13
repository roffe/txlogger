package main

import (
	"encoding/gob"
	"flag"
	"log"
	"math/rand/v2"
	"time"

	"github.com/roffe/txlogger/relayserver"
)

var (
	sessionID = "testsession"
)

func init() {
	gob.Register(&relayserver.TestStruct{})
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	flag.StringVar(&sessionID, "session", "testsession", "Session ID to join or host")
	flag.Parse()
}

func main() {
	client, err := relayserver.NewClient("localhost:9000")
	if err != nil {
		log.Fatalf("dial error: %v", err)
	}
	defer client.Close()

	if err := client.JoinSession(sessionID); err != nil {
		log.Fatalf("join session error: %v", err)
	}

	run(client)
}

func run(c *relayserver.Client) {
	go func() {
		for {
			msg := relayserver.Message{
				Kind: relayserver.MsgTypeTest,
				Body: relayserver.TestStruct{
					Foo: "Hello",
					Bar: rand.IntN(1000),
				},
			}
			if err := c.SendMessage(msg); err != nil {
				log.Fatalf("encode error: %v", err)
			}
			//log.Println(msg.String())
			time.Sleep(2 * time.Second)
		}
	}()

	for {
		msg, err := c.ReceiveMessage()
		if err != nil {
			log.Fatalf("decode error: %v", err)
		}
		log.Println("Got reply:", msg.String())
	}
}
