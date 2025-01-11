package server

import "fmt"

type MessageType int

func (m MessageType) String() string {
	switch m {
	case MessageTypeRequest:
		return "Request"
	case MessageTypeResponse:
		return "Response"
	default:
		return "Unknown"
	}
}

const (
	MessageTypeRequest MessageType = iota
	MessageTypeResponse
)

type Message struct {
	Type MessageType
	Data []byte
}

func (m *Message) String() string {
	return fmt.Sprintf("Type: %s, Data: %s", m.Type.String(), m.Data)
}
