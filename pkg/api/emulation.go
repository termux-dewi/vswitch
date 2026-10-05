package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"vswitch/pkg/emulation"
)

type emuRequest struct {
	Key     string             `json:"key"`
	Preset  string             `json:"preset"`
	Profile *emulation.Profile `json:"profile"`
}

// Emulation handles GET /api/v1/emulation, GET /api/v1/emulation/presets,
// GET /api/v1/emulation/ports, and POST/PUT/DELETE on a port key.
func (h *Handlers) Emulation(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]any{
			"profiles": h.Switch.Emulation.ListProfiles(),
			"stats":    h.Switch.Emulation.Stats(),
			"presets":  emulation.PresetNames(),
		})
	case http.MethodPost, http.MethodPut:
		var req emuRequest
		if e := json.NewDecoder(r.Body).Decode(&req); e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		if req.Key == "" {
			http.Error(w, "port key is required", 400)
			return
		}
		var prof emulation.Profile
		if req.Profile != nil {
			prof = *req.Profile
		}
		if req.Preset != "" {
			p := emulation.Preset(req.Preset)
			if p.IsZero() && req.Preset != "" {
				known := false
				for _, n := range emulation.PresetNames() {
					if n == req.Preset {
						known = true
						break
					}
				}
				if !known {
					http.Error(w, "unknown preset: "+req.Preset, 400)
					return
				}
			}
			if req.Profile == nil {
				prof = p
			}
		}
		if prof.IsZero() {
			http.Error(w, "profile or preset is required", 400)
			return
		}
		if e := h.Switch.ApplyEmulation(req.Key, prof); e != nil {
			http.Error(w, e.Error(), 404)
			return
		}
		writeJSON(w, map[string]any{"key": req.Key, "profile": prof})
	case http.MethodDelete:
		key := strings.TrimPrefix(r.URL.Path, "/api/v1/emulation/")
		if key == "" || key == r.URL.Path {
			var req emuRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			key = req.Key
		}
		if key == "" {
			http.Error(w, "port key is required", 400)
			return
		}
		h.Switch.RemoveEmulation(key)
		writeJSON(w, map[string]any{"key": key, "removed": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// EmulationPresets lists available preset profiles.
func (h *Handlers) EmulationPresets(w http.ResponseWriter, _ *http.Request) {
	out := map[string]emulation.Profile{}
	for _, n := range emulation.PresetNames() {
		out[n] = emulation.Preset(n)
	}
	writeJSON(w, out)
}

// EmulationPorts lists port keys that can be emulated.
func (h *Handlers) EmulationPorts(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, h.Switch.PortKeys())
}
