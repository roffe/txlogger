package datalogger

import (
	"fmt"

	"github.com/roffe/txlogger/relayserver"
)

func (bl *BaseLogger) runRelay() error {
	c, err := relayserver.NewClient(relayserver.SERVER_HOST)
	if err != nil {
		return fmt.Errorf("dial error: %w", err)
	}
	bl.OnMessage("Connected to relay server")

	if err := c.JoinSession("1337"); err != nil {
		return fmt.Errorf("join session error: %w", err)
	}

	bl.r = c

	go func() {
		for {
			msg, err := c.Receive()
			if err != nil {
				bl.onError()
				bl.OnMessage("Error reading from relay: " + err.Error())
				return
			}
			switch msg.Kind {
			case relayserver.MsgTypeSymbolListRequest:
				bl.OnMessage("Received symbol list request")
				err := c.Send(relayserver.Message{
					Kind: relayserver.MsgTypeSymbolListResponse,
					Body: bl.Config.Symbols,
				})
				if err != nil {
					bl.onError()
					bl.OnMessage("Error sending symbol list response: " + err.Error())
					continue
				}
			case relayserver.MsgTypeReadRequest:
				read, ok := msg.Body.(*relayserver.DataRequest)
				if !ok {
					bl.onError()
					bl.OnMessage("Invalid read request data")
					continue
				}
				bl.OnMessage(fmt.Sprintf("Remote read request: Addr=0x%X Len=%d", read.Address, read.Length))
				data, err := bl.GetRAM(read.Address, read.Length)
				if err != nil {
					bl.onError()
					bl.OnMessage("Error getting RAM for read request: " + err.Error())
					continue
				}
				if err := c.SendReadResponse(data); err != nil {
					bl.onError()
					bl.OnMessage("Error sending read response: " + err.Error())
					continue
				}
				/*
					read.Data = data
					msg := relayserver.Message{
						Kind: relayserver.MsgTypeReadResponse,
						Body: read,
					}
					if err := c.Send(msg); err != nil {
						bl.onError()
						bl.OnMessage("Error sending read response: " + err.Error())
						continue
					}
				*/
			case relayserver.MsgTypeWriteRequest:
				write, ok := msg.Body.(*relayserver.DataRequest)
				if !ok {
					bl.onError()
					bl.OnMessage("Invalid write request data")
					continue
				}
				bl.OnMessage(fmt.Sprintf("Remote write request: Addr=0x%X Len=%d", write.Address, write.Length))
				err := bl.SetRAM(write.Address, write.Data)
				if err != nil {
					bl.onError()
					bl.OnMessage("Error setting RAM for write request: " + err.Error())
					c.SendWriteResponse(false)
					continue
				}
				if err := c.SendWriteResponse(true); err != nil {
					bl.onError()
					bl.OnMessage("Error sending write response: " + err.Error())
					continue
				}
			}
		}
	}()

	return nil
}
