// for build: go build -o ws-relay ws-relay.go
/*
for streaming:
ffmpeg \
	-f v4l2 \
		-framerate 25 -video_size 640x480 -i /dev/video0 \
	-f mpegts \
		-codec:v mpeg1video -s 640x480 -b:v 1000k -bf 0 \
	http://localhost:8080/supersecret

// avconv on rpi:
avconv -f video4linux2 -i /dev/video0 -f mpegts -codec:v mpeg1video -bf 0 http://localhost:8080/supersecret

*/
package main

import (
	"net/http"
	"log"
	"flag"
	"errors"

	ws "../3rd/websocket" // "github.com/gorilla/websocket"
)

var localAddr = flag.String("l", ":8080", "")

var secret = flag.String("secret", "supersecret", "stream secret")

var wsComp = flag.Bool("wscomp", false, "ws compression")
var verbosity = flag.Int("v", 3, "verbosity")

var upgrader = ws.Upgrader{ EnableCompression: false } // use default options

var newclients chan *WsClient
var bufCh chan []byte

type WsClient struct {
	*ws.Conn
	data chan []byte
	die bool
}
func NewWsClient(c *ws.Conn) (*WsClient) {
	return &WsClient{ c, make(chan []byte, 16), false }
}
func (c *WsClient) Send(buf []byte) (error) {
	if c.die {
		return errors.New("ws connection die")
	}

	select {
	case <- c.data:
	default:
	}
	c.data <- buf

	return nil
}
func (c *WsClient) worker() {
	for {
		buf := <- c.data
		err := c.WriteMessage(ws.BinaryMessage, buf)
		if err != nil {
			c.Close()
			c.die = true
			return
		}
	}
}

func broacast() {
	clients := make(map[*WsClient]*WsClient, 0)

	for {
		data := <- bufCh
		for _, c := range clients {
			err := c.Send(data)
			if err != nil {
				delete(clients, c)
				Vln(3, "[ws][client]removed", c.RemoteAddr(), len(clients))
			}
		}
		for len(newclients) > 0 {
			c := <-newclients
			clients[c] = c
			Vln(3, "[ws][client]added", c.RemoteAddr())
		}
	}
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		Vln(2, "[ws]upgrade failed:", err)
		return
	}
	defer c.Close()

	Vln(3, "[ws][client]connect", c.RemoteAddr())
	client := NewWsClient(c)
	newclients <- client

	client.worker()

	Vln(3, "[ws][client]disconnect", c.RemoteAddr())
}

func streamHandler(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		Vln(3, "[stream][new]", r.RemoteAddr)

		for {
			buf := make([]byte, 1024 * 1024)
			n, err := r.Body.Read(buf)
			Vln(5, "[stream][recv]", n, err)
			if err != nil {
				Vln(2, "[stream][recv]err:", err)
				return
			}
			bufCh <- buf[:n]
		}
	}
}

func main() {
	log.SetFlags(log.Ldate|log.Ltime)
	flag.Parse()

	upgrader.EnableCompression = *wsComp
	Vf(1, "ws EnableCompression = %v\n", *wsComp)
	Vf(1, "server Listen @ %v\n", *localAddr)

	newclients = make(chan *WsClient, 16)
	bufCh = make(chan []byte, 1)
	go broacast()

	http.HandleFunc("/stream", wsHandler)

	secretUrl := "/" + *secret
	http.HandleFunc(secretUrl, streamHandler)

//	http.HandleFunc("/", pageHandler)
	http.Handle("/", http.FileServer(http.Dir("./")))

	err := http.ListenAndServe(*localAddr, nil)
	if err != nil {
		Vln(1, "server listen error:", err)
	}
}

func Vln(level int, v ...interface{}) {
	if level <= *verbosity {
		log.Println(v...)
	}
}
func Vf(level int, format string, v ...interface{}) {
	if level <= *verbosity {
		log.Printf(format, v...)
	}
}

