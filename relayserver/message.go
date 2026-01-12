package relayserver

import "fmt"

type RelayMessageType int

const (
	MsgTypeData RelayMessageType = iota
	MsgTypeJoinSession
	MsgTypeLeaveSession
	MsgTypeReadRequest
	MsgTypeReadResponse
	MsgTypeWriteRequest
	MsgTypeWriteResponse
	MsgTypeSymbolListRequest
	MsgTypeSymbolListResponse
)

func (rmt RelayMessageType) String() string {
	switch rmt {
	case MsgTypeData:
		return "Data"
	case MsgTypeJoinSession:
		return "JoinSession"
	case MsgTypeLeaveSession:
		return "LeaveSession"
	case MsgTypeReadRequest:
		return "ReadRequest"
	case MsgTypeReadResponse:
		return "ReadResponse"
	case MsgTypeWriteRequest:
		return "WriteRequest"
	case MsgTypeWriteResponse:
		return "WriteResponse"
	case MsgTypeSymbolListRequest:
		return "SymbolListRequest"
	case MsgTypeSymbolListResponse:
		return "SymbolListResponse"
	default:
		return fmt.Sprintf("Unknown (%d)", rmt)
	}
}

type Message struct {
	Kind RelayMessageType
	Body any
}

func (m *Message) String() string {
	return fmt.Sprintf("#%d: %q", m.Kind, m.Body)
}

type LogValue struct {
	Name  string
	Value float64
}

type LogValues []LogValue

type DataRequest struct {
	Address uint32
	Length  uint32
	Data    []byte
	Left    uint32
}

func (dr *DataRequest) String() string {
	return fmt.Sprintf("DataRequest{Address: 0x%X, Length: %d, Left: %d, Data: % X}", dr.Address, dr.Length, dr.Left, dr.Data)
}
