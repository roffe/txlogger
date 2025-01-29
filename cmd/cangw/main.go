package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/adapter"
	"github.com/roffe/gocan/proto"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		log.Fatalf("failed to get user cache dir: %v", err)
	}

	socketFile := filepath.Join(cacheDir, "gocan.sock")
	os.Remove(socketFile)

	// Start IPC server
	srv := NewServer(socketFile)
	defer srv.Close()

	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Shutting down server")
		if err := srv.Close(); err != nil {
			log.Fatalf("failed to close server: %v", err)
		}
	}()
	go func() {
		if err := srv.Run(); err != nil {
			log.Fatalf("server: %v", err)
		}
	}()

	a := app.NewWithID("com.roffe.cangw")
	if desk, ok := a.(desktop.App); ok {

		m := fyne.NewMenu("",
			fyne.NewMenuItem("Show", func() {
				w := a.NewWindow("GoCAN Gateway")
				output := widget.NewLabel("")
				adapterList := widget.NewSelect(adapter.List(), func(s string) {
					log.Printf("selected adapter: %v", s)
					ad := adapter.GetAdapterMap()[s]
					if ad != nil {
						var out strings.Builder
						out.WriteString("Description: " + ad.Description + "\n")
						out.WriteString("Capabilities:\n")
						out.WriteString("  HSCAN: " + strconv.FormatBool(ad.Capabilities.HSCAN) + "\n")
						out.WriteString("  KLine: " + strconv.FormatBool(ad.Capabilities.KLine) + "\n")
						out.WriteString("  SWCAN: " + strconv.FormatBool(ad.Capabilities.SWCAN) + "\n")
						out.WriteString("RequiresSerialPort: " + strconv.FormatBool(ad.RequiresSerialPort) + "\n")
						output.SetText(out.String())
					}
				})

				w.SetContent(container.NewBorder(
					container.NewBorder(nil, nil, widget.NewLabel("Info"), nil,
						adapterList,
					),
					nil,
					nil,
					nil,
					output,
				))
				w.Resize(fyne.Size{Width: 350, Height: 125})
				w.Show()
			}))
		desk.SetSystemTrayMenu(m)
	}
	a.Run()
	log.Println("Exiting")
}

var _ proto.GocanServer = (*Server)(nil)

type Server struct {
	proto.UnimplementedGocanServer

	l net.Listener
}

func NewServer(socketFile string) *Server {
	l, err := net.Listen("unix", socketFile)
	if err != nil {
		log.Fatal(err)
	}
	srv := &Server{l: l}

	return srv
}

var kaep = keepalive.EnforcementPolicy{
	MinTime:             5 * time.Second, // If a client pings more than once every 5 seconds, terminate the connection
	PermitWithoutStream: true,            // Allow pings even when there are no active streams
}

var kasp = keepalive.ServerParameters{
	MaxConnectionIdle:     15 * time.Second, // If a client is idle for 15 seconds, send a GOAWAY
	MaxConnectionAge:      0,                // If any connection is alive for more than 30 seconds, send a GOAWAY
	MaxConnectionAgeGrace: 5 * time.Second,  // Allow 5 seconds for pending RPCs to complete before forcibly closing connections
	Time:                  5 * time.Second,  // Ping the client if it is idle for 5 seconds to ensure the connection is still active
	Timeout:               3 * time.Second,  // Wait 1 second for the ping ack before assuming the connection is dead
}

func (s *Server) Run() error {
	sg := grpc.NewServer(grpc.KeepaliveEnforcementPolicy(kaep), grpc.KeepaliveParams(kasp))
	proto.RegisterGocanServer(sg, s)
	log.Printf("server listening at %v", s.l.Addr())
	if err := sg.Serve(s.l); err != nil {
		return fmt.Errorf("failed to serve: %w", err)
	}
	return nil
}

func (s *Server) Close() error {
	return s.l.Close()
}

