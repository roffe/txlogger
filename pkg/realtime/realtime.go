package realtime

import (
	"log"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	socketio "github.com/googollee/go-socket.io"
	"github.com/roffe/t7logger/pkg/kwp2000"
	"github.com/roffe/t7logger/pkg/sink"
)

type SymbolDefinition struct {
	Name string
	ID   int
	Type string
	Unit string
}

func StartWebserver(sm *sink.Manager, vars *kwp2000.VarDefinitionList) {
	time.Sleep(2 * time.Second)
	router := gin.New()

	router.Use(cors.New(cors.Config{
		//AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST"},
		AllowHeaders:     []string{"Origin"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		AllowOriginFunc: func(origin string) bool {
			//return origin == "https://github.com"
			return true
		},
		//MaxAge: 12 * time.Hour,
	}))

	server := socketio.NewServer(nil)

	server.OnError("/", func(s socketio.Conn, e error) {
		log.Println("socket.io error:", e)
	})

	server.OnConnect("/", func(s socketio.Conn) error {
		s.SetContext("")
		log.Println("connected:", s.ID())
		return nil
	})

	server.OnDisconnect("/", func(s socketio.Conn, reason string) {
		log.Println("closed", reason)
	})

	server.OnEvent("/", "end_session", func(s socketio.Conn) {
		s.Leave("metrics")
	})

	lastMsgs := make([][]byte, 0, 10)

	server.OnEvent("/", "start_session", func(s socketio.Conn, msg string) {
		s.Join("metrics")

	})

	server.OnEvent("/", "request_symbols", func(s socketio.Conn) {
		var symbolList []SymbolDefinition
		for _, v := range vars.Get() {
			//vis := "linegraph"
			symbolList = append(symbolList, SymbolDefinition{
				Name: v.Name,
				ID:   v.Value,
				Type: returnVis(v.Visualization),
				Unit: v.Unit,
			})
		}
		s.Emit("symbol_list", symbolList)
	})

	go func() {
		if err := server.Serve(); err != nil {
			log.Fatalf("socketio listen error: %s\n", err)
		}
	}()
	defer server.Close()

	sub := sm.NewSubscriber(func(msg *sink.Message) {
		if len(lastMsgs) > 100 {
			lastMsgs = lastMsgs[1:]
		}
		lastMsgs = append(lastMsgs, msg.Data)
		if server.Count() > 0 {
			server.BroadcastToRoom("/", "metrics", "metrics", string(msg.Data))
		}
	})
	defer sub.Close()

	router.Use(static.Serve("/", static.LocalFile("./web", false)))
	router.GET("/socket.io/*any", gin.WrapH(server))
	router.POST("/socket.io/*any", gin.WrapH(server))

	if err := router.Run(":8080"); err != nil {
		log.Fatal("failed run app: ", err)
	}
}

func returnVis(t string) string {
	if t == "" {
		return "linegraph"
	}
	return t
}
