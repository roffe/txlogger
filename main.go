package main

import (
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	//xlayout "fyne.io/x/fyne/layout"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"

	socketio "github.com/googollee/go-socket.io"
	"github.com/roffe/t7logger/pkg/kwp2000"
	"github.com/roffe/t7logger/pkg/sink"
	"github.com/roffe/t7logger/pkg/windows"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)
}

func main() {
	sm := sink.NewManager()

	vars := kwp2000.NewVarDefinitionList()

	go startWeb2(sm, vars)
	//sub := sinkManager.NewSubscriber(func(msg string) {
	//	fmt.Println("msg:", msg)
	//})
	//defer sub.Close()
	a := app.NewWithID("com.roffe.t7l")
	mw := windows.NewMainWindow(a, sm, vars)
	mw.Resize(fyne.NewSize(1400, 800))
	mw.SetContent(mw.Layout())
	mw.ShowAndRun()
}

type SymbolDefinition struct {
	Name string
	ID   int
	Type string
}

func startWeb2(sm *sink.Manager, vars *kwp2000.VarDefinitionList) {
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

	server.OnConnect("/", func(s socketio.Conn) error {
		s.SetContext("")
		log.Println("connected:", s.ID())
		return nil
	})

	server.OnEvent("/", "notice", func(s socketio.Conn, msg string) {
		log.Println("notice:", msg)
		s.Emit("reply", "have "+msg)
	})

	//server.OnEvent("/chat", "msg", func(s socketio.Conn, msg string) string {
	//	s.SetContext(msg)
	//	return "recv " + msg
	//})

	server.OnEvent("/", "bye", func(s socketio.Conn) string {
		last := s.Context().(string)
		s.Emit("bye", last)
		s.Close()
		return last
	})

	server.OnError("/", func(s socketio.Conn, e error) {
		log.Println("meet error:", e)
	})

	server.OnDisconnect("/", func(s socketio.Conn, reason string) {
		log.Println("closed", reason)
	})

	server.OnEvent("/", "start_session", func(s socketio.Conn, msg string) {
		var symbolList []SymbolDefinition
		for _, v := range vars.Get() {
			symbolList = append(symbolList, SymbolDefinition{
				Name: v.Name,
				ID:   v.Value,
				Type: returnVis(v.Visualization),
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
		if server.Count() > 0 {
			server.BroadcastToRoom("/", "metrics", string(msg.Data))
		}
	})
	defer sub.Close()

	router.Use(static.Serve("/", static.LocalFile("./web", false)))
	router.GET("/socket.io/*any", gin.WrapH(server))
	router.POST("/socket.io/*any", gin.WrapH(server))
	// router.StaticFS("/css", http.Dir("./web/css"))
	// router.StaticFS("/js", http.Dir("./web/js"))

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
