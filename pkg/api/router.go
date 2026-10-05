package api

import (
	"net/http"
	"strings"
)

type Router struct{ H *Handlers }

func (r *Router) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("/api/v1/stats", r.H.Stats)
	m.HandleFunc("/api/v1/ports", r.H.Ports)
	m.HandleFunc("/api/v1/leases", r.H.Leases)
	m.HandleFunc("/api/v1/config", r.H.Config)
	m.HandleFunc("/api/v1/emulation/presets", r.H.EmulationPresets)
	m.HandleFunc("/api/v1/emulation/ports", r.H.EmulationPorts)
	m.HandleFunc("/api/v1/emulation/", r.H.Emulation)
	m.HandleFunc("/api/v1/emulation", r.H.Emulation)
	m.HandleFunc("/ws/metrics", r.H.WebSocket)
	m.HandleFunc("/", r.H.WebUI)
	return http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		if strings.HasPrefix(q.URL.Path, "/api/") || q.URL.Path == "/ws/metrics" {
			w.Header().Set("Cache-Control", "no-store")
		}
		m.ServeHTTP(w, q)
	})
}
