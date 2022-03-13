package writer

import (
	"net"
	"net/http"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/munnik/gosk/message"
)

type WebsocketClient struct {
	writer           *SignalKWriter
	conn             *net.Conn
	subscriptions    map[string]struct{}
	sendCachedValues bool
}

func (w *SignalKWriter) NewWebsocketClient(conn *net.Conn) *WebsocketClient {
	w.sendHello(conn)
	return &WebsocketClient{writer: w, conn: conn}
}

func (w *SignalKWriter) serveWebsocket(rw http.ResponseWriter, r *http.Request) {
	conn, _, _, err := ws.UpgradeHTTP(r, rw)
	if err != nil {
		http.NotFound(rw, r)
		return
	}

	go func() {
		defer conn.Close()

		for {
			msg, op, err := wsutil.ReadClientData(conn)
			if err != nil {
				// handle error
			}
			err = wsutil.WriteServerMessage(conn, op, msg)
			if err != nil {
				// handle error
			}
		}
	}()
}

func (w *SignalKWriter) updateWebsocket(message message.Mapped) {

}

func (w *SignalKWriter) sendHello(conn *net.Conn) {
	// t, err := template.ParseFS(fs, "templates/hello.json")
	// if err != nil {
	// 	http.NotFound(rw, r)
	// 	return
	// }
	// msg, op, err := wsutil.ReadClientData(conn)
	// if err != nil {
	// 	// handle error
	// }
	// err = wsutil.WriteServerMessage(conn, op, msg)
	// if err != nil {
	// 	// handle error
	// }

	// rw.Header().Set("Content-Type", "application/json")
	// data := struct {
	// 	Version string
	// 	Now     string
	// 	Self    string
	// }{
	// 	Version: w.config.Version,
	// 	Now:     time.Now().String(),
	// 	Self:    w.config.SelfContext,
	// }
	// t.Execute(rw, data)
}
