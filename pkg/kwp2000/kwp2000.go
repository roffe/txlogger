package kwp2000

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"math"
	"runtime"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/debug"
)

var (
	INIT_MSG_ID        uint32 = 0x222
	REQ_MSG_ID         uint32 = 0x242
	INIT_RESP_ID       uint32 = 0x238
	REQ_CHUNK_CONF_ID  uint32 = 0x270
	RESP_CHUNK_CONF_ID uint32 = 0x266
)

const (
	NORMAL_MODE = 0
	DEBUG_MODE  = 1
	SILENT_MODE = 2
)

type Client struct {
	c                 *gocan.Client
	responseID        uint32
	defaultTimeout    time.Duration
	gotSequrityAccess bool
}

func New(c *gocan.Client /*canID uint32, recvID ...uint32*/) *Client {
	return &Client{
		c: c,
		//canID:          canID,
		//recvID:         recvID,
		defaultTimeout: 200 * time.Millisecond,
	}
}

func (t *Client) StartSession(ctx context.Context, id, responseID uint32) error {
	payload := []byte{0x3F, START_COMMUNICATION, 0x00, 0x11, byte(REQ_MSG_ID >> 8), byte(REQ_MSG_ID), 0x00, 0x00}
	frame := gocan.NewFrame(id, payload, gocan.ResponseRequired)
	resp, err := t.c.SendAndWait(ctx, frame, 3*time.Second, responseID)
	if err != nil {
		return fmt.Errorf("StartSession: %w", err)
	}

	if resp.Data[3] != START_COMMUNICATION|0x40 {
		return fmt.Errorf("StartSession: %w", TranslateErrorCode(GENERAL_REJECT))
	}

	t.responseID = uint32(resp.Data[6])<<8 | uint32(resp.Data[7])

	//log.Printf("ECU reports responseID: 0x%03X", t.responseID)
	//log.Println(resp.String())
	return nil
}

func (t *Client) StopSession(ctx context.Context) error {
	return t.c.Send(REQ_MSG_ID, []byte{0x40, 0xA1, 0x02, STOP_COMMUNICATION}, gocan.ResponseRequired)
}

func (t *Client) TesterPresent(ctx context.Context) error {
	payload := []byte{0x40, 0xA1, 0x01, TESTER_PRESENT}
	frame := gocan.NewFrame(REQ_MSG_ID, payload, gocan.ResponseRequired)
	resp, err := t.c.SendAndWait(ctx, frame, t.defaultTimeout, t.responseID)
	if err != nil {
		return fmt.Errorf("TesterPresent: %w", err)
	}
	//	log.Println(resp)
	return checkErr(resp)
}

func (t *Client) StartRoutineByIdentifier(ctx context.Context, id byte, extra ...byte) error {
	payload := []byte{0x40, 0xA1, 0x03, START_ROUTINE_BY_IDENTIFIER, id}
	payload = append(payload, extra...)
	payload[2] = byte(len(payload) - 3)
	frame := gocan.NewFrame(REQ_MSG_ID, payload, gocan.ResponseRequired)
	resp, err := t.c.SendAndWait(ctx, frame, t.defaultTimeout*2, t.responseID)
	if err != nil {
		return fmt.Errorf("StartRoutineByIdentifier: %w", err)
	}
	return checkErr(resp)
}

func (t *Client) StopRoutineByIdentifier(ctx context.Context, id byte) ([]byte, error) {
	frame := gocan.NewFrame(REQ_MSG_ID, []byte{0x40, 0xA1, 0x02, STOP_ROUTINE_BY_IDENTIFIER, id}, gocan.ResponseRequired)
	log.Println(frame.String())
	resp, err := t.c.SendAndWait(ctx, frame, t.defaultTimeout, t.responseID)
	if err != nil {
		return nil, fmt.Errorf("StopRoutineByIdentifier: %w", err)
	}
	log.Println(resp.String())
	return resp.Data, checkErr(resp)
}

func (t *Client) RequestRoutineResultsByLocalIdentifier(ctx context.Context, id byte) ([]byte, error) {
	frame := gocan.NewFrame(REQ_MSG_ID, []byte{0x40, 0xA1, 0x02, REQUEST_ROUTINE_RESULTS_BY_LOCAL_IDENTIFIER, id}, gocan.ResponseRequired)
	log.Println(frame.String())
	resp, err := t.c.SendAndWait(ctx, frame, t.defaultTimeout, t.responseID)
	if err != nil {
		return nil, fmt.Errorf("RequestRoutineResultsByLocalIdentifier: %w", err)
	}

	log.Println(resp.String())
	return resp.Data, checkErr(resp)
}

