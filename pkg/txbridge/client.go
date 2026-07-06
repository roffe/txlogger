package txbridge

import (
	"errors"
	"net"
	"strings"
	"time"

	"github.com/roffe/gocan/v2/pkg/serialcommand"
)

const DefaultAddress = "192.168.4.1:1337"

var (
	ErrNotConnected = errors.New("not connected")
	ErrNoData       = errors.New("no data read")
)

// ResolveAddress returns the host:port to dial for a txbridge connection.
// The host comes from a "tcp://host[:port]" port string; the port is always
// forced to 1337 (appended if missing, overwritten if wrong). Anything else
// falls back to DefaultAddress.
func ResolveAddress(port string) string {
	addr, ok := strings.CutPrefix(port, "tcp://")
	if !ok || addr == "" {
		return DefaultAddress
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr // no port present
	}
	return net.JoinHostPort(host, "1337")
}

func NewClient(address string) *Client {
	return &Client{
		address: address,
	}
}

// SetAddress updates the address used by the next Connect call.
func (c *Client) SetAddress(address string) {
	c.address = address
}

type Client struct {
	conn    net.Conn
	address string
}

func (c *Client) Connect() error {
	if c.conn != nil {
		return nil // Already connected
	}
	dialer := net.Dialer{Timeout: 2 * time.Second}
	//ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	//defer cancel()
	//if src, err := mdns.Query(ctx, "txbridge.local"); err != nil {
	//	log.Printf("failed to query mDNS: %v", err)
	//} else {
	//	if src.IsValid() {
	//		address = fmt.Sprintf("%s:%d", src.String(), 1337)
	//	} else {
	//		log.Printf("No mDNS response, using address: %s", address)
	//	}
	//}

	conn, err := dialer.Dial("tcp", c.address)
	if err != nil {
		return err
	}
	c.conn = conn
	return nil
}

func (c *Client) Disconnect() error {
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}

func (c *Client) SendCommand(command byte, data []byte) error {
	if c.conn == nil {
		return ErrNotConnected
	}
	if data == nil {
		_, err := c.conn.Write([]byte{command}) // Send command with no data
		return err
	}
	cmd, err := serialcommand.NewSerialCommand(command, data).MarshalBinary()
	if err != nil {
		return err
	}
	n, err := c.conn.Write(cmd)
	if n != len(cmd) {
		return errors.New("failed to send complete command")
	}
	return err
}

func (c *Client) ReadCommand(timeout time.Duration) (*serialcommand.SerialCommand, error) {
	if c.conn == nil {
		return nil, ErrNotConnected
	}
	readbuf := make([]byte, 260)
	deadline := time.Now().Add(timeout)
	c.conn.SetReadDeadline(deadline)
	n, err := c.conn.Read(readbuf)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrNoData // No data read
	}
	cmd := &serialcommand.SerialCommand{}
	err = cmd.UnmarshalBinary(readbuf[:n])
	if err != nil {
		return nil, err
	}
	return cmd, nil
}
