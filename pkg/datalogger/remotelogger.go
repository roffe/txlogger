package datalogger

import (
	"fmt"
	"log"

	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/relayserver"
)

type RemoteClient struct {
	*BaseLogger

	awaitingReadResult  chan *DataRequest
	awaitingWriteResult chan *DataRequest
}

func NewRemote(cfg Config, lw LogWriter) (IClient, error) {
	return &RemoteClient{
		BaseLogger:          NewBaseLogger(cfg, lw),
		awaitingReadResult:  make(chan *DataRequest, 1),
		awaitingWriteResult: make(chan *DataRequest, 1),
	}, nil
}

func (c *RemoteClient) Start() error {
	defer c.secondTicker.Stop()
	defer c.lw.Close()

	cl, err := relayserver.NewClient(relayserver.SERVER_HOST)
	if err != nil {
		return fmt.Errorf("dial error: %w", err)
	}
	defer cl.Close()

	c.OnMessage("Connected to relay server")

	if err := cl.JoinSession("1337"); err != nil {
		return fmt.Errorf("join session error: %w", err)
	}

	symbols, err := cl.GetSymbolList()
	if err != nil {
		return fmt.Errorf("get symbol list error: %w", err)
	}
	for _, sym := range symbols {
		log.Println(sym.String())
	}

	recvChan := cl.Ch()

	for {
		select {
		case <-c.quitChan:
			c.OnMessage("Stopped logging..")
			return nil
		case <-c.secondTicker.C:
			c.FpsCounter(c.capturePerSecond)
			if c.errPerSecond > 5 {
				return ErrToManyErrors
			}
			c.resetPerSecond()
		case read := <-c.readChan:
			data, err := cl.ReadRAM(read.Address, read.Length)
			if err != nil {
				c.onError()
				read.Complete(err)
				continue
			}
			read.Data = data
			read.Left = 0
			read.Complete(nil)
		case write := <-c.writeChan:
			err := cl.WriteRAM(write.Address, write.Data)
			if err != nil {
				c.onError()
				write.Complete(err)
				continue
			}
			write.Complete(nil)

		case msg := <-recvChan:
			switch msg.Kind {
			case relayserver.MsgTypeData:
				values, ok := msg.Body.(relayserver.LogValues)
				if !ok {
					c.onError()
					c.OnMessage("Invalid data values")
					continue
				}
				for _, va := range values {
					ebus.Publish(va.Name, va.Value)
				}
				c.onCapture()
			default:
				log.Println("Unknown message kind:", msg.Kind.String())
			}
		}
	}
}