func adapterConfigFromContext(ctx context.Context) (string, *gocan.AdapterConfig, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	for k, v := range md {
		log.Printf("metadata: %s: %v", k, v)
	}

	adaptername := md["adapter"][0]
	adapterPort := md["port"][0]
	portBaudrate, err := strconv.Atoi(md["port_baudrate"][0])
	if err != nil {
		return "", nil, fmt.Errorf("invalid port_baudrate: %w", err)
	}

	canrate, err := strconv.ParseFloat(md["canrate"][0], 64)
	if err != nil {
		return "", nil, fmt.Errorf("invalid canrate: %w", err)
	}

	filterIDs := strings.Split(md["canfilter"][0], ",")

	var canfilters []uint32
	for _, id := range filterIDs {
		i, err := strconv.ParseUint(id, 10, 32)
		if err != nil {
			return "", nil, fmt.Errorf("invalid canfilter: %w", err)
		}
		canfilters = append(canfilters, uint32(i))
	}

	useExtendedID, err := strconv.ParseBool(md["useextendedid"][0])
	if err != nil {
		return "", nil, fmt.Errorf("invalid useextendedid: %w", err)
	}

	minversion := md["minversion"][0]

	return adaptername, &gocan.AdapterConfig{
		Port:                   adapterPort,
		PortBaudrate:           portBaudrate,
		CANRate:                canrate,
		CANFilter:              canfilters,
		UseExtendedID:          useExtendedID,
		MinimumFirmwareVersion: minversion,
	}, nil
}

func send(srv grpc.BidiStreamingServer[proto.CANFrame, proto.CANFrame], id uint32, data []byte) error {
	frameTyp := proto.CANFrameTypeEnum_Incoming
	return srv.Send(&proto.CANFrame{
		Id:   &id,
		Data: data,
		FrameType: &proto.CANFrameType{
			FrameType: &frameTyp,
			Responses: new(uint32),
		},
	})
}

