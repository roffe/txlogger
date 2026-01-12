package datalogger

import (
	"fmt"
	"log"
	"time"
)

type DataRequest struct {
	Address uint32
	Length  uint32
	Data    []byte
	Left    uint32

	respChan chan error
}

func NewReadDataRequest(address uint32, length uint32) *DataRequest {
	return &DataRequest{
		Address:  address,
		Length:   length,
		Left:     length,
		respChan: make(chan error, 1),
	}
}

func NewWriteDataRequest(address uint32, data []byte) *DataRequest {
	return &DataRequest{
		Address:  address,
		Data:     data,
		Length:   uint32(len(data)),
		Left:     uint32(len(data)),
		respChan: make(chan error, 1),
	}
}

func (r *DataRequest) String() string {
	return fmt.Sprintf("%08X: %d", r.Address, r.Length)
}

func (r *DataRequest) Len() int {
	return int(r.Length)
}

func (r *DataRequest) Complete(err error) {
	//if r := recover(); r != nil {
	//	fmt.Println("Recovered. Error:\n", r)
	//}
	select {
	case r.respChan <- err:
	default:
		log.Println("ReadRequest respChan full")
	}
	close(r.respChan)
}

func (r *DataRequest) Wait() error {
	if r.respChan == nil {
		return fmt.Errorf("respChan is nil")
	}
	select {
	case err := <-r.respChan:
		return err
	case <-time.After(15 * time.Second):
		return fmt.Errorf("RAM read timeout")
	}
}