func (t *Client) ReadDataByLocalIdentifierMode(ctx context.Context, id, mode byte) ([]byte, error) {
	frame := gocan.NewFrame(REQ_MSG_ID, []byte{0x40, 0xA1, 0x03, READ_DATA_BY_IDENTIFIER, id, mode}, gocan.ResponseRequired)
	log.Println(frame.String())
	resp, err := t.c.SendAndWait(ctx, frame, t.defaultTimeout, t.responseID)
	if err != nil {
		return nil, fmt.Errorf("ReadDataByLocalIdentifier2: %w", err)
	}

	out := bytes.NewBuffer(nil)

	log.Println(resp.String())

	if err := checkErr(resp); err != nil {
		return nil, err
	}

	dataLenLeft := resp.Data[2] - 2
	//log.Println(resp.String())
	//log.Printf("data len left: %d", dataLenLeft)

	var thisRead byte
	if dataLenLeft > 3 {
		thisRead = 3
	} else {
		thisRead = dataLenLeft
	}

	out.Write(resp.Data[5 : 5+thisRead])
	dataLenLeft -= thisRead

	//log.Printf("data len left: %d", dataLenLeft)
	//log.Println(resp.String())

	currentChunkNumber := resp.Data[0] & 0x3F

	for currentChunkNumber != 0 {
		//log.Printf("current chunk %02X", currentChunkNumber)
		frame := gocan.NewFrame(RESP_CHUNK_CONF_ID, []byte{0x40, 0xA1, 0x3F, resp.Data[0] &^ 0x40, 0x00, 0x00, 0x00, 0x00}, gocan.ResponseRequired)
		//log.Println(frame.String())
		resp, err = t.c.SendAndWait(ctx, frame, t.defaultTimeout, t.responseID)
		if err != nil {
			return nil, err
		}
		toRead := uint8(math.Min(6, float64(dataLenLeft)))
		//log.Println("bytes to read", toRead)
		out.Write(resp.Data[2 : 2+toRead])
		dataLenLeft -= toRead
		//log.Printf("data len left: %d", dataLenLeft)
		currentChunkNumber = resp.Data[0] & 0x3F
		//log.Printf("next chunk %02X", currentChunkNumber)
	}

	return out.Bytes(), nil
}

func (client *Client) ReadDataByIdentifier(ctx context.Context, identifier byte) ([]byte, error) {
	initialFrame := gocan.NewFrame(REQ_MSG_ID, []byte{0x40, 0xA1, 0x02, READ_DATA_BY_IDENTIFIER, identifier}, gocan.ResponseRequired)
	resp, err := client.c.SendAndWait(ctx, initialFrame, client.defaultTimeout, client.responseID)
	if err != nil {
		return nil, err // Directly return the error without additional wrapping
	}

	if err := checkErr(resp); err != nil {
		return nil, err
	}

	dataLength := resp.Data[2] - 2
	chunkSize := min(dataLength, 3) // Simplify conditional assignment using a min function

	output := bytes.NewBuffer(resp.Data[5 : 5+chunkSize])
	dataLength -= chunkSize

	currentChunkNumber := resp.Data[0] & 0x3F

	for currentChunkNumber != 0 {
		nextFrame := gocan.NewFrame(RESP_CHUNK_CONF_ID, []byte{0x40, 0xA1, 0x3F, resp.Data[0] &^ 0x40}, gocan.ResponseRequired)
		resp, err = client.c.SendAndWait(ctx, nextFrame, client.defaultTimeout, client.responseID)
		if err != nil {
			return nil, err
		}
		bytesToRead := uint8(min(6, dataLength))
		output.Write(resp.Data[2 : 2+bytesToRead])
		dataLength -= bytesToRead
		currentChunkNumber = resp.Data[0] & 0x3F
		//time.Sleep(5 * time.Millisecond) // Add a small delay to avoid overwhelming the ECU
	}

	return output.Bytes(), nil
}

