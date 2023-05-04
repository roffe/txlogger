package kwp2000

import (
	"bytes"
	"context"
	"errors"
	"log"
	"math"
	"time"

	"github.com/roffe/gocan"
)

type Client struct {
	c *gocan.Client
	//canID             uint32
	//recvID            []uint32

	responseID        uint32
	defaultTimeout    time.Duration
	gotSequrityAccess bool
}

type KWPRequest struct {
}

type KWPReply struct {
}

func New(c *gocan.Client /*canID uint32, recvID ...uint32*/) *Client {
	return &Client{
		c: c,
		//canID:          canID,
		//recvID:         recvID,

		defaultTimeout: 250 * time.Millisecond,
	}
}

func (t *Client) StartSession(ctx context.Context, id, responseID uint32) error {
	payload := []byte{0x3F, START_COMMUNICATION, 0x00, 0x11, byte(REQ_MSG_ID >> 8), byte(REQ_MSG_ID), 0x00, 0x00}
	frame := gocan.NewFrame(id, payload, gocan.ResponseRequired)
	resp, err := t.c.SendAndPoll(ctx, frame, t.defaultTimeout, responseID)
	if err != nil {
		return err
	}

	data := resp.Data()
	if data[3] != START_COMMUNICATION|0x40 {
		return TranslateErrorCode(GENERAL_REJECT)
	}

	t.responseID = uint32(data[6])<<8 | uint32(data[7])

	//log.Printf("ECU reports responseID: 0x%03X", t.responseID)
	//log.Println(resp.String())
	return nil
}

func (t *Client) StopSession(ctx context.Context, id uint32) error {
	payload := []byte{0x3F, STOP_COMMUNICATION, 0x00, 0x11, byte(REQ_MSG_ID >> 8), byte(REQ_MSG_ID), 0x00, 0x00}
	frame := gocan.NewFrame(REQ_MSG_ID, payload, gocan.ResponseRequired)
	return t.c.Send(frame)
}

func (t *Client) ReadDataByLocalIdentifier(ctx context.Context, id byte) ([]byte, error) {
	frame := gocan.NewFrame(REQ_MSG_ID, []byte{0x40, 0xA1, 0x02, READ_DATA_BY_LOCAL_IDENTIFIER, 0xF0, 0x00, 0x00, 0x00}, gocan.ResponseRequired)
	resp, err := t.c.SendAndPoll(ctx, frame, 50*time.Millisecond, t.responseID)
	if err != nil {
		return nil, err
	}
	out := bytes.NewBuffer(nil)

	d := resp.Data()
	dataLenLeft := d[2] - 2
	//log.Println(resp.String())
	//log.Printf("data len left: %d", dataLenLeft)

	var thisRead byte
	if dataLenLeft > 3 {
		thisRead = 3
	} else {
		thisRead = dataLenLeft
	}

	out.Write(d[5 : 5+thisRead])
	dataLenLeft -= thisRead

	//log.Printf("data len left: %d", dataLenLeft)
	//log.Println(resp.String())

	currentChunkNumber := d[0] & 0x3F

	for currentChunkNumber != 0 {
		//log.Printf("current chunk %02X", currentChunkNumber)
		frame := gocan.NewFrame(RESP_CHUNK_CONF_ID, []byte{0x40, 0xA1, 0x3F, d[0] &^ 0x40, 0x00, 0x00, 0x00, 0x00}, gocan.ResponseRequired)
		//log.Println(frame.String())
		resp, err := t.c.SendAndPoll(ctx, frame, 450*time.Millisecond, t.responseID)
		if err != nil {
			return nil, err
		}
		d = resp.Data()

		toRead := uint8(math.Min(6, float64(dataLenLeft)))
		//log.Println("bytes to read", toRead)
		out.Write(d[2 : 2+toRead])
		dataLenLeft -= toRead
		//log.Printf("data len left: %d", dataLenLeft)
		currentChunkNumber = d[0] & 0x3F
		//log.Printf("next chunk %02X", currentChunkNumber)
	}

	return out.Bytes(), nil
}

func (t *Client) DynamicallyDefineLocalIdRequest(ctx context.Context, id int, v *VarDefinition) error {
	buff := bytes.NewBuffer(nil)

	buff.WriteByte(0xF0)

	switch v.Method {
	case VAR_METHOD_ADDRESS:
		buff.Write([]byte{0x03, byte(id), uint8(v.Length), byte(v.Value >> 16), byte(v.Value >> 8), byte(v.Value)})
	case VAR_METHOD_LOCID:
		buff.Write([]byte{0x01, byte(id), 0x00, byte(v.Value), 0x00})
	case VAR_METHOD_SYMBOL:
		buff.Write([]byte{0x03, byte(id), 0x00, 0x80, byte(v.Value >> 8), byte(v.Value)})
	}

	message := append([]byte{byte(buff.Len()), DYNAMICALLY_DEFINE_LOCAL_IDENTIFIER}, buff.Bytes()...)

	for _, msg := range t.splitRequest(message) {
		//log.Println(msg.String())
		if msg.Type().Type == 1 {
			if err := t.c.Send(msg); err != nil {
				log.Println(err)
				return err
			}
		} else {
			resp, err := t.c.SendAndPoll(ctx, msg, t.defaultTimeout, REQ_CHUNK_CONF_ID)
			if err != nil {
				log.Println(err)
				return err
			}
			if err := TranslateErrorCode(resp.Data()[3+2]); err != nil {
				return err
			}
			//log.Println(resp.String())
		}
	}

	return nil
}

func (t *Client) RequestSecurityAccess(ctx context.Context, force bool) (bool, error) {
	if t.gotSequrityAccess && !force {
		return true, nil
	}
	for i := 1; i <= 2; i++ {
		ok, err := t.requestSecurityAccessLevel(ctx, i)
		if err != nil {
			return false, err
		}
		if ok {
			t.gotSequrityAccess = true

			break
		}
	}

	return false, errors.New("security access was not granted")
}

func (t *Client) requestSecurityAccessLevel(ctx context.Context, method int) (bool, error) {
	log.Println("requestSecurityAccessLevel", method)

	return false, nil
}

func (t *Client) SendRequest(req *KWPRequest) (*KWPReply, error) {
	return nil, nil
}

func (t *Client) splitRequest(payload []byte) []gocan.CANFrame {
	msgCount := (len(payload) + 6 - 1) / 6

	var results []gocan.CANFrame

	for i := 0; i < msgCount; i++ {
		msgData := make([]byte, 8)

		flag := 0

		if i == 0 {
			flag |= 0x40 // this is the first data chunk
		}

		if i != msgCount-1 {
			flag |= 0x80 // we want confirmation for every chunk except the last one
		}
		msgData[0] = (byte)(flag | ((msgCount - i - 1) & 0x3F)) // & 0x3F is not necessary, only to show that this field is 6-bit wide
		msgData[1] = 0xA1

		start := 6 * i
		var count int
		if len(payload)-start < 6 {
			count = len(payload) - start
		} else {
			count = 6
		}

		copy(msgData[2:], payload[start:start+count])
		for j := 0; j < count; j++ {
			msgData[2+j] = payload[start+j]
		}

		if flag&0x80 == 0x80 {
			results = append(results, gocan.NewFrame(REQ_MSG_ID, msgData, gocan.ResponseRequired))
		} else {
			results = append(results, gocan.NewFrame(REQ_MSG_ID, msgData, gocan.Outgoing))
		}

	}

	return results
}
