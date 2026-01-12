package relayserver

import (
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	symbol "github.com/roffe/ecusymbol"
)

type Client struct {
	conn net.Conn
	dec  *gob.Decoder
	enc  *gob.Encoder

	recvChan chan Message
	sendChan chan Message

	recevierMap map[RelayMessageType]chan Message
	recevierMu  sync.Mutex

	closeOnce sync.Once
	done      chan struct{}
}

func NewClient(host string) (*Client, error) {
	conn, err := net.Dial("tcp", host)
	if err != nil {
		return nil, err
	}
	client := &Client{
		conn:        conn,
		dec:         gob.NewDecoder(conn),
		enc:         gob.NewEncoder(conn),
		recvChan:    make(chan Message, 100),
		sendChan:    make(chan Message, 100),
		recevierMap: make(map[RelayMessageType]chan Message),
		done:        make(chan struct{}),
	}

	go client.sendHandler()
	go client.receiveHandler()

	return client, nil
}

func (c *Client) sendHandler() {
	defer log.Println("exit sendHandler")
	for {
		select {
		case <-c.done:
			return
		case msg := <-c.sendChan:
			err := c.enc.Encode(msg)
			if err != nil {
				log.Println("Error sending message:", err.Error())
				return
			}
		}
	}

}

func (c *Client) receiveHandler() {
	defer log.Println("exit receiveHandler")
	for {
		var msg Message
		err := c.dec.Decode(&msg)
		if err != nil {
			if err != io.EOF {
				log.Println(err.Error())
			}
			close(c.recvChan)
			return
		}
		c.deliverMessage(msg)
	}
}

func (c *Client) deliverMessage(msg Message) {
	c.recevierMu.Lock()
	recvChan, exists := c.recevierMap[msg.Kind]
	c.recevierMu.Unlock()
	if exists {
		select {
		case recvChan <- msg:
		default:
			log.Println("No receiver for message kind", msg.Kind.String())
		}
	} else {
		select {
		case c.recvChan <- msg:
		default:
			log.Println("No receiver for message kind", msg.Kind.String())
		}
	}
}

func (c *Client) JoinSession(sessionID string) error {
	joinMsg := Message{
		Kind: MsgTypeJoinSession,
		Body: sessionID,
	}
	return c.Send(joinMsg)
}

func (c *Client) Send(msg Message) error {
	select {
	case c.sendChan <- msg:
		return nil
	default:
		return fmt.Errorf("send channel full, dropping message")
	}
}

func (c *Client) SendReadResponse(data []byte) error {
	msg := Message{
		Kind: MsgTypeReadResponse,
		Body: data,
	}
	return c.Send(msg)
}

func (c *Client) SendWriteResponse(success bool) error {
	msg := Message{
		Kind: MsgTypeWriteResponse,
		Body: success,
	}
	return c.Send(msg)
}

func (c *Client) Receive() (Message, error) {
	msg, ok := <-c.recvChan
	if !ok {
		return Message{}, fmt.Errorf("receive channel closed")
	}
	return msg, nil
}

func (c *Client) Ch() <-chan Message {
	return c.recvChan
}

func (c *Client) cleanup(kind RelayMessageType) {
	c.recevierMu.Lock()
	delete(c.recevierMap, kind)
	c.recevierMu.Unlock()
}

func (c *Client) ReceiveKind(kind RelayMessageType) (Message, error) {
	recvChan := c.receiveKindCH(kind)
	defer c.cleanup(kind)

	select {
	case msg := <-recvChan:
		return msg, nil
	case <-time.After(4 * time.Second):
		return Message{}, fmt.Errorf("timeout waiting for message of kind %s", kind.String())
	}
}

func (c *Client) GetSymbolList() ([]*symbol.Symbol, error) {
	recvCh := c.receiveKindCH(MsgTypeSymbolListResponse)
	defer c.cleanup(MsgTypeSymbolListResponse)

	err := c.Send(Message{
		Kind: MsgTypeSymbolListRequest,
		Body: nil,
	})
	if err != nil {
		return nil, err
	}
	select {
	case msg := <-recvCh:
		symbols, ok := msg.Body.([]*symbol.Symbol)
		if !ok {
			return nil, fmt.Errorf("invalid symbol list data")
		}
		return symbols, nil
	case <-time.After(4 * time.Second):
		return nil, fmt.Errorf("timeout waiting for symbol list response")
	}
}

func (c *Client) ReadRAM(address uint32, length uint32) ([]byte, error) {
	recvChan := c.receiveKindCH(MsgTypeReadResponse)
	defer c.cleanup(MsgTypeReadResponse)
	err := c.Send(Message{
		Kind: MsgTypeReadRequest,
		Body: DataRequest{
			Address: address,
			Length:  length,
			Left:    length,
		},
	})
	if err != nil {
		return nil, err
	}
	select {
	case msg := <-recvChan:
		data, ok := msg.Body.([]byte)
		if !ok {
			return nil, fmt.Errorf("invalid read response data")
		}
		return data, nil
	case <-time.After(4 * time.Second):
		return nil, fmt.Errorf("timeout waiting for read response")
	}
}

func (c *Client) WriteRAM(address uint32, data []byte) error {
	recvChan := c.receiveKindCH(MsgTypeWriteResponse)
	defer c.cleanup(MsgTypeWriteResponse)
	err := c.Send(Message{
		Kind: MsgTypeWriteRequest,
		Body: DataRequest{
			Address: address,
			Length:  uint32(len(data)),
			Data:    data,
			Left:    uint32(len(data)),
		},
	})
	if err != nil {
		return err
	}
	select {
	case msg := <-recvChan:
		result, ok := msg.Body.(bool)
		if !ok {
			return fmt.Errorf("invalid write response data")
		}
		if !result {
			return fmt.Errorf("write failed")
		}
		return nil
	case <-time.After(4 * time.Second):
		return fmt.Errorf("timeout waiting for write response")
	}
}

func (c *Client) receiveKindCH(kind RelayMessageType) chan Message {
	c.recevierMu.Lock()
	recvChan, exists := c.recevierMap[kind]
	if !exists {
		recvChan = make(chan Message, 10)
		c.recevierMap[kind] = recvChan
	}
	c.recevierMu.Unlock()
	return recvChan
}

func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		close(c.done)
	})
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