/*
	func (t *Client) ReadDataByIdentifier3(ctx context.Context, id byte) ([]byte, error) {
		frame := gocan.NewFrame(REQ_MSG_ID, []byte{0x40, 0xA1, 0x02, READ_DATA_BY_IDENTIFIER, id}, gocan.ResponseRequired)
		resp, err := t.c.SendAndWait(ctx, frame, t.defaultTimeout, t.responseID)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", getFunctionName(), err)
		}

		databuff := resp.Data()
		if err := checkErr(resp); err != nil {
			return nil, err
		}

		dataLenLeft := databuff[2] - 2
		//log.Println(resp.String())
		//log.Printf("data len left: %d", dataLenLeft)

		var thisRead byte
		if dataLenLeft > 3 {
			thisRead = 3
		} else {
			thisRead = dataLenLeft
		}

		out := bytes.NewBuffer(databuff[5 : 5+thisRead])
		dataLenLeft -= thisRead

		//log.Printf("data len left: %d", dataLenLeft)
		//log.Println(resp.String())

		currentChunkNumber := databuff[0] & 0x3F

		for currentChunkNumber != 0 {
			//log.Printf("current chunk %02X", currentChunkNumber)
			frame := gocan.NewFrame(RESP_CHUNK_CONF_ID, []byte{0x40, 0xA1, 0x3F, databuff[0] &^ 0x40}, gocan.ResponseRequired)
			//log.Println(frame.String())
			resp, err := t.c.SendAndWait(ctx, frame, t.defaultTimeout, t.responseID)
			if err != nil {
				return nil, err
			}

			databuff = resp.Data()

			toRead := uint8(math.Min(6, float64(dataLenLeft)))
			//log.Println("bytes to read", toRead)
			out.Write(databuff[2 : 2+toRead])
			dataLenLeft -= toRead
			//log.Printf("data len left: %d", dataLenLeft)
			currentChunkNumber = databuff[0] & 0x3F
			//log.Printf("next chunk %02X", currentChunkNumber)
		}
		return out.Bytes(), nil
	}
*/
func (t *Client) TransferData(ctx context.Context, length uint32) ([]byte, error) {
	buff := bytes.NewBuffer(nil)
outer:
	for {
		b, err := t.transferData(ctx)
		if err != nil {
			//if err.Error() == "incorrect byte count during block transfer" {
			//	return buff.Bytes(), nil
			//}
			return nil, fmt.Errorf("TransferData: %w", err)
		}
		// C0 BF 04 76 31 50 7600
		//log.Printf("transfer data: %X, size: %d", b, b[2])
		//fmt.Printf("%X\n", b)

		toRead := b[2]
		//		log.Printf("toRead %d, %02X", toRead, b[0])
		if toRead >= 5 {
			buff.WriteByte(b[7])
			toRead -= 5
		}

		if b[0] == 0x80 || b[0] == 0xC0 {
			t.Ack(b[0], gocan.Outgoing)
			break
		}

		sub := t.c.Subscribe(ctx, 0x258)
		defer sub.Close()
		if err := t.Ack(b[0], gocan.ResponseRequired); err != nil {
			return nil, err
		}
		for toRead > 0 {
			select {
			case f := <-sub.Chan():
				// log.Printf("toRead %d, %X", toRead, d)
				readThis := int(min(6, toRead))
				buff.Write(f.Data[2 : 2+readThis])
				toRead -= byte(readThis)
				if f.Data[0] == 0x80 || f.Data[0] == 0xC0 {
					t.Ack(f.Data[0], gocan.Outgoing)
				} else {
					t.Ack(f.Data[0], gocan.ResponseRequired)
				}
				if buff.Len() == int(length) {
					break outer
				}
			case <-time.After(250 * time.Millisecond):
				return nil, fmt.Errorf("timeout")
			}
		}
	}
	return buff.Bytes(), nil
}

func (t *Client) transferData(ctx context.Context) ([]byte, error) {
	frame := gocan.NewFrame(REQ_MSG_ID, []byte{0x40, 0xA1, 0x01, TRANSFER_DATA}, gocan.ResponseRequired)
	//	log.Println(frame.String())
	resp, err := t.c.SendAndWait(ctx, frame, t.defaultTimeout, t.responseID)
	if err != nil {
		return nil, fmt.Errorf("transferData: %w", err)
	}
	return resp.Data, checkErr(resp)
}

func (t *Client) RequestTransferExit(ctx context.Context) error {
	frame := gocan.NewFrame(REQ_MSG_ID, []byte{0x40, 0xA1, 0x01, REQUEST_TRANSFER_EXIT}, gocan.ResponseRequired)
	//	log.Println(frame.String())
	resp, err := t.c.SendAndWait(ctx, frame, t.defaultTimeout, t.responseID)
	if err != nil {
		return fmt.Errorf("RequestTransferExit: %w", err)
	}

	//	log.Println(resp.String())

	if err := checkErr(resp); err != nil {
		return err
	}

	if resp.Data[3] != 0x77 {
		return fmt.Errorf("RequestTransferExit: expected 0x77, got %02X", resp.Data[3])
	}
	return nil
}

func (t *Client) ClearDynamicallyDefineLocalId(ctx context.Context) error {
	frame := gocan.NewFrame(REQ_MSG_ID, []byte{0x40, 0xA1, DYNAMICALLY_DEFINE_LOCAL_IDENTIFIER, DM_CDDLI}, gocan.ResponseRequired)
	//	log.Println(frame.String())
	resp, err := t.c.SendAndWait(ctx, frame, t.defaultTimeout*2, t.responseID)
	if err != nil {
		return fmt.Errorf("DynamicallyClearDefineLocalId: %w", err)
	}
	return checkErr(resp)
}

func (t *Client) DynamicallyDefineLocalIdRequest(ctx context.Context, index int, v *symbol.Symbol) error {
	buff := bytes.NewBuffer(nil)
	buff.WriteByte(0xF0)
	/*
		switch v.Method {
		case VAR_METHOD_ADDRESS:
			buff.Write([]byte{DM_DBMA, byte(index), uint8(v.Length), byte(v.Value >> 16), byte(v.Value >> 8), byte(v.Value)})
		case VAR_METHOD_LOCID:
			buff.Write([]byte{DM_DBLI, byte(index), 0x00, byte(v.Value), 0x00})
		case VAR_METHOD_SYMBOL:
			buff.Write([]byte{DM_DBMA, byte(index), 0x00, 0x80, byte(v.Value >> 8), byte(v.Value)})
		}
	*/
	buff.Write([]byte{DM_DBMA, byte(index), 0x00, 0x80, byte(v.Number >> 8), byte(v.Number)})

	message := append([]byte{byte(buff.Len() + 1), DYNAMICALLY_DEFINE_LOCAL_IDENTIFIER}, buff.Bytes()...)
	for _, msg := range t.splitRequest(message, false) {
		if msg.FrameType.Type == 1 {
			if err := t.c.SendFrame(msg); err != nil {
				return fmt.Errorf("DynamicallyDefineLocalIdRequest1: %w", err)
			}
		} else {
			resp, err := t.c.SendAndWait(ctx, msg, t.defaultTimeout, REQ_CHUNK_CONF_ID)
			if err != nil {
				return fmt.Errorf("DynamicallyDefineLocalIdRequest2: %w", err)
			}
			if err := TranslateErrorCode(resp.Data[3+2]); err != nil {
				return fmt.Errorf("DynamicallyDefineLocalIdRequest3: %w", err)
			}
		}
	}
	return nil
}

