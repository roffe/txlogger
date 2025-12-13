package relayserver

import (
	"encoding/gob"
	"net"
)

type Client struct {
	conn      net.Conn
	dec       *gob.Decoder
	enc       *gob.Encoder
	sessionID string
}

func NewClient(host string) (*Client, error) {
	conn, err := net.Dial("tcp", host)
	if err != nil {
		return nil, err
	}
	client := &Client{
		conn: conn,
		dec:  gob.NewDecoder(conn),
		enc:  gob.NewEncoder(conn),
	}
	return client, nil
}

func (c *Client) JoinSession(sessionID string) error {
	joinMsg := Message{
		Kind: MsgTypeJoinSession,
		Body: sessionID,
	}
	return c.SendMessage(joinMsg)
}

func (c *Client) SendMessage(msg Message) error {
	return c.enc.Encode(msg)
}

func (c *Client) ReceiveMessage() (Message, error) {
	var msg Message
	err := c.dec.Decode(&msg)
	return msg, err
}

func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
