package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/adapter"
	"github.com/roffe/gocan/pkg/kwp2000"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

var vars = []kwp2000.VarDefinition{
	{
		Name:   "ActualIn.n_Engine",
		Method: kwp2000.VAR_METHOD_LOCID,
		Value:  84,
		Type:   kwp2000.TYPE_WORD,
		Signed: true,
	},

	{
		Name:   "ActualIn.p_AirInlet",
		Method: kwp2000.VAR_METHOD_LOCID,
		Value:  85,
		Type:   kwp2000.TYPE_WORD,
		Signed: true,
	},
	{
		Name:   "ActualIn.T_Engine",
		Method: kwp2000.VAR_METHOD_LOCID,
		Value:  93,
		Type:   kwp2000.TYPE_WORD,
		Signed: true,
	},
	{
		Name:   "ActualIn.T_AirInlet",
		Method: kwp2000.VAR_METHOD_LOCID,
		Value:  94,
		Type:   kwp2000.TYPE_WORD,
		Signed: true,
	},
	/*
		{
			Name:   "ActualIn.Q_AirInlet",
			Method: kwp2000.VAR_METHOD_LOCID,
			Value:  104,
			Type:   kwp2000.TYPE_WORD,
			Signed: true,
		},
		{
			Name:   "MAF.m_AirInlet",
			Method: kwp2000.VAR_METHOD_LOCID,
			Value:  105,
			Type:   kwp2000.TYPE_WORD,
			Signed: false,
		},
	*/
}

func main() {
	quitChan := make(chan os.Signal, 2)
	signal.Notify(quitChan, os.Interrupt, syscall.SIGTERM)
	var devName string

	for _, d := range adapter.List() {
		if strings.HasPrefix(d, "Mongoose") {
			devName = d
		}
	}

	dev, err := adapter.New(
		devName,
		&gocan.AdapterConfig{
			//Port:         `C:\Program Files (x86)\Drew Technologies, Inc\J2534\MongoosePro GM II\monpa432.dll`,
			//Port:         "COM7",
			//PortBaudrate: 3000000,
			CANRate:   500,
			CANFilter: []uint32{0x238, 0x258, 0x270},
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	c, err := gocan.New(ctx, dev)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	k := kwp2000.New(c)
	if err := k.StartSession(ctx, kwp2000.INIT_MSG_ID, kwp2000.INIT_RESP_ID); err != nil {
		log.Println(err)
		return
	}

	for i, v := range vars {
		if err := k.DynamicallyDefineLocalIdRequest(ctx, i, v); err != nil {
			log.Println(err)
			return
		}
	}

	count := 0
	ticker := time.NewTicker(time.Second)
	go func() {
		for range ticker.C {
			executionsPerSecond := count
			count = 0
			fmt.Printf("%d executions per second\n", executionsPerSecond)
		}
	}()

	for {
		select {
		case <-quitChan:
			log.Println("Exiting...")
			if err := k.StopSession(ctx, kwp2000.INIT_MSG_ID, kwp2000.INIT_RESP_ID); err != nil {
				log.Println(err)
			}
			time.Sleep(100 * time.Millisecond)
			return
		default:
			d, err := k.ReadDataByLocalIdentifier(ctx, 0xF0)
			if err != nil {
				log.Println(err)
				return
			}
			_ = d
			//	log.Printf("%X", d)

			//time.Sleep(300 * time.Millisecond)
			//if err := c.SendFrame(kwp2000.RESP_CHUNK_CONF_ID, []byte{0x40, 0xA1, 0x3F, 0x00 &^ 0x40, 0x00, 0x00, 0x00, 0x00}, gocan.Outgoing); err != nil {
			//	log.Println(err)
			//}
			//frame := gocan.NewFrame(kwp2000.REQ_CHUNK_CONF_ID, []byte{0x40, 0xA1, 0x3F, 0x00 &^ 0x40, 0x00, 0x00, 0x00, 0x00}, gocan.ResponseRequired)
			count++
		}
	}

}