func (t *Client) RequestSecurityAccess(ctx context.Context, force bool) (bool, error) {
	if t.gotSequrityAccess && !force {
		return true, nil
	}
	for i := 0; i <= 4; i++ {
		ok, err := t.letMeIn(ctx, i)
		if err != nil {
			debug.Log(fmt.Sprintf("failed to obtain security access: %v", err))
			time.Sleep(3 * time.Second)
			continue
		}
		if ok {
			t.gotSequrityAccess = true
			return true, nil
		}
	}

	return false, retry.Unrecoverable(fmt.Errorf("security access was not granted"))
}

func (t *Client) letMeIn(ctx context.Context, method int) (bool, error) {
	msg := []byte{0x40, 0xA1, 0x02, SECURITY_ACCESS, DEVELOPMENT_PRIORITY}
	ff, err := t.c.SendAndWait(ctx, gocan.NewFrame(REQ_MSG_ID, msg, gocan.ResponseRequired), t.defaultTimeout, t.responseID)
	if err != nil {
		return false, fmt.Errorf("request seed: %v", err)
	}

	if err := checkErr(ff); err != nil {
		return false, err
	}

	seed := int(ff.Data[5])<<8 | int(ff.Data[6])
	key := CalcKey(seed, method)

	msgReply := []byte{0x40, 0xA1, 0x04, SECURITY_ACCESS, DEVELOPMENT_PRIORITY + 1, byte(int(key) >> 8), byte(key)}
	f2, err := t.c.SendAndWait(ctx, gocan.NewFrame(REQ_MSG_ID, msgReply, gocan.ResponseRequired), t.defaultTimeout*2, t.responseID)
	if err != nil {
		return false, fmt.Errorf("send seed: %v", err)

	}
	if f2.Data[3] == 0x67 && f2.Data[5] == 0x34 {
		return true, nil
	} else {
		err := fmt.Errorf("invalid response security access: %X", f2.Data)
		debug.Log(err.Error())
		return false, err
	}
}

// 266h Send acknowledgement, has 0x3F on 3rd!
func (t *Client) Ack(val byte, typ gocan.CANFrameType) error {
	//debug.Log(fmt.Sprintf("Ack: %s", frame.String()))
	return t.c.Send(0x266, []byte{0x40, 0xA1, 0x3F, val & 0xBF}, typ)
}

func CalcKey(seed int, method int) int {
	key := seed << 2
	key &= 0xFFFF
	switch method {
	case 0:
		key ^= 0x8142
		key -= 0x2356
	case 1:
		key ^= 0x4081
		key -= 0x1F6F
	case 2:
		key ^= 0x03DC
		key -= 0x2356
	case 3:
		key ^= 0x03D7
		key -= 0x2356
	case 4:
		key ^= 0x0409
		key -= 0x2356
	}
	key &= 0xFFFF
	return key
}

func CalcKeyCustom(seed int, xor, minus int) int {
	key := seed << 2
	key &= 0xFFFF
	key ^= xor
	key -= minus
	key &= 0xFFFF
	return key
}

func (t *Client) splitRequest(payload []byte, responseRequired bool) []*gocan.CANFrame {
	chunkSize := 6
	msgCount := (len(payload) + chunkSize - 1) / chunkSize

	var results []*gocan.CANFrame

	for i := range msgCount {
		start := chunkSize * i
		end := min(start+chunkSize, len(payload))

		count := end - start

		msgData := make([]byte, 2+count)
		flag := 0

		if i == 0 {
			flag |= 0x40 // this is the first data chunk
		}

		if i != msgCount-1 {
			flag |= 0x80 // we want confirmation for every chunk except the last one
		}

		msgData[0] = byte(flag | ((msgCount - i - 1) & 0x3F))
		msgData[1] = 0xA1

		copy(msgData[2:], payload[start:end])

		if flag&0x80 == 0x80 || responseRequired {
			results = append(results, gocan.NewFrame(REQ_MSG_ID, msgData, gocan.ResponseRequired))
		} else {
			results = append(results, gocan.NewFrame(REQ_MSG_ID, msgData, gocan.Outgoing))
		}
	}

	return results
}

/*
func (t *Client) splitRequest43(payload []byte, responseRequired bool) []gocan.CANFrame {
	msgCount := (len(payload) + 6 - 1) / 6

	left := len(payload)
	var results []gocan.CANFrame

	msgLen := func() int {
		if left >= 6 {
			left -= 6
			return 6
		} else {
			return left
		}
	}
	for i := 0; i < msgCount; i++ {
		count := msgLen()
		msgData := make([]byte, 2+count)
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

		copy(msgData[2:], payload[start:start+count])
		for j := 0; j < count; j++ {
			msgData[2+j] = payload[start+j]
		}

		if flag&0x80 == 0x80 {
			results = append(results, gocan.NewFrame(REQ_MSG_ID, msgData, gocan.ResponseRequired))
		} else {
			if responseRequired {
				results = append(results, gocan.NewFrame(REQ_MSG_ID, msgData, gocan.ResponseRequired))
			} else {
				results = append(results, gocan.NewFrame(REQ_MSG_ID, msgData, gocan.Outgoing))
			}
		}

	}
	return results
}
*/

