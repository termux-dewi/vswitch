package api

import (
	"github.com/gorilla/websocket"
	"net/http"
	"time"
)

func (h *Handlers) WebSocket(w http.ResponseWriter, r *http.Request) {
	u := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	c, e := u.Upgrade(w, r, nil)
	if e != nil {
		return
	}
	defer c.Close()
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-t.C:
			if e := c.WriteJSON(h.Metrics.Snapshot()); e != nil {
				return
			}
		}
	}
}
