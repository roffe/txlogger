package relayserver

import "fmt"

type RelayMessageType int

const (
	MsgTypeData RelayMessageType = iota
	MsgTypeJoinSession
	MsgTypeLeaveSession
	MsgTypeTest
	MsgTypeReadRequest
	MsgTypeWriteRequest
)

type Message struct {
	Kind RelayMessageType
	Body any
}

func (m *Message) String() string {
	return fmt.Sprintf("#%d: %q", m.Kind, m.Body)
}

type TestStruct struct {
	Foo string
	Bar int
}

type LogValue struct {
	Name  string
	Value float64
}

type LogValues []LogValue