func (t *Client) recvData2(ctx context.Context, length int) ([]byte, error) {
	var receivedBytes, payloadLeft int
	out := bytes.NewBuffer([]byte{})
	sub := t.c.Subscribe(ctx, t.responseID)
	if err := t.c.Send(REQ_MSG_ID, []byte{0x40, 0xA1, 0x02, READ_DATA_BY_IDENTIFIER, 0xF0, 0x00, 0x00, 0x00}, gocan.ResponseRequired); err != nil {
		return nil, err
	}

outer:
	for receivedBytes < length {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(t.defaultTimeout):
			return nil, fmt.Errorf("timeout")
		case f := <-sub.Chan():
			if f.Data[0]&0x40 == 0x40 {
				payloadLeft = int(f.Data[2]) - 2 // subtract two non-payload bytes
				if payloadLeft > 0 && receivedBytes < length {
					out.WriteByte(f.Data[5])
					receivedBytes++
					payloadLeft--
				}
				if payloadLeft > 0 && receivedBytes < length {
					out.WriteByte(f.Data[6])
					receivedBytes++
					payloadLeft--
				}
				if payloadLeft > 0 && receivedBytes < length {
					out.WriteByte(f.Data[7])
					receivedBytes++
					payloadLeft--
				}
			} else {
				for i := 0; i < 6; i++ {
					if receivedBytes < length {
						out.WriteByte(f.Data[2+i])
						receivedBytes++
						payloadLeft--
						if payloadLeft == 0 {
							break
						}
					}
				}
			}
			if f.Data[0] == 0x80 || f.Data[0] == 0xC0 {
				t.Ack(f.Data[0], gocan.Outgoing)
				break outer
			} else {
				t.Ack(f.Data[0], gocan.ResponseRequired)
			}
		}
	}
	return out.Bytes(), nil
}

func (t *Client) ReadFlash(ctx context.Context, addr, length int) ([]byte, error) {
	var readPos = addr
	out := bytes.NewBuffer([]byte{})
	for readPos < addr+length {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			var readLength int
			if (addr+length)-readPos >= 0xF0 {
				readLength = 0xF0
			} else {
				readLength = (addr + length) - readPos
			}
			err := retry.Do(func() error {
				//log.Printf("Reading memory by address, pos: 0x%X, length: 0x%X", readPos, readLength)
				b, err := t.ReadMemoryByAddressF0(ctx, readPos, readLength)
				if err != nil {
					return err
				}
				out.Write(b)
				return nil
			},
				retry.Context(ctx),
				retry.Attempts(3),
				retry.OnRetry(func(n uint, err error) {
					log.Printf("failed to read memory by address, pos: 0x%X, length: 0x%X, retrying: %v", readPos, readLength, err)
				}),
				retry.LastErrorOnly(true),
			)
			if err != nil {
				return nil, fmt.Errorf("failed to read memory by address, pos: 0x%X, length: 0x%X", readPos, readLength)
			}
			readPos += readLength
		}
	}

	return out.Bytes(), nil
}

func (t *Client) ReadMemoryByAddressF0(ctx context.Context, address, length int) ([]byte, error) {
	// Jump to read adress
	t.c.Send(REQ_MSG_ID, []byte{0x41, 0xA1, 0x08, DYNAMICALLY_DEFINE_LOCAL_IDENTIFIER, 0xF0, 0x03, 0x00, byte(length)}, gocan.Outgoing)
	frame := gocan.NewFrame(REQ_MSG_ID, []byte{0x00, 0xA1, byte((address >> 16) & 0xFF), byte((address >> 8) & 0xFF), byte(address & 0xFF), 0x00, 0x00, 0x00}, gocan.ResponseRequired)
	f, err := t.c.SendAndWait(ctx, frame, t.defaultTimeout*2, t.responseID)
	if err != nil {
		return nil, err
	}
	t.Ack(f.Data[0], gocan.Outgoing)

	if f.Data[3] != 0x6C || f.Data[4] != 0xF0 {
		if f.Data[3] == 0x7F && f.Data[4] == 0x2C {
			return nil, fmt.Errorf("jump to address failed: %w", TranslateErrorCode(f.Data[5]))
		}
		return nil, fmt.Errorf("failed to jump to 0x%X got response: %s", address, f.String())
	}
	b, err := t.recvData(ctx, length)
	if err != nil {
		return nil, fmt.Errorf("recvData failed: %w", err)
	}

	return b, nil
}