func (s *Server) Stream(srv grpc.BidiStreamingServer[proto.CANFrame, proto.CANFrame]) error {
	// gctx, cancel := context.WithCancel(srv.Context())
	gctx := srv.Context()

	adaptername, adapterConfig, err := adapterConfigFromContext(gctx)
	if err != nil {
		return fmt.Errorf("failed to create adapter config: %w", err)
	}

	adapterConfig.OnError = func(err error) {
		send(srv, adapter.SystemMsgError, []byte(err.Error()))
		log.Printf("adapter error: %v", err)

	}

	adapterConfig.OnMessage = func(s string) {
		log.Printf("adapter message: %v", s)
	}

	dev, err := adapter.New(adaptername, adapterConfig)
	if err != nil {
		return fmt.Errorf("failed to create adapter: %w", err)
	}

	errg, ctx := errgroup.WithContext(gctx)

	c, err := gocan.New(ctx, dev)
	if err != nil {
		send(srv, 0, []byte(err.Error()))
		return fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	canRX := c.SubscribeChan(ctx)

	// send mesage from canbus adapter to IPC
	errg.Go(func() error {
		for {
			select {
			case msg, ok := <-canRX:
				if !ok {
					return errors.New("canRX closed")
				}
				id := msg.Identifier()
				frameTyp := proto.CANFrameTypeEnum(msg.Type().Type)
				responses := uint32(msg.Type().Responses)
				mmsg := &proto.CANFrame{
					Id:   &id,
					Data: msg.Data(),
					FrameType: &proto.CANFrameType{
						FrameType: &frameTyp,
						Responses: &responses,
					},
				}
				if err := srv.Send(mmsg); err != nil {
					return fmt.Errorf("failed to send message: %w", err)
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	})

	// send message from IPC to canbus adapter
	errg.Go(func() error {
		for {
			msg, err := srv.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return nil // Client closed connection
				}
				return fmt.Errorf("failed to receive outgoing %w", err) // Something unexpected happened
			}
			r := gocan.CANFrameType{
				Type:      int(*msg.FrameType.FrameType),
				Responses: int(*msg.FrameType.Responses),
			}
			frame := gocan.NewFrame(*msg.Id, msg.Data, r)
			if err := c.Send(frame); err != nil {
				return err
			}
		}
	})
	send(srv, 0, []byte("OK"))
	return errg.Wait()
}

func (s *Server) GetAdapters(ctx context.Context, _ *emptypb.Empty) (*proto.Adapters, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	for k, v := range md {
		log.Printf("metadata: %s: %v", k, v)
	}
	var adapters []*proto.AdapterInfo
	for _, a := range adapter.GetAdapterMap() {
		adapter := &proto.AdapterInfo{
			Name:        &a.Name,
			Description: &a.Description,
			Capabilities: &proto.AdapterCapabilities{
				HSCAN: &a.Capabilities.HSCAN,
				KLine: &a.Capabilities.KLine,
				SWCAN: &a.Capabilities.SWCAN,
			},
			RequireSerialPort: &a.RequiresSerialPort,
		}
		adapters = append(adapters, adapter)
	}
	return &proto.Adapters{
		Adapters: adapters,
	}, nil
}

func (s *Server) GetSerialPorts(ctx context.Context, _ *emptypb.Empty) (*proto.SerialPorts, error) {

	return nil, nil
}

// ##############################

const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue

type gocanGatewayService struct {
	c      *Server
	r      <-chan svc.ChangeRequest
	status chan<- svc.Status

	socketFile string
}

func (m *gocanGatewayService) Execute(args []string, r <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}

	m.r = r
	m.status = status

	var err error
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		log.Fatalf("failed to get user cache dir: %v", err)
	}

	m.socketFile = filepath.Join(cacheDir, "gocan.sock")

	defer os.Remove(m.socketFile)

	m.c, err = m.startGateway()
	if err != nil {
		log.Printf("failed to start gateway: %v", err)
		status <- svc.Status{State: svc.StopPending}
		time.Sleep(100 * time.Millisecond)
		return false, 1
	}

	status <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Shutting down server")
		if err := m.c.Close(); err != nil {
			log.Fatalf("failed to close server: %v", err)
		}
	}()

	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			log.Println("Interrogate...!", c.CurrentStatus)
			status <- c.CurrentStatus
			continue
		case svc.Stop, svc.Shutdown:
			log.Print("Shutting service...!")
			status <- svc.Status{State: svc.StopPending}
			time.Sleep(100 * time.Millisecond)
			return false, 1
		case svc.Pause:
			log.Print("Pausing service...!")
			m.c.Close()
			m.c = nil
			status <- svc.Status{State: svc.Paused, Accepts: cmdsAccepted}
			continue
		case svc.Continue:
			log.Print("Continuing service...!")
			m.c, err = m.startGateway()
			if err != nil {
				log.Printf("failed to start gateway: %v", err)
				status <- svc.Status{State: svc.StopPending}
				continue
			}

			status <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
			continue
		default:
			log.Printf("Unexpected service control request #%d", c)
			continue
		}
	}
	status <- svc.Status{State: svc.StopPending}
	return false, 1
}

func (m *gocanGatewayService) startGateway() (*Server, error) {

	// Start IPC server
	srv := NewServer(m.socketFile)
	//defer srv.Close()

	go func() {
		if err := srv.Run(); err != nil {
			log.Fatalf("server: %v", err)
		}
	}()
	return srv, nil
}

func runService(name string, isDebug bool) {
	if isDebug {
		err := debug.Run(name, &gocanGatewayService{})
		if err != nil {
			log.Fatalln("Error running service in debug mode.")
		}
	} else {
		err := svc.Run(name, &gocanGatewayService{})
		if err != nil {
			log.Fatalln("Error running service in Service Control mode.")
		}
	}
}

func main2() {
	f, err := os.OpenFile("E:\\gocan_gateway_debug.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalln(fmt.Errorf("error opening file: %v", err))
	}
	defer f.Close()

	log.SetOutput(f)
	runService("myservice", true)
}
