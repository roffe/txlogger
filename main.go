package main

import (
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	//xlayout "fyne.io/x/fyne/layout"

	"github.com/roffe/t7logger/pkg/kwp2000"
	"github.com/roffe/t7logger/pkg/realtime"
	"github.com/roffe/t7logger/pkg/sink"
	"github.com/roffe/t7logger/pkg/windows"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)
}

func main() {
	sm := sink.NewManager()

	vars := kwp2000.NewVarDefinitionList()

	go realtime.StartWebserver(sm, vars)
	//sub := sinkManager.NewSubscriber(func(msg string) {
	//	fmt.Println("msg:", msg)
	//})
	//defer sub.Close()
	a := app.NewWithID("com.roffe.t7l")
	mw := windows.NewMainWindow(a, sm, vars)
	mw.SetMaster()
	mw.Resize(fyne.NewSize(1400, 800))
	mw.SetContent(mw.Layout())
	mw.ShowAndRun()
}

/*
	var upgrader = websocket.Upgrader{
		//check origin will check the cross region source (note : please not using in production)
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			log.Println("origin:", origin)
			//return origin == "chrome-extension://cbcbkhdmedgianpaifchdaddpnmgnknn"
			return true
		},
	}

	func startWeb(sm *sink.Manager, vars *kwp2000.VarDefinitionList) {
		r := gin.Default()
		r.Use(cors.New(cors.Config{
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
		r.StaticFS("/public", http.Dir("./web"))
		r.GET("/ws", func(c *gin.Context) {
			//upgrade get request to websocket protocol
			ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
			if err != nil {
				fmt.Println(err)
				return
			}

			var symbolList []SymbolDefinition
			for _, v := range vars.Get() {
				symbolList = append(symbolList, SymbolDefinition{
					Name: v.Name,
					ID:   v.Value,
					Type: returnVis(v.Visualization),
				})
			}

			defer ws.Close()

			for {
				//Read Message from client
				mt, message, err := ws.ReadMessage()
				if err != nil {
					fmt.Println(err)
					break
				}

				log.Printf("type %d, message %s", mt, message)

				if string(message) == "start_session" {
					log.Println("start_session")
					if err := ws.WriteJSON(gin.H{
						"type": "symbols",
						"data": symbolList,
					}); err != nil {
						fmt.Println(err)
						return
					}
					sub := sm.NewSubscriber(func(m *sink.Message) {
						if err := ws.WriteJSON(gin.H{
							"type": "metric",
							"data": string(m.Data),
						}); err != nil {
							fmt.Println(err)
							return
						}
					})
					defer sub.Close()
				}

				//If client message is ping will return pong
				//if string(message) == "ping" {
				//	message = []byte("pong")
				//}

				////Response message to client
				//err = ws.WriteMessage(mt, message)
				//if err != nil {
				//	fmt.Println(err)
				//	break
				//}

			}
		})
		r.Run(":8080") // listen and serve on 0.0.0.0:8080
	}
*/