func (t *Client) recvData(ctx context.Context, length int) ([]byte, error) {
	// Initialize variables for tracking received bytes and remaining payload
	var receivedBytes, payloadLeft int
	out := bytes.NewBuffer(nil) // Simplified buffer initialization

	// Subscribe to responseID and send the start transfer command
	sub := t.c.Subscribe(ctx, t.responseID)
	startTransferCmd := []byte{0x40, 0xA1, 0x02, READ_DATA_BY_IDENTIFIER, 0xF0, 0x00, 0x00, 0x00}
	if err := t.c.Send(REQ_MSG_ID, startTransferCmd, gocan.ResponseRequired); err != nil {
		return nil, err
	}

	for receivedBytes < length {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(t.defaultTimeout):
			return nil, fmt.Errorf("timeout")
		case frame := <-sub.Chan():
			// Handle frame based on control byte
			if frame.Data[0]&0x40 == 0x40 { // Start or continuation frame
				// Calculate remaining payload excluding non-payload bytes
				payloadLeft = int(frame.Data[2]) - 2
				for i := 5; i < 8 && payloadLeft > 0 && receivedBytes < length; i++ {
					out.WriteByte(frame.Data[i])
					receivedBytes++
					payloadLeft--
				}
			} else { // Consecutive frame
				for i, max := 0, 6; i < max && receivedBytes < length; i++ {
					out.WriteByte(frame.Data[2+i])
					receivedBytes++
					payloadLeft--
					if payloadLeft == 0 {
						break
					}
				}
			}

			// Check if it's the last frame
			if frame.Data[0] == 0x80 || frame.Data[0] == 0xC0 {
				t.Ack(frame.Data[0], gocan.Outgoing)
				return out.Bytes(), nil // Directly return from the case block
			}
			t.Ack(frame.Data[0], gocan.ResponseRequired)
		}
	}
	return out.Bytes(), nil
}

// Reset ECU
func (t *Client) ResetECU(ctx context.Context) error {
	frame := gocan.NewFrame(REQ_MSG_ID, []byte{0x40, 0xA1, 0x02, ECU_RESET, 0x01}, gocan.ResponseRequired)
	f, err := t.c.SendAndWait(ctx, frame, t.defaultTimeout, t.responseID)
	if err != nil {
		return err
	}
	if err := checkErr(f); err != nil {
		return err
	}
	if f.Data[3] != 0x51 || f.Data[4] != 0x81 {
		return fmt.Errorf("abnormal ecu reset response: %X", f.Data[3:])
	}
	return nil
}

// -----
/*
func (t *Client) ReadDataByAddress(ctx context.Context, address int, length byte) ([]byte, error) {
	frame := gocan.NewFrame(REQ_MSG_ID, []byte{0x40, 0xA1, 0x05, READ_MEMORY_BY_ADDRESS, byte(address >> 16), byte(address >> 8), byte(address), length}, gocan.ResponseRequired)
	f, err := t.c.SendAndWait(ctx, frame, t.defaultTimeout*3, t.responseID)
	if err != nil {
		return nil, err
	}
	log.Println(f.String())

	return t.recvReadDataByAddress(ctx, int(length))

}

func (t *Client) recvReadDataByAddress(ctx context.Context, length int) ([]byte, error) {
	var receivedBytes, payloadLeft int
	out := bytes.NewBuffer([]byte{})

	sub := t.c.Subscribe(ctx, t.responseID)
	startTransfer := gocan.NewFrame(REQ_MSG_ID, []byte{0x40, 0xA1, 0x02, 0x21, 0x00, 0x00, 0x00, 0x00}, gocan.ResponseRequired)
	if err := t.c.Send(startTransfer); err != nil {
		return nil, err
	}

outer:
	for receivedBytes < length {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(t.defaultTimeout * 4):
			return nil, fmt.Errorf("timeout")
		case f := <-sub:
			d := f.Data()
			if d[0]&0x40 == 0x40 {
				payloadLeft = int(d[2]) - 2 // subtract two non-payload bytes
				if payloadLeft > 0 && receivedBytes < length {
					out.WriteByte(d[5])
					receivedBytes++
					payloadLeft--
				}
				if payloadLeft > 0 && receivedBytes < length {
					out.WriteByte(d[6])
					receivedBytes++
					payloadLeft--
				}
				if payloadLeft > 0 && receivedBytes < length {
					out.WriteByte(d[7])
					receivedBytes++
					payloadLeft--
				}
			} else {
				for i := 0; i < 6; i++ {
					if receivedBytes < length {
						out.WriteByte(d[2+i])
						receivedBytes++
						payloadLeft--
						if payloadLeft == 0 {
							break
						}
					}
				}
			}
			if d[0] == 0x80 || d[0] == 0xC0 {
				t.Ack(d[0], gocan.Outgoing)
				break outer
			} else {
				t.Ack(d[0], gocan.ResponseRequired)
			}
		}
	}
	return out.Bytes(), nil
}

func (t *Client) ReadDataBySymbol2(ctx context.Context, symbolNo int) ([]byte, error) {
	frame := gocan.NewFrame(REQ_MSG_ID, []byte{0x40, 0xA1, 0x04, READ_MEMORY_BY_ADDRESS, SYMBOL_IDENTIFICATION, byte(symbolNo >> 8), byte(symbolNo), 0x00}, gocan.ResponseRequired)
	f, err := t.c.SendAndWait(ctx, frame, t.defaultTimeout*3, t.responseID)
	if err != nil {
		return nil, err
	}
	log.Println(f.String())

	return nil, nil

}
*/

