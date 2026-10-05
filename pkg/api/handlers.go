package api

import (
	"encoding/json"
	"net/http"
	"vswitch/pkg/config"
	"vswitch/pkg/core"
	"vswitch/pkg/dhcp"
	"vswitch/pkg/metrics"
	"vswitch/web"
)

type Handlers struct {
	Metrics    *metrics.Metrics
	Switch     *core.L2Switch
	LeaseStore *dhcp.Store
	ConfigPath string
	Cfg        *config.Config
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func (h *Handlers) Stats(w http.ResponseWriter, _ *http.Request) { writeJSON(w, h.Metrics.Snapshot()) }
func (h *Handlers) Ports(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, h.Switch.PortSummary())
}
func (h *Handlers) Leases(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, h.LeaseStore.List())
}
func (h *Handlers) Config(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, h.Cfg)
		return
	}
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var c config.Config
	if e := json.NewDecoder(r.Body).Decode(&c); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	if e := config.SaveConfig(h.ConfigPath, &c); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	h.Cfg = &c
	writeJSON(w, c)
}
func (h *Handlers) WebUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	b, e := web.FS.ReadFile("dist/index.html")
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b)
}