func (t *Client) ReadMemoryByAddress(ctx context.Context, address, length int) ([]byte, error) {
	buff := bytes.NewBuffer(make([]byte, 0, length))
	if length > 244 {
		left := length
		for left > 0 {
			//			log.Println(left)
			toGet := min(244, left)

			//			log.Printf("Reading memory by address, pos: 0x%X, length: 0x%X", address+buff.Len(), toGet)
			b, err := t.readMemoryByAddress(ctx, address+buff.Len(), byte(toGet))
			if err != nil {
				return nil, err
			}
			left -= len(b)
			buff.Write(b)
		}
		return buff.Bytes(), nil
	}
	return t.readMemoryByAddress(ctx, address, byte(length))
}

func (t *Client) readMemoryByAddress(ctx context.Context, address int, length byte) ([]byte, error) {
	frame := gocan.NewFrame(REQ_MSG_ID, []byte{0x40, 0xA1, 0x05, READ_MEMORY_BY_ADDRESS, byte(address >> 16), byte(address >> 8), byte(address), length}, gocan.ResponseRequired)
	resp, err := t.c.SendAndWait(ctx, frame, t.defaultTimeout, t.responseID)
	if err != nil {
		return nil, fmt.Errorf("ReadDataByAddress: %w", err)
	}
	out := bytes.NewBuffer(nil)

	if err := checkErr(resp); err != nil {
		return nil, err
	}

	dataLenLeft := resp.Data[2] - 4
	var thisRead byte
	if dataLenLeft > 1 {
		thisRead = 1
	} else {
		thisRead = dataLenLeft
	}

	out.Write(resp.Data[7 : 7+thisRead])
	dataLenLeft -= thisRead

	currentChunkNumber := resp.Data[0] & 0x3F
	for currentChunkNumber != 0 {
		frame := gocan.NewFrame(RESP_CHUNK_CONF_ID, []byte{0x40, 0xA1, 0x3F, resp.Data[0] &^ 0x40}, gocan.ResponseRequired)
		resp, err = t.c.SendAndWait(ctx, frame, t.defaultTimeout, t.responseID)
		if err != nil {
			return nil, err
		}
		toRead := uint8(math.Min(6, float64(dataLenLeft)))
		out.Write(resp.Data[2 : 2+toRead])
		dataLenLeft -= toRead
		currentChunkNumber = resp.Data[0] & 0x3F
	}
	return out.Bytes(), nil
}

func (t *Client) WriteDataByAddress(ctx context.Context, address uint32, data []byte) error {
	message := append([]byte{byte(4 + len(data)), WRITE_DATA_BY_ADDRESS, byte(address >> 16), byte(address >> 8), byte(address), byte(len(data))}, data...)
	if err := t.sendLong(ctx, message); err != nil {
		return fmt.Errorf("WriteDataToAddress: %w", err)
	}
	return nil
}

// ----

func (t *Client) RequestUpload(ctx context.Context, address, length uint32) error {
	message := []byte{0x07, REQUEST_UPLOAD, byte(address >> 16), byte(address >> 8), byte(address), byte(length >> 16), byte(length >> 8), byte(length)}
	for _, msg := range t.splitRequest(message, false) {
		//		log.Println(msg.String())
		if msg.FrameType.Type == 1 {
			resp, err := t.c.SendAndWait(ctx, msg, t.defaultTimeout, t.responseID)
			if err != nil {
				return err
			}
			//			log.Println(i, resp.String())
			if err := checkErr(resp); err != nil {
				return err
			}

		} else {
			resp, err := t.c.SendAndWait(ctx, msg, t.defaultTimeout, REQ_CHUNK_CONF_ID)
			if err != nil {
				return fmt.Errorf("RequestUpload: %w", err)
			}
			if err := TranslateErrorCode(resp.Data[5]); err != nil {
				return fmt.Errorf("RequestUpload: %w", err)
			}
			//			log.Println(i, resp.String())
		}
	}

	return nil
}

func (t *Client) sendLong(ctx context.Context, data []byte) error {
	messages := t.splitRequest(data, true)
	for i, msg := range messages {
		// log.Printf("msg %x: %X", msg.Identifier(), msg.Data())
		if i == len(messages)-1 {
			// log.Println(msg.Type())

			//if err := t.c.Send(msg); err != nil {
			//	return err
			//}

			resp, err := t.c.SendAndWait(ctx, msg, t.defaultTimeout, t.responseID)
			if err != nil {
				return err
			}
			// log.Println(resp.String())
			if err := checkErr(resp); err != nil {
				return err
			}
		} else {
			resp, err := t.c.SendAndWait(ctx, msg, t.defaultTimeout, REQ_CHUNK_CONF_ID)
			if err != nil {
				return fmt.Errorf("%s2: %w", getFunctionName(), err)
			}
			if err := checkErr(resp); err != nil {
				return err
			}
			// log.Printf("chunk %d: %X", i, resp.Data())
		}
	}
	return nil
}

func (t *Client) ReadROM(ctx context.Context, address, sramOffset uint32, length uint32) ([]byte, error) {
	if err := t.RequestUpload(ctx, address-sramOffset, length); err != nil {
		return nil, err
	}
	b, err := t.TransferData(ctx, length)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (t *Client) ReadRAM(ctx context.Context, address, length uint32) ([]byte, error) {
	data, err := t.ReadMemoryByAddress(ctx, int(address), int(length))
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (t *Client) SaveRAM(ctx context.Context, address uint32, data []byte) error {
	start := time.Now()
	defer func(t time.Time) {
		log.Println("saveRam took:", time.Since(t))
	}(start)
	if err := t.WriteDataByAddress(ctx, address, data); err != nil {
		return err
	}
	return nil
}

func (t *Client) SaveROM(ctx context.Context, address uint32, data []byte) error {
	start := time.Now()

	if err := t.RequestDownload(ctx, address, uint32(len(data))); err != nil {
		return err
	}

	defer func(t time.Time) {
		log.Println("saveROM took:", time.Since(t))
	}(start)

	msgs := t.splitRequest2(append([]byte{byte(len(data) + 1), TRANSFER_DATA}, data...))
	for _, msg := range msgs {
		log.Println(msg.String())
		if msg.FrameType.Type == 1 {
			if err := t.c.SendFrame(msg); err != nil {
				return fmt.Errorf("%s1: %w", getFunctionName(), err)
			}
		} else {
			resp, err := t.c.SendAndWait(ctx, msg, t.defaultTimeout, REQ_CHUNK_CONF_ID)
			if err != nil {
				return fmt.Errorf("%s2: %w", getFunctionName(), err)
			}
			log.Println(resp.String())
			if err := checkErr(resp); err != nil {
				return err
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	return nil
	//return t.writeRange(ctx, int(address), int(address)+len(data), data)
}

func (t *Client) splitRequest2(payload []byte) []*gocan.CANFrame {
	msgCount := (len(payload) + 6 - 1) / 6

	left := len(payload)
	var results []*gocan.CANFrame

	msgLen := func() int {
		if left >= 6 {
			left -= 6
			return 6
		} else {
			return left
		}
	}

	for i := 0; i < msgCount; i++ {
		count := msgLen()
		msgData := make([]byte, 2+count)
		flag := 0

		if i == 0 {
			flag |= 0x40 // this is the first data chunk
		}
		/*
			if i != msgCount-1 {
				flag |= 0x80 // we want confirmation for every chunk except the last one
			}
		*/
		msgData[0] = (byte)(flag | (msgCount-i-1)&0x3F) // & 0x3F is not necessary, only to show that this field is 6-bit wide
		msgData[1] = 0xA1

		start := 6 * i

		copy(msgData[2:], payload[start:start+count])
		for j := 0; j < count; j++ {
			msgData[2+j] = payload[start+j]
		}

		if i < msgCount-1 {
			results = append(results, gocan.NewFrame(REQ_MSG_ID, msgData, gocan.Outgoing))
		} else if i == msgCount-1 {
			results = append(results, gocan.NewFrame(REQ_MSG_ID, msgData, gocan.ResponseRequired))
		}

		/*
			if flag&0x80 == 0x80 {
				results = append(results, gocan.NewFrame(REQ_MSG_ID, msgData, gocan.ResponseRequired))
			} else {
				results = append(results, gocan.NewFrame(REQ_MSG_ID, msgData, gocan.Outgoing))
			}
		*/
	}
	return results
}

func (t *Client) RequestDownload(ctx context.Context, address uint32, length uint32) error {
	message := []byte{0x08, REQUEST_DOWNLOAD, byte(address >> 16), byte(address >> 8), byte(address), 0x00, byte(length >> 16), byte(length >> 8), byte(length)}
	for _, msg := range t.splitRequest2(message) {
		log.Println(msg.String())
		if msg.FrameType.Type == 1 {
			if err := t.c.SendFrame(msg); err != nil {
				return fmt.Errorf("%s1: %w", getFunctionName(), err)
			}
		} else {
			resp, err := t.c.SendAndWait(ctx, msg, t.defaultTimeout, t.responseID)
			if err != nil {
				return fmt.Errorf("%s2: %w", getFunctionName(), err)
			}
			if err := checkErr(resp); err != nil {
				return err
			}
			log.Println(resp.String())
			if resp.Data[3] != 0x74 {
				return fmt.Errorf("%s5: invalid response enabling download mode", getFunctionName())
			}
		}
	}

	return nil
}

func getFunctionName() string {
	return getFunctionNameN(2)
}

func getFunctionNameN(depth int) string {
	pc, _, _, ok := runtime.Caller(depth)
	if !ok {
		return "unknown"
	}

	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return "unknown"
	}

	return textAfterLastDot(fn.Name())

}

func checkErr(f *gocan.CANFrame) error {
	if f.Data[3] == 0x7F {
		return fmt.Errorf("%s: %s %w", getFunctionNameN(2), TranslateServiceID(f.Data[4]), TranslateErrorCode(f.Data[5]))
	}
	return nil
}

func textAfterLastDot(s string) string {
	lastDotIndex := strings.LastIndex(s, ".")
	if lastDotIndex == -1 || lastDotIndex == len(s)-1 {
		return ""
	}
	return s[lastDotIndex+1:]
}
